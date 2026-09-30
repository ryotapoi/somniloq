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
		{"cursor-agent", core.ImportSourceCursorAgent},
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
	_, err := parseImportSource("claude")
	if err == nil {
		t.Fatal("parseImportSource should reject unknown values")
	}
	want := `invalid --source "claude" (want all, claude-code, codex, or cursor-agent)`
	if err.Error() != want {
		t.Fatalf("error: got %q, want %q", err.Error(), want)
	}
}
