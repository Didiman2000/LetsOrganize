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
	"unicode"
	"unicode/utf8"
)

// ContactField ist eine Telefonnummer, E-Mail-Adresse, Anschrift oder Webseite.
type ContactField struct {
	Kind  string `json:"kind"`  // phone, email, address, url
	Label string `json:"label"` // z. B. work, home, mobile – frei wählbar
	Value string `json:"value"`
}

// Contact ist ein Eintrag im Adressbuch.
type Contact struct {
	ID           int64          `json:"id"`
	GivenName    string         `json:"givenName"`
	FamilyName   string         `json:"familyName"`
	DisplayName  string         `json:"displayName"`
	Organization string         `json:"organization"`
	JobTitle     string         `json:"jobTitle"`
	Birthday     *string        `json:"birthday,omitempty"` // YYYY-MM-DD oder --MM-DD (ohne Jahr)
	Notes        string         `json:"notes"`
	Fields       []ContactField `json:"fields"`
	Letter       string         `json:"letter"` // Register, unter dem der Kontakt steht
	UpdatedAt    time.Time      `json:"updatedAt"`
}

var contactKinds = map[string]bool{"phone": true, "email": true, "address": true, "url": true}

const maxContactFields = 50

func (c *Contact) normalize() error {
	for _, p := range []*string{&c.GivenName, &c.FamilyName, &c.DisplayName, &c.Organization, &c.JobTitle} {
		*p = strings.TrimSpace(*p)
		if utf8.RuneCountInString(*p) > maxTitle {
			return &ValidationError{"name", "zu lang"}
		}
	}
	if utf8.RuneCountInString(c.Notes) > maxText {
		return &ValidationError{"notes", "zu lang"}
	}
	if c.DisplayName == "" {
		c.DisplayName = strings.TrimSpace(c.GivenName + " " + c.FamilyName)
	}
	if c.DisplayName == "" {
		c.DisplayName = c.Organization
	}
	if c.DisplayName == "" {
		return &ValidationError{"name", "Name oder Firma fehlt"}
	}
	if c.Birthday != nil {
		b := strings.TrimSpace(*c.Birthday)
		switch {
		case b == "":
			c.Birthday = nil
		case strings.HasPrefix(b, "--"):
			if _, err := time.Parse("2006-01-02", "2000"+b[1:]); err != nil || len(b) != 7 {
				return &ValidationError{"birthday", "ohne Jahr im Format --MM-TT angeben"}
			}
			c.Birthday = &b
		default:
			if _, err := time.Parse(time.DateOnly, b); err != nil {
				return &ValidationError{"birthday", "Datum im Format JJJJ-MM-TT angeben"}
			}
			c.Birthday = &b
		}
	}
	if len(c.Fields) > maxContactFields {
		return &ValidationError{"fields", "zu viele Einträge"}
	}
	fields := c.Fields[:0]
	for _, f := range c.Fields {
		f.Kind = strings.TrimSpace(f.Kind)
		f.Label = strings.TrimSpace(f.Label)
		f.Value = strings.TrimSpace(f.Value)
		if f.Value == "" {
			continue // leere Zeilen aus dem Formular ignorieren
		}
		if !contactKinds[f.Kind] {
			return &ValidationError{"fields", fmt.Sprintf("unbekannte Art %q", f.Kind)}
		}
		if utf8.RuneCountInString(f.Label) > 100 || utf8.RuneCountInString(f.Value) > 2000 {
			return &ValidationError{"fields", "Eintrag zu lang"}
		}
		if f.Kind == "email" && (!strings.Contains(f.Value, "@") || strings.ContainsAny(f.Value, " \t\r\n")) {
			return &ValidationError{"fields", fmt.Sprintf("%q ist keine E-Mail-Adresse", f.Value)}
		}
		fields = append(fields, f)
	}
	c.Fields = fields
	return nil
}

