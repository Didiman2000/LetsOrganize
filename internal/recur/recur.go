// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (C) 2026 Andreas "Nupela Didiman" Neugebauer

// Package recur versteht Wiederholungsregeln im Format RRULE (RFC 5545),
// wie sie auch Google Kalender verwendet, und berechnet die Vorkommen.
//
// Unterstützt wird die Teilmenge, die ein Organizer im Alltag braucht:
//
//	FREQ=DAILY|WEEKLY|MONTHLY|YEARLY
//	INTERVAL=n            (alle n Tage/Wochen/Monate/Jahre)
//	BYDAY=MO,TU,…         (nur bei WEEKLY: an welchen Wochentagen)
//	COUNT=n               (endet nach n Vorkommen)
//	UNTIL=YYYYMMDD[THHMMSSZ] (endet an diesem Tag)
//
// Andere Angaben werden abgelehnt statt still ignoriert, damit keine Serie
// anders angezeigt wird, als sie bei Google gemeint ist.
package recur

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	// Zeitzonen-Datenbank einbetten: Windows bringt keine mit.
	_ "time/tzdata"
)

// Freq ist die Grundfrequenz einer Regel.
type Freq string

const (
	Daily   Freq = "DAILY"
	Weekly  Freq = "WEEKLY"
	Monthly Freq = "MONTHLY"
	Yearly  Freq = "YEARLY"
)

// Rule ist eine geprüfte Wiederholungsregel.
type Rule struct {
	Freq     Freq
	Interval int
	ByDay    []time.Weekday // sortiert ab Montag
	Count    int            // 0 = unbegrenzt
	Until    time.Time      // Nullwert = unbegrenzt; einschließlich
}

var dayCodes = map[string]time.Weekday{
	"MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday, "TH": time.Thursday,
	"FR": time.Friday, "SA": time.Saturday, "SU": time.Sunday,
}

var dayNames = map[time.Weekday]string{
	time.Monday: "MO", time.Tuesday: "TU", time.Wednesday: "WE", time.Thursday: "TH",
	time.Friday: "FR", time.Saturday: "SA", time.Sunday: "SU",
}

// mondayIndex: Montag = 0 … Sonntag = 6 (Wochenbeginn nach RFC 5545).
func mondayIndex(d time.Weekday) int { return (int(d) + 6) % 7 }

// Parse liest eine Regel. Ein vorangestelltes „RRULE:“ ist erlaubt.
func Parse(s string) (*Rule, error) {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "RRULE:"))
	if s == "" {
		return nil, errors.New("leere Regel")
	}
	r := &Rule{Interval: 1}
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ";") {
		key, val, ok := strings.Cut(part, "=")
		key = strings.ToUpper(strings.TrimSpace(key))
		val = strings.ToUpper(strings.TrimSpace(val))
		if !ok || key == "" || val == "" {
			return nil, fmt.Errorf("ungültiger Teil %q", part)
		}
		if seen[key] {
			return nil, fmt.Errorf("%s doppelt", key)
		}
		seen[key] = true
		switch key {
		case "FREQ":
			switch Freq(val) {
			case Daily, Weekly, Monthly, Yearly:
				r.Freq = Freq(val)
			default:
				return nil, fmt.Errorf("FREQ=%s wird nicht unterstützt", val)
			}
		case "INTERVAL":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 1000 {
				return nil, fmt.Errorf("INTERVAL muss zwischen 1 und 1000 liegen")
			}
			r.Interval = n
		case "COUNT":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 10000 {
				return nil, fmt.Errorf("COUNT muss zwischen 1 und 10000 liegen")
			}
			r.Count = n
		case "UNTIL":
			t, err := parseUntil(val)
			if err != nil {
				return nil, err
			}
			r.Until = t
		case "BYDAY":
			days := map[time.Weekday]bool{}
			for _, code := range strings.Split(val, ",") {
				d, ok := dayCodes[strings.TrimSpace(code)]
				if !ok {
					return nil, fmt.Errorf("BYDAY=%s wird nicht unterstützt (nur MO … SU)", code)
				}
				days[d] = true
			}
			for d := range days {
				r.ByDay = append(r.ByDay, d)
			}
			sort.Slice(r.ByDay, func(i, j int) bool { return mondayIndex(r.ByDay[i]) < mondayIndex(r.ByDay[j]) })
		case "WKST":
			if val != "MO" {
				return nil, errors.New("nur WKST=MO wird unterstützt")
			}
		default:
			return nil, fmt.Errorf("%s wird noch nicht unterstützt", key)
		}
	}
	if r.Freq == "" {
		return nil, errors.New("FREQ fehlt")
	}
	if len(r.ByDay) > 0 && r.Freq != Weekly {
		return nil, errors.New("BYDAY wird nur bei FREQ=WEEKLY unterstützt")
	}
	if r.Count > 0 && !r.Until.IsZero() {
		return nil, errors.New("COUNT und UNTIL schließen sich aus")
	}
	return r, nil
}

