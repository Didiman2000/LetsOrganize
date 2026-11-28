// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Didiman2000/LetsOrganize/internal/recur"
)

// Zeiten werden immer in UTC mit fester Länge gespeichert. So lassen sie sich
// in SQL als Text vergleichen und sortieren.
const timeLayout = "2006-01-02T15:04:05Z"

func formatTime(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTime(s string) (time.Time, error) { return time.Parse(timeLayout, s) }

// Event ist ein Termin.
//
// Ganztägige Termine beginnen um 00:00 UTC des ersten Tages und enden um
// 00:00 UTC nach dem letzten Tag (wie bei Google Kalender: Ende exklusiv).
type Event struct {
	ID       int64     `json:"id"`
	Title    string    `json:"title"`
	Location string    `json:"location"`
	Notes    string    `json:"notes"`
	AllDay   bool      `json:"allDay"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	RRule    string    `json:"rrule,omitempty"`
	// TimeZone ist die IANA-Zeitzone (z. B. Europe/Berlin), in der eine Serie
	// gilt. Damit bleibt „jeden Montag 9 Uhr“ auch nach der Zeitumstellung 9 Uhr.
	TimeZone        string    `json:"timeZone,omitempty"`
	ReminderMinutes *int      `json:"reminderMinutes,omitempty"`
	CategoryID      *int64    `json:"categoryId,omitempty"`
	UpdatedAt       time.Time `json:"updatedAt"`

	// Nur in Listen: Bei Vorkommen einer Serie stehen in Start/End die Zeiten
	// dieses Vorkommens, in SeriesStart der Beginn des ersten Termins.
	SeriesStart *time.Time `json:"seriesStart,omitempty"`

	exdates map[string]bool // Beginn gelöschter Vorkommen im Format timeLayout
}

// Grenzen für Eingaben, damit niemand die Datenbank mit Riesentexten füllt.
const (
	maxTitle = 500
	maxText  = 20_000
)

// ValidationError meldet eine ungültige Eingabe; Field nennt das Feld.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

func (e *Event) normalize() error {
	e.Title = strings.TrimSpace(e.Title)
	e.Location = strings.TrimSpace(e.Location)
	switch {
	case e.Title == "":
		return &ValidationError{"title", "fehlt"}
	case utf8.RuneCountInString(e.Title) > maxTitle:
		return &ValidationError{"title", "zu lang"}
	case utf8.RuneCountInString(e.Location) > maxTitle:
		return &ValidationError{"location", "zu lang"}
	case utf8.RuneCountInString(e.Notes) > maxText:
		return &ValidationError{"notes", "zu lang"}
	case utf8.RuneCountInString(e.RRule) > maxTitle:
		return &ValidationError{"rrule", "zu lang"}
	case e.Start.IsZero() || e.End.IsZero():
		return &ValidationError{"start", "Beginn und Ende fehlen"}
	}
	e.Start, e.End = e.Start.UTC().Truncate(time.Second), e.End.UTC().Truncate(time.Second)
	if e.AllDay {
		e.Start = time.Date(e.Start.Year(), e.Start.Month(), e.Start.Day(), 0, 0, 0, 0, time.UTC)
		end := time.Date(e.End.Year(), e.End.Month(), e.End.Day(), 0, 0, 0, 0, time.UTC)
		if !end.After(e.Start) {
			end = e.Start.AddDate(0, 0, 1)
		}
		e.End = end
	}
	if e.End.Before(e.Start) {
		return &ValidationError{"end", "liegt vor dem Beginn"}
	}
	e.TimeZone = strings.TrimSpace(e.TimeZone)
	if e.TimeZone != "" {
		if len(e.TimeZone) > 64 {
			return &ValidationError{"timeZone", "zu lang"}
		}
		if _, err := time.LoadLocation(e.TimeZone); err != nil {
			return &ValidationError{"timeZone", "unbekannte Zeitzone"}
		}
	}
	e.RRule = strings.TrimSpace(e.RRule)
	if e.RRule != "" {
		r, err := recur.Parse(e.RRule)
		if err != nil {
			return &ValidationError{"rrule", err.Error()}
		}
		e.RRule = r.String()
		if e.End.Sub(e.Start) > 31*24*time.Hour {
			return &ValidationError{"end", "ein Termin einer Serie darf höchstens 31 Tage dauern"}
		}
	}
	if e.ReminderMinutes != nil && (*e.ReminderMinutes < 0 || *e.ReminderMinutes > 40_320) {
		return &ValidationError{"reminderMinutes", "muss zwischen 0 und 40320 liegen"}
	}
	return nil
}

const eventColumns = `id, title, location, notes, all_day, starts_at, ends_at, rrule, time_zone, exdates, reminder_minutes, category_id, updated_at`

func scanEvent(row interface{ Scan(...any) error }) (*Event, error) {
	var e Event
	var start, end, updated, exdates string
	var reminder sql.NullInt64
	var category sql.NullInt64
	if err := row.Scan(&e.ID, &e.Title, &e.Location, &e.Notes, &e.AllDay, &start, &end, &e.RRule, &e.TimeZone, &exdates,
		&reminder, &category, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var err error
	if e.Start, err = parseTime(start); err != nil {
		return nil, fmt.Errorf("Termin %d: Beginn %q: %w", e.ID, start, err)
	}
	if e.End, err = parseTime(end); err != nil {
		return nil, fmt.Errorf("Termin %d: Ende %q: %w", e.ID, end, err)
	}
	e.UpdatedAt, _ = parseTime(updated)
	for _, x := range strings.Fields(exdates) {
		if e.exdates == nil {
			e.exdates = map[string]bool{}
		}
		e.exdates[x] = true
	}
	if reminder.Valid {
		m := int(reminder.Int64)
		e.ReminderMinutes = &m
	}
	if category.Valid {
		c := category.Int64
		e.CategoryID = &c
	}
	return &e, nil
}

// ListEvents liefert die Termine des Benutzers, die den Zeitraum [from, to)
// berühren. Serien werden in ihre einzelnen Vorkommen aufgelöst.
func (s *Store) ListEvents(ctx context.Context, userID int64, from, to time.Time) ([]Event, error) {
	if !to.After(from) {
		return nil, &ValidationError{"to", "muss nach from liegen"}
	}
	if to.Sub(from) > 400*24*time.Hour {
		return nil, &ValidationError{"to", "Zeitraum höchstens 400 Tage"}
	}
	out := []Event{}
	single, err := s.queryEvents(ctx,
		`SELECT `+eventColumns+` FROM events
		  WHERE user_id = ? AND deleted_at IS NULL AND rrule = ''
		    AND starts_at < ? AND (ends_at > ? OR (ends_at = starts_at AND starts_at >= ?))`,
		userID, formatTime(to), formatTime(from), formatTime(from))
	if err != nil {
		return nil, err
	}
	out = append(out, single...)

	series, err := s.queryEvents(ctx,
		`SELECT `+eventColumns+` FROM events
		  WHERE user_id = ? AND deleted_at IS NULL AND rrule <> '' AND starts_at < ?`,
		userID, formatTime(to))
	if err != nil {
		return nil, err
	}
	for _, e := range series {
		out = append(out, e.occurrences(from, to)...)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay
		}
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (s *Store) queryEvents(ctx context.Context, q string, args ...any) ([]Event, error) {
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// location liefert die Zeitzone, in der eine Serie berechnet wird.
// Ganztägige Termine rechnen in UTC, weil sie reine Tagesangaben sind.
func (e *Event) location() *time.Location {
	if e.AllDay || e.TimeZone == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(e.TimeZone); err == nil {
		return loc
	}
	return time.UTC
}

func (e *Event) occurrences(from, to time.Time) []Event {
	rule, err := recur.Parse(e.RRule)
	if err != nil {
		// Unlesbare Regel: wenigstens den ersten Termin zeigen.
		if e.Start.Before(to) && e.End.After(from) {
			return []Event{*e}
		}
		return nil
	}
	loc := e.location()
	dur := e.End.Sub(e.Start)
	seriesStart := e.Start
	var out []Event
	skip := func(t time.Time) bool { return e.exdates[formatTime(t)] }
	for _, t := range rule.Between(e.Start.In(loc), dur, from, to, skip) {
		occ := *e
		occ.Start = t.UTC()
		occ.End = t.Add(dur).UTC()
		occ.SeriesStart = &seriesStart
		occ.exdates = nil
		out = append(out, occ)
	}
	return out
}

// GetEvent liefert einen Termin des Benutzers.
func (s *Store) GetEvent(ctx context.Context, userID, id int64) (*Event, error) {
	return scanEvent(s.DB.QueryRowContext(ctx,
		`SELECT `+eventColumns+` FROM events WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, userID))
}

