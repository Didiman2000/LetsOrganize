// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

// crud verbindet die üblichen fünf Schnittstellen eines Registers mit dem Store.
// T ist der Datentyp; setID trägt die ID aus dem Pfad ein.
type crud[T any] struct {
	get    func(ctx context.Context, userID, id int64) (*T, error)
	create func(ctx context.Context, userID int64, v T) (*T, error)
	update func(ctx context.Context, userID int64, v T) (*T, error)
	del    func(ctx context.Context, userID, id int64) error
	setID  func(v *T, id int64)
}

func registerCRUD[T any](s *Server, base string, c crud[T]) {
	s.mux.HandleFunc("GET "+base+"/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		v, err := c.get(r.Context(), s.userID(r), id)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, v)
	})
	s.mux.HandleFunc("POST "+base, s.guardWrite(func(w http.ResponseWriter, r *http.Request) {
		var v T
		if !decodeJSON(w, r, &v) {
			return
		}
		created, err := c.create(r.Context(), s.userID(r), v)
		if err != nil {
			s.storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		writeJSON(w, created)
	}))
	s.mux.HandleFunc("PUT "+base+"/{id}", s.guardWrite(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		var v T
		if !decodeJSON(w, r, &v) {
			return
		}
		c.setID(&v, id)
		updated, err := c.update(r.Context(), s.userID(r), v)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, updated)
	}))
	s.mux.HandleFunc("DELETE "+base+"/{id}", s.guardWrite(func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := c.del(r.Context(), s.userID(r), id); err != nil {
			s.storeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
}

func (s *Server) routesRegisters() {
	st := s.cfg.Store

	s.mux.HandleFunc("GET /api/notes", func(w http.ResponseWriter, r *http.Request) {
		list, err := st.ListNotes(r.Context(), s.userID(r))
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, list)
	})
	registerCRUD(s, "/api/notes", crud[store.Note]{
		get: st.GetNote, create: st.CreateNote, update: st.UpdateNote, del: st.DeleteNote,
		setID: func(v *store.Note, id int64) { v.ID = id },
	})

	// GET /api/planner?year=2026 (ohne Angabe: aktuelles Jahr)
	s.mux.HandleFunc("GET /api/planner", func(w http.ResponseWriter, r *http.Request) {
		year := time.Now().Year()
		if v := r.URL.Query().Get("year"); v != "" {
			y, err := strconv.Atoi(v)
			if err != nil {
				writeError(w, http.StatusBadRequest, "bad_year", "year muss eine Zahl sein")
				return
			}
			year = y
		}
		list, err := st.ListPlanner(r.Context(), s.userID(r), year)
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, list)
	})
	registerCRUD(s, "/api/planner", crud[store.PlannerEntry]{
		get: st.GetPlanner, create: st.CreatePlanner, update: st.UpdatePlanner, del: st.DeletePlanner,
		setID: func(v *store.PlannerEntry, id int64) { v.ID = id },
	})

	s.mux.HandleFunc("GET /api/anniversaries", func(w http.ResponseWriter, r *http.Request) {
		list, err := st.ListAnniversaries(r.Context(), s.userID(r))
		if err != nil {
			s.storeError(w, err)
			return
		}
		writeJSON(w, list)
	})
	registerCRUD(s, "/api/anniversaries", crud[store.Anniversary]{
		get: st.GetAnniversary, create: st.CreateAnniversary, update: st.UpdateAnniversary, del: st.DeleteAnniversary,
		setID: func(v *store.Anniversary, id int64) { v.ID = id },
	})
}
