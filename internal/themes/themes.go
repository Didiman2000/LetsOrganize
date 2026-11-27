// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

// Package themes lädt die Themen, die das Aussehen des Ringbuchs bestimmen.
//
// Ein Thema ist ein Ordner mit einer theme.json. Es legt nur Farben, Schriften,
// Registerform und Papierart je Abschnitt fest – keine Texte. Beschriftungen
// kommen immer aus den Sprachdateien, damit jedes Thema in jeder Sprache
// funktioniert. Da Themen später als Datei ausgetauscht werden, wird jeder
// Wert streng geprüft, bevor er in CSS landet.
package themes

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
)

// Default ist das Thema, das ohne eigene Wahl verwendet wird.
const Default = "leder-ringbuch"

// Sections sind die Abschnitte (Register) des Organizers in fester Reihenfolge.
var Sections = []string{"calendar", "todo", "addresses", "notes", "planner", "anniversaries"}

var (
	tabShapes  = map[string]bool{"rounded": true, "square": true, "notched": true}
	paperKinds = map[string]bool{"plain": true, "lined": true, "grid": true, "dotted": true, "diary": true}

	reID    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,40}$`)
	reKey   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)
	reValue = regexp.MustCompile(`^[#A-Za-z0-9 ,.%()'"\-]{1,200}$`)
)

// Theme beschreibt ein Thema.
type Theme struct {
	ID       string            `json:"id"`
	Name     map[string]string `json:"name"`
	Author   string            `json:"author,omitempty"`
	Version  string            `json:"version,omitempty"`
	Colors   map[string]string `json:"colors"`
	Fonts    map[string]string `json:"fonts"`
	Tabs     map[string]string `json:"tabs"`
	TabShape string            `json:"tabShape"`
	Paper    map[string]string `json:"paper"`
	BuiltIn  bool              `json:"builtIn"`
}

// Info ist die Kurzbeschreibung für Auswahllisten.
type Info struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	BuiltIn bool   `json:"builtIn"`
}

// DisplayName liefert den Namen in der Sprache lang, sonst Englisch, sonst die ID.
func (t *Theme) DisplayName(lang string) string {
	if n := t.Name[lang]; n != "" {
		return n
	}
	if n := t.Name["en"]; n != "" {
		return n
	}
	return t.ID
}

// Validate prüft das Thema und ergänzt fehlende Angaben.
func (t *Theme) Validate() error {
	if !reID.MatchString(t.ID) {
		return fmt.Errorf("ungültige Themen-ID %q", t.ID)
	}
	if t.TabShape == "" {
		t.TabShape = "rounded"
	}
	if !tabShapes[t.TabShape] {
		return fmt.Errorf("Thema %s: unbekannte Registerform %q", t.ID, t.TabShape)
	}
	for _, group := range []map[string]string{t.Colors, t.Fonts, t.Tabs} {
		for k, v := range group {
			if !reKey.MatchString(k) {
				return fmt.Errorf("Thema %s: ungültiger Name %q", t.ID, k)
			}
			if !reValue.MatchString(v) || strings.Contains(strings.ToLower(v), "url(") {
				return fmt.Errorf("Thema %s: ungültiger Wert für %s", t.ID, k)
			}
		}
	}
	if t.Paper == nil {
		t.Paper = map[string]string{}
	}
	for _, s := range Sections {
		if t.Paper[s] == "" {
			t.Paper[s] = "lined"
		}
	}
	for s, p := range t.Paper {
		if !paperKinds[p] {
			return fmt.Errorf("Thema %s: unbekannte Papierart %q für %s", t.ID, p, s)
		}
	}
	return nil
}

// CSS erzeugt die CSS-Variablen des Themas.
func (t *Theme) CSS() string {
	var b strings.Builder
	b.WriteString(":root {\n")
	write := func(prefix string, m map[string]string) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "  --lo-%s-%s: %s;\n", prefix, k, m[k])
		}
	}
	write("color", t.Colors)
	write("font", t.Fonts)
	write("tab", t.Tabs)
	b.WriteString("}\n")
	return b.String()
}

// Registry enthält alle verfügbaren Themen.
type Registry struct {
	themes map[string]*Theme
}

// Load lädt die mitgelieferten Themen aus builtin/dir und die eigenen Themen
// aus userDir. Ein eigenes Thema mit gleicher ID ersetzt das mitgelieferte.
// Fehlerhafte eigene Themen werden übersprungen und als Fehlerliste gemeldet.
func Load(builtin fs.FS, dir, userDir string) (*Registry, []error, error) {
	r := &Registry{themes: map[string]*Theme{}}
	if err := r.loadDir(builtin, dir, true); err != nil {
		return nil, nil, err
	}
	if _, ok := r.themes[Default]; !ok {
		return nil, nil, fmt.Errorf("Standardthema %s fehlt", Default)
	}
	var warnings []error
	if userDir != "" {
		if _, err := os.Stat(userDir); err == nil {
			if err := r.loadDir(os.DirFS(userDir), ".", false); err != nil {
				warnings = append(warnings, err)
			}
		}
	}
	return r, warnings, nil
}

func (r *Registry) loadDir(fsys fs.FS, dir string, builtIn bool) error {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return fmt.Errorf("Themen lesen: %w", err)
	}
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name(), "theme.json"))
		if err != nil {
			continue
		}
		var t Theme
		if err := json.Unmarshal(raw, &t); err != nil {
			errs = append(errs, fmt.Errorf("Thema %s: %w", e.Name(), err))
			continue
		}
		if t.ID != e.Name() {
			errs = append(errs, fmt.Errorf("Thema %s: id %q passt nicht zum Ordnernamen", e.Name(), t.ID))
			continue
		}
		if err := t.Validate(); err != nil {
			errs = append(errs, err)
			continue
		}
		t.BuiltIn = builtIn
		r.themes[t.ID] = &t
	}
	return errors.Join(errs...)
}

// CSSFor liefert die CSS-Variablen für das Thema id. Die Werte des
// Standardthemas stehen davor, damit unvollständige eigene Themen trotzdem
// vollständig dargestellt werden.
func (r *Registry) CSSFor(id string) string {
	base := r.themes[Default]
	t := r.Get(id)
	if t == base {
		return base.CSS()
	}
	return base.CSS() + t.CSS()
}

// Get liefert das Thema id, bei unbekannter ID das Standardthema.
func (r *Registry) Get(id string) *Theme {
	if t, ok := r.themes[id]; ok {
		return t
	}
	return r.themes[Default]
}

// Has meldet, ob es das Thema gibt.
func (r *Registry) Has(id string) bool {
	_, ok := r.themes[id]
	return ok
}

// List liefert alle Themen mit Namen in der Sprache lang, sortiert nach Namen.
func (r *Registry) List(lang string) []Info {
	out := make([]Info, 0, len(r.themes))
	for _, t := range r.themes {
		out = append(out, Info{ID: t.ID, Name: t.DisplayName(lang), BuiltIn: t.BuiltIn})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
