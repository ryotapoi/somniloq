package main

import (
	"fmt"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

// buildSessionFilter resolves the time flags and expands the --project value
// through the config's alias groups, so callers cannot forget the expansion.
func buildSessionFilter(since, until, project string, cfg config) (core.SessionFilter, error) {
	now := time.Now().UTC()
	var filter core.SessionFilter
	if since != "" {
		s, err := resolveTimeFlag(since, now, false, time.Local)
		if err != nil {
			return filter, err
		}
		filter.Since = s
	}
	if until != "" {
		u, err := resolveTimeFlag(until, now, true, time.Local)
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

func resolveTimeFlag(value string, now time.Time, isUntil bool, loc *time.Location) (string, error) {
	t, dateOnly, err := core.ParseTimeRef(value, now, loc)
	if err != nil {
		return "", err
	}
	if isUntil && dateOnly {
		t = t.AddDate(0, 0, 1)
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
}
