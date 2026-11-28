// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"errors"
	"testing"
)

func TestContacts(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)
	other, _ := s.CreateUser(ctx, "andere", "", "")

	müller, err := s.CreateContact(ctx, u.ID, Contact{
		GivenName: "Anna", FamilyName: "Ärmel", Organization: "Beispiel GmbH",
		Birthday: ptr("--03-15"),
		Fields: []ContactField{
			{Kind: "phone", Label: "work", Value: "+49 30 1234567"},
			{Kind: "email", Label: "work", Value: "anna@example.com"},
			{Kind: "phone", Label: "", Value: "  "}, // leere Zeile wird ignoriert
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if müller.DisplayName != "Anna Ärmel" || müller.Letter != "A" || len(müller.Fields) != 2 {
		t.Errorf("angelegt: %+v", müller)
	}
	firma, _ := s.CreateContact(ctx, u.ID, Contact{Organization: "Zentrale Stadtwerke"})
	if firma.DisplayName != "Zentrale Stadtwerke" || firma.Letter != "Z" {
		t.Errorf("nur Firma: %+v", firma)
	}
	num, _ := s.CreateContact(ctx, u.ID, Contact{DisplayName: "112 Notruf"})
	if num.Letter != "#" {
		t.Errorf("Ziffer: %q", num.Letter)
	}
	s.CreateContact(ctx, u.ID, Contact{GivenName: "Bernd", FamilyName: "Bauer"})

	var ve *ValidationError
	if _, err := s.CreateContact(ctx, u.ID, Contact{}); !errors.As(err, &ve) {
		t.Error("leerer Kontakt akzeptiert")
	}
	if _, err := s.CreateContact(ctx, u.ID, Contact{GivenName: "x", Fields: []ContactField{{Kind: "fax", Value: "1"}}}); !errors.As(err, &ve) {
		t.Error("unbekannte Feldart akzeptiert")
	}
	if _, err := s.CreateContact(ctx, u.ID, Contact{GivenName: "x", Fields: []ContactField{{Kind: "email", Value: "kein-at"}}}); !errors.As(err, &ve) {
		t.Error("ungültige E-Mail akzeptiert")
	}
	if _, err := s.CreateContact(ctx, u.ID, Contact{GivenName: "x", Birthday: ptr("--13-40")}); !errors.As(err, &ve) {
		t.Error("ungültiger Geburtstag akzeptiert")
	}

	all, err := s.ListContacts(ctx, u.ID, "")
	if err != nil || len(all) != 4 {
		t.Fatalf("alle: %v %d", err, len(all))
	}
	order := []string{all[0].DisplayName, all[1].DisplayName, all[2].DisplayName, all[3].DisplayName}
	if order[0] != "112 Notruf" || order[1] != "Anna Ärmel" || order[2] != "Bernd Bauer" || order[3] != "Zentrale Stadtwerke" {
		t.Errorf("Reihenfolge: %v", order)
	}
	if len(all[1].Fields) != 2 {
		t.Errorf("Felder in Liste: %+v", all[1].Fields)
	}

	// Suche in Name, Firma und Feldern; Platzhalter werden nicht als Muster verstanden.
	for q, want := range map[string]int{"beispiel": 1, "1234567": 1, "anna@": 1, "%": 0, "bauer": 1} {
		if got, _ := s.ListContacts(ctx, u.ID, q); len(got) != want {
			t.Errorf("Suche %q: %d Treffer, erwartet %d", q, len(got), want)
		}
	}

	// Ändern ersetzt die Felder.
	müller.Fields = []ContactField{{Kind: "address", Label: "home", Value: "Hauptstr. 1\n10115 Berlin"}}
	upd, err := s.UpdateContact(ctx, u.ID, *müller)
	if err != nil || len(upd.Fields) != 1 || upd.Fields[0].Kind != "address" {
		t.Fatalf("ändern: %v %+v", err, upd)
	}

	// Fremde Benutzer.
	if got, _ := s.ListContacts(ctx, other.ID, ""); len(got) != 0 {
		t.Error("fremder Benutzer sieht Kontakte")
	}
	if _, err := s.UpdateContact(ctx, other.ID, *müller); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann ändern")
	}
	if err := s.DeleteContact(ctx, other.ID, müller.ID); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann löschen")
	}

	if err := s.DeleteContact(ctx, u.ID, müller.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetContact(ctx, u.ID, müller.ID); !errors.Is(err, ErrNotFound) {
		t.Error("gelöschter Kontakt sichtbar")
	}
}

func TestLetter(t *testing.T) {
	if got := Letter("(KITA)"); got != "#" {
		t.Errorf("Klammer: %q", got)
	}
	if got := Letter(sortKey(&Contact{FamilyName: "Øster"})); got != "O" {
		t.Errorf("Øster: %q", got)
	}
	if got := Letter(sortKey(&Contact{FamilyName: "šimek"})); got != "S" {
		t.Errorf("šimek: %q", got)
	}
	if got := Letter(sortKey(&Contact{DisplayName: "أحمد"})); got != "أ" {
		t.Errorf("arabisch: %q", got)
	}
	if got := Letter(""); got != "#" {
		t.Errorf("leer: %q", got)
	}
}
