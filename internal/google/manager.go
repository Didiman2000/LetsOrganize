// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/secret"
	"github.com/Didiman2000/LetsOrganize/internal/store"
)

// Manager hält die Google-Verbindungen und führt Abgleiche aus.
type Manager struct {
	Store  *store.Store
	Box    *secret.Box
	Client *Client // nil, wenn keine google-client.json vorhanden ist
	HTTP   *http.Client
	// NewAPI erzeugt den API-Zugang; in Tests zeigt er auf einen Testserver.
	NewAPI func(hc *http.Client, ts TokenSource) *API

	mu      sync.Mutex
	pending map[string]*Pending // nach state
	running map[int64]bool      // Benutzer, deren Abgleich gerade läuft
	last    map[int64]Status
}

// Status beschreibt den Zustand der Google-Verbindung eines Benutzers.
type Status struct {
	Configured bool      `json:"configured"` // google-client.json vorhanden
	Connected  bool      `json:"connected"`
	Email      string    `json:"email,omitempty"`
	Running    bool      `json:"running"`
	LastSync   time.Time `json:"lastSync,omitempty"`
	LastError  string    `json:"lastError,omitempty"`
	Report     *Report   `json:"report,omitempty"`
}

// NewManager erzeugt einen Manager. client darf nil sein.
func NewManager(st *store.Store, box *secret.Box, client *Client) *Manager {
	return &Manager{
		Store: st, Box: box, Client: client,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		NewAPI:  NewAPI,
		pending: map[string]*Pending{},
		running: map[int64]bool{},
		last:    map[int64]Status{},
	}
}

// ErrNotConfigured meldet, dass google-client.json fehlt.
var ErrNotConfigured = errors.New("Google ist nicht eingerichtet: google-client.json fehlt im Datenordner (siehe docs/GOOGLE-EINRICHTUNG.md)")

// Begin startet die Anmeldung und liefert die Adresse bei Google.
func (m *Manager) Begin(redirect string) (string, error) {
	if m.Client == nil {
		return "", ErrNotConfigured
	}
	authURL, p := m.Client.Begin(redirect)
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, old := range m.pending {
		if time.Since(old.Created) > 10*time.Minute {
			delete(m.pending, k)
		}
	}
	m.pending[p.State] = p
	return authURL, nil
}

// IsPending meldet, ob state zu einer laufenden Anmeldung gehört.
func (m *Manager) IsPending(state string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.pending[state]
	return ok
}

func tokenPurpose(userID int64) []byte {
	return []byte("google-token:" + strconv.FormatInt(userID, 10))
}

// Finish schließt die Anmeldung ab: Code tauschen, Konto ermitteln, Token
// verschlüsselt speichern.
func (m *Manager) Finish(ctx context.Context, userID int64, state, code string) (string, error) {
	m.mu.Lock()
	p := m.pending[state]
	delete(m.pending, state) // jede Anmeldung nur einmal
	m.mu.Unlock()
	if p == nil {
		return "", errors.New("unbekannte oder bereits verwendete Anmeldung")
	}
	tok, err := m.Client.Exchange(ctx, m.HTTP, p, code)
	if err != nil {
		return "", err
	}
	email, err := m.Client.Email(ctx, m.HTTP, tok)
	if err != nil {
		return "", err
	}
	enc, err := m.seal(userID, tok)
	if err != nil {
		return "", err
	}
	if _, err := m.Store.SaveGoogleAccount(ctx, userID, email, enc, tok.Scope); err != nil {
		return "", err
	}
	m.setLast(userID, func(s *Status) { s.LastError = "" })
	return email, nil
}

func (m *Manager) seal(userID int64, tok *Token) ([]byte, error) {
	raw, err := json.Marshal(tok)
	if err != nil {
		return nil, err
	}
	return m.Box.Seal(raw, tokenPurpose(userID))
}

func (m *Manager) open(userID int64, enc []byte) (*Token, error) {
	raw, err := m.Box.Open(enc, tokenPurpose(userID))
	if err != nil {
		return nil, err
	}
	var t Token
	return &t, json.Unmarshal(raw, &t)
}

