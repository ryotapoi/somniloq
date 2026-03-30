package main

import (
	"path/filepath"
	"strings"
)

const worktreeMarker = "/.claude/worktrees/"

// shortenProject returns a short project name from the cwd path.
// If cwd is empty, it falls back to projectDir unchanged.
func shortenProject(cwd, projectDir string) string {
	if cwd == "" {
		return projectDir
	}
	if idx := strings.LastIndex(cwd, worktreeMarker); idx > 0 {
		return filepath.Base(cwd[:idx])
	}
	return filepath.Base(cwd)
}
