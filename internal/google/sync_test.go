// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/secret"
	"github.com/Didiman2000/LetsOrganize/internal/store"
)

type syncFixture struct {
	ctx  context.Context
	st   *store.Store
	f    *fakeGoogle
	m    *Manager
	user int64
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	st, err := store.Open(ctx, filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secret.LoadOrCreate(filepath.Join(dir, "secret.key"))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := st.EnsureLocalUser(ctx)
	f := newFakeGoogle(t)
	m := NewManager(st, box, f.client())
	m.HTTP = f.srv.Client()
	m.NewAPI = func(hc *http.Client, ts TokenSource) *API { return f.api(ts) }
	RevokeURL = f.srv.URL + "/revoke"

	authURL, err := m.Begin("http://127.0.0.1:8470/")
	if err != nil {
		t.Fatal(err)
	}
	state, code := f.grant(authURL)
	email, err := m.Finish(ctx, u.ID, state, code)
	if err != nil || email != "andreas@example.com" {
		t.Fatalf("Finish: %v %q", err, email)
	}
	if _, err := m.Finish(ctx, u.ID, state, code); err == nil {
		t.Error("Anmeldung zweimal verwendbar")
	}
	return &syncFixture{ctx: ctx, st: st, f: f, m: m, user: u.ID}
}

func (x *syncFixture) sync(t *testing.T) *Report {
	t.Helper()
	rep, err := x.m.Sync(x.ctx, x.user)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(rep.Problems) > 0 {
		t.Errorf("Probleme: %v", rep.Problems)
	}
	return rep
}

func (x *syncFixture) week(t *testing.T, y int, m time.Month, d int) []store.Event {
	t.Helper()
	from := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	list, err := x.st.ListEvents(x.ctx, x.user, from, from.AddDate(0, 0, 7))
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestSyncCalendar(t *testing.T) {
	x := newSyncFixture(t)
	ctx, st, f := x.ctx, x.st, x.f

	acc, err := st.GoogleAccountOf(ctx, x.user)
	if err != nil || acc.Email != "andreas@example.com" {
		t.Fatalf("Konto: %v", err)
	}

	// 1. Von Google holen: Einzeltermin und Serie.
	audit := f.put(Event{Summary: "Audit Kunde B", Location: "Hamburg",
		Start: &EventTime{DateTime: "2026-12-01T10:00:00+01:00", TimeZone: "Europe/Berlin"},
		End:   &EventTime{DateTime: "2026-12-01T12:00:00+01:00", TimeZone: "Europe/Berlin"}})
	jf := f.put(Event{Summary: "Jour fixe", Recurrence: []string{"RRULE:FREQ=WEEKLY;BYDAY=MO"},
		Start: &EventTime{DateTime: "2026-11-30T09:00:00+01:00", TimeZone: "Europe/Berlin"},
		End:   &EventTime{DateTime: "2026-11-30T10:00:00+01:00", TimeZone: "Europe/Berlin"}})
	rep := x.sync(t)
	if rep.Pulled != 2 || rep.Pushed != 0 {
		t.Errorf("erster Abgleich: %+v", rep)
	}
	if list := x.week(t, 2026, 11, 30); len(list) != 2 {
		t.Fatalf("Woche nach Abgleich: %+v", list)
	}
	if list := x.week(t, 2026, 12, 14); len(list) != 1 || list[0].Title != "Jour fixe" {
		t.Errorf("Serie nicht aufgelöst: %+v", list)
	}

	// 2. Nichts geändert: kein erneutes Übernehmen, nichts hochladen.
	if rep := x.sync(t); rep.Pulled != 0 || rep.Pushed != 0 {
		t.Errorf("Leerlauf: %+v", rep)
	}

	// 3. Lokal anlegen → hochladen, auch Serie mit Zeitzone.
	berlin, _ := time.LoadLocation("Europe/Berlin")
	s := time.Date(2026, 12, 2, 18, 0, 0, 0, berlin)
	sport, err := st.CreateEvent(ctx, x.user, store.Event{Title: "Sport", Start: s, End: s.Add(time.Hour),
		RRule: "FREQ=WEEKLY", TimeZone: "Europe/Berlin"})
	if err != nil {
		t.Fatal(err)
	}
	if rep := x.sync(t); rep.Pushed != 1 {
		t.Errorf("Hochladen: %+v", rep)
	}
	var remoteSport *Event
	for _, e := range f.events {
		if e.Summary == "Sport" {
			remoteSport = e
		}
	}
	if remoteSport == nil || remoteSport.Recurrence[0] != "RRULE:FREQ=WEEKLY" || remoteSport.Start.TimeZone != "Europe/Berlin" {
		t.Fatalf("bei Google: %+v", remoteSport)
	}
	// Der hochgeladene Termin kommt beim nächsten Holen nicht doppelt zurück.
	if rep := x.sync(t); rep.Pulled != 0 {
		t.Errorf("Echo: %+v", rep)
	}
	if list := x.week(t, 2026, 11, 30); len(list) != 3 {
		t.Errorf("nach Hochladen: %d Termine", len(list))
	}

	// 4. Bei Google geändert und gelöscht.
	audit.Summary = "Audit Kunde B (verschoben)"
	f.put(*audit)
	jfDel := f.get(jf.ID)
	f.mu.Lock()
	jfDel.Status = "cancelled"
	f.touch(jfDel)
	f.mu.Unlock()
	rep = x.sync(t)
	if rep.Pulled != 1 || rep.Deleted != 1 {
		t.Errorf("Änderungen von Google: %+v", rep)
	}
	for _, e := range x.week(t, 2026, 11, 30) {
		if e.Title == "Jour fixe" {
			t.Error("bei Google gelöschte Serie noch da")
		}
		if e.Title == "Audit Kunde B" {
			t.Error("Änderung von Google nicht übernommen")
		}
	}

	// 5. Lokal ändern und löschen → zu Google.
	sport.Title = "Sport (Halle 2)"
	if _, err := st.UpdateEvent(ctx, x.user, *sport); err != nil {
		t.Fatal(err)
	}
	if rep := x.sync(t); rep.Pushed != 1 {
		t.Errorf("Änderung hochladen: %+v", rep)
	}
	if f.get(remoteSport.ID).Summary != "Sport (Halle 2)" {
		t.Error("Änderung nicht bei Google")
	}
	if err := st.DeleteEvent(ctx, x.user, sport.ID); err != nil {
		t.Fatal(err)
	}
	if rep := x.sync(t); rep.Removed != 1 {
		t.Errorf("Löschen hochladen: %+v", rep)
	}
	if f.get(remoteSport.ID).Status != "cancelled" {
		t.Error("bei Google nicht gelöscht")
	}

	// 6. Konflikt: beide ändern; die jüngere Änderung gewinnt.
	localAudit := x.week(t, 2026, 11, 30)[0]
	time.Sleep(5 * time.Millisecond)
	ga := f.get(audit.ID)
	ga.Location = "Hamburg, Raum 3 (Google)"
	f.put(*ga)
	time.Sleep(1100 * time.Millisecond) // lokale Zeitstempel haben Sekundengenauigkeit
	localAudit.Location = "Lübeck (lokal, später)"
	if _, err := st.UpdateEvent(ctx, x.user, localAudit); err != nil {
		t.Fatal(err)
	}
	rep = x.sync(t)
	if rep.Kept != 1 || rep.Pushed != 1 {
		t.Errorf("Konflikt: %+v", rep)
	}
	if got := f.get(audit.ID).Location; got != "Lübeck (lokal, später)" {
		t.Errorf("Konflikt bei Google: %q", got)
	}

	// 7. Sync-Token abgelaufen → vollständiger Abgleich ohne Doppelte.
	st.SetSyncState(ctx, acc.ID, "calendar:primary", "expired")
	x.sync(t)
	n := 0
	for _, e := range x.week(t, 2026, 11, 30) {
		if e.Title == "Audit Kunde B (verschoben)" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("nach vollem Abgleich %d× Audit", n)
	}
}

func TestSyncSeriesExceptions(t *testing.T) {
	x := newSyncFixture(t)
	f := x.f
	jf := f.put(Event{Summary: "Jour fixe", Recurrence: []string{"RRULE:FREQ=WEEKLY"},
		Start: &EventTime{DateTime: "2026-11-30T09:00:00+01:00", TimeZone: "Europe/Berlin"},
		End:   &EventTime{DateTime: "2026-11-30T10:00:00+01:00", TimeZone: "Europe/Berlin"}})
	// Vorkommen am 7.12. auf Dienstag verschoben, am 14.12. abgesagt.
	f.put(Event{ID: jf.ID + "_20261207T080000Z", Summary: "Jour fixe (Dienstag)", RecurringEventID: jf.ID,
		OriginalStartTime: &EventTime{DateTime: "2026-12-07T09:00:00+01:00"},
		Start:             &EventTime{DateTime: "2026-12-08T09:00:00+01:00", TimeZone: "Europe/Berlin"},
		End:               &EventTime{DateTime: "2026-12-08T10:00:00+01:00", TimeZone: "Europe/Berlin"}})
	f.put(Event{ID: jf.ID + "_20261214T080000Z", Status: "cancelled", RecurringEventID: jf.ID,
		OriginalStartTime: &EventTime{DateTime: "2026-12-14T09:00:00+01:00"}})
	x.sync(t)

	w1 := x.week(t, 2026, 12, 7)
	if len(w1) != 1 || w1[0].Title != "Jour fixe (Dienstag)" || w1[0].Start.Day() != 8 {
		t.Errorf("verschobenes Vorkommen: %+v", w1)
	}
	if w2 := x.week(t, 2026, 12, 14); len(w2) != 0 {
		t.Errorf("abgesagtes Vorkommen: %+v", w2)
	}
	if w3 := x.week(t, 2026, 12, 21); len(w3) != 1 {
		t.Errorf("Folgewoche: %+v", w3)
	}
}

func TestTokenRefreshAndRevoke(t *testing.T) {
	x := newSyncFixture(t)
	// Zugangstoken bei Google ungültig machen: der Abgleich erneuert es selbst.
	x.f.mu.Lock()
	x.f.access = "anderes"
	x.f.mu.Unlock()
	x.sync(t)
	if x.f.count("POST /token") < 2 {
		t.Error("Token wurde nicht erneuert")
	}
	// Trennen widerruft das Token und entfernt das Konto.
	if err := x.m.Disconnect(x.ctx, x.user); err != nil {
		t.Fatal(err)
	}
	if !x.f.revoked {
		t.Error("Token nicht widerrufen")
	}
	if st := x.m.Status(x.ctx, x.user); st.Connected {
		t.Error("noch verbunden")
	}
	if _, err := x.m.Sync(x.ctx, x.user); err == nil {
		t.Error("Abgleich ohne Verbindung möglich")
	}
}
