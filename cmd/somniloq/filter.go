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
	if filter.Since != "" && filter.Until != "" && filter.Since >= filter.Until {
		return filter, fmt.Errorf("--since must be before --until")
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
		t = t.Add(boundary.offset)
	}
	if isUntil && dateOnly {
		t = t.AddDate(0, 0, 1)
	}
	// Normalize to three-digit UTC representation compatible with the lexical range
	// comparisons in internal/core/db_query.go. Source JSONL timestamps are
	// stored in sessions.started_at and messages.timestamp with RFC3339
	// second-or-finer precision, so an equal seconds-precision value (…:05Z)
	// remains >= this boundary (…:05.000Z).
	return t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
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
	return t.In(loc).Add(-boundary.offset).Format("2006-01-02")
}