func parseUntil(v string) (time.Time, error) {
	for _, layout := range []string{"20060102T150405Z", "20060102"} {
		if t, err := time.Parse(layout, v); err == nil {
			if layout == "20060102" {
				// Ganzer Tag einschließlich.
				t = t.Add(24*time.Hour - time.Second)
			}
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("UNTIL=%s: Format JJJJMMTT oder JJJJMMTTTHHMMSSZ", v)
}

// String liefert die Regel in kanonischer Schreibweise (ohne „RRULE:“).
func (r *Rule) String() string {
	parts := []string{"FREQ=" + string(r.Freq)}
	if r.Interval > 1 {
		parts = append(parts, "INTERVAL="+strconv.Itoa(r.Interval))
	}
	if len(r.ByDay) > 0 {
		codes := make([]string, len(r.ByDay))
		for i, d := range r.ByDay {
			codes[i] = dayNames[d]
		}
		parts = append(parts, "BYDAY="+strings.Join(codes, ","))
	}
	if r.Count > 0 {
		parts = append(parts, "COUNT="+strconv.Itoa(r.Count))
	}
	if !r.Until.IsZero() {
		parts = append(parts, "UNTIL="+r.Until.UTC().Format("20060102T150405Z"))
	}
	return strings.Join(parts, ";")
}

// maxSteps begrenzt die Rechenarbeit je Abfrage (z. B. tägliche Serie über Jahrzehnte).
const maxSteps = 200_000

// Between liefert die Beginnzeiten aller Vorkommen, die den Zeitraum
// [from, to) berühren. start ist der Beginn des ersten Termins in der
// Zeitzone, in der die Serie gilt – so bleiben Termine bei Sommer- und
// Winterzeit auf derselben Uhrzeit. dur ist die Dauer eines Vorkommens.
// skip meldet Vorkommen, die einzeln gelöscht wurden; nil heißt keine.
func (r *Rule) Between(start time.Time, dur time.Duration, from, to time.Time, skip func(time.Time) bool) []time.Time {
	var out []time.Time
	n := 0
	emit := func(t time.Time) (stop bool) {
		if t.Before(start) {
			return false
		}
		if !r.Until.IsZero() && t.After(r.Until) {
			return true
		}
		n++
		if r.Count > 0 && n > r.Count {
			return true
		}
		if !t.Before(to) {
			return true
		}
		end := t.Add(dur)
		if (end.After(from) || (dur == 0 && !t.Before(from))) && (skip == nil || !skip(t)) {
			out = append(out, t)
		}
		return false
	}

	y, mo, d := start.Date()
	h, mi, s := start.Clock()
	loc := start.Location()
	at := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, h, mi, s, 0, loc) }

	for step := 0; step < maxSteps; step++ {
		switch r.Freq {
		case Daily:
			if emit(at(y, mo, d+step*r.Interval)) {
				return out
			}
		case Weekly:
			days := r.ByDay
			if len(days) == 0 {
				days = []time.Weekday{start.Weekday()}
			}
			monday := d - mondayIndex(start.Weekday()) + step*7*r.Interval
			for _, wd := range days {
				if emit(at(y, mo, monday+mondayIndex(wd))) {
					return out
				}
			}
		case Monthly:
			// Monate ohne diesen Tag (z. B. 31.) werden übersprungen, wie in RFC 5545.
			first := time.Date(y, mo+time.Month(step*r.Interval), 1, 0, 0, 0, 0, loc)
			if d <= daysIn(first.Year(), first.Month()) {
				if emit(at(first.Year(), first.Month(), d)) {
					return out
				}
			} else if !first.Before(to) {
				return out
			}
		case Yearly:
			year := y + step*r.Interval
			if d <= daysIn(year, mo) {
				if emit(at(year, mo, d)) {
					return out
				}
			} else if time.Date(year, mo, 1, 0, 0, 0, 0, loc).After(to) {
				return out
			}
		}
	}
	return out
}

func daysIn(y int, m time.Month) int { return time.Date(y, m+1, 0, 0, 0, 0, 0, time.UTC).Day() }
