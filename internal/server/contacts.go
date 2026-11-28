// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"net/http"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func (s *Server) routesContacts() {
	s.mux.HandleFunc("GET /api/contacts", s.handleListContacts)
	s.mux.HandleFunc("GET /api/contacts/{id}", s.handleGetContact)
	s.mux.HandleFunc("POST /api/contacts", s.guardWrite(s.handleCreateContact))
	s.mux.HandleFunc("PUT /api/contacts/{id}", s.guardWrite(s.handleUpdateContact))
	s.mux.HandleFunc("DELETE /api/contacts/{id}", s.guardWrite(s.handleDeleteContact))
}

// GET /api/contacts?q=suchtext
func (s *Server) handleListContacts(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Store.ListContacts(r.Context(), s.userID(r), r.URL.Query().Get("q"))
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, list)
}

func (s *Server) handleGetContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := s.cfg.Store.GetContact(r.Context(), s.userID(r), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, c)
}

func (s *Server) handleCreateContact(w http.ResponseWriter, r *http.Request) {
	var c store.Contact
	if !decodeJSON(w, r, &c) {
		return
	}
	created, err := s.cfg.Store.CreateContact(r.Context(), s.userID(r), c)
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

func (s *Server) handleUpdateContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var c store.Contact
	if !decodeJSON(w, r, &c) {
		return
	}
	c.ID = id
	updated, err := s.cfg.Store.UpdateContact(r.Context(), s.userID(r), c)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, updated)
}

func (s *Server) handleDeleteContact(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.cfg.Store.DeleteContact(r.Context(), s.userID(r), id); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
