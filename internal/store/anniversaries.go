// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Anniversary ist ein eigener Jahrestag (z. B. Hochzeitstag, Firmenjubiläum).
type Anniversary struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Month     int       `json:"month"`
	Day       int       `json:"day"`
	Year      *int      `json:"year,omitempty"` // Ursprungsjahr, wenn bekannt
	UpdatedAt time.Time `json:"updatedAt"`
}

// AnniversaryItem ist ein Eintrag in der Jahrestage-Übersicht: entweder ein
// eigener Jahrestag oder ein Geburtstag aus dem Adressbuch.
type AnniversaryItem struct {
	Kind      string `json:"kind"`                // "custom" oder "birthday"
	ID        int64  `json:"id,omitempty"`        // bei custom
	ContactID int64  `json:"contactId,omitempty"` // bei birthday
	Title     string `json:"title"`
	Month     int    `json:"month"`
	Day       int    `json:"day"`
	Year      *int   `json:"year,omitempty"`
}

func validMonthDay(month, day int) bool {
	if month < 1 || month > 12 || day < 1 {
		return false
	}
	// 2000 ist ein Schaltjahr, damit ist der 29. Februar erlaubt.
	return day <= time.Date(2000, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func (a *Anniversary) normalize() error {
	a.Title = strings.TrimSpace(a.Title)
	switch {
	case a.Title == "":
		return &ValidationError{"title", "fehlt"}
	case utf8.RuneCountInString(a.Title) > maxTitle:
		return &ValidationError{"title", "zu lang"}
	case !validMonthDay(a.Month, a.Day):
		return &ValidationError{"day", "ungültiges Datum"}
	case a.Year != nil && (*a.Year < 1 || *a.Year > 9999):
		return &ValidationError{"year", "ungültiges Jahr"}
	}
	return nil
}

func scanAnniversary(row interface{ Scan(...any) error }) (*Anniversary, error) {
	var a Anniversary
	var year sql.NullInt64
	var updated string
	if err := row.Scan(&a.ID, &a.Title, &a.Month, &a.Day, &year, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if year.Valid {
		y := int(year.Int64)
		a.Year = &y
	}
	a.UpdatedAt, _ = parseTime(updated)
	return &a, nil
}

// GetAnniversary liefert einen eigenen Jahrestag.
func (s *Store) GetAnniversary(ctx context.Context, userID, id int64) (*Anniversary, error) {
	return scanAnniversary(s.DB.QueryRowContext(ctx,
		`SELECT id, title, month, day, year, updated_at FROM anniversaries WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, userID))
}

// ListAnniversaries liefert eigene Jahrestage und Geburtstage aus dem
// Adressbuch, sortiert nach Monat und Tag.
func (s *Store) ListAnniversaries(ctx context.Context, userID int64) ([]AnniversaryItem, error) {
	out := []AnniversaryItem{}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, title, month, day, year, updated_at FROM anniversaries WHERE user_id = ? AND deleted_at IS NULL`, userID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		a, err := scanAnniversary(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, AnniversaryItem{Kind: "custom", ID: a.ID, Title: a.Title, Month: a.Month, Day: a.Day, Year: a.Year})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	brows, err := s.DB.QueryContext(ctx,
		`SELECT id, display_name, birthday FROM contacts
		  WHERE user_id = ? AND deleted_at IS NULL AND birthday IS NOT NULL AND birthday <> ''`, userID)
	if err != nil {
		return nil, err
	}
	defer brows.Close()
	for brows.Next() {
		var id int64
		var name, b string
		if err := brows.Scan(&id, &name, &b); err != nil {
			return nil, err
		}
		if item, ok := birthdayItem(id, name, b); ok {
			out = append(out, item)
		}
	}
	if err := brows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Month != out[j].Month {
			return out[i].Month < out[j].Month
		}
		if out[i].Day != out[j].Day {
			return out[i].Day < out[j].Day
		}
		return out[i].Title < out[j].Title
	})
	return out, nil
}

// birthdayItem zerlegt YYYY-MM-DD oder --MM-DD.
func birthdayItem(contactID int64, name, b string) (AnniversaryItem, bool) {
	item := AnniversaryItem{Kind: "birthday", ContactID: contactID, Title: name}
	var md string
	if strings.HasPrefix(b, "--") {
		md = b[2:]
	} else if len(b) == 10 {
		y, err := strconv.Atoi(b[:4])
		if err != nil {
			return item, false
		}
		item.Year = &y
		md = b[5:]
	} else {
		return item, false
	}
	if len(md) != 5 || md[2] != '-' {
		return item, false
	}
	m, err1 := strconv.Atoi(md[:2])
	d, err2 := strconv.Atoi(md[3:])
	if err1 != nil || err2 != nil || !validMonthDay(m, d) {
		return item, false
	}
	item.Month, item.Day = m, d
	return item, true
}

// CreateAnniversary legt einen eigenen Jahrestag an.
func (s *Store) CreateAnniversary(ctx context.Context, userID int64, a Anniversary) (*Anniversary, error) {
	if err := a.normalize(); err != nil {
		return nil, err
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO anniversaries (user_id, title, month, day, year, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		userID, a.Title, a.Month, a.Day, nullable(a.Year), now, now)
	if err != nil {
		return nil, fmt.Errorf("Jahrestag anlegen: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetAnniversary(ctx, userID, id)
}

// UpdateAnniversary ändert einen eigenen Jahrestag.
func (s *Store) UpdateAnniversary(ctx context.Context, userID int64, a Anniversary) (*Anniversary, error) {
	if err := a.normalize(); err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE anniversaries SET title = ?, month = ?, day = ?, year = ?, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		a.Title, a.Month, a.Day, nullable(a.Year), formatTime(time.Now()), a.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("Jahrestag ändern: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetAnniversary(ctx, userID, a.ID)
}

// DeleteAnniversary löscht einen eigenen Jahrestag weich.
func (s *Store) DeleteAnniversary(ctx context.Context, userID, id int64) error {
	return s.softDelete(ctx, "anniversaries", userID, id)
}
