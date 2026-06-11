package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func insertOutlineMessage(t *testing.T, db *core.DB, sessionID, uuid, role, content, timestamp string, sidechain bool) {
	t.Helper()
	if err := db.InsertMessage(core.NormalizedMessage{
		UUID:        uuid,
		Source:      core.SourceClaudeCode,
		SessionID:   sessionID,
		Role:        role,
		Content:     content,
		Timestamp:   timestamp,
		IsSidechain: sidechain,
	}); err != nil {
		t.Fatalf("InsertMessage(%s): %v", uuid, err)
	}
}

func newOutlineTestDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "sess-1",
		CWD:       "/Users/test/proj",
		StartedAt: "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	insertOutlineMessage(t, db, "sess-1", "u1", "user", "first question\nwith detail", "2026-03-28T15:00:00Z", false)
	insertOutlineMessage(t, db, "sess-1", "a1", "assistant", "answer one", "2026-03-28T15:01:00Z", false)
	insertOutlineMessage(t, db, "sess-1", "s1", "user", "sidechain prompt", "2026-03-28T15:02:00Z", true)
	insertOutlineMessage(t, db, "sess-1", "u2", "user", "\n\nsecond\tquestion after blank lines", "2026-03-28T15:03:00Z", false)
	return db
}

func TestOutlineCmd_ListsUserTurns(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := outlineCmd([]string{"sess-1"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("outlineCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	want := fmt.Sprintf("1\t%s\tfirst question\n2\t%s\tsecond question after blank lines\n",
		formatLocalTime("2026-03-28T15:00:00Z", time.Local),
		formatLocalTime("2026-03-28T15:03:00Z", time.Local))
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestOutlineCmd_SessionNotFound(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := outlineCmd([]string{"no-such"}, staticDB(db), &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "session not found") {
		t.Errorf("err = %v, want session not found", err)
	}
}

func TestOutlineCmd_MissingSessionIDPrintsUsage(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := outlineCmd(nil, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("outlineCmd: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: "+outlineUsageLine) {
		t.Errorf("stderr = %q, want usage line", errOut.String())
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"single", "single"},
		{"first\nsecond", "first"},
		{"\n\n  lead blank\nrest", "lead blank"},
		{"crlf line\r\nnext", "crlf line"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := firstLine(tt.in); got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