// tokenSource liefert Zugangstokens für ein Konto und speichert erneuerte Tokens.
func (m *Manager) tokenSource(userID int64, acc *store.GoogleAccount) (TokenSource, error) {
	tok, err := m.open(userID, acc.TokenEnc)
	if err != nil {
		return nil, err
	}
	var mu sync.Mutex
	return func(ctx context.Context, force bool) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		if tok.Valid() && !force {
			return tok.AccessToken, nil
		}
		nt, err := m.Client.Refresh(ctx, m.HTTP, tok)
		if err != nil {
			return "", err
		}
		tok = nt
		if enc, err := m.seal(userID, tok); err == nil {
			if err := m.Store.UpdateGoogleToken(ctx, acc.ID, enc); err != nil {
				log.Printf("Google-Token speichern: %v", err)
			}
		}
		return tok.AccessToken, nil
	}, nil
}

// Disconnect trennt die Verbindung. Das Token wird bei Google widerrufen,
// soweit Google erreichbar ist; die lokalen Daten bleiben.
func (m *Manager) Disconnect(ctx context.Context, userID int64) error {
	acc, err := m.Store.GoogleAccountOf(ctx, userID)
	if err != nil {
		return err
	}
	if tok, err := m.open(userID, acc.TokenEnc); err == nil && tok.RefreshToken != "" && m.Client != nil {
		revoke(ctx, m.HTTP, tok.RefreshToken)
	}
	if err := m.Store.DeleteGoogleAccount(ctx, userID, acc.ID); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.last, userID)
	m.mu.Unlock()
	return nil
}

// RevokeURL ist der Endpunkt zum Widerrufen; in Tests änderbar.
var RevokeURL = "https://oauth2.googleapis.com/revoke"

func revoke(ctx context.Context, hc *http.Client, token string) {
	// Token im Inhalt statt in der Adresse, damit es in keinem Protokoll landet.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RevokeURL, strings.NewReader(url.Values{"token": {token}}.Encode()))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if res, err := hc.Do(req); err == nil {
		res.Body.Close()
	}
}

func (m *Manager) setLast(userID int64, f func(*Status)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.last[userID]
	f(&s)
	m.last[userID] = s
}

// Status liefert den Zustand für die Oberfläche.
func (m *Manager) Status(ctx context.Context, userID int64) Status {
	m.mu.Lock()
	s := m.last[userID]
	s.Running = m.running[userID]
	m.mu.Unlock()
	s.Configured = m.Client != nil
	if acc, err := m.Store.GoogleAccountOf(ctx, userID); err == nil {
		s.Connected = true
		s.Email = acc.Email
		if st, err := m.Store.GetSyncState(ctx, acc.ID, "calendar:"+PrimaryCalendar); err == nil && s.LastSync.IsZero() {
			s.LastSync = st.LastSync
		}
	}
	return s
}

// ErrBusy meldet, dass schon ein Abgleich läuft.
var ErrBusy = errors.New("Abgleich läuft bereits")

// Sync gleicht alle Bereiche des Benutzers mit Google ab.
func (m *Manager) Sync(ctx context.Context, userID int64) (*Report, error) {
	if m.Client == nil {
		return nil, ErrNotConfigured
	}
	acc, err := m.Store.GoogleAccountOf(ctx, userID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errors.New("nicht mit Google verbunden")
		}
		return nil, err
	}
	m.mu.Lock()
	if m.running[userID] {
		m.mu.Unlock()
		return nil, ErrBusy
	}
	m.running[userID] = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		delete(m.running, userID)
		m.mu.Unlock()
	}()

	ts, err := m.tokenSource(userID, acc)
	if err != nil {
		return nil, fmt.Errorf("Google-Token nicht lesbar – bitte neu verbinden: %w", err)
	}
	api := m.NewAPI(m.HTTP, ts)
	rep, err := SyncCalendar(ctx, m.Store, api, userID, acc.ID)
	m.setLast(userID, func(s *Status) {
		s.LastSync = time.Now()
		s.Report = rep
		s.LastError = ""
		if err != nil {
			s.LastError = err.Error()
		}
	})
	return rep, err
}

// Run gleicht alle verbundenen Benutzer regelmäßig ab, bis ctx endet.
func (m *Manager) Run(ctx context.Context, userIDs func() []int64, every time.Duration) {
	if m.Client == nil {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		for _, id := range userIDs() {
			if _, err := m.Store.GoogleAccountOf(ctx, id); err != nil {
				continue
			}
			if _, err := m.Sync(ctx, id); err != nil && !errors.Is(err, ErrBusy) {
				log.Printf("Google-Abgleich: %v", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
