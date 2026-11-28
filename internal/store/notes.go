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

// Note ist eine Notizseite.
type Note struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	Position  int       `json:"position"`
	UpdatedAt time.Time `json:"updatedAt"`
}

const maxNoteBody = 200_000

func (n *Note) normalize() error {
	n.Title = strings.TrimSpace(n.Title)
	if utf8.RuneCountInString(n.Title) > maxTitle {
		return &ValidationError{"title", "zu lang"}
	}
	if utf8.RuneCountInString(n.Body) > maxNoteBody {
		return &ValidationError{"body", "zu lang"}
	}
	if n.Title == "" && strings.TrimSpace(n.Body) == "" {
		return &ValidationError{"title", "Titel oder Text fehlt"}
	}
	return nil
}

func scanNote(row interface{ Scan(...any) error }) (*Note, error) {
	var n Note
	var updated string
	if err := row.Scan(&n.ID, &n.Title, &n.Body, &n.Position, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	n.UpdatedAt, _ = parseTime(updated)
	return &n, nil
}

// ListNotes liefert alle Notizen, zuletzt geänderte zuerst.
func (s *Store) ListNotes(ctx context.Context, userID int64) ([]Note, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, title, body, position, updated_at FROM notes
		  WHERE user_id = ? AND deleted_at IS NULL ORDER BY updated_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Note{}
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// GetNote liefert eine Notiz.
func (s *Store) GetNote(ctx context.Context, userID, id int64) (*Note, error) {
	return scanNote(s.DB.QueryRowContext(ctx,
		`SELECT id, title, body, position, updated_at FROM notes WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, userID))
}

// CreateNote legt eine Notiz an.
func (s *Store) CreateNote(ctx context.Context, userID int64, n Note) (*Note, error) {
	if err := n.normalize(); err != nil {
		return nil, err
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO notes (user_id, title, body, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, n.Title, n.Body, n.Position, now, now)
	if err != nil {
		return nil, fmt.Errorf("Notiz anlegen: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetNote(ctx, userID, id)
}

// UpdateNote ändert eine Notiz.
func (s *Store) UpdateNote(ctx context.Context, userID int64, n Note) (*Note, error) {
	if err := n.normalize(); err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE notes SET title = ?, body = ?, position = ?, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		n.Title, n.Body, n.Position, formatTime(time.Now()), n.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("Notiz ändern: %w", err)
	}
	if c, _ := res.RowsAffected(); c == 0 {
		return nil, ErrNotFound
	}
	return s.GetNote(ctx, userID, n.ID)
}

// DeleteNote löscht eine Notiz weich.
func (s *Store) DeleteNote(ctx context.Context, userID, id int64) error {
	return s.softDelete(ctx, "notes", userID, id)
}

// softDelete markiert einen Eintrag einer Benutzertabelle als gelöscht.
func (s *Store) softDelete(ctx context.Context, table string, userID, id int64) error {
	switch table {
	case "notes", "anniversaries", "planner_entries":
	default:
		return fmt.Errorf("softDelete: Tabelle %q nicht erlaubt", table)
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE `+table+` SET deleted_at = ?, updated_at = ? WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		now, now, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
