// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func (s *Server) routesTasks() {
	s.mux.HandleFunc("GET /api/tasks", s.handleListTasks)
	s.mux.HandleFunc("GET /api/tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("POST /api/tasks", s.guardWrite(s.handleCreateTask))
	s.mux.HandleFunc("PUT /api/tasks/{id}", s.guardWrite(s.handleUpdateTask))
	s.mux.HandleFunc("PUT /api/tasks/{id}/done", s.guardWrite(s.handleTaskDone))
	s.mux.HandleFunc("DELETE /api/tasks/{id}", s.guardWrite(s.handleDeleteTask))
}

// GET /api/tasks?status=open|done|all&limit=n
func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := store.TasksOpen
	switch q.Get("status") {
	case "", "open":
	case "done":
		filter = store.TasksDone
	case "all":
		filter = store.TasksAll
	default:
		writeError(w, http.StatusBadRequest, "bad_status", "status muss open, done oder all sein")
		return
	}
	limit := 0
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, "bad_limit", "limit zwischen 1 und 1000")
			return
		}
		limit = n
	}
	tasks, err := s.cfg.Store.ListTasks(r.Context(), s.userID(r), filter, limit)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, tasks)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.cfg.Store.GetTask(r.Context(), s.userID(r), id)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, t)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "bad_json", "ungültiges JSON: "+err.Error())
		return false
	}
	return true
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var t store.Task
	if !decodeJSON(w, r, &t) {
		return
	}
	created, err := s.cfg.Store.CreateTask(r.Context(), s.userID(r), t)
	if err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, created)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var t store.Task
	if !decodeJSON(w, r, &t) {
		return
	}
	t.ID = id
	updated, err := s.cfg.Store.UpdateTask(r.Context(), s.userID(r), t)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, updated)
}

// PUT /api/tasks/{id}/done mit {"done": true|false}
func (s *Server) handleTaskDone(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Done *bool `json:"done"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Done == nil {
		writeError(w, http.StatusBadRequest, "bad_json", "done fehlt")
		return
	}
	t, err := s.cfg.Store.SetTaskDone(r.Context(), s.userID(r), id, *body.Done)
	if err != nil {
		s.storeError(w, err)
		return
	}
	writeJSON(w, t)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.cfg.Store.DeleteTask(r.Context(), s.userID(r), id); err != nil {
		s.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
