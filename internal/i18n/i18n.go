// SPDX-License-Identifier: GPL-3.0-or-later

// Package i18n lädt die Sprachdateien und liefert übersetzte Texte.
//
// Jede Sprache liegt als JSON-Datei im Ordner locales, z. B. de.json:
//
//	{
//	  "meta": {"name": "Deutsch", "dir": "ltr", "weekStart": 1},
//	  "messages": {"tab.calendar": "Kalender", ...}
//	}
//
// "dir" ist "ltr" oder "rtl" (Schreibrichtung), "weekStart" der erste
// Wochentag (0 = Sonntag, 1 = Montag, 6 = Samstag).
package i18n

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// Fallback ist die Sprache, auf die bei fehlenden Texten zurückgegriffen wird.
const Fallback = "en"

// Meta beschreibt eine Sprache.
type Meta struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	WeekStart int    `json:"weekStart"`
}

// Locale ist eine geladene Sprache.
type Locale struct {
	Code     string            `json:"code"`
	Meta     Meta              `json:"meta"`
	Messages map[string]string `json:"messages"`
}

// Info ist die Kurzbeschreibung einer Sprache für Auswahllisten.
type Info struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Dir  string `json:"dir"`
}

// Catalog enthält alle geladenen Sprachen.
type Catalog struct {
	locales map[string]*Locale
}

// Load liest alle *.json-Dateien aus dir im Dateisystem fsys.
func Load(fsys fs.FS, dir string) (*Catalog, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("Sprachdateien lesen: %w", err)
	}
	c := &Catalog{locales: map[string]*Locale{}}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		code := strings.ToLower(strings.TrimSuffix(e.Name(), ".json"))
		raw, err := fs.ReadFile(fsys, path.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var l Locale
		if err := json.Unmarshal(raw, &l); err != nil {
			return nil, fmt.Errorf("Sprachdatei %s: %w", e.Name(), err)
		}
		l.Code = code
		if l.Meta.Dir != "rtl" {
			l.Meta.Dir = "ltr"
		}
		if l.Meta.WeekStart < 0 || l.Meta.WeekStart > 6 {
			return nil, fmt.Errorf("Sprachdatei %s: weekStart muss zwischen 0 und 6 liegen", e.Name())
		}
		if l.Messages == nil {
			l.Messages = map[string]string{}
		}
		c.locales[code] = &l
	}
	if _, ok := c.locales[Fallback]; !ok {
		return nil, fmt.Errorf("Sprachdatei %s.json fehlt", Fallback)
	}
	return c, nil
}

// Has meldet, ob es die Sprache gibt.
func (c *Catalog) Has(code string) bool {
	_, ok := c.locales[strings.ToLower(code)]
	return ok
}

// Get liefert die Sprache, bei unbekanntem Code die Ersatzsprache.
func (c *Catalog) Get(code string) *Locale {
	if l, ok := c.locales[strings.ToLower(code)]; ok {
		return l
	}
	return c.locales[Fallback]
}

// T übersetzt key in die Sprache code. Platzhalter wie {n} werden durch
// args ersetzt, die paarweise als Name und Wert übergeben werden.
// Fehlt der Text, wird die Ersatzsprache und zuletzt der Schlüssel selbst verwendet.
func (c *Catalog) T(code, key string, args ...string) string {
	msg, ok := c.Get(code).Messages[key]
	if !ok {
		if msg, ok = c.locales[Fallback].Messages[key]; !ok {
			msg = key
		}
	}
	for i := 0; i+1 < len(args); i += 2 {
		msg = strings.ReplaceAll(msg, "{"+args[i]+"}", args[i+1])
	}
	return msg
}

// Messages liefert alle Texte einer Sprache, ergänzt um fehlende Texte der Ersatzsprache.
func (c *Catalog) Messages(code string) map[string]string {
	out := map[string]string{}
	for k, v := range c.locales[Fallback].Messages {
		out[k] = v
	}
	for k, v := range c.Get(code).Messages {
		out[k] = v
	}
	return out
}

// List liefert alle Sprachen, sortiert nach Code.
func (c *Catalog) List() []Info {
	out := make([]Info, 0, len(c.locales))
	for code, l := range c.locales {
		out = append(out, Info{Code: code, Name: l.Meta.Name, Dir: l.Meta.Dir})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Missing liefert je Sprache die Schlüssel, die gegenüber der Ersatzsprache fehlen.
func (c *Catalog) Missing() map[string][]string {
	out := map[string][]string{}
	for code, l := range c.locales {
		for key := range c.locales[Fallback].Messages {
			if _, ok := l.Messages[key]; !ok {
				out[code] = append(out[code], key)
			}
		}
		sort.Strings(out[code])
	}
	return out
}

// Match wählt anhand eines Accept-Language-Headers die passende Sprache.
// Ohne Treffer wird die Ersatzsprache geliefert.
func (c *Catalog) Match(acceptLanguage string) string {
	type cand struct {
		code string
		q    float64
	}
	var cands []cand
	for i, part := range strings.Split(acceptLanguage, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for _, f := range fields[1:] {
			if v, ok := strings.CutPrefix(strings.TrimSpace(f), "q="); ok {
				if p, err := strconv.ParseFloat(v, 64); err == nil {
					q = p
				}
			}
		}
		// Reihenfolge als Feinsortierung bei gleichem q.
		cands = append(cands, cand{tag, q - float64(i)*1e-6})
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].q > cands[j].q })
	for _, cd := range cands {
		if c.Has(cd.code) {
			return cd.code
		}
		base, _, _ := strings.Cut(cd.code, "-")
		if c.Has(base) {
			return base
		}
	}
	return Fallback
}
