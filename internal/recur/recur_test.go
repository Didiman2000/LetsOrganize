// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

package recur

import (
	"strings"
	"testing"
	"time"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func days(ts []time.Time, loc *time.Location) string {
	var parts []string
	for _, t := range ts {
		parts = append(parts, t.In(loc).Format("2006-01-02 15:04"))
	}
	return strings.Join(parts, ", ")
}

func mustParse(t *testing.T, s string) *Rule {
	t.Helper()
	r, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%q): %v", s, err)
	}
	return r
}

func TestParseAndString(t *testing.T) {
	cases := map[string]string{
		"FREQ=DAILY": "FREQ=DAILY",
		"RRULE:freq=weekly;byday=fr,mo;interval=2": "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,FR",
		"FREQ=MONTHLY;COUNT=12":                    "FREQ=MONTHLY;COUNT=12",
		"FREQ=YEARLY;UNTIL=20301231":               "FREQ=YEARLY;UNTIL=20301231T235959Z",
		"FREQ=WEEKLY;WKST=MO;INTERVAL=1":           "FREQ=WEEKLY",
	}
	for in, want := range cases {
		if got := mustParse(t, in).String(); got != want {
			t.Errorf("%q → %q, erwartet %q", in, got, want)
		}
	}
	for _, bad := range []string{
		"", "FREQ=HOURLY", "INTERVAL=2", "FREQ=DAILY;INTERVAL=0", "FREQ=DAILY;COUNT=3;UNTIL=20300101",
		"FREQ=MONTHLY;BYDAY=2TU", "FREQ=WEEKLY;BYDAY=XX", "FREQ=DAILY;BYMONTH=3", "FREQ=DAILY;FREQ=DAILY",
		"FREQ=DAILY;UNTIL=morgen", "FREQ=MONTHLY;BYDAY=MO",
	} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q fälschlich akzeptiert", bad)
		}
	}
}

// Wöchentlich um 9 Uhr bleibt über die Zeitumstellung hinweg 9 Uhr.
func TestWeeklyAcrossDST(t *testing.T) {
	loc := berlin(t)
	start := time.Date(2026, 3, 16, 9, 0, 0, 0, loc) // Montag, Winterzeit
	r := mustParse(t, "FREQ=WEEKLY")
	got := r.Between(start, time.Hour, time.Date(2026, 3, 1, 0, 0, 0, 0, loc), time.Date(2026, 4, 7, 0, 0, 0, 0, loc), nil)
	want := "2026-03-16 09:00, 2026-03-23 09:00, 2026-03-30 09:00, 2026-04-06 09:00"
	if days(got, loc) != want {
		t.Errorf("got  %s\nwant %s", days(got, loc), want)
	}
	if got[0].UTC().Hour() != 8 || got[2].UTC().Hour() != 7 {
		t.Error("UTC-Stunden müssen sich mit der Sommerzeit verschieben")
	}
}

func TestWeeklyByDayIntervalCount(t *testing.T) {
	loc := berlin(t)
	start := time.Date(2026, 12, 2, 18, 30, 0, 0, loc) // Mittwoch
	r := mustParse(t, "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE,FR;COUNT=5")
	got := r.Between(start, 30*time.Minute, start.AddDate(0, 0, -30), start.AddDate(1, 0, 0), nil)
	// Montag 30.11. liegt vor dem Beginn und zählt nicht.
	want := "2026-12-02 18:30, 2026-12-04 18:30, 2026-12-14 18:30, 2026-12-16 18:30, 2026-12-18 18:30"
	if days(got, loc) != want {
		t.Errorf("got  %s\nwant %s", days(got, loc), want)
	}
}

func TestMonthlySkipsShortMonths(t *testing.T) {
	loc := time.UTC
	start := time.Date(2026, 1, 31, 10, 0, 0, 0, loc)
	r := mustParse(t, "FREQ=MONTHLY;COUNT=4")
	got := r.Between(start, time.Hour, start, start.AddDate(2, 0, 0), nil)
	want := "2026-01-31 10:00, 2026-03-31 10:00, 2026-05-31 10:00, 2026-07-31 10:00"
	if days(got, loc) != want {
		t.Errorf("got  %s\nwant %s", days(got, loc), want)
	}
}

func TestYearlyLeapDayAndUntil(t *testing.T) {
	loc := time.UTC
	start := time.Date(2024, 2, 29, 0, 0, 0, 0, loc)
	r := mustParse(t, "FREQ=YEARLY;UNTIL=20330101")
	got := r.Between(start, 24*time.Hour, start, time.Date(2040, 1, 1, 0, 0, 0, 0, loc), nil)
	want := "2024-02-29 00:00, 2028-02-29 00:00, 2032-02-29 00:00"
	if days(got, loc) != want {
		t.Errorf("got  %s\nwant %s", days(got, loc), want)
	}
}

func TestWindowSkipAndOverlap(t *testing.T) {
	loc := berlin(t)
	start := time.Date(2026, 11, 2, 23, 0, 0, 0, loc) // täglich 23–1 Uhr
	r := mustParse(t, "FREQ=DAILY")
	from := time.Date(2026, 11, 10, 0, 0, 0, 0, loc)
	to := time.Date(2026, 11, 12, 0, 0, 0, 0, loc)
	gone := time.Date(2026, 11, 10, 23, 0, 0, 0, loc)
	skip := func(t time.Time) bool { return t.Equal(gone) }
	got := r.Between(start, 2*time.Hour, from, to, skip)
	// 9.11. 23 Uhr reicht in den 10.11. hinein; 10.11. ist gelöscht; 11.11. bleibt.
	want := "2026-11-09 23:00, 2026-11-11 23:00"
	if days(got, loc) != want {
		t.Errorf("got  %s\nwant %s", days(got, loc), want)
	}
}

func TestLongSeriesIsBounded(t *testing.T) {
	start := time.Date(1990, 1, 1, 8, 0, 0, 0, time.UTC)
	r := mustParse(t, "FREQ=DAILY")
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := r.Between(start, time.Hour, from, from.AddDate(0, 0, 7), nil)
	if len(got) != 7 {
		t.Errorf("%d Vorkommen, erwartet 7", len(got))
	}
}
