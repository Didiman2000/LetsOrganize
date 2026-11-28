// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"

	"github.com/Didiman2000/LetsOrganize/internal/google"
	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func (s *Server) routesGoogle() {
	s.mux.HandleFunc("GET /api/google/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.cfg.Google.Status(r.Context(), s.userID(r)))
	})
	s.mux.HandleFunc("POST /api/google/connect", s.guardWrite(s.handleGoogleConnect))
	s.mux.HandleFunc("POST /api/google/sync", s.guardWrite(s.handleGoogleSync))
	s.mux.HandleFunc("DELETE /api/google", s.guardWrite(func(w http.ResponseWriter, r *http.Request) {
		if err := s.cfg.Google.Disconnect(r.Context(), s.userID(r)); err != nil && !errors.Is(err, store.ErrNotFound) {
			s.storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

// isLoopback meldet, ob host (mit Port) eine lokale Adresse ist. Nur dorthin
// darf Google bei einem Desktop-Zugang zurückleiten.
func isLoopback(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	if h == "localhost" {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleGoogleConnect(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r.Host) {
		writeError(w, http.StatusBadRequest, "not_local", "Die Anmeldung bei Google geht im Moment nur, wenn LetsOrganize lokal läuft (127.0.0.1).")
		return
	}
	authURL, err := s.cfg.Google.Begin("http://" + r.Host + "/")
	if err != nil {
		writeError(w, http.StatusConflict, "not_configured", err.Error())
		return
	}
	writeJSON(w, map[string]string{"url": authURL})
}

func (s *Server) handleGoogleSync(w http.ResponseWriter, r *http.Request) {
	rep, err := s.cfg.Google.Sync(r.Context(), s.userID(r))
	if errors.Is(err, google.ErrBusy) {
		writeError(w, http.StatusConflict, "busy", err.Error())
		return
	}
	if err != nil {
		log.Printf("Google-Abgleich: %v", err)
		writeError(w, http.StatusBadGateway, "sync_failed", err.Error())
		return
	}
	writeJSON(w, rep)
}

// googleCallback verarbeitet die Rückkehr von Google auf die Startseite
// (?state=…&code=… oder ?state=…&error=…). Liefert true, wenn die Anfrage
// erledigt ist.
func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.Google == nil {
		return false
	}
	q := r.URL.Query()
	state := q.Get("state")
	if state == "" || !s.cfg.Google.IsPending(state) {
		return false
	}
	result := url.Values{}
	if e := q.Get("error"); e != "" {
		// z. B. access_denied, wenn die Freigabe abgebrochen wurde
		s.cfg.Google.Finish(r.Context(), s.userID(r), state, "") //nolint:errcheck // nur aufräumen
		result.Set("google", "error")
		result.Set("message", e)
	} else if email, err := s.cfg.Google.Finish(r.Context(), s.userID(r), state, q.Get("code")); err != nil {
		log.Printf("Google-Anmeldung: %v", err)
		result.Set("google", "error")
		result.Set("message", err.Error())
	} else {
		result.Set("google", "connected")
		result.Set("email", email)
	}
	// Weiterleiten, damit der Code nicht in der Adresszeile und im Verlauf bleibt.
	http.Redirect(w, r, "/?"+result.Encode(), http.StatusSeeOther)
	return true
}
