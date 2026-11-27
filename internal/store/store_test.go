// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "test.db")
	for i := 0; i < 2; i++ {
		s, err := Open(ctx, file)
		if err != nil {
			t.Fatalf("Durchgang %d: %v", i, err)
		}
		v, err := s.Version(ctx)
		if err != nil || v != LatestVersion() || v == 0 {
			t.Fatalf("Version = %d, %v; erwartet %d", v, err, LatestVersion())
		}
		s.Close()
	}
}

func TestForeignKeysAndChecks(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, err := s.EnsureLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Termin, dessen Ende vor dem Beginn liegt, muss abgelehnt werden.
	_, err = s.DB.ExecContext(ctx, `INSERT INTO events (user_id, title, starts_at, ends_at) VALUES (?, 'x', '2026-11-27T10:00:00Z', '2026-11-27T09:00:00Z')`, u.ID)
	if err == nil {
		t.Error("CHECK ends_at >= starts_at greift nicht")
	}
	// Unbekannter Benutzer muss am Fremdschlüssel scheitern.
	_, err = s.DB.ExecContext(ctx, `INSERT INTO notes (user_id, title) VALUES (9999, 'x')`)
	if err == nil {
		t.Error("Fremdschlüssel werden nicht geprüft")
	}
	// Löschen des Benutzers löscht seine Daten mit.
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO notes (user_id, title) VALUES (?, 'Notiz')`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, u.ID); err != nil {
		t.Fatal(err)
	}
	var n int
	s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes`).Scan(&n)
	if n != 0 {
		t.Error("ON DELETE CASCADE greift nicht")
	}
}

func TestUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	local, err := s.EnsureLocalUser(ctx)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := s.EnsureLocalUser(ctx)
	if again.ID != local.ID {
		t.Error("lokaler Benutzer doppelt angelegt")
	}
	if _, err := s.Authenticate(ctx, LocalUser, ""); !errors.Is(err, ErrBadLogin) {
		t.Error("Benutzer ohne Passwort darf sich nicht anmelden")
	}

	u, err := s.CreateUser(ctx, "Andreas", "Andreas", "geheim123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "andreas", "", ""); err == nil {
		t.Error("Benutzername muss unabhängig von Groß-/Kleinschreibung eindeutig sein")
	}
	if _, err := s.Authenticate(ctx, "andreas", "falsch"); !errors.Is(err, ErrBadLogin) {
		t.Error("falsches Passwort akzeptiert")
	}
	if _, err := s.Authenticate(ctx, "niemand", "geheim123"); !errors.Is(err, ErrBadLogin) {
		t.Error("unbekannter Benutzer akzeptiert")
	}
	got, err := s.Authenticate(ctx, "ANDREAS", "geheim123")
	if err != nil || got.ID != u.ID || !got.HasPassword {
		t.Fatalf("Anmeldung: %v", err)
	}

	token, err := s.CreateSession(ctx, u.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if su, err := s.SessionUser(ctx, token); err != nil || su.ID != u.ID {
		t.Fatalf("Sitzung: %v", err)
	}
	if _, err := s.SessionUser(ctx, token+"x"); !errors.Is(err, ErrNotFound) {
		t.Error("fremdes Token akzeptiert")
	}
	// Passwortwechsel beendet alle Sitzungen.
	if err := s.SetPassword(ctx, u.ID, "neuesPasswort"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SessionUser(ctx, token); !errors.Is(err, ErrNotFound) {
		t.Error("Sitzung nach Passwortwechsel noch gültig")
	}
	expired, _ := s.CreateSession(ctx, u.ID, -time.Minute)
	if _, err := s.SessionUser(ctx, expired); !errors.Is(err, ErrNotFound) {
		t.Error("abgelaufene Sitzung akzeptiert")
	}
	if err := s.PurgeExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestPasswordHashFormat(t *testing.T) {
	h, err := hashPassword("abc")
	if err != nil {
		t.Fatal(err)
	}
	if !checkPassword(h, "abc") || checkPassword(h, "abd") || checkPassword("unsinn", "abc") {
		t.Error("Passwortprüfung fehlerhaft")
	}
}
