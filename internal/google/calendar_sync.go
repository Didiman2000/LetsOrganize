// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

// PrimaryCalendar ist der Hauptkalender des Google-Kontos.
const PrimaryCalendar = "primary"

// Report fasst einen Abgleich zusammen.
type Report struct {
	Pulled   int      `json:"pulled"`   // von Google übernommen
	Deleted  int      `json:"deleted"`  // bei Google gelöscht
	Pushed   int      `json:"pushed"`   // zu Google hochgeladen
	Removed  int      `json:"removed"`  // bei Google gelöscht, weil lokal gelöscht
	Kept     int      `json:"kept"`     // Konflikte, bei denen die lokale Änderung neuer war
	Problems []string `json:"problems"` // einzelne Termine, die nicht abgeglichen werden konnten
}

const timeLayoutUTC = "2006-01-02T15:04:05Z"

// SyncCalendar gleicht den Hauptkalender in beide Richtungen ab:
// erst die Änderungen von Google holen, dann lokale Änderungen hochladen.
// Bei Konflikten gewinnt die jüngere Änderung.
func SyncCalendar(ctx context.Context, st *store.Store, api *API, userID, accountID int64) (*Report, error) {
	rep := &Report{Problems: []string{}}
	resource := "calendar:" + PrimaryCalendar
	state, err := st.GetSyncState(ctx, accountID, resource)
	if err != nil {
		return nil, err
	}
	items, next, err := api.ListEvents(ctx, PrimaryCalendar, state.Token)
	if errors.Is(err, ErrSyncTokenExpired) {
		items, next, err = api.ListEvents(ctx, PrimaryCalendar, "")
	}
	if err != nil {
		return nil, err
	}

	// Erst Serien und Einzeltermine, dann Vorkommen von Serien – so ist die
	// Serie schon da, wenn ein abgesagtes Vorkommen eingetragen wird.
	var instances []Event
	for _, ge := range items {
		if ge.RecurringEventID != "" {
			instances = append(instances, ge)
			continue
		}
		if err := pullOne(ctx, st, userID, accountID, ge, rep); err != nil {
			rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", nameOf(ge), err))
		}
	}
	for _, ge := range instances {
		if err := pullOne(ctx, st, userID, accountID, ge, rep); err != nil {
			rep.Problems = append(rep.Problems, fmt.Sprintf("%s: %v", nameOf(ge), err))
		}
	}
	if next != "" {
		if err := st.SetSyncState(ctx, accountID, resource, next); err != nil {
			return nil, err
		}
	}

	if err := pushCalendar(ctx, st, api, userID, accountID, rep); err != nil {
		return rep, err
	}
	return rep, nil
}

func nameOf(ge Event) string {
	if ge.Summary != "" {
		return fmt.Sprintf("„%s“", ge.Summary)
	}
	return "Termin " + ge.ID
}

