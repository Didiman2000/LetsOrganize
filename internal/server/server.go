// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

// Package server stellt die Weboberfläche und die Schnittstellen bereit.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/i18n"
	"github.com/Didiman2000/LetsOrganize/internal/store"
	"github.com/Didiman2000/LetsOrganize/internal/themes"
)

// Config enthält die Einstellungen des Servers.
type Config struct {
	// Assets enthält die Ordner web, locales und themes.
	Assets fs.FS
	// DataDir ist der Ordner für Daten und eigene Themen.
	DataDir string
	// Store ist die Datenbank; nil nur in Tests der Oberfläche.
	Store *store.Store
}

// Server ist der HTTP-Handler von LetsOrganize.
type Server struct {
	cfg    Config
	lang   *i18n.Catalog
	themes *themes.Registry
	index  *template.Template
	mux    *http.ServeMux

	localUserID int64
}

const (
	cookieLang  = "lo_lang"
	cookieTheme = "lo_theme"
)

// New erzeugt den Server.
func New(cfg Config) (*Server, error) {
	lang, err := i18n.Load(cfg.Assets, "locales")
	if err != nil {
		return nil, err
	}
	userThemes := ""
	if cfg.DataDir != "" {
		userThemes = filepath.Join(cfg.DataDir, "themes")
	}
	reg, warnings, err := themes.Load(cfg.Assets, "themes", userThemes)
	if err != nil {
		return nil, err
	}
	for _, w := range warnings {
		log.Printf("Eigenes Thema übersprungen: %v", w)
	}

	s := &Server{cfg: cfg, lang: lang, themes: reg, mux: http.NewServeMux()}
	s.index, err = template.New("index.html").Funcs(template.FuncMap{
		"t": func(code, key string, args ...string) string { return s.lang.T(code, key, args...) },
	}).ParseFS(cfg.Assets, "web/index.html")
	if err != nil {
		return nil, fmt.Errorf("Vorlage laden: %w", err)
	}

	static, err := fs.Sub(cfg.Assets, "web/static")
	if err != nil {
		return nil, err
	}
	s.mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	s.mux.HandleFunc("GET /{$}", s.handleIndex)
	s.mux.HandleFunc("GET /theme.css", s.handleThemeCSS)
	s.mux.HandleFunc("GET /api/languages", s.handleLanguages)
	s.mux.HandleFunc("GET /api/messages/{lang}", s.handleMessages)
	s.mux.HandleFunc("GET /api/themes", s.handleThemes)
	s.mux.HandleFunc("GET /api/themes/{id}", s.handleTheme)
	s.mux.HandleFunc("GET /api/status", s.handleStatus)

	if cfg.Store != nil {
		u, err := cfg.Store.EnsureLocalUser(context.Background())
		if err != nil {
			return nil, err
		}
		s.localUserID = u.ID
		s.routesEvents()
	}
	return s, nil
}

// ServeHTTP setzt Sicherheits-Header und gibt an den Router weiter.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	s.mux.ServeHTTP(w, r)
}

// prefs ermittelt Sprache und Thema: erst ?lang= / ?theme= (wird gemerkt),
// dann Cookie, dann Browsersprache bzw. Standardthema.
func (s *Server) prefs(w http.ResponseWriter, r *http.Request) (lang, theme string) {
	q := r.URL.Query()
	remember := func(name, value string) {
		http.SetCookie(w, &http.Cookie{
			Name: name, Value: value, Path: "/",
			MaxAge: int((400 * 24 * time.Hour).Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode,
		})
	}

	if l := q.Get("lang"); l != "" && s.lang.Has(l) {
		lang = l
		remember(cookieLang, l)
	} else if c, err := r.Cookie(cookieLang); err == nil && s.lang.Has(c.Value) {
		lang = c.Value
	} else {
		lang = s.lang.Match(r.Header.Get("Accept-Language"))
	}

	if t := q.Get("theme"); t != "" && s.themes.Has(t) {
		theme = t
		remember(cookieTheme, t)
	} else if c, err := r.Cookie(cookieTheme); err == nil && s.themes.Has(c.Value) {
		theme = c.Value
	} else {
		theme = themes.Default
	}
	return lang, theme
}

type tabView struct {
	ID    string
	Label string
	Paper string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	lang, themeID := s.prefs(w, r)
	loc := s.lang.Get(lang)
	th := s.themes.Get(themeID)

	tabs := make([]tabView, 0, len(themes.Sections))
	for _, id := range themes.Sections {
		tabs = append(tabs, tabView{ID: id, Label: s.lang.T(lang, "tab."+id), Paper: th.Paper[id]})
	}

	data := map[string]any{
		"Lang":      loc.Code,
		"Dir":       loc.Meta.Dir,
		"WeekStart": loc.Meta.WeekStart,
		"Theme":     th,
		"TabShape":  th.TabShape,
		"Tabs":      tabs,
		"Languages": s.lang.List(),
		"Themes":    s.themes.List(lang),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := s.index.Execute(w, data); err != nil {
		log.Printf("Startseite: %v", err)
	}
}

func (s *Server) handleThemeCSS(w http.ResponseWriter, r *http.Request) {
	_, themeID := s.prefs(w, r)
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, s.themes.CSSFor(themeID))
}

func (s *Server) handleLanguages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.lang.List())
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("lang")
	if !s.lang.Has(code) {
		http.NotFound(w, r)
		return
	}
	loc := s.lang.Get(code)
	writeJSON(w, map[string]any{
		"code":     loc.Code,
		"meta":     loc.Meta,
		"messages": s.lang.Messages(code),
	})
}

func (s *Server) handleThemes(w http.ResponseWriter, r *http.Request) {
	lang, _ := s.prefs(w, r)
	writeJSON(w, s.themes.List(lang))
}

func (s *Server) handleTheme(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !s.themes.Has(id) {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, s.themes.Get(id))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := map[string]any{"ok": true, "schemaLatest": store.LatestVersion()}
	if s.cfg.Store != nil {
		v, err := s.cfg.Store.Version(r.Context())
		if err != nil {
			log.Printf("Status: %v", err)
			status["ok"] = false
		}
		status["schema"] = v
	}
	writeJSON(w, status)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("JSON: %v", err)
	}
}
