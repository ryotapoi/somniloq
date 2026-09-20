package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestParseSessionSource(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  core.Source
	}{
		{"claude_code", core.SourceClaudeCode},
		{"claude-code", core.SourceClaudeCode},
		{"codex", core.SourceCodex},
		{"cursor_agent", core.SourceCursorAgent},
		{"cursor-agent", core.SourceCursorAgent},
	} {
		got, err := parseSessionSource(tt.value)
		if err != nil || got != tt.want {
			t.Errorf("parseSessionSource(%q) = %q, %v; want %q, nil", tt.value, got, err, tt.want)
		}
	}
}

func TestSessionSourceValidationDoesNotOpenDB(t *testing.T) {
	for _, tt := range []struct {
		name string
		run  func(func() (*core.DB, error)) (int, error)
	}{
		{
			name: "show empty source",
			run: func(openDB func() (*core.DB, error)) (int, error) {
				return showCmd([]string{"--source", "", "same-id"}, openDB, config{}, &bytes.Buffer{}, &bytes.Buffer{})
			},
		},
		{
			name: "outline all source",
			run: func(openDB func() (*core.DB, error)) (int, error) {
				return outlineCmd([]string{"--source", "all", "same-id"}, openDB, &bytes.Buffer{}, &bytes.Buffer{})
			},
		},
		{
			name: "show unknown source",
			run: func(openDB func() (*core.DB, error)) (int, error) {
				return showCmd([]string{"--source", "unknown", "same-id"}, openDB, config{}, &bytes.Buffer{}, &bytes.Buffer{})
			},
		},
		{
			name: "show source with time range",
			run: func(openDB func() (*core.DB, error)) (int, error) {
				return showCmd([]string{"--source", "codex", "--since", "24h"}, openDB, config{}, &bytes.Buffer{}, &bytes.Buffer{})
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			code, err := tt.run(func() (*core.DB, error) {
				opened = true
				return nil, nil
			})
			if code != 1 || err == nil {
				t.Fatalf("command = %d, %v; want 1, validation error", code, err)
			}
			if opened {
				t.Fatal("openDB was called")
			}
		})
	}
}

func TestSessionSourceSelectsOnlyRequestedSource(t *testing.T) {
	db := newSourceSelectionTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--source", "codex", "--turn", "1", "--format", "json", "same-id"}, staticDB(db), config{}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("show = %d, %v (stderr %q)", code, err, errOut.String())
	}
	if !strings.Contains(out.String(), `"source": "codex"`) || !strings.Contains(out.String(), "codex question") || strings.Contains(out.String(), "claude question") {
		t.Fatalf("show output = %q", out.String())
	}

	db = newSourceSelectionTestDB(t)
	out.Reset()
	errOut.Reset()
	code, err = outlineCmd([]string{"--source", "claude-code", "same-id"}, staticDB(db), &out, &errOut)
	if err != nil || code != 0 || !strings.Contains(out.String(), "claude question") || strings.Contains(out.String(), "codex question") {
		t.Fatalf("outline = %d, %v, %q (stderr %q)", code, err, out.String(), errOut.String())
	}
}

func newSourceSelectionTestDB(t *testing.T) *core.DB {
	t.Helper()
	db := newCrossSourceSessionTestDB(t)
	for _, message := range []core.NormalizedMessage{
		{Source: core.SourceClaudeCode, UUID: "claude-user", SessionID: "same-id", Role: "user", Content: "claude question", Timestamp: "2026-03-28T15:00:00Z"},
		{Source: core.SourceCodex, UUID: "codex-user", SessionID: "same-id", Role: "user", Content: "codex question", Timestamp: "2026-03-28T15:00:00Z"},
		{Source: core.SourceCodex, UUID: "codex-answer", SessionID: "same-id", Role: "assistant", Content: "codex answer", Timestamp: "2026-03-28T15:01:00Z"},
	} {
		if err := db.InsertMessage(message); err != nil {
			t.Fatalf("InsertMessage(%s): %v", message.UUID, err)
		}
	}
	return db
}

func TestSessionSourceDoesNotFallBackToAnotherSource(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--source", "cursor_agent", "same-id"}, staticDB(db), config{}, &out, &errOut)
	if code != 1 || err == nil || err.Error() != "session not found: same-id" || out.Len() != 0 {
		t.Fatalf("show = %d, %v, stdout %q", code, err, out.String())
	}
}
