// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

// Package store verwaltet die SQLite-Datenbank von LetsOrganize.
//
// Das Schema liegt als nummerierte SQL-Dateien im Ordner migrations
// (0001_name.sql, 0002_name.sql, …). Beim Öffnen werden alle noch nicht
// ausgeführten Dateien der Reihe nach in je einer Transaktion eingespielt.
// Einmal veröffentlichte Dateien werden nie mehr geändert; Änderungen kommen
// immer als neue Datei dazu.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Store ist die geöffnete Datenbank.
type Store struct {
	DB *sql.DB
}

// Open öffnet (oder erzeugt) die Datenbankdatei und spielt fehlende Migrationen ein.
func Open(ctx context.Context, file string) (*Store, error) {
	db, err := sql.Open(driverName, dsn(file))
	if err != nil {
		return nil, fmt.Errorf("Datenbank öffnen: %w", err)
	}
	// SQLite verträgt nur einen Schreiber; eine Verbindung vermeidet „database is locked“.
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("Datenbank %s: %w", file, err)
	}
	s := &Store{DB: db}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// dsn baut die Verbindungsangabe mit den nötigen Einstellungen:
// Fremdschlüssel prüfen, WAL-Modus für Absturzsicherheit, Wartezeit bei Sperren.
func dsn(file string) string {
	q := url.Values{}
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + file + "?" + q.Encode()
}

// Close schließt die Datenbank.
func (s *Store) Close() error { return s.DB.Close() }

type migration struct {
	version int
	name    string
	sql     string
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		num, _, ok := strings.Cut(name, "_")
		v, err := strconv.Atoi(num)
		if !ok || err != nil || v <= 0 {
			return nil, fmt.Errorf("Migration %s: Name muss mit einer Nummer und _ beginnen", name)
		}
		if other, dup := seen[v]; dup {
			return nil, fmt.Errorf("Migrationen %s und %s haben dieselbe Nummer", other, name)
		}
		seen[v] = name
		raw, err := fs.ReadFile(migrationFiles, path.Join("migrations", name))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: v, name: name, sql: string(raw)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// LatestVersion ist die höchste mitgelieferte Schemaversion.
func LatestVersion() int {
	m, err := loadMigrations()
	if err != nil || len(m) == 0 {
		return 0
	}
	return m[len(m)-1].version
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    INTEGER PRIMARY KEY,
		name       TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("Migrationstabelle: %w", err)
	}
	current, err := s.Version(ctx)
	if err != nil {
		return err
	}
	all, err := loadMigrations()
	if err != nil {
		return err
	}
	if len(all) > 0 && current > all[len(all)-1].version {
		return fmt.Errorf("die Datenbank hat Schemaversion %d, dieses Programm kennt nur bis %d – bitte LetsOrganize aktualisieren", current, all[len(all)-1].version)
	}
	for _, m := range all {
		if m.version <= current {
			continue
		}
		if err := s.apply(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) apply(ctx context.Context, m migration) (err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	if _, err = tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("Migration %s: %w", m.name, err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name, applied_at) VALUES (?, ?, ?)`,
		m.version, m.name, time.Now().UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("Migration %s eintragen: %w", m.name, err)
	}
	return tx.Commit()
}

// Version liefert die aktuelle Schemaversion der Datenbank (0 = leer).
func (s *Store) Version(ctx context.Context) (int, error) {
	var v sql.NullInt64
	err := s.DB.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("Schemaversion lesen: %w", err)
	}
	return int(v.Int64), nil
}
