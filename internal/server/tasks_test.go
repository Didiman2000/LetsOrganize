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

func TestTasksAPI(t *testing.T) {
	s := newDBServer(t)

	if rec := do(s, "POST", "/api/tasks", `{"title":"x"}`, map[string]string{"Content-Type": "application/json"}); rec.Code != http.StatusForbidden {
		t.Errorf("ohne Kopf: %d", rec.Code)
	}
	rec := do(s, "POST", "/api/tasks", `{"title":"Angebot prüfen","due":"2026-12-04","priority":3}`, jsonHeaders)
	if rec.Code != http.StatusCreated {
		t.Fatalf("anlegen: %d %s", rec.Code, rec.Body)
	}
	var task store.Task
	json.Unmarshal(rec.Body.Bytes(), &task)
	url := "/api/tasks/" + jsonID(task.ID)

	if rec := do(s, "POST", "/api/tasks", `{"title":"x","due":"morgen"}`, jsonHeaders); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("falsches Datum: %d", rec.Code)
	}
	if rec := do(s, "GET", "/api/tasks?status=kaputt", "", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("falscher Status: %d", rec.Code)
	}
	if rec := do(s, "PUT", url+"/done", `{}`, jsonHeaders); rec.Code != http.StatusBadRequest {
		t.Errorf("done fehlt: %d", rec.Code)
	}
	if rec := do(s, "PUT", url+"/done", `{"done":true}`, jsonHeaders); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"done": true`) {
		t.Errorf("abhaken: %d %s", rec.Code, rec.Body)
	}
	if rec := do(s, "GET", "/api/tasks", "", nil); strings.Contains(rec.Body.String(), "Angebot") {
		t.Error("erledigte Aufgabe in offener Liste")
	}
	if rec := do(s, "GET", "/api/tasks?status=done&limit=5", "", nil); !strings.Contains(rec.Body.String(), "Angebot") {
		t.Error("erledigte Aufgabe fehlt")
	}
	if rec := do(s, "PUT", url, `{"title":"Angebot prüfen (Kunde B)","priority":1}`, jsonHeaders); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Kunde B") {
		t.Errorf("ändern: %d %s", rec.Code, rec.Body)
	}
	if rec := do(s, "DELETE", url, "", map[string]string{csrfHeader: "1"}); rec.Code != http.StatusNoContent {
		t.Errorf("löschen: %d", rec.Code)
	}
	if rec := do(s, "GET", url, "", nil); rec.Code != http.StatusNotFound {
		t.Errorf("nach Löschen: %d", rec.Code)
	}
}
