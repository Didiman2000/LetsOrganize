// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import "net/http"

func (s *Server) routesCalendars() {
	// GET /api/calendars – Kalender des Benutzers (leer ohne Google-Verbindung)
	s.mux.HandleFunc("GET /api/calendars", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.cfg.Store.ListCalendars(r.Context(), s.userID(r))
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, list)
	})
	// PUT /api/calendars/{id} mit {"enabled": true|false}
	s.mux.HandleFunc("PUT /api/calendars/{id}", s.guardWrite(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var body struct {
			Enabled *bool `json:"enabled"`
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if body.Enabled == nil {
			writeError(w, http.StatusBadRequest, "bad_json", "enabled fehlt")
			return
		}
		c, err := s.cfg.Store.SetCalendarEnabled(r.Context(), s.userID(r), id, *body.Enabled)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, c)
	}))
}
