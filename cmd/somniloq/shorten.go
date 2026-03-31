package main

import (
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

const projectWorktreeMarker = "--claude-worktrees-"

// normalizeProjectDir strips the worktree suffix from a project_dir string.
// e.g. "-Users-ryota-Sources-Brimday--claude-worktrees-xyz" → "-Users-ryota-Sources-Brimday"
func normalizeProjectDir(projectDir string) string {
	if idx := strings.Index(projectDir, projectWorktreeMarker); idx > 0 {
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

// mergeProjects normalizes project_dir values and merges rows that share the
// same normalized name. Session counts are summed; the first occurrence's
// position in the slice is preserved.
func mergeProjects(rows []core.ProjectRow) []core.ProjectRow {
	type entry struct {
		row core.ProjectRow
	}
	seen := make(map[string]*entry, len(rows))
	var order []string

	for _, r := range rows {
		key := normalizeProjectDir(r.ProjectDir)
		if e, ok := seen[key]; ok {
			e.row.SessionCount += r.SessionCount
		} else {
			seen[key] = &entry{row: core.ProjectRow{
				ProjectDir:   key,
				SessionCount: r.SessionCount,
			}}
			order = append(order, key)
		}
	}

	result := make([]core.ProjectRow, len(order))
	for i, key := range order {
		result[i] = seen[key].row
	}
	return result
}
