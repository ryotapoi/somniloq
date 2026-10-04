package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/ryotapoi/somniloq/internal/core"
)

const configSetupHint = "Run somniloq config init, then use --config default."

var configNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// config holds validated CLI settings and canonical filesystem paths.
type config struct {
	DB             string
	Inputs         []core.Input
	Path           string
	ProjectAliases map[string][]string
	DayBoundary    string
	// Retained for the existing summary command's in-process API only.
	ExcludeUserMessagePatterns []string
}

// ConfigIOError distinguishes filesystem failures from invalid settings.
type ConfigIOError struct{ Err error }

func (e *ConfigIOError) Error() string { return e.Err.Error() }
func (e *ConfigIOError) Unwrap() error { return e.Err }

type tomlConfig struct {
	DB             string              `toml:"db"`
	DayBoundary    *string             `toml:"dayBoundary"`
	ProjectAliases map[string][]string `toml:"projectAliases"`
	Inputs         []tomlInput         `toml:"inputs"`
}

type tomlInput struct {
	Name   string `toml:"name"`
	Source string `toml:"source"`
	Root   string `toml:"root"`
}

func configLocation(nameOrPath string) (string, error) {
	if nameOrPath == "" {
		return "", fmt.Errorf("missing --config NAME_OR_PATH\n%s", configSetupHint)
	}
	if configNamePattern.MatchString(nameOrPath) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".somniloq", "config", nameOrPath+".toml"), nil
	}
	return absoluteConfigPath(nameOrPath)
}

func absoluteConfigPath(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, path[2:])
		}
	}
	return filepath.Abs(path)
}

func loadConfig(nameOrPath string) (config, error) {
	path, err := configLocation(nameOrPath)
	if err != nil {
		if nameOrPath == "" {
			return config{}, err
		}
		return config{}, &ConfigIOError{Err: err}
	}
	realPath, err := filepath.EvalSymlinks(path)
	if os.IsNotExist(err) {
		return config{}, fmt.Errorf("missing config %s\n%s", path, configSetupHint)
	}
	if err != nil {
		return config{}, &ConfigIOError{Err: fmt.Errorf("resolve config %s: %w", path, err)}
	}
	realPath, err = filepath.Abs(realPath)
	if err != nil {
		return config{}, &ConfigIOError{Err: err}
	}
	data, err := os.ReadFile(realPath)
	if err != nil {
		return config{}, &ConfigIOError{Err: fmt.Errorf("read config %s: %w", path, err)}
	}
	var raw tomlConfig
	if err := toml.NewDecoder(bytes.NewReader(data)).DisallowUnknownFields().Decode(&raw); err != nil {
		return config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	fail := func(err error) (config, error) { return config{}, fmt.Errorf("parse config %s: %w", path, err) }
	if raw.DB == "" {
		return fail(fmt.Errorf("db must not be empty"))
	}
	if len(raw.Inputs) == 0 {
		return fail(fmt.Errorf("inputs must not be empty"))
	}
	c := config{Path: realPath, DayBoundary: "00:00", ProjectAliases: raw.ProjectAliases}
	if raw.DayBoundary != nil {
		if *raw.DayBoundary == "" {
			return fail(fmt.Errorf("invalid dayBoundary %q (use HH:MM)", *raw.DayBoundary))
		}
		c.DayBoundary = *raw.DayBoundary
	}
	if _, err := parseDayBoundary(c.DayBoundary); err != nil {
		return fail(err)
	}
	if err := validateProjectAliases(c.ProjectAliases); err != nil {
		return fail(err)
	}
	c.DB, err = core.CanonicalPath(raw.DB, filepath.Dir(realPath))
	if err != nil {
		return fail(&ConfigIOError{Err: fmt.Errorf("db: %w", err)})
	}
	seen := make(map[string]bool)
	for i, input := range raw.Inputs {
		var source core.Source
		switch input.Source {
		case "claude-code":
			source = core.SourceClaudeCode
		case "codex":
			source = core.SourceCodex
		case "cursor-agent":
			source = core.SourceCursorAgent
		default:
			return fail(fmt.Errorf("inputs[%d]: unknown source %q", i, input.Source))
		}
		if input.Root == "" {
			return fail(fmt.Errorf("inputs[%d]: root must not be empty", i))
		}
		root, err := core.CanonicalPath(input.Root, filepath.Dir(realPath))
		if err != nil {
			return fail(&ConfigIOError{Err: fmt.Errorf("inputs[%d].root: %w", i, err)})
		}
		key := string(source) + "\x00" + root
		if seen[key] {
			continue
		}
		seen[key] = true
		c.Inputs = append(c.Inputs, core.Input{Source: source, Root: root, Name: input.Name})
	}
	return c, nil
}

func validateProjectAliases(groups map[string][]string) error {
	owners := make(map[string]string)
	names := make([]string, 0, len(groups))
	for canonical := range groups {
		names = append(names, canonical)
	}
	slices.Sort(names)
	for _, canonical := range names {
		for _, name := range append([]string{canonical}, groups[canonical]...) {
			if owner, ok := owners[name]; ok && owner != canonical {
				return fmt.Errorf("projectAliases: %q belongs to both %q and %q", name, owner, canonical)
			}
			owners[name] = canonical
		}
	}
	return nil
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
	if len(parts) != 2 || len(parts[0]) != 2 || len(parts[1]) != 2 ||
		parts[0][0] < '0' || parts[0][0] > '9' || parts[0][1] < '0' || parts[0][1] > '9' ||
		parts[1][0] < '0' || parts[1][0] > '9' || parts[1][1] < '0' || parts[1][1] > '9' {
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
