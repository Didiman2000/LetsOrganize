// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"
)

// Calendar ist ein Kalender eines Google-Kontos.
type Calendar struct {
	ID         int64  `json:"id"`
	RemoteID   string `json:"remoteId"` // wie events.calendarId; Hauptkalender = "primary"
	Name       string `json:"name"`
	Color      string `json:"color"` // #rrggbb oder leer
	AccessRole string `json:"accessRole"`
	Primary    bool   `json:"primary"`
	Enabled    bool   `json:"enabled"`
	Writable   bool   `json:"writable"`
	Position   int    `json:"position"`
}

// writableRoles sind die Google-Rollen, mit denen Termine geändert werden dürfen.
var writableRoles = map[string]bool{"owner": true, "writer": true}

var reHexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

const calendarColumns = `id, remote_id, name, color, access_role, is_primary, enabled, position`

func scanCalendar(row interface{ Scan(...any) error }) (*Calendar, error) {
	var c Calendar
	if err := row.Scan(&c.ID, &c.RemoteID, &c.Name, &c.Color, &c.AccessRole, &c.Primary, &c.Enabled, &c.Position); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.Writable = writableRoles[c.AccessRole]
	return &c, nil
}

// ListCalendars liefert die Kalender des Benutzers: Hauptkalender zuerst.
func (s *Store) ListCalendars(ctx context.Context, userID int64) ([]Calendar, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+calendarColumns+` FROM calendars WHERE user_id = ? ORDER BY is_primary DESC, position, name, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Calendar{}
	for rows.Next() {
		c, err := scanCalendar(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// CalendarByRemote liefert einen Kalender anhand seiner Google-ID.
func (s *Store) CalendarByRemote(ctx context.Context, userID int64, remoteID string) (*Calendar, error) {
	return scanCalendar(s.DB.QueryRowContext(ctx,
		`SELECT `+calendarColumns+` FROM calendars WHERE user_id = ? AND remote_id = ?`, userID, remoteID))
}

// SetCalendarEnabled schaltet Abgleich und Anzeige eines Kalenders.
func (s *Store) SetCalendarEnabled(ctx context.Context, userID, id int64, enabled bool) (*Calendar, error) {
	res, err := s.DB.ExecContext(ctx, `UPDATE calendars SET enabled = ? WHERE id = ? AND user_id = ?`, enabled, id, userID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return scanCalendar(s.DB.QueryRowContext(ctx, `SELECT `+calendarColumns+` FROM calendars WHERE id = ?`, id))
}

// RemoteCalendar ist ein Eintrag der Kalenderliste von Google.
type RemoteCalendar struct {
	RemoteID   string
	Name       string
	Color      string
	AccessRole string
	Primary    bool
	Selected   bool // bei Google eingeblendet: Voreinstellung für neue Kalender
}

// SyncCalendarList gleicht die Kalenderliste ab: neue Kalender anlegen,
// Namen, Farben und Rechte aktualisieren (die eigene Auswahl bleibt),
// verschwundene Kalender samt ihrer Termine entfernen.
func (s *Store) SyncCalendarList(ctx context.Context, userID, accountID int64, list []RemoteCalendar) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	keep := map[string]bool{}
	for i, rc := range list {
		if rc.RemoteID == "" {
			continue
		}
		if !reHexColor.MatchString(rc.Color) {
			rc.Color = ""
		}
		keep[rc.RemoteID] = true
		enabled := rc.Selected || rc.Primary
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO calendars (user_id, account_id, remote_id, name, color, access_role, is_primary, enabled, position)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT (account_id, remote_id) DO UPDATE SET
			   name = excluded.name, color = excluded.color, access_role = excluded.access_role,
			   is_primary = excluded.is_primary, position = excluded.position`,
			userID, accountID, rc.RemoteID, rc.Name, rc.Color, rc.AccessRole, rc.Primary, enabled, i); err != nil {
			return fmt.Errorf("Kalender %s speichern: %w", rc.Name, err)
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, remote_id FROM calendars WHERE account_id = ?`, accountID)
	if err != nil {
		return err
	}
	var gone []struct {
		id  int64
		rid string
	}
	for rows.Next() {
		var id int64
		var rid string
		if err := rows.Scan(&id, &rid); err != nil {
			rows.Close()
			return err
		}
		if !keep[rid] {
			gone = append(gone, struct {
				id  int64
				rid string
			}{id, rid})
		}
	}
	rows.Close()
	now := formatTime(time.Now())
	for _, g := range gone {
		if _, err := tx.ExecContext(ctx,
			`UPDATE events SET deleted_at = COALESCE(deleted_at, ?), dirty = 0 WHERE user_id = ? AND calendar_id = ?`,
			now, userID, g.rid); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM calendars WHERE id = ?`, g.id); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sync_state WHERE account_id = ? AND resource = ?`, accountID, "calendar:"+g.rid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// checkCalendarWritable stellt sicher, dass in den Kalender geschrieben werden
// darf. Leere ID heißt: Hauptkalender bzw. nur lokal.
func (s *Store) checkCalendarWritable(ctx context.Context, userID int64, calendarID string) error {
	if calendarID == "" {
		return nil
	}
	c, err := s.CalendarByRemote(ctx, userID, calendarID)
	if errors.Is(err, ErrNotFound) {
		if calendarID == "primary" {
			return nil
		}
		return &ValidationError{"calendarId", "unbekannter Kalender"}
	}
	if err != nil {
		return err
	}
	if !c.Writable {
		return &ValidationError{"calendarId", fmt.Sprintf("im Kalender „%s“ darfst du nur lesen", c.Name)}
	}
	return nil
}
