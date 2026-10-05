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
