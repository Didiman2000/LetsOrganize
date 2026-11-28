// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRegistersAPI(t *testing.T) {
	s := newDBServer(t)
	cases := []struct {
		base, create, update, list, want string
	}{
		{"/api/notes", `{"title":"Ideen","body":"Thema in Grün"}`, `{"title":"Ideen","body":"Thema in Blau"}`, "/api/notes", "Blau"},
		{"/api/planner", `{"title":"Urlaub","firstDay":"2026-08-03","lastDay":"2026-08-14","color":"green"}`, `{"title":"Urlaub Ostsee","firstDay":"2026-08-03","lastDay":"2026-08-14","color":"green"}`, "/api/planner?year=2026", "Ostsee"},
		{"/api/anniversaries", `{"title":"Firmenjubiläum","month":4,"day":1,"year":2010}`, `{"title":"Firmenjubiläum N&O","month":4,"day":1}`, "/api/anniversaries", "N&O"},
	}
	for _, c := range cases {
		if rec := do(s, "POST", c.base, c.create, map[string]string{"Content-Type": "application/json"}); rec.Code != http.StatusForbidden {
			t.Errorf("%s ohne Kopf: %d", c.base, rec.Code)
		}
		rec := do(s, "POST", c.base, c.create, jsonHeaders)
		if rec.Code != http.StatusCreated {
			t.Fatalf("%s anlegen: %d %s", c.base, rec.Code, rec.Body)
		}
		var v struct{ ID int64 }
		json.Unmarshal(rec.Body.Bytes(), &v)
		url := c.base + "/" + jsonID(v.ID)
		if rec := do(s, "PUT", url, c.update, jsonHeaders); rec.Code != 200 {
			t.Errorf("%s ändern: %d %s", c.base, rec.Code, rec.Body)
		}
		if rec := do(s, "GET", c.list, "", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s auflisten: %d %s", c.base, rec.Code, rec.Body)
		}
		if rec := do(s, "DELETE", url, "", map[string]string{csrfHeader: "1"}); rec.Code != http.StatusNoContent {
			t.Errorf("%s löschen: %d", c.base, rec.Code)
		}
		if rec := do(s, "GET", url, "", nil); rec.Code != http.StatusNotFound {
			t.Errorf("%s nach Löschen: %d", c.base, rec.Code)
		}
	}
	if rec := do(s, "GET", "/api/planner?year=abc", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("Planer ungültiges Jahr: %d", rec.Code)
	}
	if rec := do(s, "POST", "/api/anniversaries", `{"title":"x","month":2,"day":30}`, jsonHeaders); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("30. Februar: %d", rec.Code)
	}
}
