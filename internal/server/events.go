// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"encoding/json"
	"errors"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

// Schreibende Anfragen müssen JSON sein und den Kopf X-LetsOrganize tragen.
// Beides kann eine fremde Webseite nicht ohne CORS-Vorabprüfung senden, die
// dieser Server nie erlaubt. So kann keine andere Seite im Browser heimlich
// Termine im lokalen Organizer anlegen (CSRF).
const csrfHeader = "X-LetsOrganize"

const maxBody = 1 << 20

func (s *Server) routesEvents() {
	s.mux.HandleFunc("GET /api/events", s.handleListEvents)
	s.mux.HandleFunc("GET /api/events/{id}", s.handleGetEvent)
	s.mux.HandleFunc("POST /api/events", s.guardWrite(s.handleCreateEvent))
	s.mux.HandleFunc("PUT /api/events/{id}", s.guardWrite(s.handleUpdateEvent))
	s.mux.HandleFunc("DELETE /api/events/{id}", s.guardWrite(s.handleDeleteEvent))
}

func (s *Server) guardWrite(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(csrfHeader) != "1" {
			writeError(w, http.StatusForbidden, "missing_header", "Kopf "+csrfHeader+" fehlt")
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || u.Host != r.Host {
				writeError(w, http.StatusForbidden, "bad_origin", "fremde Herkunft")
				return
			}
		}
		if r.Method != http.MethodDelete {
			mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
			if mt != "application/json" {
				writeError(w, http.StatusUnsupportedMediaType, "json_required", "Content-Type muss application/json sein")
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		next(w, r)
	}
}

// userID liefert den angemeldeten Benutzer. Im lokalen Betrieb ist das immer
// der lokale Benutzer; die Anmeldung für den Serverbetrieb folgt.
func (s *Server) userID(r *http.Request) int64 { return s.localUserID }

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": code, "message": msg})
}

func (s *Server) storeError(w http.ResponseWriter, err error) {
	var ve *store.ValidationError
	switch {
	case errors.As(err, &ve):
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid", "field": ve.Field, "message": ve.Msg})
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "nicht gefunden")
	default:
		log.Printf("Datenbank: %v", err)
		writeError(w, http.StatusInternalServerError, "internal", "interner Fehler")
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "bad_id", "ungültige ID")
		return 0, false
	}
	return id, true
}

func (s *Server) handleListEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, err1 := time.Parse(time.RFC3339, q.Get("from"))
	to, err2 := time.Parse(time.RFC3339, q.Get("to"))
	if err1 != nil || err2 != nil {
		writeError(w, http.StatusBadRequest, "bad_range", "from und to als RFC 3339 angeben")
		return
	}
	events, err := s.cfg.Store.ListEvents(r.Context(), s.userID(r), from, to)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) handleGetEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	e, err := s.cfg.Store.GetEvent(r.Context(), s.userID(r), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, e)
}

func decodeEvent(w http.ResponseWriter, r *http.Request) (store.Event, bool) {
	var e store.Event
	return e, decodeJSON(w, r, &e)
}

func (s *Server) handleCreateEvent(w http.ResponseWriter, r *http.Request) {
	e, ok := decodeEvent(w, r)
	if !ok {
		return
	}
	created, err := s.cfg.Store.CreateEvent(r.Context(), s.userID(r), e)
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

func (s *Server) handleUpdateEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	e, ok := decodeEvent(w, r)
	if !ok {
		return
	}
	e.ID = id
	updated, err := s.cfg.Store.UpdateEvent(r.Context(), s.userID(r), e)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, updated)
}

// DELETE /api/events/{id} löscht den Termin bzw. die ganze Serie.
// DELETE /api/events/{id}?occurrence=<RFC 3339> löscht nur dieses Vorkommen einer Serie.
func (s *Server) handleDeleteEvent(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var err error
	if occ := r.URL.Query().Get("occurrence"); occ != "" {
		t, perr := time.Parse(time.RFC3339, occ)
		if perr != nil {
			writeError(w, http.StatusBadRequest, "bad_occurrence", "occurrence als RFC 3339 angeben")
			return
		}
		err = s.cfg.Store.DeleteOccurrence(r.Context(), s.userID(r), id, t)
	} else {
		err = s.cfg.Store.DeleteEvent(r.Context(), s.userID(r), id)
	}
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
