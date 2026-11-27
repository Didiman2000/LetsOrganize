// SPDX-License-Identifier: GPL-3.0-or-later

package themes

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinThemes(t *testing.T) {
	r, warn, err := Load(os.DirFS("../.."), "themes", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) > 0 {
		t.Fatal(warn)
	}
	if len(r.List("de")) < 2 {
		t.Fatal("erwartet mindestens zwei Themen")
	}
	css := r.Get(Default).CSS()
	if !strings.Contains(css, "--lo-color-cover:") || !strings.Contains(css, "--lo-tab-calendar:") {
		t.Errorf("CSS unvollständig:\n%s", css)
	}
	if r.Get("gibt-es-nicht").ID != Default {
		t.Error("unbekanntes Thema muss auf Standard zurückfallen")
	}
}

// Ein importiertes Thema darf kein fremdes CSS einschleusen.
func TestRejectsInjection(t *testing.T) {
	bad := []Theme{
		{ID: "x1", Colors: map[string]string{"cover": "red; } body { display:none"}},
		{ID: "x2", Colors: map[string]string{"cover": "url(https://example.com/a.png)"}},
		{ID: "x3", Fonts: map[string]string{"body": "</style><script>"}},
		{ID: "x4", Colors: map[string]string{"Cover;": "red"}},
		{ID: "../x5"},
		{ID: "x6", TabShape: "kreis"},
		{ID: "x7", Paper: map[string]string{"notes": "papyrus"}},
	}
	for _, th := range bad {
		if err := th.Validate(); err == nil {
			t.Errorf("Thema %s wurde fälschlich akzeptiert", th.ID)
		}
	}
}

// Ein eigenes Thema ersetzt ein mitgeliefertes mit gleicher ID; fehlerhafte werden gemeldet.
func TestUserThemes(t *testing.T) {
	dir := t.TempDir()
	write := func(id, body string) {
		os.MkdirAll(filepath.Join(dir, id), 0o755)
		os.WriteFile(filepath.Join(dir, id, "theme.json"), []byte(body), 0o644)
	}
	write("leder-ringbuch", `{"id":"leder-ringbuch","name":{"de":"Mein Leder"},"colors":{"cover":"#000000"}}`)
	write("kaputt", `{"id":"kaputt","colors":{"cover":"red;}"}}`)
	r, warn, err := Load(os.DirFS("../.."), "themes", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(warn) != 1 {
		t.Errorf("erwartet eine Warnung, erhalten %v", warn)
	}
	if got := r.Get(Default).DisplayName("de"); got != "Mein Leder" {
		t.Errorf("eigenes Thema nicht übernommen: %q", got)
	}
	if r.Has("kaputt") {
		t.Error("fehlerhaftes Thema wurde geladen")
	}
}
