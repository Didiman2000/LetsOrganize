// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Task ist eine Aufgabe.
type Task struct {
	ID         int64      `json:"id"`
	Title      string     `json:"title"`
	Notes      string     `json:"notes"`
	Due        *string    `json:"due,omitempty"` // YYYY-MM-DD
	Priority   int        `json:"priority"`      // 0 = keine, 1 = niedrig, 2 = mittel, 3 = hoch
	Done       bool       `json:"done"`
	DoneAt     *time.Time `json:"doneAt,omitempty"`
	Position   int        `json:"position"`
	CategoryID *int64     `json:"categoryId,omitempty"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

func (t *Task) normalize() error {
	t.Title = strings.TrimSpace(t.Title)
	switch {
	case t.Title == "":
		return &ValidationError{"title", "fehlt"}
	case utf8.RuneCountInString(t.Title) > maxTitle:
		return &ValidationError{"title", "zu lang"}
	case utf8.RuneCountInString(t.Notes) > maxText:
		return &ValidationError{"notes", "zu lang"}
	case t.Priority < 0 || t.Priority > 3:
		return &ValidationError{"priority", "muss zwischen 0 und 3 liegen"}
	}
	if t.Due != nil {
		d := strings.TrimSpace(*t.Due)
		if d == "" {
			t.Due = nil
		} else if _, err := time.Parse(time.DateOnly, d); err != nil {
			return &ValidationError{"due", "Datum im Format JJJJ-MM-TT angeben"}
		} else {
			t.Due = &d
		}
	}
	return nil
}

const taskColumns = `id, title, notes, due_date, priority, done_at, position, category_id, updated_at`

func scanTask(row interface{ Scan(...any) error }) (*Task, error) {
	var t Task
	var due, doneAt sql.NullString
	var category sql.NullInt64
	var updated string
	if err := row.Scan(&t.ID, &t.Title, &t.Notes, &due, &t.Priority, &doneAt, &t.Position, &category, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if due.Valid {
		t.Due = &due.String
	}
	if doneAt.Valid {
		if v, err := parseTime(doneAt.String); err == nil {
			t.DoneAt = &v
		}
		t.Done = true
	}
	if category.Valid {
		c := category.Int64
		t.CategoryID = &c
	}
	t.UpdatedAt, _ = parseTime(updated)
	return &t, nil
}

// TaskFilter wählt Aufgaben aus.
type TaskFilter int

const (
	TasksOpen TaskFilter = iota // offene Aufgaben
	TasksDone                   // erledigte, neueste zuerst
	TasksAll
)

// ListTasks liefert Aufgaben des Benutzers. Offene sind sortiert nach
// Fälligkeit (ohne Datum zuletzt), dann Priorität, dann eigener Reihenfolge.
// limit <= 0 heißt ohne Begrenzung.
func (s *Store) ListTasks(ctx context.Context, userID int64, filter TaskFilter, limit int) ([]Task, error) {
	where := `user_id = ? AND deleted_at IS NULL`
	order := `due_date IS NULL, due_date, priority DESC, position, id`
	switch filter {
	case TasksOpen:
		where += ` AND done_at IS NULL`
	case TasksDone:
		where += ` AND done_at IS NOT NULL`
		order = `done_at DESC, id DESC`
	}
	q := `SELECT ` + taskColumns + ` FROM tasks WHERE ` + where + ` ORDER BY ` + order
	args := []any{userID}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// GetTask liefert eine Aufgabe des Benutzers.
func (s *Store) GetTask(ctx context.Context, userID, id int64) (*Task, error) {
	return scanTask(s.DB.QueryRowContext(ctx,
		`SELECT `+taskColumns+` FROM tasks WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, userID))
}

func doneValue(done bool, now string) any {
	if done {
		return now
	}
	return nil
}

// CreateTask legt eine Aufgabe an.
func (s *Store) CreateTask(ctx context.Context, userID int64, t Task) (*Task, error) {
	if err := t.normalize(); err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, userID, t.CategoryID); err != nil {
		return nil, err
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO tasks (user_id, title, notes, due_date, priority, done_at, position, category_id, dirty, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		userID, t.Title, t.Notes, nullable(t.Due), t.Priority, doneValue(t.Done, now), t.Position,
		nullable(t.CategoryID), now, now)
	if err != nil {
		return nil, fmt.Errorf("Aufgabe anlegen: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetTask(ctx, userID, id)
}

// UpdateTask ändert eine Aufgabe. Wird sie erledigt, gilt der jetzige
// Zeitpunkt; bleibt sie erledigt, bleibt der alte Zeitpunkt erhalten.
func (s *Store) UpdateTask(ctx context.Context, userID int64, t Task) (*Task, error) {
	if err := t.normalize(); err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, userID, t.CategoryID); err != nil {
		return nil, err
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE tasks SET title = ?, notes = ?, due_date = ?, priority = ?,
		       done_at = CASE WHEN ? THEN COALESCE(done_at, ?) ELSE NULL END,
		       position = ?, category_id = ?, dirty = 1, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		t.Title, t.Notes, nullable(t.Due), t.Priority, t.Done, now,
		t.Position, nullable(t.CategoryID), now, t.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("Aufgabe ändern: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetTask(ctx, userID, t.ID)
}

// SetTaskDone hakt eine Aufgabe ab oder öffnet sie wieder.
func (s *Store) SetTaskDone(ctx context.Context, userID, id int64, done bool) (*Task, error) {
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE tasks SET done_at = CASE WHEN ? THEN COALESCE(done_at, ?) ELSE NULL END, dirty = 1, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, done, now, now, id, userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetTask(ctx, userID, id)
}

// DeleteTask löscht eine Aufgabe weich.
func (s *Store) DeleteTask(ctx context.Context, userID, id int64) error {
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE tasks SET deleted_at = ?, updated_at = ?, dirty = 1
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, now, now, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
