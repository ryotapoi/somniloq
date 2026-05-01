package main

import "path/filepath"

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
