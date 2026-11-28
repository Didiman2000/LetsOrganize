// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGoogle bildet die benötigten Teile von Google nach: Anmeldung,
// Kontoinformation und Kalender mit ETags und Sync-Tokens.
type fakeGoogle struct {
	t   *testing.T
	srv *httptest.Server

	mu        sync.Mutex
	verifiers map[string]string // code → code_challenge
	access    string
	refresh   string
	expireNow bool // nächstes Zugangstoken sofort ungültig
	revoked   bool
	events    map[string]*Event // Schlüssel: Kalender + "|" + id
	changed   map[string]int    // Schlüssel wie events → Version, in der zuletzt geändert
	calendars []CalendarListEntry
	version   int
	seq       int
	requests  []string
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	f := &fakeGoogle{t: t, verifiers: map[string]string{}, events: map[string]*Event{}, changed: map[string]int{}, refresh: "refresh-1",
		calendars: []CalendarListEntry{{ID: "andreas@example.com", Summary: "andreas@example.com", Primary: true, Selected: true, AccessRole: "owner", BackgroundColor: "#4285f4"}}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", f.token)
	mux.HandleFunc("GET /userinfo", f.userinfo)
	mux.HandleFunc("GET /calendar/users/me/calendarList", f.calendarList)
	mux.HandleFunc("GET /calendar/calendars/{cal}/events", f.list)
	mux.HandleFunc("POST /calendar/calendars/{cal}/events", f.insert)
	mux.HandleFunc("PUT /calendar/calendars/{cal}/events/{id}", f.update)
	mux.HandleFunc("DELETE /calendar/calendars/{cal}/events/{id}", f.delete)
	mux.HandleFunc("POST /revoke", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.revoked = true
		f.mu.Unlock()
	})
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.mu.Unlock()
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeGoogle) client() *Client {
	return &Client{ID: "cid", Secret: "csecret", AuthURL: f.srv.URL + "/auth", TokenURL: f.srv.URL + "/token", UserinfoURL: f.srv.URL + "/userinfo"}
}

func (f *fakeGoogle) api(ts TokenSource) *API {
	return &API{HTTP: f.srv.Client(), Token: ts, CalendarURL: f.srv.URL + "/calendar"}
}

// grant simuliert, dass der Nutzer bei Google zugestimmt hat.
func (f *fakeGoogle) grant(authURL string) (state, code string) {
	f.t.Helper()
	u := mustURL(f.t, authURL)
	q := u.Query()
	code = fmt.Sprintf("code-%d", len(f.verifiers)+1)
	f.mu.Lock()
	f.verifiers[code] = q.Get("code_challenge")
	f.mu.Unlock()
	return q.Get("state"), code
}

func (f *fakeGoogle) newAccess() string {
	f.seq++
	f.access = fmt.Sprintf("access-%d", f.seq)
	return f.access
}

func (f *fakeGoogle) token(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Form.Get("client_id") != "cid" || r.Form.Get("client_secret") != "csecret" {
		http.Error(w, `{"error":"invalid_client"}`, 401)
		return
	}
	expires := 3600
	if f.expireNow {
		expires = 0
		f.expireNow = false
	}
	switch r.Form.Get("grant_type") {
	case "authorization_code":
		challenge, ok := f.verifiers[r.Form.Get("code")]
		delete(f.verifiers, r.Form.Get("code"))
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": f.newAccess(), "refresh_token": f.refresh, "expires_in": expires, "scope": "calendar"})
	case "refresh_token":
		if f.revoked || r.Form.Get("refresh_token") != f.refresh {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"access_token": f.newAccess(), "expires_in": expires})
	default:
		http.Error(w, "unsupported", 400)
	}
}

func (f *fakeGoogle) authorized(w http.ResponseWriter, r *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer "+f.access || f.access == "" {
		http.Error(w, "unauthorized", 401)
		return false
	}
	return true
}

func (f *fakeGoogle) userinfo(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"email": "andreas@example.com", "email_verified": true})
}

// calKey bildet "primary" und die E-Mail-Adresse auf denselben Kalender ab.
func calKey(cal string) string {
	if cal == "andreas@example.com" {
		return "primary"
	}
	return cal
}

