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
)

// GoogleAccount ist eine Verbindung zu einem Google-Konto.
type GoogleAccount struct {
	ID        int64
	Email     string
	TokenEnc  []byte // verschlüsselt, siehe internal/secret
	Scopes    string
	CreatedAt time.Time
}

// SaveGoogleAccount speichert die Verbindung (neu oder erneuert) und liefert die ID.
func (s *Store) SaveGoogleAccount(ctx context.Context, userID int64, email string, tokenEnc []byte, scopes string) (int64, error) {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO google_accounts (user_id, email, token_enc, scopes) VALUES (?, ?, ?, ?)
		 ON CONFLICT (user_id, email) DO UPDATE SET token_enc = excluded.token_enc, scopes = excluded.scopes`,
		userID, email, tokenEnc, scopes)
	if err != nil {
		return 0, fmt.Errorf("Google-Konto speichern: %w", err)
	}
	var id int64
	err = s.DB.QueryRowContext(ctx, `SELECT id FROM google_accounts WHERE user_id = ? AND email = ?`, userID, email).Scan(&id)
	return id, err
}

// GoogleAccountOf liefert die Google-Verbindung des Benutzers (ErrNotFound, wenn keine).
func (s *Store) GoogleAccountOf(ctx context.Context, userID int64) (*GoogleAccount, error) {
	var a GoogleAccount
	var created string
	err := s.DB.QueryRowContext(ctx,
		`SELECT id, email, token_enc, scopes, created_at FROM google_accounts WHERE user_id = ? ORDER BY id LIMIT 1`, userID).
		Scan(&a.ID, &a.Email, &a.TokenEnc, &a.Scopes, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.CreatedAt, _ = parseTime(created)
	return &a, nil
}

// UpdateGoogleToken speichert ein erneuertes Token.
func (s *Store) UpdateGoogleToken(ctx context.Context, accountID int64, tokenEnc []byte) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE google_accounts SET token_enc = ? WHERE id = ?`, tokenEnc, accountID)
	return err
}

// DeleteGoogleAccount trennt die Verbindung. Die abgeglichenen Daten bleiben
// lokal erhalten und behalten ihre Google-Kennung, damit ein erneutes
// Verbinden keine doppelten Einträge erzeugt.
func (s *Store) DeleteGoogleAccount(ctx context.Context, userID, accountID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM google_accounts WHERE id = ? AND user_id = ?`, accountID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SyncState ist der Stand des letzten Abgleichs eines Bereichs.
type SyncState struct {
	Token    string
	LastSync time.Time
}

// GetSyncState liefert den Abgleich-Stand (leer, wenn noch nie abgeglichen).
func (s *Store) GetSyncState(ctx context.Context, accountID int64, resource string) (SyncState, error) {
	var st SyncState
	var last string
	err := s.DB.QueryRowContext(ctx,
		`SELECT sync_token, last_sync_at FROM sync_state WHERE account_id = ? AND resource = ?`, accountID, resource).
		Scan(&st.Token, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	st.LastSync, _ = parseTime(last)
	return st, err
}

// SetSyncState speichert den Abgleich-Stand.
func (s *Store) SetSyncState(ctx context.Context, accountID int64, resource, token string) error {
	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO sync_state (account_id, resource, sync_token, last_sync_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT (account_id, resource) DO UPDATE SET sync_token = excluded.sync_token, last_sync_at = excluded.last_sync_at`,
		accountID, resource, token, formatTime(time.Now()))
	return err
}

// SyncEvent ist ein Termin mit allem, was der Abgleich braucht.
type SyncEvent struct {
	Event
	RemoteID       string
	ETag           string
	Dirty          bool
	Deleted        bool
	Exdates        []string // UTC, timeLayout
	SeriesRemoteID string
	OriginalStart  string
}

const syncColumns = eventColumns + `, remote_id, remote_etag, dirty, deleted_at IS NOT NULL, series_remote_id, original_start`

func scanSyncEvent(row interface{ Scan(...any) error }) (*SyncEvent, error) {
	var se SyncEvent
	var start, end, updated, exdates string
	var reminder, category sql.NullInt64
	if err := row.Scan(&se.ID, &se.Title, &se.Location, &se.Notes, &se.AllDay, &start, &end, &se.RRule, &se.TimeZone, &exdates,
		&reminder, &category, &updated, &se.CalendarID, &se.RemoteID, &se.ETag, &se.Dirty, &se.Deleted, &se.SeriesRemoteID, &se.OriginalStart); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	se.Start, _ = parseTime(start)
	se.End, _ = parseTime(end)
	se.UpdatedAt, _ = parseTime(updated)
	se.Exdates = strings.Fields(exdates)
	if reminder.Valid {
		m := int(reminder.Int64)
		se.ReminderMinutes = &m
	}
	return &se, nil
}

// EventByRemote sucht einen Termin anhand seiner Google-Kennung (auch gelöschte).
func (s *Store) EventByRemote(ctx context.Context, userID int64, calendarID, remoteID string) (*SyncEvent, error) {
	return scanSyncEvent(s.DB.QueryRowContext(ctx,
		`SELECT `+syncColumns+` FROM events WHERE user_id = ? AND calendar_id = ? AND remote_id = ?`,
		userID, calendarID, remoteID))
}

