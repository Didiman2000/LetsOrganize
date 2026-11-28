// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
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

func TestSyncMultipleCalendars(t *testing.T) {
	x := newSyncFixture(t)
	ctx, st, f := x.ctx, x.st, x.f
	f.mu.Lock()
	f.calendars = append(f.calendars,
		CalendarListEntry{ID: "team@group.calendar.google.com", Summary: "Team", AccessRole: "writer", Selected: true, BackgroundColor: "#0b8043"},
		CalendarListEntry{ID: "de.german#holiday@group.v.calendar.google.com", Summary: "Feiertage in Deutschland", AccessRole: "reader", Selected: true, BackgroundColor: "#7986cb"},
		CalendarListEntry{ID: "alt@group.calendar.google.com", Summary: "Altes Projekt", AccessRole: "owner", Selected: false, BackgroundColor: "javascript:alert(1)"},
	)
	f.mu.Unlock()
	const team, holiday = "team@group.calendar.google.com", "de.german#holiday@group.v.calendar.google.com"
	f.putIn(team, Event{Summary: "Teamrunde",
		Start: &EventTime{DateTime: "2026-12-02T14:00:00+01:00", TimeZone: "Europe/Berlin"},
		End:   &EventTime{DateTime: "2026-12-02T15:00:00+01:00", TimeZone: "Europe/Berlin"}})
	f.putIn(holiday, Event{Summary: "2. Advent", Start: &EventTime{Date: "2026-12-06"}, End: &EventTime{Date: "2026-12-07"}})
	f.putIn("alt@group.calendar.google.com", Event{Summary: "Altlast", Start: &EventTime{Date: "2026-12-03"}, End: &EventTime{Date: "2026-12-04"}})

	rep := x.sync(t)
	if rep.Calendars != 3 || rep.Pulled != 2 {
		t.Errorf("Abgleich: %+v", rep)
	}
	cals, _ := st.ListCalendars(ctx, x.user)
	if len(cals) != 4 || !cals[0].Primary || cals[0].RemoteID != "primary" {
		t.Fatalf("Kalender: %+v", cals)
	}
	byName := map[string]store.Calendar{}
	for _, c := range cals {
		byName[c.Name] = c
	}
	if byName["Altes Projekt"].Enabled || byName["Altes Projekt"].Color != "" {
		t.Errorf("nicht eingeblendeter Kalender oder unsichere Farbe: %+v", byName["Altes Projekt"])
	}
	if byName["Feiertage in Deutschland"].Writable || !byName["Team"].Writable {
		t.Error("Schreibrechte falsch")
	}

	week := x.week(t, 2026, 11, 30)
	got := map[string]string{}
	for _, e := range week {
		got[e.Title] = e.CalendarID
	}
	if got["Teamrunde"] != team || got["2. Advent"] != holiday {
		t.Errorf("Woche: %v", got)
	}
	if _, ok := got["Altlast"]; ok {
		t.Error("Termin aus ausgeschaltetem Kalender sichtbar")
	}

	// Neuer Termin im Teamkalender landet dort bei Google.
	s := time.Date(2026, 12, 3, 9, 0, 0, 0, time.UTC)
	if _, err := st.CreateEvent(ctx, x.user, store.Event{Title: "Planung", Start: s, End: s.Add(time.Hour), CalendarID: team}); err != nil {
		t.Fatal(err)
	}
	x.sync(t)
	found := false
	f.mu.Lock()
	for k, e := range f.events {
		if e.Summary == "Planung" && strings.HasPrefix(k, team+"|") {
			found = true
		}
	}
	f.mu.Unlock()
	if !found {
		t.Error("Termin nicht im Teamkalender bei Google")
	}

	// Nur lesbarer Kalender: nicht anlegen, nicht ändern, nicht löschen.
	var ve *store.ValidationError
	if _, err := st.CreateEvent(ctx, x.user, store.Event{Title: "x", Start: s, End: s.Add(time.Hour), CalendarID: holiday}); !errors.As(err, &ve) {
		t.Errorf("Anlegen im Feiertagskalender: %v", err)
	}
	var advent store.Event
	for _, e := range x.week(t, 2026, 11, 30) {
		if e.Title == "2. Advent" {
			advent = e
		}
	}
	advent.Title = "geändert"
	if _, err := st.UpdateEvent(ctx, x.user, advent); !errors.As(err, &ve) {
		t.Errorf("Ändern im Feiertagskalender: %v", err)
	}
	if err := st.DeleteEvent(ctx, x.user, advent.ID); !errors.As(err, &ve) {
		t.Errorf("Löschen im Feiertagskalender: %v", err)
	}
	if _, err := st.CreateEvent(ctx, x.user, store.Event{Title: "x", Start: s, End: s.Add(time.Hour), CalendarID: "gibt-es-nicht"}); !errors.As(err, &ve) {
		t.Errorf("unbekannter Kalender: %v", err)
	}

	// Ausschalten: Termine verschwinden aus der Ansicht, Kalender wird nicht mehr abgefragt.
	if _, err := st.SetCalendarEnabled(ctx, x.user, byName["Team"].ID, false); err != nil {
		t.Fatal(err)
	}
	for _, e := range x.week(t, 2026, 11, 30) {
		if e.CalendarID == team {
			t.Errorf("ausgeschalteter Kalender sichtbar: %s", e.Title)
		}
	}
	before := f.count("GET /calendar/calendars/" + team)
	x.sync(t)
	if f.count("GET /calendar/calendars/"+team) != before {
		t.Error("ausgeschalteter Kalender wird abgefragt")
	}

	// Kalender bei Google entfernt: verschwindet samt Terminen.
	f.mu.Lock()
	f.calendars = f.calendars[:1]
	f.mu.Unlock()
	x.sync(t)
	if cals, _ := st.ListCalendars(ctx, x.user); len(cals) != 1 {
		t.Errorf("nach Entfernen: %+v", cals)
	}
	for _, e := range x.week(t, 2026, 11, 30) {
		if e.CalendarID == holiday {
			t.Error("Termin aus entferntem Kalender sichtbar")
		}
	}
}
