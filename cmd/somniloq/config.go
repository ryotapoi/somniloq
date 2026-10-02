package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// config is the optional CLI configuration loaded from
// ~/.somniloq/config.json (overridable with the global --config flag).
type config struct {
	// ProjectAliases groups project names that refer to the same project
	// over time (e.g. a renamed repository): canonical name -> old names.
	ProjectAliases map[string][]string `json:"projectAliases"`
	// ExcludeUserMessagePatterns filters user messages from outline and show summaries.
	ExcludeUserMessagePatterns []string `json:"excludeUserMessagePatterns"`
	// DayBoundary shifts date-only filters and sessions logical-day display.
	// Empty means the calendar day starts at 00:00 local time.
	DayBoundary string `json:"dayBoundary"`
}

// loadConfig reads the config file at path. A missing file is an empty
// config, not an error; invalid JSON is an error so a typo cannot silently
// disable aliases.
func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return config{}, nil
	}
	if err != nil {
		return config{}, fmt.Errorf("read config: %w", err)
	}
	var c config
	if err := json.Unmarshal(data, &c); err != nil {
		return config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if _, err := compileUserMessagePatterns(c.ExcludeUserMessagePatterns); err != nil {
		return config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if _, err := parseDayBoundary(c.DayBoundary); err != nil {
		return config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return c, nil
}

type dayBoundary struct {
	offset time.Duration
}

func resolveDayBoundary(flagValue string, cfg config) (dayBoundary, error) {
	if flagValue != "" {
		return parseDayBoundary(flagValue)
	}
	return parseDayBoundary(cfg.DayBoundary)
}

func parseDayBoundary(value string) (dayBoundary, error) {
	if value == "" {
		return dayBoundary{}, nil
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 {
		return dayBoundary{}, fmt.Errorf("invalid dayBoundary %q (use HH:MM)", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return dayBoundary{}, fmt.Errorf("invalid dayBoundary %q (use HH:MM)", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil {
		return dayBoundary{}, fmt.Errorf("invalid dayBoundary %q (use HH:MM)", value)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return dayBoundary{}, fmt.Errorf("invalid dayBoundary %q (use HH:MM)", value)
	}
	return dayBoundary{offset: time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute}, nil
}

// expandProject resolves a --project value against the alias groups: when it
// exactly equals a group's canonical name or one of its old names, the whole
// group is returned so any of the names matches. Otherwise the value is
// passed through unchanged. Exact equality keeps expansion predictable —
// substring matching still happens in SQL against each returned pattern.
func (c config) expandProject(project string) []string {
	if project == "" {
		return nil
	}
	for canonical, oldNames := range c.ProjectAliases {
		if project != canonical && !slices.Contains(oldNames, project) {
			continue
		}
		return append([]string{canonical}, oldNames...)
	}
	return []string{project}
}
