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

// PlannerColors sind die erlaubten Farben im Jahresplaner. Das Thema legt
// fest, wie sie aussehen.
var PlannerColors = []string{"red", "orange", "green", "blue", "purple", "brown"}

// PlannerEntry ist ein Zeitraum im Jahresplaner, z. B. Urlaub.
type PlannerEntry struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	FirstDay  string    `json:"firstDay"` // YYYY-MM-DD
	LastDay   string    `json:"lastDay"`  // YYYY-MM-DD, einschließlich
	Color     string    `json:"color"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (p *PlannerEntry) normalize() error {
	p.Title = strings.TrimSpace(p.Title)
	if p.Title == "" {
		return &ValidationError{"title", "fehlt"}
	}
	if utf8.RuneCountInString(p.Title) > maxTitle {
		return &ValidationError{"title", "zu lang"}
	}
	first, err := time.Parse(time.DateOnly, p.FirstDay)
	if err != nil {
		return &ValidationError{"firstDay", "Datum im Format JJJJ-MM-TT angeben"}
	}
	if p.LastDay == "" {
		p.LastDay = p.FirstDay
	}
	last, err := time.Parse(time.DateOnly, p.LastDay)
	if err != nil {
		return &ValidationError{"lastDay", "Datum im Format JJJJ-MM-TT angeben"}
	}
	if last.Before(first) {
		return &ValidationError{"lastDay", "liegt vor dem ersten Tag"}
	}
	if last.Sub(first) > 366*24*time.Hour {
		return &ValidationError{"lastDay", "Zeitraum höchstens ein Jahr"}
	}
	if p.Color == "" {
		p.Color = "blue"
	}
	for _, c := range PlannerColors {
		if c == p.Color {
			return nil
		}
	}
	return &ValidationError{"color", "unbekannte Farbe"}
}

func scanPlanner(row interface{ Scan(...any) error }) (*PlannerEntry, error) {
	var p PlannerEntry
	var updated string
	if err := row.Scan(&p.ID, &p.Title, &p.FirstDay, &p.LastDay, &p.Color, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.UpdatedAt, _ = parseTime(updated)
	return &p, nil
}

const plannerColumns = `id, title, first_day, last_day, color, updated_at`

// ListPlanner liefert alle Einträge, die das Jahr year berühren.
func (s *Store) ListPlanner(ctx context.Context, userID int64, year int) ([]PlannerEntry, error) {
	if year < 1900 || year > 2200 {
		return nil, &ValidationError{"year", "ungültiges Jahr"}
	}
	from, to := fmt.Sprintf("%04d-01-01", year), fmt.Sprintf("%04d-12-31", year)
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+plannerColumns+` FROM planner_entries
		  WHERE user_id = ? AND deleted_at IS NULL AND first_day <= ? AND last_day >= ?
		  ORDER BY first_day, id`, userID, to, from)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PlannerEntry{}
	for rows.Next() {
		p, err := scanPlanner(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPlanner liefert einen Planer-Eintrag.
func (s *Store) GetPlanner(ctx context.Context, userID, id int64) (*PlannerEntry, error) {
	return scanPlanner(s.DB.QueryRowContext(ctx,
		`SELECT `+plannerColumns+` FROM planner_entries WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, userID))
}

// CreatePlanner legt einen Planer-Eintrag an.
func (s *Store) CreatePlanner(ctx context.Context, userID int64, p PlannerEntry) (*PlannerEntry, error) {
	if err := p.normalize(); err != nil {
		return nil, err
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO planner_entries (user_id, title, first_day, last_day, color, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`, userID, p.Title, p.FirstDay, p.LastDay, p.Color, now, now)
	if err != nil {
		return nil, fmt.Errorf("Planer-Eintrag anlegen: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetPlanner(ctx, userID, id)
}

// UpdatePlanner ändert einen Planer-Eintrag.
func (s *Store) UpdatePlanner(ctx context.Context, userID int64, p PlannerEntry) (*PlannerEntry, error) {
	if err := p.normalize(); err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE planner_entries SET title = ?, first_day = ?, last_day = ?, color = ?, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		p.Title, p.FirstDay, p.LastDay, p.Color, formatTime(time.Now()), p.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("Planer-Eintrag ändern: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetPlanner(ctx, userID, p.ID)
}

// DeletePlanner löscht einen Planer-Eintrag weich.
func (s *Store) DeletePlanner(ctx context.Context, userID, id int64) error {
	return s.softDelete(ctx, "planner_entries", userID, id)
}
