package main

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

// config is the optional CLI configuration loaded from
// ~/.somniloq/config.json (overridable with the global --config flag).
type config struct {
	// ProjectAliases groups project names that refer to the same project
	// over time (e.g. a renamed repository): canonical name -> old names.
	ProjectAliases map[string][]string `json:"projectAliases"`
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
