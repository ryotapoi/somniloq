package main

import (
	"path/filepath"
	"slices"
)

// resolveDisplayName returns repoPath as-is, or its basename when short is true.
// Empty repoPath returns empty string (rather than filepath.Base("") = ".").
func resolveDisplayName(repoPath string, short bool) string {
	if repoPath == "" {
		return ""
	}
	if short {
		return filepath.Base(repoPath)
	}
	return repoPath
}

// resolveProjectDisplayName applies configured project aliases before the
// legacy short/raw path display rule. Aliased projects are displayed as the
// canonical name only, so old project names do not leak into CLI output.
func resolveProjectDisplayName(repoPath string, short bool, cfg config) string {
	if canonical, ok := cfg.canonicalProjectName(repoPath); ok {
		return canonical
	}
	return resolveDisplayName(repoPath, short)
}

func (c config) canonicalProjectName(repoPath string) (string, bool) {
	if repoPath == "" {
		return "", false
	}
	base := filepath.Base(repoPath)
	for canonical, oldNames := range c.ProjectAliases {
		if repoPath == canonical || base == canonical || slices.Contains(oldNames, repoPath) || slices.Contains(oldNames, base) {
			return canonical, true
		}
	}
	return "", false
}
