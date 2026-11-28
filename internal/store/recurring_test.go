// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRecurringEvents(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)
	loc, _ := time.LoadLocation("Europe/Berlin")

	// Jour fixe jeden Montag 9–10 Uhr Berliner Zeit, ab 16. März 2026 (Winterzeit).
	start := time.Date(2026, 3, 16, 9, 0, 0, 0, loc)
	jf, err := s.CreateEvent(ctx, u.ID, Event{
		Title: "Jour fixe", Start: start, End: start.Add(time.Hour),
		RRule: "freq=weekly;byday=mo", TimeZone: "Europe/Berlin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if jf.RRule != "FREQ=WEEKLY;BYDAY=MO" || jf.TimeZone != "Europe/Berlin" {
		t.Errorf("gespeichert: %q %q", jf.RRule, jf.TimeZone)
	}

	// Geburtstagsessen jedes Jahr am 4. Juli, ganztägig.
	if _, err := s.CreateEvent(ctx, u.ID, Event{
		Title: "Sommerfest", AllDay: true,
		Start: time.Date(2025, 7, 4, 0, 0, 0, 0, time.UTC), End: time.Date(2025, 7, 5, 0, 0, 0, 0, time.UTC),
		RRule: "FREQ=YEARLY",
	}); err != nil {
		t.Fatal(err)
	}

	var ve *ValidationError
	if _, err := s.CreateEvent(ctx, u.ID, Event{Title: "x", Start: start, End: start.Add(time.Hour), RRule: "FREQ=HOURLY"}); !errors.As(err, &ve) || ve.Field != "rrule" {
		t.Errorf("ungültige Regel: %v", err)
	}
	if _, err := s.CreateEvent(ctx, u.ID, Event{Title: "x", Start: start, End: start.Add(time.Hour), TimeZone: "Mond/Krater"}); !errors.As(err, &ve) || ve.Field != "timeZone" {
		t.Errorf("ungültige Zeitzone: %v", err)
	}

	// Woche vor und nach der Zeitumstellung (29. März 2026).
	week := func(y int, m time.Month, d int) []Event {
		from := time.Date(y, m, d, 0, 0, 0, 0, loc)
		list, err := s.ListEvents(ctx, u.ID, from, from.AddDate(0, 0, 7))
		if err != nil {
			t.Fatal(err)
		}
		return list
	}
	before, after := week(2026, 3, 23), week(2026, 3, 30)
	if len(before) != 1 || len(after) != 1 {
		t.Fatalf("Vorkommen: %d / %d", len(before), len(after))
	}
	if h := before[0].Start.In(loc).Hour(); h != 9 {
		t.Errorf("vor Umstellung %d Uhr", h)
	}
	if h := after[0].Start.In(loc).Hour(); h != 9 {
		t.Errorf("nach Umstellung %d Uhr", h)
	}
	if after[0].SeriesStart == nil || !after[0].SeriesStart.Equal(start) || after[0].ID != jf.ID {
		t.Errorf("Serienbezug fehlt: %+v", after[0])
	}
	if list := week(2026, 3, 9); len(list) != 0 {
		t.Error("Vorkommen vor dem Serienbeginn")
	}

	// Ganztägige Jahresserie erscheint 2026 und 2027.
	for _, y := range []int{2026, 2027} {
		list := week(y, 7, 1)
		found := false
		for _, e := range list {
			if e.Title == "Sommerfest" && e.Start.Format(time.DateOnly) == time.Date(y, 7, 4, 0, 0, 0, 0, time.UTC).Format(time.DateOnly) {
				found = true
			}
		}
		if !found {
			t.Errorf("Sommerfest %d fehlt: %+v", y, list)
		}
	}

	// Einzelnes Vorkommen löschen.
	occ := after[0].Start
	if err := s.DeleteOccurrence(ctx, u.ID, jf.ID, occ.Add(time.Minute)); !errors.As(err, &ve) {
		t.Errorf("falscher Zeitpunkt akzeptiert: %v", err)
	}
	if err := s.DeleteOccurrence(ctx, u.ID, jf.ID, occ); err != nil {
		t.Fatal(err)
	}
	if list := week(2026, 3, 30); len(list) != 0 {
		t.Errorf("gelöschtes Vorkommen noch da: %+v", list)
	}
	if list := week(2026, 4, 6); len(list) != 1 {
		t.Error("Folgewoche fehlt")
	}
	// Bearbeiten der Serie behält gelöschte Vorkommen.
	jf.Title = "Jour fixe Team"
	if _, err := s.UpdateEvent(ctx, u.ID, *jf); err != nil {
		t.Fatal(err)
	}
	if list := week(2026, 3, 30); len(list) != 0 {
		t.Error("gelöschtes Vorkommen nach Bearbeiten wieder da")
	}
	if list := week(2026, 4, 6); len(list) != 1 || list[0].Title != "Jour fixe Team" {
		t.Errorf("Serie nicht geändert: %+v", list)
	}

	// Einfacher Termin ist keine Serie.
	single, _ := s.CreateEvent(ctx, u.ID, Event{Title: "Einmalig", Start: start, End: start.Add(time.Hour)})
	if err := s.DeleteOccurrence(ctx, u.ID, single.ID, start); !errors.As(err, &ve) {
		t.Errorf("Einzeltermin als Serie behandelt: %v", err)
	}
	// Ganze Serie löschen.
	if err := s.DeleteEvent(ctx, u.ID, jf.ID); err != nil {
		t.Fatal(err)
	}
	if list := week(2026, 4, 6); len(list) != 0 {
		t.Error("gelöschte Serie noch da")
	}
}
