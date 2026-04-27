package main

import (
	"testing"
)

func TestNormalizeProjectDir(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "normal path",
			in:   "-Users-ryota-Sources-Brimday",
			want: "-Users-ryota-Sources-Brimday",
		},
		{
			name: "worktree path",
			in:   "-Users-ryota-Sources-Brimday--claude-worktrees-cheerful-sprouting-globe",
			want: "-Users-ryota-Sources-Brimday",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "marker at end",
			in:   "-Users-ryota-Sources-Brimday--claude-worktrees-",
			want: "-Users-ryota-Sources-Brimday",
		},
		{
			name: "multiple markers",
			in:   "-Users-ryota--claude-worktrees-foo--claude-worktrees-bar",
			want: "-Users-ryota",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeProjectDir(tt.in)
			if got != tt.want {
				t.Errorf("normalizeProjectDir(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShortenProject(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "normal path",
			in:   "-Users-ryota-Sources-Brimday",
			want: "Brimday",
		},
		{
			name: "worktree path",
			in:   "-Users-ryota-Sources-Brimday--claude-worktrees-cheerful-sprouting-globe",
			want: "Brimday",
		},
		{
			name: "hyphenated folder name",
			in:   "-Users-ryota-Sources-202512-phase2",
			want: "phase2",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "dash only",
			in:   "-",
			want: "-",
		},
		{
			name: "no dash",
			in:   "Brimday",
			want: "Brimday",
		},
		{
			name: "marker at end",
			in:   "-Users-ryota-Sources-Brimday--claude-worktrees-",
			want: "Brimday",
		},
		{
			name: "leading dash only",
			in:   "-Brimday",
			want: "Brimday",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenProject(tt.in)
			if got != tt.want {
				t.Errorf("shortenProject(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestResolveDisplayName(t *testing.T) {
	tests := []struct {
		name       string
		projectDir string
		repoPath   string
		short      bool
		want       string
	}{
		{
			name:       "repo_path present, short=false",
			projectDir: "-Users-ryota-Sources-Brimday",
			repoPath:   "/Users/ryota/Sources/Brimday",
			short:      false,
			want:       "/Users/ryota/Sources/Brimday",
		},
		{
			name:       "repo_path present, short=true",
			projectDir: "-Users-ryota-Sources-Brimday",
			repoPath:   "/Users/ryota/Sources/Brimday",
			short:      true,
			want:       "Brimday",
		},
		{
			name:       "repo_path with hyphen, short=true preserves hyphen",
			projectDir: "-Users-ryota-Sources-my-repo",
			repoPath:   "/Users/ryota/Sources/my-repo",
			short:      true,
			want:       "my-repo",
		},
		{
			name:       "repo_path empty, plain path, short=false",
			projectDir: "-Users-ryota-Sources-Brimday",
			repoPath:   "",
			short:      false,
			want:       "-Users-ryota-Sources-Brimday",
		},
		{
			name:       "repo_path empty, plain path, short=true",
			projectDir: "-Users-ryota-Sources-Brimday",
			repoPath:   "",
			short:      true,
			want:       "Brimday",
		},
		{
			name:       "repo_path empty, worktree path, short=false",
			projectDir: "-Users-ryota-Sources-Brimday--claude-worktrees-x",
			repoPath:   "",
			short:      false,
			want:       "-Users-ryota-Sources-Brimday",
		},
		{
			name:       "repo_path empty, worktree path, short=true",
			projectDir: "-Users-ryota-Sources-Brimday--claude-worktrees-x",
			repoPath:   "",
			short:      true,
			want:       "Brimday",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDisplayName(tt.projectDir, tt.repoPath, tt.short)
			if got != tt.want {
				t.Errorf("resolveDisplayName(%q, %q, %v) = %q, want %q", tt.projectDir, tt.repoPath, tt.short, got, tt.want)
			}
		})
	}
}
