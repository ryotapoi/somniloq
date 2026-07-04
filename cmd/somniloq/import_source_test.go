package main

import (
	"bytes"
	"errors"
	"strings"
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
	_, err := parseImportSource("claude")
	if err == nil {
		t.Fatal("parseImportSource should reject unknown values")
	}
	want := `invalid --source "claude" (want all, claude-code, or codex)`
	if err.Error() != want {
		t.Fatalf("error: got %q, want %q", err.Error(), want)
	}
}

func TestImportHelpUsesCoreSourceChoices(t *testing.T) {
	openDB := func() (*core.DB, error) {
		return nil, errors.New("openDB must not be called for --help")
	}

	var errOut bytes.Buffer
	code, err := importCmd([]string{"--help"}, openDB, "/claude", "/codex", strings.NewReader(""), &bytes.Buffer{}, &errOut, false)
	if err != nil {
		t.Fatalf("importCmd --help failed: %v", err)
	}
	if code != 0 {
		t.Fatalf("importCmd --help code: got %d, want 0", code)
	}

	out := errOut.String()
	for _, want := range []string{
		"somniloq import [--source " + importSourcePipeList() + "] [flags]",
		"source to import: " + importSourceCommaList(),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q; got:\n%s", want, out)
		}
	}
}
