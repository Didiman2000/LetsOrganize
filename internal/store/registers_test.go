// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"errors"
	"testing"
)

func TestNotes(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)
	other, _ := s.CreateUser(ctx, "andere", "", "")

	var ve *ValidationError
	if _, err := s.CreateNote(ctx, u.ID, Note{Title: " ", Body: " "}); !errors.As(err, &ve) {
		t.Error("leere Notiz akzeptiert")
	}
	a, err := s.CreateNote(ctx, u.ID, Note{Title: "Ideen", Body: "Ringbuch-Thema in Grün"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := s.CreateNote(ctx, u.ID, Note{Body: "Nur Text"})
	s.DB.ExecContext(ctx, `UPDATE notes SET updated_at = '2020-01-01T00:00:00Z' WHERE id = ?`, a.ID)
	list, _ := s.ListNotes(ctx, u.ID)
	if len(list) != 2 || list[0].ID != b.ID {
		t.Errorf("Reihenfolge: %+v", list)
	}
	a.Body = "geändert"
	if upd, err := s.UpdateNote(ctx, u.ID, *a); err != nil || upd.Body != "geändert" {
		t.Errorf("ändern: %v", err)
	}
	if _, err := s.UpdateNote(ctx, other.ID, *a); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann ändern")
	}
	if err := s.DeleteNote(ctx, other.ID, a.ID); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann löschen")
	}
	if err := s.DeleteNote(ctx, u.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListNotes(ctx, u.ID); len(list) != 1 {
		t.Error("gelöschte Notiz sichtbar")
	}
}

func TestPlanner(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)
	other, _ := s.CreateUser(ctx, "andere", "", "")

	urlaub, err := s.CreatePlanner(ctx, u.ID, PlannerEntry{Title: "Urlaub", FirstDay: "2026-12-28", LastDay: "2027-01-03", Color: "green"})
	if err != nil {
		t.Fatal(err)
	}
	eintag, _ := s.CreatePlanner(ctx, u.ID, PlannerEntry{Title: "Messe", FirstDay: "2026-03-10"})
	if eintag.LastDay != "2026-03-10" || eintag.Color != "blue" {
		t.Errorf("Standardwerte: %+v", eintag)
	}

	var ve *ValidationError
	for _, bad := range []PlannerEntry{
		{Title: "", FirstDay: "2026-01-01"},
		{Title: "x", FirstDay: "01.01.2026"},
		{Title: "x", FirstDay: "2026-02-01", LastDay: "2026-01-01"},
		{Title: "x", FirstDay: "2026-01-01", LastDay: "2028-01-01"},
		{Title: "x", FirstDay: "2026-01-01", Color: "pink"},
	} {
		if _, err := s.CreatePlanner(ctx, u.ID, bad); !errors.As(err, &ve) {
			t.Errorf("akzeptiert: %+v", bad)
		}
	}

	y26, _ := s.ListPlanner(ctx, u.ID, 2026)
	y27, _ := s.ListPlanner(ctx, u.ID, 2027)
	y25, _ := s.ListPlanner(ctx, u.ID, 2025)
	if len(y26) != 2 || len(y27) != 1 || y27[0].ID != urlaub.ID || len(y25) != 0 {
		t.Errorf("Jahre: %d %d %d", len(y26), len(y27), len(y25))
	}
	if _, err := s.ListPlanner(ctx, u.ID, 12); !errors.As(err, &ve) {
		t.Error("ungültiges Jahr akzeptiert")
	}
	if list, _ := s.ListPlanner(ctx, other.ID, 2026); len(list) != 0 {
		t.Error("fremder Benutzer sieht Planer")
	}
	urlaub.Color = "red"
	if upd, err := s.UpdatePlanner(ctx, u.ID, *urlaub); err != nil || upd.Color != "red" {
		t.Errorf("ändern: %v", err)
	}
	if err := s.DeletePlanner(ctx, u.ID, urlaub.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListPlanner(ctx, u.ID, 2027); len(list) != 0 {
		t.Error("gelöschter Eintrag sichtbar")
	}
}

func TestAnniversaries(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)

	var ve *ValidationError
	if _, err := s.CreateAnniversary(ctx, u.ID, Anniversary{Title: "x", Month: 2, Day: 30}); !errors.As(err, &ve) {
		t.Error("30. Februar akzeptiert")
	}
	if _, err := s.CreateAnniversary(ctx, u.ID, Anniversary{Title: "Schalttag", Month: 2, Day: 29}); err != nil {
		t.Errorf("29. Februar: %v", err)
	}
	hochzeit, err := s.CreateAnniversary(ctx, u.ID, Anniversary{Title: "Hochzeitstag", Month: 6, Day: 12, Year: ptr(2001)})
	if err != nil {
		t.Fatal(err)
	}
	s.CreateContact(ctx, u.ID, Contact{GivenName: "Anna", FamilyName: "Ärmel", Birthday: ptr("1980-03-15")})
	s.CreateContact(ctx, u.ID, Contact{GivenName: "Bernd", Birthday: ptr("--06-12")})
	s.CreateContact(ctx, u.ID, Contact{GivenName: "Ohne"})

	list, err := s.ListAnniversaries(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("Anzahl: %+v", list)
	}
	if list[0].Title != "Schalttag" || list[1].Title != "Anna Ärmel" || list[1].Kind != "birthday" || list[1].Year == nil || *list[1].Year != 1980 {
		t.Errorf("Anfang: %+v %+v", list[0], list[1])
	}
	// Gleicher Tag: alphabetisch.
	if list[2].Title != "Bernd" || list[2].Year != nil || list[3].Title != "Hochzeitstag" {
		t.Errorf("12. Juni: %+v %+v", list[2], list[3])
	}
	hochzeit.Title = "Hochzeitstag Eltern"
	if upd, err := s.UpdateAnniversary(ctx, u.ID, *hochzeit); err != nil || upd.Title != "Hochzeitstag Eltern" {
		t.Errorf("ändern: %v", err)
	}
	if err := s.DeleteAnniversary(ctx, u.ID, hochzeit.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListAnniversaries(ctx, u.ID); len(list) != 3 {
		t.Error("gelöschter Jahrestag sichtbar")
	}
}

func TestBirthdayItem(t *testing.T) {
	for _, b := range []string{"", "-", "--1", "--13-01", "1980-02-30", "19800315", "--0315"} {
		if _, ok := birthdayItem(1, "x", b); ok {
			t.Errorf("%q akzeptiert", b)
		}
	}
	if it, ok := birthdayItem(1, "x", "--12-24"); !ok || it.Month != 12 || it.Day != 24 || it.Year != nil {
		t.Errorf("--12-24: %+v", it)
	}
}
