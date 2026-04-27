package main

import (
	"reflect"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
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
		short      bool
		want       string
	}{
		{
			name:       "plain path, short=false",
			projectDir: "-Users-ryota-Sources-Brimday",
			short:      false,
			want:       "-Users-ryota-Sources-Brimday",
		},
		{
			name:       "plain path, short=true",
			projectDir: "-Users-ryota-Sources-Brimday",
			short:      true,
			want:       "Brimday",
		},
		{
			name:       "worktree path, short=false",
			projectDir: "-Users-ryota-Sources-Brimday--claude-worktrees-x",
			short:      false,
			want:       "-Users-ryota-Sources-Brimday",
		},
		{
			name:       "worktree path, short=true",
			projectDir: "-Users-ryota-Sources-Brimday--claude-worktrees-x",
			short:      true,
			want:       "Brimday",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDisplayName(tt.projectDir, tt.short)
			if got != tt.want {
				t.Errorf("resolveDisplayName(%q, %v) = %q, want %q", tt.projectDir, tt.short, got, tt.want)
			}
		})
	}
}

func TestMergeProjects(t *testing.T) {
	tests := []struct {
		name string
		in   []core.ProjectRow
		want []core.ProjectRow
	}{
		{
			name: "root and worktree merged",
			in: []core.ProjectRow{
				{ProjectDir: "-Users-ryota-Sources-Brimday", SessionCount: 5},
				{ProjectDir: "-Users-ryota-Sources-Brimday--claude-worktrees-cheerful-sprouting-globe", SessionCount: 3},
			},
			want: []core.ProjectRow{
				{ProjectDir: "-Users-ryota-Sources-Brimday", SessionCount: 8},
			},
		},
		{
			name: "worktree first then root",
			in: []core.ProjectRow{
				{ProjectDir: "-Users-ryota-Sources-Brimday--claude-worktrees-cheerful-sprouting-globe", SessionCount: 3},
				{ProjectDir: "-Users-ryota-Sources-Brimday", SessionCount: 5},
			},
			want: []core.ProjectRow{
				{ProjectDir: "-Users-ryota-Sources-Brimday", SessionCount: 8},
			},
		},
		{
			name: "no merge needed",
			in: []core.ProjectRow{
				{ProjectDir: "-Users-ryota-Sources-Brimday", SessionCount: 5},
				{ProjectDir: "-Users-ryota-Sources-Other", SessionCount: 3},
			},
			want: []core.ProjectRow{
				{ProjectDir: "-Users-ryota-Sources-Brimday", SessionCount: 5},
				{ProjectDir: "-Users-ryota-Sources-Other", SessionCount: 3},
			},
		},
		{
			name: "empty input",
			in:   []core.ProjectRow{},
			want: []core.ProjectRow{},
		},
		{
			name: "nil input",
			in:   nil,
			want: []core.ProjectRow{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeProjects(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("mergeProjects() = %v, want %v", got, tt.want)
			}
		})
	}
}
