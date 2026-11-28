// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// TokenSource liefert ein gültiges Zugangstoken und erneuert es bei Bedarf.
// force erzwingt die Erneuerung (nach einer 401-Antwort).
type TokenSource func(ctx context.Context, force bool) (string, error)

// API ruft die REST-Schnittstellen von Google auf.
type API struct {
	HTTP        *http.Client
	Token       TokenSource
	CalendarURL string // https://www.googleapis.com/calendar/v3
}

// NewAPI erzeugt einen API-Zugang mit den echten Google-Adressen.
func NewAPI(hc *http.Client, ts TokenSource) *API {
	return &API{HTTP: hc, Token: ts, CalendarURL: "https://www.googleapis.com/calendar/v3"}
}

// StatusError ist eine Fehlerantwort von Google.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string { return fmt.Sprintf("Google antwortet %d: %s", e.Code, e.Body) }

// IsStatus meldet, ob err eine Google-Antwort mit einem dieser Codes ist.
func IsStatus(err error, codes ...int) bool {
	var se *StatusError
	if !errors.As(err, &se) {
		return false
	}
	for _, c := range codes {
		if se.Code == c {
			return true
		}
	}
	return false
}

// do sendet eine Anfrage; bei 401 wird das Token einmal erneuert.
// header darf nil sein; out darf nil sein.
func (a *API) do(ctx context.Context, method, u string, header http.Header, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 2; attempt++ {
		tok, err := a.Token(ctx, attempt > 0)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		if in != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		for k, v := range header {
			req.Header[k] = v
		}
		res, err := a.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("Google nicht erreichbar: %w", err)
		}
		data, _ := io.ReadAll(io.LimitReader(res.Body, 20<<20))
		res.Body.Close()
		if res.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if res.StatusCode >= 300 {
			msg := string(data)
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return &StatusError{Code: res.StatusCode, Body: msg}
		}
		if out != nil && len(data) > 0 {
			if err := json.Unmarshal(data, out); err != nil {
				return fmt.Errorf("Antwort von Google unlesbar: %w", err)
			}
		}
		return nil
	}
	return &StatusError{Code: http.StatusUnauthorized, Body: "nicht angemeldet"}
}

// EventTime ist Beginn oder Ende eines Google-Termins.
type EventTime struct {
	Date     string `json:"date,omitempty"`     // ganztägig: YYYY-MM-DD
	DateTime string `json:"dateTime,omitempty"` // RFC 3339
	TimeZone string `json:"timeZone,omitempty"`
}

// Time liefert den Zeitpunkt; ganztägige Angaben als 00:00 UTC.
func (t EventTime) Time() (time.Time, bool, error) {
	if t.Date != "" {
		d, err := time.Parse(time.DateOnly, t.Date)
		return d, true, err
	}
	d, err := time.Parse(time.RFC3339, t.DateTime)
	return d, false, err
}

// Event ist ein Termin, wie Google Kalender ihn liefert.
type Event struct {
	ID                string     `json:"id,omitempty"`
	ETag              string     `json:"etag,omitempty"`
	Status            string     `json:"status,omitempty"` // confirmed, tentative, cancelled
	Summary           string     `json:"summary"`
	Description       string     `json:"description"`
	Location          string     `json:"location"`
	Start             *EventTime `json:"start,omitempty"`
	End               *EventTime `json:"end,omitempty"`
	Recurrence        []string   `json:"recurrence,omitempty"`
	RecurringEventID  string     `json:"recurringEventId,omitempty"`
	OriginalStartTime *EventTime `json:"originalStartTime,omitempty"`
	Updated           string     `json:"updated,omitempty"`
}

// EventPage ist eine Seite der Terminliste.
type EventPage struct {
	Items         []Event `json:"items"`
	NextPageToken string  `json:"nextPageToken"`
	NextSyncToken string  `json:"nextSyncToken"`
}

// ErrSyncTokenExpired meldet, dass Google einen vollständigen Abgleich verlangt.
var ErrSyncTokenExpired = errors.New("Sync-Token abgelaufen")

// ListEvents liefert alle Änderungen seit syncToken (leer = alles).
// Serien kommen als eine Serie, nicht als einzelne Vorkommen.
func (a *API) ListEvents(ctx context.Context, calendarID, syncToken string) (items []Event, nextSync string, err error) {
	pageToken := ""
	for page := 0; page < 1000; page++ {
		q := url.Values{"showDeleted": {"true"}, "maxResults": {"250"}}
		if syncToken != "" {
			q.Set("syncToken", syncToken)
		}
		if pageToken != "" {
			q.Set("pageToken", pageToken)
		}
		var p EventPage
		err := a.do(ctx, http.MethodGet, a.CalendarURL+"/calendars/"+url.PathEscape(calendarID)+"/events?"+q.Encode(), nil, nil, &p)
		if IsStatus(err, http.StatusGone) {
			return nil, "", ErrSyncTokenExpired
		}
		if err != nil {
			return nil, "", err
		}
		items = append(items, p.Items...)
		if p.NextPageToken == "" {
			return items, p.NextSyncToken, nil
		}
		pageToken = p.NextPageToken
	}
	return nil, "", errors.New("zu viele Seiten von Google")
}

// InsertEvent legt einen Termin bei Google an.
func (a *API) InsertEvent(ctx context.Context, calendarID string, e *Event) (*Event, error) {
	var out Event
	err := a.do(ctx, http.MethodPost, a.CalendarURL+"/calendars/"+url.PathEscape(calendarID)+"/events", nil, e, &out)
	return &out, err
}

// UpdateEvent ersetzt einen Termin bei Google. Mit etag schlägt die Änderung
// fehl (412), wenn der Termin dort inzwischen geändert wurde.
func (a *API) UpdateEvent(ctx context.Context, calendarID string, e *Event, etag string) (*Event, error) {
	var out Event
	h := http.Header{}
	if etag != "" {
		h.Set("If-Match", etag)
	}
	err := a.do(ctx, http.MethodPut, a.CalendarURL+"/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(e.ID), h, e, &out)
	return &out, err
}

// DeleteEvent löscht einen Termin bei Google. Bereits gelöschte gelten als erledigt.
func (a *API) DeleteEvent(ctx context.Context, calendarID, id string) error {
	err := a.do(ctx, http.MethodDelete, a.CalendarURL+"/calendars/"+url.PathEscape(calendarID)+"/events/"+url.PathEscape(id), nil, nil, nil)
	if IsStatus(err, http.StatusNotFound, http.StatusGone) {
		return nil
	}
	return err
}
