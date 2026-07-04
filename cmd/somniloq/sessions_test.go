package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSessionsCmd_OutputColumns(t *testing.T) {
	db := newOutlineTestDB(t)

	// The CLI must print exactly what ListSessions returns; the value
	// semantics (bytes, sidechain exclusion) are pinned by core tests.
	// Read before sessionsCmd, which closes the DB on exit.
	rows, err := db.ListSessions(core.SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows))
	}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd(nil, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	line := strings.TrimSuffix(out.String(), "\n")
	fields := strings.Split(line, "\t")
	if len(fields) != 8 {
		t.Fatalf("fields = %d, want 8 (SessionID, TimeRange, Project, Title, MessageCount, BodySize, NonCommandUserTurnCount, FirstNonCommandUserLine): %q", len(fields), line)
	}
	if want := strconv.Itoa(rows[0].MessageCount); fields[4] != want {
		t.Errorf("MessageCount column = %s, want %s", fields[4], want)
	}
	if want := strconv.Itoa(rows[0].BodySize); fields[5] != want {
		t.Errorf("BodySize column = %s, want %s", fields[5], want)
	}
	if fields[6] != "2" {
		t.Errorf("NonCommandUserTurnCount column = %s, want 2", fields[6])
	}
	if fields[7] != "first question" {
		t.Errorf("FirstNonCommandUserLine column = %q, want first question", fields[7])
	}
	if rows[0].BodySize == 0 {
		t.Error("fixture BodySize should be non-zero")
	}
}

func newSessionSkipHintsDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceCodex,
		SessionID: "skip-hints",
		CWD:       "/Users/test/proj",
		RepoPath:  "/Users/test/proj",
		StartedAt: "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	messages := []struct {
		uuid      string
		role      string
		content   string
		timestamp string
		sidechain bool
	}{
		{"u1", "user", "/briefing daily", "2026-03-28T15:00:00Z", false},
		{"a1", "assistant", "briefed", "2026-03-28T15:01:00Z", false},
		{"u2", "user", "日報生成\nfor today", "2026-03-28T15:02:00Z", false},
		{"u-side", "user", "sidechain real text", "2026-03-28T15:03:00Z", true},
		{"u3", "user", "\n\nreal work\trequest\nwith detail", "2026-03-28T15:04:00Z", false},
		{"u4", "user", "follow up", "2026-03-28T15:05:00Z", false},
	}
	for _, m := range messages {
		if err := db.InsertMessage(core.NormalizedMessage{
			Source:      core.SourceCodex,
			UUID:        m.uuid,
			SessionID:   "skip-hints",
			Role:        m.role,
			Content:     m.content,
			Timestamp:   m.timestamp,
			IsSidechain: m.sidechain,
		}); err != nil {
			t.Fatalf("InsertMessage(%s): %v", m.uuid, err)
		}
	}
	return db
}

func TestSessionsCmd_SkipHintColumnsExcludeCommands(t *testing.T) {
	db := newSessionSkipHintsDB(t)
	cfg := config{CommandPatterns: []string{`^日報生成`}}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd(nil, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	line := strings.TrimSuffix(out.String(), "\n")
	fields := strings.Split(line, "\t")
	if len(fields) != 8 {
		t.Fatalf("fields = %d, want 8: %q", len(fields), line)
	}
	if fields[6] != "2" {
		t.Errorf("NonCommandUserTurnCount column = %s, want 2", fields[6])
	}
	if fields[7] != "real work request" {
		t.Errorf("FirstNonCommandUserLine column = %q, want sanitized first non-command line", fields[7])
	}
}

func TestSessionsCmd_InvalidCommandPatternFailsBeforeOpeningDB(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid commandPatterns")
		return nil, nil
	}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd(nil, openDB, config{CommandPatterns: []string{"["}}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "invalid commandPatterns pattern") {
		t.Errorf("err = %v, want invalid commandPatterns pattern", err)
	}
}
