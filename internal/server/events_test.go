// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func newDBServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(Config{Assets: os.DirFS("../.."), DataDir: dir, Store: db})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func do(s *Server, method, url, body string, header map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

var jsonHeaders = map[string]string{"Content-Type": "application/json", csrfHeader: "1"}

func TestEventsAPI(t *testing.T) {
	s := newDBServer(t)
	body := `{"title":"Kickoff","start":"2026-12-01T09:00:00Z","end":"2026-12-01T10:00:00Z"}`

	// Ohne Schutzkopf oder mit fremder Herkunft wird nichts angelegt.
	if rec := do(s, "POST", "/api/events", body, map[string]string{"Content-Type": "application/json"}); rec.Code != http.StatusForbidden {
		t.Errorf("ohne Kopf: %d", rec.Code)
	}
	if rec := do(s, "POST", "/api/events", body, map[string]string{"Content-Type": "text/plain", csrfHeader: "1"}); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("text/plain: %d", rec.Code)
	}
	h := map[string]string{"Content-Type": "application/json", csrfHeader: "1", "Origin": "https://boese.example"}
	if rec := do(s, "POST", "/api/events", body, h); rec.Code != http.StatusForbidden {
		t.Errorf("fremde Herkunft: %d", rec.Code)
	}

	rec := do(s, "POST", "/api/events", body, jsonHeaders)
	if rec.Code != http.StatusCreated {
		t.Fatalf("anlegen: %d %s", rec.Code, rec.Body)
	}
	var created store.Event
	json.Unmarshal(rec.Body.Bytes(), &created)

	if rec := do(s, "POST", "/api/events", `{"title":"","start":"2026-12-01T09:00:00Z","end":"2026-12-01T10:00:00Z"}`, jsonHeaders); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("ungültig: %d", rec.Code)
	}
	if rec := do(s, "POST", "/api/events", `{"title":"x","boese":1}`, jsonHeaders); rec.Code != http.StatusBadRequest {
		t.Errorf("unbekanntes Feld: %d", rec.Code)
	}

	list := do(s, "GET", "/api/events?from=2026-11-30T00:00:00Z&to=2026-12-07T00:00:00Z", "", nil)
	if list.Code != 200 || !strings.Contains(list.Body.String(), "Kickoff") {
		t.Errorf("auflisten: %d %s", list.Code, list.Body)
	}
	if rec := do(s, "GET", "/api/events?from=gestern", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("ungültiger Zeitraum: %d", rec.Code)
	}

	url := "/api/events/" + jsonID(created.ID)
	upd := `{"title":"Kickoff verschoben","start":"2026-12-02T09:00:00Z","end":"2026-12-02T10:00:00Z"}`
	if rec := do(s, "PUT", url, upd, jsonHeaders); rec.Code != 200 || !strings.Contains(rec.Body.String(), "verschoben") {
		t.Errorf("ändern: %d %s", rec.Code, rec.Body)
	}
	if rec := do(s, "DELETE", url, "", map[string]string{csrfHeader: "1"}); rec.Code != http.StatusNoContent {
		t.Errorf("löschen: %d", rec.Code)
	}
	if rec := do(s, "GET", url, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("nach Löschen: %d", rec.Code)
	}
}

func jsonID(id int64) string {
	b, _ := json.Marshal(id)
	return string(b)
}
