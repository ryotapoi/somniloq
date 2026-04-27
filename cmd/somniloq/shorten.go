package main

import (
	"path/filepath"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

// normalizeProjectDir strips the worktree suffix from a project_dir string.
// e.g. "-Users-ryota-Sources-Brimday--claude-worktrees-xyz" → "-Users-ryota-Sources-Brimday"
func normalizeProjectDir(projectDir string) string {
	if idx := strings.Index(projectDir, core.ProjectDirWorktreeMarker); idx > 0 {
		return projectDir[:idx]
	}
	return projectDir
}

// shortenProject returns the last hyphen-separated element of a project_dir string.
// Applies normalizeProjectDir first to strip any worktree suffix.
// e.g. "-Users-ryota-Sources-Brimday" → "Brimday"
func shortenProject(projectDir string) string {
	normalized := normalizeProjectDir(projectDir)
	if normalized == "" || normalized == "-" {
		return normalized
	}
	if idx := strings.LastIndex(normalized, "-"); idx >= 0 {
		return normalized[idx+1:]
	}
	return normalized
}

// resolveDisplayName prefers repoPath; falls back to project_dir-based normalization.
func resolveDisplayName(projectDir, repoPath string, short bool) string {
	if repoPath != "" {
		if short {
			return filepath.Base(repoPath)
		}
		return repoPath
	}
	if short {
		return shortenProject(projectDir)
	}
	return normalizeProjectDir(projectDir)
}
