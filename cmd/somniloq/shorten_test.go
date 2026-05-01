package main

import (
	"testing"
)

func TestResolveDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		repoPath string
		short    bool
		want     string
	}{
		{
			name:     "repo_path present, short=false",
			repoPath: "/Users/ryota/Sources/Brimday",
			short:    false,
			want:     "/Users/ryota/Sources/Brimday",
		},
		{
			name:     "repo_path present, short=true",
			repoPath: "/Users/ryota/Sources/Brimday",
			short:    true,
			want:     "Brimday",
		},
		{
			name:     "basename keeps hyphen",
			repoPath: "/Users/ryota/Sources/my-repo",
			short:    true,
			want:     "my-repo",
		},
		{
			name:     "empty repo_path, short=false",
			repoPath: "",
			short:    false,
			want:     "",
		},
		{
			name:     "empty repo_path, short=true",
			repoPath: "",
			short:    true,
			want:     "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveDisplayName(tt.repoPath, tt.short)
			if got != tt.want {
				t.Errorf("resolveDisplayName(%q, %v) = %q, want %q", tt.repoPath, tt.short, got, tt.want)
			}
		})
	}
}
