// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package i18n

import (
	"os"
	"testing"
)

func load(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load(os.DirFS("../.."), "locales")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Alle mitgelieferten Sprachen müssen vollständig sein.
func TestAllLocalesComplete(t *testing.T) {
	for code, keys := range load(t).Missing() {
		if len(keys) > 0 {
			t.Errorf("Sprache %s: fehlende Texte %v", code, keys)
		}
	}
}

func TestRTL(t *testing.T) {
	if d := load(t).Get("ar").Meta.Dir; d != "rtl" {
		t.Errorf("ar: dir = %q, erwartet rtl", d)
	}
}

func TestTAndFallback(t *testing.T) {
	c := load(t)
	if got := c.T("de", "calendar.week", "n", "48"); got != "KW 48" {
		t.Errorf("T = %q", got)
	}
	if got := c.T("xx", "tab.calendar"); got != "Calendar" {
		t.Errorf("Ersatzsprache: %q", got)
	}
	if got := c.T("de", "gibt.es.nicht"); got != "gibt.es.nicht" {
		t.Errorf("unbekannter Schlüssel: %q", got)
	}
}

func TestMatch(t *testing.T) {
	c := load(t)
	cases := map[string]string{
		"de-DE,de;q=0.9,en;q=0.8": "de",
		"fr-FR,ar;q=0.5":          "ar",
		"fr":                      "en",
		"":                        "en",
		"en;q=0.3,de;q=0.7":       "de",
	}
	for header, want := range cases {
		if got := c.Match(header); got != want {
			t.Errorf("Match(%q) = %q, erwartet %q", header, got, want)
		}
	}
}
