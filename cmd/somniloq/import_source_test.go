package main

import (
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestParseImportSource(t *testing.T) {
	for _, c := range []struct {
		value string
		want  core.ImportSource
	}{
		{"all", core.ImportSourceAll},
		{"claude-code", core.ImportSourceClaudeCode},
		{"codex", core.ImportSourceCodex},
	} {
		got, err := parseImportSource(c.value)
		if err != nil {
			t.Fatalf("parseImportSource(%q) failed: %v", c.value, err)
		}
		if got != c.want {
			t.Errorf("parseImportSource(%q): got %q, want %q", c.value, got, c.want)
		}
	}
}

func TestParseImportSourceRejectsUnknownValue(t *testing.T) {
	if _, err := parseImportSource("claude"); err == nil {
		t.Fatal("parseImportSource should reject unknown values")
	}
}