// sortKey bestimmt Sortierung und Register: Nachname, sonst Anzeigename.
// Großbuchstaben und vereinfachte Umlaute/Akzente, damit „Ärger“ unter A
// und „Øster“ unter O steht.
func sortKey(c *Contact) string {
	base := c.FamilyName
	if base == "" {
		base = c.DisplayName
	}
	if c.FamilyName != "" && c.GivenName != "" {
		base += " " + c.GivenName
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(base) {
		if f, ok := foldLatin[r]; ok {
			b.WriteString(f)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Letter liefert den Registerbuchstaben zu einem Sortierschlüssel.
// Zahlen und Zeichen landen unter „#“.
func Letter(key string) string {
	for _, r := range key {
		if unicode.IsLetter(r) {
			return string(r)
		}
		if unicode.IsDigit(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			return "#"
		}
	}
	return "#"
}

var foldLatin = map[rune]string{
	'Ä': "A", 'Á': "A", 'À': "A", 'Â': "A", 'Ã': "A", 'Å': "A", 'Ą': "A",
	'Æ': "AE", 'Ç': "C", 'Č': "C", 'Ć': "C", 'Ď': "D", 'Đ': "D",
	'É': "E", 'È': "E", 'Ê': "E", 'Ë': "E", 'Ě': "E", 'Ę': "E",
	'Í': "I", 'Ì': "I", 'Î': "I", 'Ï': "I", 'Ł': "L", 'Ñ': "N", 'Ň': "N", 'Ń': "N",
	'Ö': "O", 'Ó': "O", 'Ò': "O", 'Ô': "O", 'Õ': "O", 'Ø': "O", 'Œ': "OE",
	'Ř': "R", 'Š': "S", 'Ś': "S", 'Ş': "S", 'ẞ': "SS", 'Ť': "T",
	'Ü': "U", 'Ú': "U", 'Ù': "U", 'Û': "U", 'Ů': "U", 'Ý': "Y", 'Ÿ': "Y",
	'Ž': "Z", 'Ź': "Z", 'Ż': "Z",
}

const contactColumns = `id, given_name, family_name, display_name, organization, job_title, birthday, notes, sort_key, updated_at`

func scanContact(row interface{ Scan(...any) error }) (*Contact, error) {
	var c Contact
	var birthday sql.NullString
	var key, updated string
	if err := row.Scan(&c.ID, &c.GivenName, &c.FamilyName, &c.DisplayName, &c.Organization, &c.JobTitle,
		&birthday, &c.Notes, &key, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if birthday.Valid {
		c.Birthday = &birthday.String
	}
	c.Letter = Letter(key)
	c.UpdatedAt, _ = parseTime(updated)
	c.Fields = []ContactField{}
	return &c, nil
}

// ListContacts liefert die Kontakte des Benutzers sortiert nach Register.
// Mit search werden nur Kontakte geliefert, deren Name, Firma oder ein Feld
// den Text enthält (Groß-/Kleinschreibung egal).
func (s *Store) ListContacts(ctx context.Context, userID int64, search string) ([]Contact, error) {
	q := `SELECT ` + contactColumns + ` FROM contacts c WHERE user_id = ? AND deleted_at IS NULL`
	args := []any{userID}
	if search = strings.TrimSpace(search); search != "" {
		if utf8.RuneCountInString(search) > 200 {
			return nil, &ValidationError{"q", "Suchtext zu lang"}
		}
		like := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(search) + "%"
		q += ` AND (display_name LIKE ? ESCAPE '\' OR organization LIKE ? ESCAPE '\' OR notes LIKE ? ESCAPE '\'
		        OR EXISTS (SELECT 1 FROM contact_fields f WHERE f.contact_id = c.id AND f.value LIKE ? ESCAPE '\'))`
		args = append(args, like, like, like, like)
	}
	q += ` ORDER BY sort_key, id`
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	out := []Contact{}
	index := map[int64]int{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[c.ID] = len(out)
		out = append(out, *c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	// Felder aller Kontakte in einer Abfrage nachladen.
	frows, err := s.DB.QueryContext(ctx,
		`SELECT f.contact_id, f.kind, f.label, f.value FROM contact_fields f
		   JOIN contacts c ON c.id = f.contact_id
		  WHERE c.user_id = ? AND c.deleted_at IS NULL ORDER BY f.contact_id, f.position, f.id`, userID)
	if err != nil {
		return nil, err
	}
	defer frows.Close()
	for frows.Next() {
		var id int64
		var f ContactField
		if err := frows.Scan(&id, &f.Kind, &f.Label, &f.Value); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			out[i].Fields = append(out[i].Fields, f)
		}
	}
	return out, frows.Err()
}

// GetContact liefert einen Kontakt mit seinen Feldern.
func (s *Store) GetContact(ctx context.Context, userID, id int64) (*Contact, error) {
	c, err := scanContact(s.DB.QueryRowContext(ctx,
		`SELECT `+contactColumns+` FROM contacts WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, id, userID))
	if err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx,
		`SELECT kind, label, value FROM contact_fields WHERE contact_id = ? ORDER BY position, id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f ContactField
		if err := rows.Scan(&f.Kind, &f.Label, &f.Value); err != nil {
			return nil, err
		}
		c.Fields = append(c.Fields, f)
	}
	return c, rows.Err()
}

func writeFields(ctx context.Context, tx *sql.Tx, contactID int64, fields []ContactField) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM contact_fields WHERE contact_id = ?`, contactID); err != nil {
		return err
	}
	for i, f := range fields {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO contact_fields (contact_id, kind, label, value, position) VALUES (?, ?, ?, ?, ?)`,
			contactID, f.Kind, f.Label, f.Value, i); err != nil {
			return err
		}
	}
	return nil
}

// CreateContact legt einen Kontakt an.
func (s *Store) CreateContact(ctx context.Context, userID int64, c Contact) (*Contact, error) {
	if err := c.normalize(); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := formatTime(time.Now())
	res, err := tx.ExecContext(ctx,
		`INSERT INTO contacts (user_id, given_name, family_name, display_name, sort_key, organization, job_title,
		                       birthday, notes, dirty, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		userID, c.GivenName, c.FamilyName, c.DisplayName, sortKey(&c), c.Organization, c.JobTitle,
		nullable(c.Birthday), c.Notes, now, now)
	if err != nil {
		return nil, fmt.Errorf("Kontakt anlegen: %w", err)
	}
	id, _ := res.LastInsertId()
	if err := writeFields(ctx, tx, id, c.Fields); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetContact(ctx, userID, id)
}

// UpdateContact ändert einen Kontakt; die Felder werden vollständig ersetzt.
func (s *Store) UpdateContact(ctx context.Context, userID int64, c Contact) (*Contact, error) {
	if err := c.normalize(); err != nil {
		return nil, err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE contacts SET given_name = ?, family_name = ?, display_name = ?, sort_key = ?, organization = ?,
		       job_title = ?, birthday = ?, notes = ?, dirty = 1, updated_at = ?
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`,
		c.GivenName, c.FamilyName, c.DisplayName, sortKey(&c), c.Organization, c.JobTitle,
		nullable(c.Birthday), c.Notes, formatTime(time.Now()), c.ID, userID)
	if err != nil {
		return nil, fmt.Errorf("Kontakt ändern: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if err := writeFields(ctx, tx, c.ID, c.Fields); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetContact(ctx, userID, c.ID)
}

// DeleteContact löscht einen Kontakt weich.
func (s *Store) DeleteContact(ctx context.Context, userID, id int64) error {
	now := formatTime(time.Now())
	res, err := s.DB.ExecContext(ctx,
		`UPDATE contacts SET deleted_at = ?, updated_at = ?, dirty = 1
		  WHERE id = ? AND user_id = ? AND deleted_at IS NULL`, now, now, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