func nullable[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// checkCategory stellt sicher, dass eine Kategorie dem Benutzer gehört.
func (s *Store) checkCategory(ctx context.Context, userID int64, id *int64) error {
	if id == nil {
		return nil
	}
	var n int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM categories WHERE id = ? AND user_id = ?`, *id, userID).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		return &ValidationError{"categoryId", "unbekannt"}
	}
	return nil
}

// CreateEvent legt einen Termin an.
func (s *Store) CreateEvent(ctx context.Context, userID int64, e Event) (*Event, error) {
	if err := e.normalize(); err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, userID, e.CategoryID); err != nil {
		return nil, err
	}
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO events (user_id, title, location, notes, all_day, starts_at, ends_at, rrule, time_zone,
		                     reminder_minutes, category_id, dirty, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		userID, e.Title, e.Location, e.Notes, e.AllDay, formatTime(e.Start), formatTime(e.End), e.RRule, e.TimeZone,
		nullable(e.ReminderMinutes), nullable(e.CategoryID), now, now)
	if err != nil {
		return nil, fmt.Errorf("Termin anlegen: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetEvent(ctx, userID, id)
}

// UpdateEvent ändert einen Termin. Er wird für den nächsten Abgleich vorgemerkt.
func (s *Store) UpdateEvent(ctx context.Context, userID int64, e Event) (*Event, error) {
	if err := e.normalize(); err != nil {
		return nil, err
	}
	if err := s.checkCategory(ctx, userID, e.CategoryID); err != nil {
		return nil, err
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE events SET title = ?, location = ?, notes = ?, all_day = ?, starts_at = ?, ends_at = ?,
		       rrule = ?, time_zone = ?, reminder_minutes = ?, category_id = ?, dirty = 1, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		e.Title, e.Location, e.Notes, e.AllDay, formatTime(e.Start), formatTime(e.End),
		e.RRule, e.TimeZone, nullable(e.ReminderMinutes), nullable(e.CategoryID), formatTime(time.Now()),
		e.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("Termin ändern: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.GetEvent(ctx, userID, e.ID)
}

// DeleteEvent löscht einen Termin weich, damit die Löschung abgeglichen werden kann.
func (s *Store) DeleteEvent(ctx context.Context, userID, id int64) error {
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE events SET deleted_at = ?, updated_at = ?, dirty = 1
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, now, now, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteOccurrence löscht ein einzelnes Vorkommen einer Serie. occurrence ist
// dessen Beginn. Die Serie selbst bleibt bestehen.
func (s *Store) DeleteOccurrence(ctx context.Context, userID, id int64, occurrence time.Time) error {
	e, err := s.GetEvent(ctx, userID, id)
	if err != nil {
		return err
	}
	if e.RRule == "" {
		return &ValidationError{"occurrence", "der Termin ist keine Serie"}
	}
	occurrence = occurrence.UTC().Truncate(time.Second)
	found := false
	for _, o := range e.occurrences(occurrence, occurrence.Add(time.Second)) {
		if o.Start.Equal(occurrence) {
			found = true
			break
		}
	}
	if !found {
		return &ValidationError{"occurrence", "an diesem Zeitpunkt beginnt kein Termin der Serie"}
	}
	res, err := s.DB.ExecContext(ctx,
		`UPDATE events SET exdates = TRIM(exdates || ' ' || ?), dirty = 1, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		formatTime(occurrence), formatTime(time.Now()), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
