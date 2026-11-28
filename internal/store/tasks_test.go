// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"errors"
	"testing"
)

func ptr[T any](v T) *T { return &v }

func TestTasks(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.EnsureLocalUser(ctx)
	other, _ := s.CreateUser(ctx, "andere", "", "")

	a, err := s.CreateTask(ctx, u.ID, Task{Title: " Bericht schreiben ", Due: ptr("2026-12-05"), Priority: 2})
	if err != nil {
		t.Fatal(err)
	}
	if a.Title != "Bericht schreiben" || a.Done {
		t.Errorf("angelegt: %+v", a)
	}
	b, _ := s.CreateTask(ctx, u.ID, Task{Title: "Ohne Datum", Priority: 3})
	c, _ := s.CreateTask(ctx, u.ID, Task{Title: "Früher fällig", Due: ptr("2026-12-01")})
	if _, err := s.CreateTask(ctx, u.ID, Task{Title: "Leeres Datum", Due: ptr(" ")}); err != nil {
		t.Errorf("leeres Datum: %v", err)
	}

	var ve *ValidationError
	if _, err := s.CreateTask(ctx, u.ID, Task{Title: "x", Due: ptr("05.12.2026")}); !errors.As(err, &ve) || ve.Field != "due" {
		t.Errorf("falsches Datum: %v", err)
	}
	if _, err := s.CreateTask(ctx, u.ID, Task{Title: "x", Priority: 4}); !errors.As(err, &ve) || ve.Field != "priority" {
		t.Errorf("falsche Priorität: %v", err)
	}

	open, _ := s.ListTasks(ctx, u.ID, TasksOpen, 0)
	if len(open) != 4 || open[0].ID != c.ID || open[1].ID != a.ID || open[2].ID != b.ID {
		t.Errorf("Reihenfolge offen: %+v", open)
	}

	done, err := s.SetTaskDone(ctx, u.ID, a.ID, true)
	if err != nil || !done.Done || done.DoneAt == nil {
		t.Fatalf("abhaken: %v %+v", err, done)
	}
	first := *done.DoneAt
	// Erneut speichern als erledigt behält den Zeitpunkt.
	done.Notes = "mit Anhang"
	again, _ := s.UpdateTask(ctx, u.ID, *done)
	if again.DoneAt == nil || !again.DoneAt.Equal(first) {
		t.Error("Erledigt-Zeitpunkt nicht erhalten")
	}
	if list, _ := s.ListTasks(ctx, u.ID, TasksDone, 10); len(list) != 1 || list[0].ID != a.ID {
		t.Errorf("erledigt: %+v", list)
	}
	reopened, _ := s.SetTaskDone(ctx, u.ID, a.ID, false)
	if reopened.Done || reopened.DoneAt != nil {
		t.Error("wieder öffnen")
	}

	// Fremde Benutzer.
	if list, _ := s.ListTasks(ctx, other.ID, TasksAll, 0); len(list) != 0 {
		t.Error("fremder Benutzer sieht Aufgaben")
	}
	if _, err := s.SetTaskDone(ctx, other.ID, a.ID, true); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann abhaken")
	}
	if err := s.DeleteTask(ctx, other.ID, a.ID); !errors.Is(err, ErrNotFound) {
		t.Error("fremder Benutzer kann löschen")
	}

	if err := s.DeleteTask(ctx, u.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetTask(ctx, u.ID, b.ID); !errors.Is(err, ErrNotFound) {
		t.Error("gelöschte Aufgabe sichtbar")
	}
}
