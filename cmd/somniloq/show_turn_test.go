package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestShowCmd_TurnRange(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--turn", "2", "sess-1"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "second\tquestion after blank lines") {
		t.Errorf("output should contain turn 2 message, got %q", out.String())
	}
	if strings.Contains(out.String(), "first question") {
		t.Errorf("output should not contain turn 1 message, got %q", out.String())
	}
}

func TestShowCmd_Tail(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--tail", "1", "sess-1"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "second\tquestion after blank lines") {
		t.Errorf("output should contain the last turn, got %q", out.String())
	}
	if strings.Contains(out.String(), "answer one") {
		t.Errorf("output should not contain turn 1 reply, got %q", out.String())
	}
}

func TestShowCmd_TurnRangeKeepsAssistantReplies(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--turn", "1", "sess-1"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "first question") || !strings.Contains(out.String(), "answer one") {
		t.Errorf("turn 1 should include the user message and its reply, got %q", out.String())
	}
	if strings.Contains(out.String(), "sidechain prompt") {
		t.Errorf("sidechain message must stay excluded, got %q", out.String())
	}
}

func TestShowCmd_TurnFilterAppliesPerSessionInBulkMode(t *testing.T) {
	db := newOutlineTestDB(t)
	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "sess-2",
		CWD:       "/Users/test/other",
		StartedAt: "2026-03-29T09:00:00Z",
	}, "2026-03-29T09:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	insertOutlineMessage(t, db, "sess-2", "o1", "user", "only question", "2026-03-29T09:00:00Z", false)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--since", "2020-01-01", "--tail", "1"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	for _, want := range []string{"second\tquestion after blank lines", "only question"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output should contain each session's last turn (%q), got %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "first question") {
		t.Errorf("output should not contain sess-1 turn 1, got %q", out.String())
	}
}

func TestShowCmd_TurnFlagErrors(t *testing.T) {
	// Flag validation must reject these before the DB is ever opened.
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid flag combinations")
		return nil, nil
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"turn with tail", []string{"--turn", "1", "--tail", "2", "sess-1"}, "either --turn or --tail"},
		{"turn with summary", []string{"--turn", "1", "--summary", "1", "sess-1"}, "cannot be combined with --summary"},
		{"tail with summary", []string{"--tail", "1", "--summary", "1", "sess-1"}, "cannot be combined with --summary"},
		{"bad range", []string{"--turn", "5..3", "sess-1"}, "--turn"},
		{"empty turn", []string{"--turn", "", "sess-1"}, "--turn must be N or N..M"},
		{"negative tail", []string{"--tail", "-1", "sess-1"}, "--tail must be >= 0"},
	}
	for _, tt := range tests {
		var out, errOut bytes.Buffer
		code, err := showCmd(tt.args, openDB, &out, &errOut)
		if code != 1 {
			t.Errorf("%s: exit code = %d, want 1", tt.name, code)
		}
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want one containing %q", tt.name, err, tt.want)
		}
	}
}
