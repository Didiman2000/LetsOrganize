// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package google

import (
	"reflect"
	"testing"
	"time"

	"github.com/Didiman2000/LetsOrganize/internal/store"
)

func TestParseExdate(t *testing.T) {
	cases := map[string][]string{
		"EXDATE;TZID=Europe/Berlin:20261207T090000,20260713T090000": {"2026-12-07T08:00:00Z", "2026-07-13T07:00:00Z"},
		"EXDATE:20261207T080000Z":                                   {"2026-12-07T08:00:00Z"},
		"EXDATE;VALUE=DATE:20261224":                                {"2026-12-24T00:00:00Z"},
		"EXDATE":                                                    nil,
	}
	for line, want := range cases {
		if got := parseExdate(line, ""); !reflect.DeepEqual(got, want) {
			t.Errorf("%s → %v, erwartet %v", line, got, want)
		}
	}
	// Ohne TZID gilt die Zeitzone des Termins.
	if got := parseExdate("EXDATE:20261207T090000", "Europe/Berlin"); !reflect.DeepEqual(got, []string{"2026-12-07T08:00:00Z"}) {
		t.Errorf("Zeitzone des Termins: %v", got)
	}
}

func TestRoundTrip(t *testing.T) {
	berlin, _ := time.LoadLocation("Europe/Berlin")
	start := time.Date(2026, 3, 16, 9, 0, 0, 0, berlin)
	local := store.SyncEvent{
		Event: store.Event{
			Title: "Jour fixe", Location: "Büro", Notes: "Agenda vorher schicken",
			Start: start.UTC(), End: start.Add(time.Hour).UTC(),
			RRule: "FREQ=WEEKLY;BYDAY=MO", TimeZone: "Europe/Berlin",
		},
		Exdates: []string{"2026-03-30T07:00:00Z"},
	}
	ge := toGoogle(local)
	if ge.Start.TimeZone != "Europe/Berlin" || ge.Start.DateTime != "2026-03-16T09:00:00+01:00" {
		t.Errorf("Beginn: %+v", ge.Start)
	}
	if len(ge.Recurrence) != 2 || ge.Recurrence[0] != "RRULE:FREQ=WEEKLY;BYDAY=MO" || ge.Recurrence[1] != "EXDATE:20260330T070000Z" {
		t.Errorf("Wiederholung: %v", ge.Recurrence)
	}
	ge.ID, ge.ETag = "g1", `"1"`
	back, err := fromGoogle(*ge)
	if err != nil {
		t.Fatal(err)
	}
	if back.Title != local.Title || !back.Start.Equal(local.Start) || back.RRule != local.RRule ||
		back.TimeZone != local.TimeZone || !reflect.DeepEqual(back.Exdates, local.Exdates) {
		t.Errorf("Hin und zurück:\n%+v\n%+v", back, local)
	}

	allDay := store.SyncEvent{Event: store.Event{
		Title: "Urlaub", AllDay: true,
		Start: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), End: time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
	}}
	g2 := toGoogle(allDay)
	if g2.Start.Date != "2026-08-03" || g2.End.Date != "2026-08-15" || g2.Start.DateTime != "" {
		t.Errorf("ganztägig: %+v %+v", g2.Start, g2.End)
	}
	b2, _ := fromGoogle(*g2)
	if !b2.AllDay || !b2.Start.Equal(allDay.Start) || !b2.End.Equal(allDay.End) {
		t.Errorf("ganztägig zurück: %+v", b2)
	}

	// Abweichender Einzeltermin einer Serie.
	inst := Event{ID: "g1_20260323T080000Z", Summary: "Jour fixe (Dienstag)", RecurringEventID: "g1",
		Start:             &EventTime{DateTime: "2026-03-24T09:00:00+01:00", TimeZone: "Europe/Berlin"},
		End:               &EventTime{DateTime: "2026-03-24T10:00:00+01:00", TimeZone: "Europe/Berlin"},
		OriginalStartTime: &EventTime{DateTime: "2026-03-23T09:00:00+01:00"}}
	bi, err := fromGoogle(inst)
	if err != nil || bi.SeriesRemoteID != "g1" || bi.OriginalStart != "2026-03-23T08:00:00Z" || bi.RRule != "" {
		t.Errorf("Einzeltermin: %v %+v", err, bi)
	}
	if _, err := fromGoogle(Event{ID: "x"}); err == nil {
		t.Error("Termin ohne Zeiten akzeptiert")
	}
}
