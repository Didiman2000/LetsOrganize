// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func TestContactsAPI(t *testing.T) {
	s := newDBServer(t)
	body := `{"givenName":"Anna","familyName":"Ärmel","fields":[{"kind":"email","label":"work","value":"anna@example.com"}]}`
	if rec := do(s, "POST", "/api/contacts", body, map[string]string{"Content-Type": "application/json"}); rec.Code != http.StatusForbidden {
		t.Errorf("ohne Kopf: %d", rec.Code)
	}
	rec := do(s, "POST", "/api/contacts", body, jsonHeaders)
	if rec.Code != http.StatusCreated {
		t.Fatalf("anlegen: %d %s", rec.Code, rec.Body)
	}
	var c store.Contact
	json.Unmarshal(rec.Body.Bytes(), &c)
	if c.Letter != "A" || len(c.Fields) != 1 {
		t.Errorf("angelegt: %+v", c)
	}
	if rec := do(s, "POST", "/api/contacts", `{"fields":[]}`, jsonHeaders); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("ohne Namen: %d", rec.Code)
	}
	if rec := do(s, "GET", "/api/contacts?q=example", "", nil); !strings.Contains(rec.Body.String(), "Ärmel") {
		t.Errorf("Suche: %s", rec.Body)
	}
	url := "/api/contacts/" + jsonID(c.ID)
	if rec := do(s, "PUT", url, `{"givenName":"Anna","familyName":"Ärmel","organization":"Beispiel GmbH","fields":[]}`, jsonHeaders); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Beispiel GmbH") {
		t.Errorf("ändern: %d %s", rec.Code, rec.Body)
	}
	if rec := do(s, "DELETE", url, "", map[string]string{csrfHeader: "1"}); rec.Code != http.StatusNoContent {
		t.Errorf("löschen: %d", rec.Code)
	}
	if rec := do(s, "GET", url, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("nach Löschen: %d", rec.Code)
	}
}
