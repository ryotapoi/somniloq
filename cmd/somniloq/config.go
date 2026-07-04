package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
)

// config is the optional CLI configuration loaded from
// ~/.somniloq/config.json (overridable with the global --config flag).
type config struct {
	// ProjectAliases groups project names that refer to the same project
	// over time (e.g. a renamed repository): canonical name -> old names.
	ProjectAliases map[string][]string `json:"projectAliases"`
	// CommandPatterns marks user turns that should be treated as commands
	// when deriving sessions skip-hint columns.
	CommandPatterns []string `json:"commandPatterns"`
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
	if _, err := compileCommandPatterns(c.CommandPatterns); err != nil {
		return config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	return c, nil
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

type commandMatcher struct {
	patterns []*regexp.Regexp
}

func newCommandMatcher(cfg config) (commandMatcher, error) {
	patterns, err := compileCommandPatterns(cfg.CommandPatterns)
	if err != nil {
		return commandMatcher{}, err
	}
	return commandMatcher{patterns: patterns}, nil
}

func compileCommandPatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid commandPatterns pattern %q: %w", pattern, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

func (m commandMatcher) isCommand(content string) bool {
	text := strings.TrimSpace(content)
	if strings.HasPrefix(text, "/") {
		return true
	}
	for _, pattern := range m.patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}