// DirtyEvents liefert lokal geänderte oder gelöschte Termine eines Kalenders,
// die noch zu Google müssen. Termine ohne Kalender gehören zum Hauptkalender.
func (s *Store) DirtyEvents(ctx context.Context, userID int64, calendarID string) ([]SyncEvent, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT `+syncColumns+` FROM events
		  WHERE user_id = ? AND dirty = 1 AND (calendar_id = ? OR (? = 'primary' AND calendar_id = ''))
		  ORDER BY id`, userID, calendarID, calendarID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SyncEvent
	for rows.Next() {
		se, err := scanSyncEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *se)
	}
	return out, rows.Err()
}

// ApplyRemoteEvent übernimmt einen Termin von Google: neu anlegen oder den
// lokalen ersetzen. Er gilt danach als abgeglichen (dirty = 0).
// Prüfungen auf Pflichtfelder entfallen bewusst: Was Google speichert, wird übernommen.
func (s *Store) ApplyRemoteEvent(ctx context.Context, userID, accountID int64, calendarID string, se SyncEvent) error {
	if se.End.Before(se.Start) {
		se.End = se.Start
	}
	exdates := strings.Join(se.Exdates, " ")
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE events SET title = ?, location = ?, notes = ?, all_day = ?, starts_at = ?, ends_at = ?, rrule = ?, time_zone = ?,
		       exdates = ?, series_remote_id = ?, original_start = ?, account_id = ?, remote_etag = ?, dirty = 0,
		       deleted_at = NULL, updated_at = ?
		  WHERE user_id = ? AND calendar_id = ? AND remote_id = ?`,
		se.Title, se.Location, se.Notes, se.AllDay, formatTime(se.Start), formatTime(se.End), se.RRule, se.TimeZone,
		exdates, se.SeriesRemoteID, se.OriginalStart, accountID, se.ETag, now,
		userID, calendarID, se.RemoteID)
	if err != nil {
		return fmt.Errorf("Termin von Google übernehmen: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO events (user_id, title, location, notes, all_day, starts_at, ends_at, rrule, time_zone, exdates,
		                     series_remote_id, original_start, account_id, calendar_id, remote_id, remote_etag, dirty,
		                     created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`,
		userID, se.Title, se.Location, se.Notes, se.AllDay, formatTime(se.Start), formatTime(se.End), se.RRule, se.TimeZone,
		exdates, se.SeriesRemoteID, se.OriginalStart, accountID, calendarID, se.RemoteID, se.ETag, now, now)
	if err != nil {
		return fmt.Errorf("Termin von Google anlegen: %w", err)
	}
	return nil
}

// KeepLocalOverRemote merkt sich bei einem Konflikt den neuen Stand von
// Google (etag), behält aber die lokale Änderung, die danach hochgeladen wird.
func (s *Store) KeepLocalOverRemote(ctx context.Context, eventID int64, etag string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE events SET remote_etag = ? WHERE id = ?`, etag, eventID)
	return err
}

// RemoteDeleted übernimmt eine Löschung von Google, auch für die abweichenden
// Einzeltermine einer gelöschten Serie.
func (s *Store) RemoteDeleted(ctx context.Context, userID int64, calendarID, remoteID string) error {
	now := formatTime(time.Now())
	_, err := s.DB.ExecContext(ctx,
		`UPDATE events SET deleted_at = COALESCE(deleted_at, ?), dirty = 0, updated_at = ?
		  WHERE user_id = ? AND calendar_id = ? AND (remote_id = ? OR series_remote_id = ?)`,
		now, now, userID, calendarID, remoteID, remoteID)
	return err
}

// AddRemoteExdate trägt ein bei Google abgesagtes Vorkommen in die Serie ein,
// ohne die Serie als lokal geändert zu markieren.
func (s *Store) AddRemoteExdate(ctx context.Context, userID int64, calendarID, seriesRemoteID string, occurrence time.Time) error {
	x := formatTime(occurrence)
	_, err := s.DB.ExecContext(ctx,
		`UPDATE events SET exdates = TRIM(exdates || ' ' || ?)
		  WHERE user_id = ? AND calendar_id = ? AND remote_id = ? AND (' ' || exdates || ' ') NOT LIKE ('% ' || ? || ' %')`,
		x, userID, calendarID, seriesRemoteID, x)
	return err
}

// MarkEventSynced vermerkt, dass ein Termin bei Google angekommen ist.
// updatedAt ist der Stand, der hochgeladen wurde: Hat sich der Termin
// inzwischen erneut geändert, bleibt er für den nächsten Abgleich vorgemerkt.
func (s *Store) MarkEventSynced(ctx context.Context, eventID, accountID int64, calendarID, remoteID, etag string, updatedAt time.Time) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE events SET account_id = ?, calendar_id = ?, remote_id = ?, remote_etag = ?,
		       dirty = CASE WHEN updated_at = ? THEN 0 ELSE dirty END
		  WHERE id = ?`,
		accountID, calendarID, remoteID, etag, formatTime(updatedAt), eventID)
	return err
}
