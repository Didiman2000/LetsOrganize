// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestEventsCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)
	other, _ := s.CreateUser(ctx, "andere", "", "")

	e, err := s.CreateEvent(ctx, u.ID, Event{
		Title: "  Audit Kunde  ", Location: "Büro",
		Start: mustTime(t, "2026-12-01T10:00:00+01:00"), End: mustTime(t, "2026-12-01T11:30:00+01:00"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Title != "Audit Kunde" || e.Start.Format(time.RFC3339) != "2026-12-01T09:00:00Z" {
		t.Errorf("gespeichert: %+v", e)
	}

	// Ganztägig über zwei Tage: Ende exklusiv auf Mitternacht.
	allDay, err := s.CreateEvent(ctx, u.ID, Event{
		Title: "Urlaub", AllDay: true,
		Start: mustTime(t, "2026-12-03T15:00:00Z"), End: mustTime(t, "2026-12-04T08:00:00Z"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if allDay.Start.Format(time.DateOnly) != "2026-12-03" || allDay.End.Format(time.DateOnly) != "2026-12-04" {
		t.Errorf("ganztägig: %v – %v", allDay.Start, allDay.End)
	}

	// Ungültige Eingaben.
	var ve *ValidationError
	if _, err := s.CreateEvent(ctx, u.ID, Event{Title: "", Start: e.Start, End: e.End}); !errors.As(err, &ve) || ve.Field != "title" {
		t.Errorf("leerer Betreff: %v", err)
	}
	if _, err := s.CreateEvent(ctx, u.ID, Event{Title: "x", Start: e.End, End: e.Start}); !errors.As(err, &ve) || ve.Field != "end" {
		t.Errorf("Ende vor Beginn: %v", err)
	}

	// Zeitraum: Woche vom 30.11. bis 7.12.
	week, err := s.ListEvents(ctx, u.ID, mustTime(t, "2026-11-30T00:00:00Z"), mustTime(t, "2026-12-07T00:00:00Z"))
	if err != nil || len(week) != 2 || !week[0].AllDay {
		t.Fatalf("Woche: %v %+v", err, week)
	}
	none, _ := s.ListEvents(ctx, u.ID, mustTime(t, "2026-12-01T09:30:00Z"), mustTime(t, "2026-12-01T09:30:00Z").Add(time.Minute))
	if len(none) != 1 {
		t.Errorf("Überlappung innerhalb des Termins nicht gefunden: %d", len(none))
	}
	after, _ := s.ListEvents(ctx, u.ID, mustTime(t, "2026-12-01T10:30:00Z"), mustTime(t, "2026-12-02T00:00:00Z"))
	if len(after) != 0 {
		t.Errorf("Termin nach seinem Ende gefunden: %+v", after)
	}

	// Fremde Benutzer sehen und ändern nichts.
	if list, _ := s.ListEvents(ctx, other.ID, mustTime(t, "2026-11-30T00:00:00Z"), mustTime(t, "2026-12-07T00:00:00Z")); len(list) != 0 {
		t.Error("fremder Benutzer sieht Termine")
	}
	e.Title = "übernommen"
	if _, err := s.UpdateEvent(ctx, other.ID, *e); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann Termin ändern")
	}
	if err := s.DeleteEvent(ctx, other.ID, e.ID); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann Termin löschen")
	}

	// Ändern merkt den Termin für den Abgleich vor.
	s.DB.ExecContext(ctx, `UPDATE events SET dirty = 0 WHERE id = ?`, e.ID)
	e.Title = "Audit verschoben"
	e.Start, e.End = e.Start.Add(time.Hour), e.End.Add(time.Hour)
	upd, err := s.UpdateEvent(ctx, u.ID, *e)
	if err != nil || upd.Title != "Audit verschoben" {
		t.Fatalf("ändern: %v", err)
	}
	var dirty int
	s.DB.QueryRowContext(ctx, `SELECT dirty FROM events WHERE id = ?`, e.ID).Scan(&dirty)
	if dirty != 1 {
		t.Error("Änderung nicht für Abgleich vorgemerkt")
	}

	// Löschen ist weich.
	if err := s.DeleteEvent(ctx, u.ID, e.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetEvent(ctx, u.ID, e.ID); !errors.Is(err, ErrNotFound) {
		t.Error("gelöschter Termin noch sichtbar")
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM events WHERE id = ? AND deleted_at IS NOT NULL`, e.ID).Scan(&n)
	if n != 1 {
		t.Error("Löschung nicht weich")
	}

	// Kategorie eines anderen Benutzers ist nicht erlaubt.
	res, _ := s.DB.ExecContext(ctx, `INSERT INTO categories (user_id, name) VALUES (?, 'fremd')`, other.ID)
	cat, _ := res.LastInsertId()
	if _, err := s.CreateEvent(ctx, u.ID, Event{Title: "x", Start: e.Start, End: e.End, CategoryID: &cat}); !errors.As(err, &ve) {
		t.Errorf("fremde Kategorie akzeptiert: %v", err)
	}
}
