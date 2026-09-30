package main

import (
	"fmt"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

// buildSessionFilter resolves the time flags and expands the --project value
// through the config's alias groups, so callers cannot forget the expansion.
func buildSessionFilter(since, until, project string, cfg config, boundary dayBoundary) (core.SessionFilter, error) {
	return buildSessionFilterAt(time.Now().UTC(), since, until, project, cfg, boundary)
}

// buildSessionFilterAt resolves the time flags using the supplied current time.
func buildSessionFilterAt(now time.Time, since, until, project string, cfg config, boundary dayBoundary) (core.SessionFilter, error) {
	var filter core.SessionFilter
	if since != "" {
		s, err := resolveTimeFlag(since, now, false, time.Local, boundary)
		if err != nil {
			return filter, err
		}
		filter.Since = s
	}
	if until != "" {
		u, err := resolveTimeFlag(until, now, true, time.Local, boundary)
		if err != nil {
			return filter, err
		}
		filter.Until = u
	}
	if filter.Since != "" && filter.Until != "" {
		sinceTime, err := time.Parse(time.RFC3339Nano, filter.Since)
		if err != nil {
			return filter, err
		}
		untilTime, err := time.Parse(time.RFC3339Nano, filter.Until)
		if err != nil {
			return filter, err
		}
		if !sinceTime.Before(untilTime) {
			return filter, fmt.Errorf("--since must be before --until")
		}
	}
	filter.Projects = cfg.expandProject(project)
	return filter, nil
}

func resolveTimeFlag(value string, now time.Time, isUntil bool, loc *time.Location, boundary dayBoundary) (string, error) {
	t, dateOnly, err := core.ParseTimeRef(value, now, loc)
	if err != nil {
		return "", err
	}
	if dateOnly {
		// Preserve the input calendar date even when local midnight is a DST gap.
		t, err = time.Parse("2006-01-02", value)
		if err != nil {
			return "", err
		}
		dayOffset := 0
		if isUntil {
			dayOffset = 1
		}
		t = boundary.onDate(t, dayOffset, loc)
	}
	return t.UTC().Format(time.RFC3339Nano), nil
}

// resolveImportedSince resolves an imported_at lower bound. imported_at is
// stored at whole-second precision, so a boundary between seconds must advance
// to the next stored second instead of admitting the preceding one lexically.
func resolveImportedSince(value string, now time.Time, loc *time.Location) (string, error) {
	t, _, err := core.ParseTimeRef(value, now, loc)
	if err != nil {
		return "", err
	}
	t = t.UTC()
	if t.Nanosecond() != 0 {
		t = t.Truncate(time.Second).Add(time.Second)
	}
	return t.Format("2006-01-02T15:04:05.000Z"), nil
}

func sessionLogicalDay(session core.SessionRow, boundary dayBoundary, loc *time.Location) string {
	value := session.EndedAt
	if value == "" {
		value = session.StartedAt
	}
	if value == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return ""
	}
	local := t.In(loc)
	year, month, day := local.Date()
	if t.Before(boundary.onDate(local, 0, loc)) {
		day--
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// onDate constructs a local clock boundary, rather than adding elapsed time
// to midnight, because DST days need not contain 24 hours.
func (boundary dayBoundary) onDate(date time.Time, dayOffset int, loc *time.Location) time.Time {
	year, month, day := date.Date()
	hour := int(boundary.offset / time.Hour)
	minute := int(boundary.offset % time.Hour / time.Minute)
	return time.Date(year, month, day+dayOffset, hour, minute, 0, 0, loc)
}