func (f *fakeGoogle) touchIn(cal string, e *Event) {
	f.version++
	e.ETag = fmt.Sprintf(`"%d"`, f.version)
	e.Updated = time.Now().UTC().Add(time.Duration(f.version) * time.Millisecond).Format(time.RFC3339Nano)
	k := calKey(cal) + "|" + e.ID
	f.changed[k] = f.version
	f.events[k] = e
}

func (f *fakeGoogle) touch(e *Event) { f.touchIn("primary", e) }

func (f *fakeGoogle) calendarList(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	json.NewEncoder(w).Encode(map[string]any{"items": f.calendars})
}

// putIn legt einen Termin direkt in einem Kalender „bei Google“ an oder ändert ihn.
func (f *fakeGoogle) putIn(cal string, e Event) *Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e.ID == "" {
		f.seq++
		e.ID = fmt.Sprintf("g%d", f.seq)
	}
	if e.Status == "" {
		e.Status = "confirmed"
	}
	f.touchIn(cal, &e)
	return &e
}

func (f *fakeGoogle) getIn(cal, id string) *Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e := f.events[calKey(cal)+"|"+id]; e != nil {
		c := *e
		return &c
	}
	return nil
}

// put legt einen Termin direkt „bei Google“ an oder ändert ihn (wie in der Google-Oberfläche).
func (f *fakeGoogle) put(e Event) *Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e.ID == "" {
		f.seq++
		e.ID = fmt.Sprintf("g%d", f.seq)
	}
	if e.Status == "" {
		e.Status = "confirmed"
	}
	f.touch(&e)
	return &e
}

func (f *fakeGoogle) get(id string) *Event { return f.getIn("primary", id) }

func (f *fakeGoogle) list(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	since := 0
	if tok := r.URL.Query().Get("syncToken"); tok != "" {
		if tok == "expired" {
			http.Error(w, `{"error":"gone"}`, 410)
			return
		}
		fmt.Sscanf(tok, "v%d", &since)
	}
	page := EventPage{Items: []Event{}, NextSyncToken: fmt.Sprintf("v%d", f.version)}
	prefix := calKey(r.PathValue("cal")) + "|"
	for k, v := range f.changed {
		if v > since && strings.HasPrefix(k, prefix) {
			page.Items = append(page.Items, *f.events[k])
		}
	}
	json.NewEncoder(w).Encode(page)
}

func (f *fakeGoogle) insert(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	var e Event
	json.NewDecoder(r.Body).Decode(&e)
	if len(e.Recurrence) > 0 && (e.Start.DateTime != "" && e.Start.TimeZone == "") {
		http.Error(w, `{"error":"timeZone required for recurring"}`, 400)
		return
	}
	f.mu.Lock()
	f.seq++
	e.ID = fmt.Sprintf("g%d", f.seq)
	e.Status = "confirmed"
	f.touchIn(r.PathValue("cal"), &e)
	f.mu.Unlock()
	json.NewEncoder(w).Encode(e)
}

func (f *fakeGoogle) update(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	id, cal := r.PathValue("id"), r.PathValue("cal")
	f.mu.Lock()
	defer f.mu.Unlock()
	old := f.events[calKey(cal)+"|"+id]
	if old == nil || old.Status == "cancelled" {
		http.Error(w, "not found", 404)
		return
	}
	if m := r.Header.Get("If-Match"); m != "" && m != old.ETag {
		http.Error(w, "precondition", 412)
		return
	}
	var e Event
	json.NewDecoder(r.Body).Decode(&e)
	e.ID, e.Status = id, "confirmed"
	f.touchIn(cal, &e)
	json.NewEncoder(w).Encode(e)
}

func (f *fakeGoogle) delete(w http.ResponseWriter, r *http.Request) {
	if !f.authorized(w, r) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cal := r.PathValue("cal")
	e := f.events[calKey(cal)+"|"+r.PathValue("id")]
	if e == nil {
		http.Error(w, "not found", 404)
		return
	}
	e.Status = "cancelled"
	f.touchIn(cal, e)
	w.WriteHeader(204)
}

func (f *fakeGoogle) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.requests {
		if strings.HasPrefix(r, prefix) {
			n++
		}
	}
	return n
}
