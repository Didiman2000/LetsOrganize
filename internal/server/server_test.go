// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	s, err := New(Config{Assets: os.DirFS("../.."), DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func get(t *testing.T, s *Server, url string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestIndexLanguages(t *testing.T) {
	s := newTestServer(t)

	de := get(t, s, "/", map[string]string{"Accept-Language": "de-DE,de;q=0.9"}).Body.String()
	if !strings.Contains(de, `lang="de" dir="ltr"`) || !strings.Contains(de, ">Kalender<") {
		t.Error("deutsche Startseite falsch")
	}

	rec := get(t, s, "/?lang=ar", nil)
	ar := rec.Body.String()
	if !strings.Contains(ar, `lang="ar" dir="rtl"`) || !strings.Contains(ar, "التقويم") {
		t.Error("arabische Startseite nicht rechts-nach-links")
	}
	if !strings.Contains(rec.Header().Get("Set-Cookie"), "lo_lang=ar") {
		t.Error("Sprachwahl wird nicht gemerkt")
	}
	if rec.Header().Get("Content-Security-Policy") == "" {
		t.Error("CSP fehlt")
	}
}

func TestThemeCSS(t *testing.T) {
	s := newTestServer(t)
	css := get(t, s, "/theme.css?theme=nachtblau-leinen", nil).Body.String()
	if !strings.Contains(css, "#1f2d4a") {
		t.Errorf("Thema nicht angewendet:\n%s", css)
	}
	// Unbekanntes Thema fällt auf Standard zurück.
	if !strings.Contains(get(t, s, "/theme.css?theme=<script>", nil).Body.String(), "--lo-color-cover: #6b3a22") {
		t.Error("Rückfall auf Standardthema fehlt")
	}
}

func TestAPI(t *testing.T) {
	s := newTestServer(t)
	if rec := get(t, s, "/api/messages/de", nil); rec.Code != 200 || !strings.Contains(rec.Body.String(), "Aufgaben") {
		t.Error("/api/messages/de")
	}
	if rec := get(t, s, "/api/messages/xx", nil); rec.Code != 404 {
		t.Error("unbekannte Sprache muss 404 liefern")
	}
	if rec := get(t, s, "/api/themes", nil); !strings.Contains(rec.Body.String(), "leder-ringbuch") {
		t.Error("/api/themes")
	}
	if rec := get(t, s, "/static/app.js", nil); rec.Code != 200 {
		t.Error("statische Dateien")
	}
}

func TestIsLoopback(t *testing.T) {
	for host, want := range map[string]bool{
		"127.0.0.1:8470": true, "localhost:8470": true, "[::1]:8470": true, "127.0.0.1": true,
		"192.168.1.10:8470": false, "organizer.example.com": false, "localhost.evil.com:80": false,
	} {
		if got := isLoopback(host); got != want {
			t.Errorf("isLoopback(%q) = %v", host, got)
		}
	}
}

// Ohne laufende Anmeldung wird ?state= ignoriert und die Startseite gezeigt.
func TestUnknownGoogleStateShowsIndex(t *testing.T) {
	s := newTestServer(t)
	rec := get(t, s, "/?state=fremd&code=x", nil)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "binder") {
		t.Errorf("Startseite: %d", rec.Code)
	}
}