func pullOne(ctx context.Context, st *store.Store, userID, accountID int64, ge Event, rep *Report) error {
	local, err := st.EventByRemote(ctx, userID, PrimaryCalendar, ge.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if local != nil && local.ETag == ge.ETag {
		return nil // unser eigener Stand oder unverändert
	}

	if ge.Status == "cancelled" {
		if ge.RecurringEventID != "" && ge.OriginalStartTime != nil {
			// Einzelnes Vorkommen einer Serie abgesagt.
			t, _, err := ge.OriginalStartTime.Time()
			if err != nil {
				return err
			}
			if local != nil {
				// Es gab einen abweichenden Einzeltermin: der entfällt.
				if err := st.RemoteDeleted(ctx, userID, PrimaryCalendar, ge.ID); err != nil {
					return err
				}
			}
			rep.Deleted++
			return st.AddRemoteExdate(ctx, userID, PrimaryCalendar, ge.RecurringEventID, t)
		}
		if local == nil || local.Deleted {
			return nil
		}
		rep.Deleted++
		return st.RemoteDeleted(ctx, userID, PrimaryCalendar, ge.ID)
	}

	se, err := fromGoogle(ge)
	if err != nil {
		return err
	}
	if local != nil && local.Dirty {
		remoteUpdated, _ := time.Parse(time.RFC3339, ge.Updated)
		if local.UpdatedAt.After(remoteUpdated) {
			rep.Kept++
			return st.KeepLocalOverRemote(ctx, local.ID, ge.ETag)
		}
	}
	rep.Pulled++
	return st.ApplyRemoteEvent(ctx, userID, accountID, PrimaryCalendar, se)
}

func pushCalendar(ctx context.Context, st *store.Store, api *API, userID, accountID int64, rep *Report) error {
	dirty, err := st.DirtyEvents(ctx, userID)
	if err != nil {
		return err
	}
	for _, se := range dirty {
		if err := ctx.Err(); err != nil {
			return err
		}
		var res *Event
		switch {
		case se.Deleted && se.RemoteID == "":
			// Nie bei Google gewesen: nur vormerken, dass nichts mehr zu tun ist.
			res = &Event{}
		case se.Deleted:
			if err = api.DeleteEvent(ctx, PrimaryCalendar, se.RemoteID); err == nil {
				rep.Removed++
				res = &Event{ID: se.RemoteID, ETag: se.ETag}
			}
		case se.RemoteID == "":
			if res, err = api.InsertEvent(ctx, PrimaryCalendar, toGoogle(se)); err == nil {
				rep.Pushed++
			}
		default:
			ge := toGoogle(se)
			ge.ID = se.RemoteID
			res, err = api.UpdateEvent(ctx, PrimaryCalendar, ge, se.ETag)
			if IsStatus(err, http.StatusPreconditionFailed) {
				// Bei Google zwischendurch geändert: beim nächsten Abgleich entscheiden.
				rep.Problems = append(rep.Problems, fmt.Sprintf("„%s“: bei Google zwischendurch geändert, nächster Abgleich klärt das", se.Title))
				continue
			}
			if IsStatus(err, http.StatusNotFound, http.StatusGone) {
				// Bei Google gelöscht, lokal geändert: neu anlegen, damit nichts verloren geht.
				ge.ID, ge.ETag = "", ""
				res, err = api.InsertEvent(ctx, PrimaryCalendar, ge)
			}
			if err == nil {
				rep.Pushed++
			}
		}
		if err != nil {
			if IsStatus(err, http.StatusUnauthorized) || errors.Is(err, ErrRevoked) {
				return err
			}
			log.Printf("Google-Abgleich, Termin %d: %v", se.ID, err)
			rep.Problems = append(rep.Problems, fmt.Sprintf("„%s“: %v", se.Title, err))
			continue
		}
		if err := st.MarkEventSynced(ctx, se.ID, accountID, PrimaryCalendar, res.ID, res.ETag, se.UpdatedAt); err != nil {
			return err
		}
	}
	return nil
}

// fromGoogle wandelt einen Google-Termin in einen lokalen um.
func fromGoogle(ge Event) (store.SyncEvent, error) {
	se := store.SyncEvent{RemoteID: ge.ID, ETag: ge.ETag}
	se.Title, se.Location, se.Notes = ge.Summary, ge.Location, ge.Description
	if ge.Start == nil || ge.End == nil {
		return se, errors.New("Beginn oder Ende fehlt")
	}
	start, allDay, err := ge.Start.Time()
	if err != nil {
		return se, fmt.Errorf("Beginn: %w", err)
	}
	end, _, err := ge.End.Time()
	if err != nil {
		return se, fmt.Errorf("Ende: %w", err)
	}
	se.Start, se.End, se.AllDay = start.UTC(), end.UTC(), allDay
	se.TimeZone = ge.Start.TimeZone
	if se.TimeZone != "" {
		if _, err := time.LoadLocation(se.TimeZone); err != nil {
			se.TimeZone = ""
		}
	}
	for _, line := range ge.Recurrence {
		switch {
		case strings.HasPrefix(line, "RRULE:"):
			se.RRule = strings.TrimPrefix(line, "RRULE:")
		case strings.HasPrefix(line, "EXDATE"):
			se.Exdates = append(se.Exdates, parseExdate(line, se.TimeZone)...)
		}
	}
	if ge.RecurringEventID != "" && ge.OriginalStartTime != nil {
		if t, _, err := ge.OriginalStartTime.Time(); err == nil {
			se.SeriesRemoteID = ge.RecurringEventID
			se.OriginalStart = t.UTC().Format(timeLayoutUTC)
		}
	}
	return se, nil
}

// parseExdate liest Zeilen wie
//
//	EXDATE;TZID=Europe/Berlin:20261207T090000,20261214T090000
//	EXDATE:20261207T080000Z
//	EXDATE;VALUE=DATE:20261207
func parseExdate(line, defaultTZ string) []string {
	head, values, ok := strings.Cut(line, ":")
	if !ok {
		return nil
	}
	loc := time.UTC
	if defaultTZ != "" {
		if l, err := time.LoadLocation(defaultTZ); err == nil {
			loc = l
		}
	}
	for _, p := range strings.Split(head, ";")[1:] {
		if tz, ok := strings.CutPrefix(p, "TZID="); ok {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
	}
	var out []string
	for _, v := range strings.Split(values, ",") {
		v = strings.TrimSpace(v)
		var t time.Time
		var err error
		switch {
		case strings.HasSuffix(v, "Z"):
			t, err = time.Parse("20060102T150405Z", v)
		case len(v) == 8:
			t, err = time.Parse("20060102", v)
		default:
			t, err = time.ParseInLocation("20060102T150405", v, loc)
		}
		if err == nil {
			out = append(out, t.UTC().Format(timeLayoutUTC))
		}
	}
	return out
}

// toGoogle wandelt einen lokalen Termin in einen Google-Termin um.
func toGoogle(se store.SyncEvent) *Event {
	ge := &Event{Summary: se.Title, Description: se.Notes, Location: se.Location, ETag: se.ETag}
	if se.AllDay {
		ge.Start = &EventTime{Date: se.Start.UTC().Format(time.DateOnly)}
		ge.End = &EventTime{Date: se.End.UTC().Format(time.DateOnly)}
	} else {
		tz := se.TimeZone
		loc, err := time.LoadLocation(tz)
		if tz == "" || err != nil {
			tz, loc = "UTC", time.UTC
		}
		// Google verlangt bei Serien eine Zeitzone.
		ge.Start = &EventTime{DateTime: se.Start.In(loc).Format(time.RFC3339), TimeZone: tz}
		ge.End = &EventTime{DateTime: se.End.In(loc).Format(time.RFC3339), TimeZone: tz}
	}
	if se.RRule != "" {
		ge.Recurrence = []string{"RRULE:" + se.RRule}
		if len(se.Exdates) > 0 {
			var vals []string
			for _, x := range se.Exdates {
				t, err := time.Parse(timeLayoutUTC, x)
				if err != nil {
					continue
				}
				if se.AllDay {
					vals = append(vals, t.Format("20060102"))
				} else {
					vals = append(vals, t.Format("20060102T150405Z"))
				}
			}
			if len(vals) > 0 {
				prefix := "EXDATE:"
				if se.AllDay {
					prefix = "EXDATE;VALUE=DATE:"
				}
				ge.Recurrence = append(ge.Recurrence, prefix+strings.Join(vals, ","))
			}
		}
	}
	if se.SeriesRemoteID != "" && se.OriginalStart != "" {
		ge.RecurringEventID = se.SeriesRemoteID
		if t, err := time.Parse(timeLayoutUTC, se.OriginalStart); err == nil {
			if se.AllDay {
				ge.OriginalStartTime = &EventTime{Date: t.Format(time.DateOnly)}
			} else {
				ge.OriginalStartTime = &EventTime{DateTime: t.Format(time.RFC3339), TimeZone: ge.Start.TimeZone}
			}
		}
	}
	return ge
}
