// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package store

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// LocalUser ist der Benutzer, der im lokalen Betrieb ohne Anmeldung verwendet wird.
const LocalUser = "lokal"

// ErrNotFound meldet, dass ein Eintrag nicht existiert.
var ErrNotFound = errors.New("nicht gefunden")

// ErrBadLogin meldet falschen Benutzernamen oder falsches Passwort.
var ErrBadLogin = errors.New("Benutzername oder Passwort falsch")

// User ist ein Benutzer.
type User struct {
	ID          int64
	Username    string
	DisplayName string
	Lang        string
	Theme       string
	TimeZone    string
	HasPassword bool
}

// Passwörter werden mit PBKDF2-SHA256 gespeichert (Empfehlung OWASP 2023:
// 600 000 Durchläufe). Format: pbkdf2-sha256$<durchläufe>$<salz>$<hash>.
const pbkdf2Iterations = 600_000

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations, enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func checkPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

const userColumns = `id, username, display_name, lang, theme, time_zone, password_hash <> ''`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Lang, &u.Theme, &u.TimeZone, &u.HasPassword); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &u, nil
}

// CreateUser legt einen Benutzer an. Ein leeres Passwort bedeutet: keine Anmeldung möglich.
func (s *Store) CreateUser(ctx context.Context, username, displayName, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("Benutzername fehlt")
	}
	hash := ""
	if password != "" {
		var err error
		if hash, err = hashPassword(password); err != nil {
			return nil, err
		}
	}
	res, err := s.DB.ExecContext(ctx,
		`INSERT INTO users (username, display_name, password_hash) VALUES (?, ?, ?)`,
		username, displayName, hash)
	if err != nil {
		return nil, fmt.Errorf("Benutzer %s anlegen: %w", username, err)
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, id)
}

// UserByID liefert einen Benutzer.
func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
}

// UserByName liefert einen Benutzer (Groß-/Kleinschreibung egal).
func (s *Store) UserByName(ctx context.Context, username string) (*User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE username = ?`, strings.TrimSpace(username)))
}

// EnsureLocalUser liefert den Benutzer für den lokalen Betrieb und legt ihn bei Bedarf an.
func (s *Store) EnsureLocalUser(ctx context.Context) (*User, error) {
	u, err := s.UserByName(ctx, LocalUser)
	if errors.Is(err, ErrNotFound) {
		return s.CreateUser(ctx, LocalUser, "", "")
	}
	return u, err
}

// SetPassword setzt ein neues Passwort; alle Sitzungen des Benutzers werden beendet.
func (s *Store) SetPassword(ctx context.Context, userID int64, password string) error {
	if len(password) < 8 {
		return errors.New("das Passwort muss mindestens 8 Zeichen haben")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

// SetPreferences speichert Sprache und Thema eines Benutzers.
func (s *Store) SetPreferences(ctx context.Context, userID int64, lang, theme string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET lang = ?, theme = ? WHERE id = ?`, lang, theme, userID)
	return err
}

// Authenticate prüft Benutzername und Passwort.
func (s *Store) Authenticate(ctx context.Context, username, password string) (*User, error) {
	var id int64
	var hash string
	err := s.DB.QueryRowContext(ctx, `SELECT id, password_hash FROM users WHERE username = ?`, strings.TrimSpace(username)).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		// Gleiche Rechenzeit wie bei vorhandenem Benutzer, damit sich Namen nicht erraten lassen.
		checkPassword(dummyHash, password)
		return nil, ErrBadLogin
	}
	if err != nil {
		return nil, err
	}
	if hash == "" || !checkPassword(hash, password) {
		return nil, ErrBadLogin
	}
	return s.UserByID(ctx, id)
}

var dummyHash = func() string { h, _ := hashPassword("dummy"); return h }()

// Sitzungen: Der Browser bekommt ein zufälliges Token, in der Datenbank
// steht nur dessen SHA-256-Wert. Ein Datenbankleck verrät so keine Sitzungen.

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession legt eine Sitzung an und liefert das Token für den Browser.
func (s *Store) CreateSession(ctx context.Context, userID int64, ttl time.Duration) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	expires := time.Now().UTC().Add(ttl).Format(time.RFC3339)
	if _, err := s.DB.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`,
		tokenHash(token), userID, expires); err != nil {
		return "", err
	}
	return token, nil
}

// SessionUser liefert den Benutzer zu einem gültigen Token.
func (s *Store) SessionUser(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return scanUser(s.DB.QueryRowContext(ctx,
		`SELECT u.id, u.username, u.display_name, u.lang, u.theme, u.time_zone, u.password_hash <> ''
		   FROM sessions s JOIN users u ON u.id = s.user_id
		  WHERE s.token_hash = ? AND s.expires_at > ?`, tokenHash(token), now))
}

// DeleteSession beendet eine Sitzung (Abmelden).
func (s *Store) DeleteSession(ctx context.Context, token string) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash(token))
	return err
}

// PurgeExpiredSessions löscht abgelaufene Sitzungen.
func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}
