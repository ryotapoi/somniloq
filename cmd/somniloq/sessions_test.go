package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

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
	if len(fields) != 9 {
		t.Fatalf("fields = %d, want 9 (SessionID, TimeRange, LogicalDay, Project, Title, MessageCount, BodySize, NonCommandUserTurnCount, FirstNonCommandUserLine): %q", len(fields), line)
	}
	if want := sessionLogicalDay(rows[0], dayBoundary{}, time.Local); fields[2] != want {
		t.Errorf("LogicalDay column = %s, want %s", fields[2], want)
	}
	if want := strconv.Itoa(rows[0].MessageCount); fields[5] != want {
		t.Errorf("MessageCount column = %s, want %s", fields[5], want)
	}
	if want := strconv.Itoa(rows[0].BodySize); fields[6] != want {
		t.Errorf("BodySize column = %s, want %s", fields[6], want)
	}
	if fields[7] != "2" {
		t.Errorf("NonCommandUserTurnCount column = %s, want 2", fields[7])
	}
	if fields[8] != "first question" {
		t.Errorf("FirstNonCommandUserLine column = %q, want first question", fields[8])
	}
	if rows[0].BodySize == 0 {
		t.Error("fixture BodySize should be non-zero")
	}
}

func TestSessionsCmd_DayBoundaryFiltersDateOnlySinceAndDisplaysLogicalDay(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	defer func() { time.Local = oldLocal }()

	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	sessions := []core.SessionMeta{
		{Source: core.SourceClaudeCode, SessionID: "before", RepoPath: "/Users/test/proj", StartedAt: "2026-03-28T18:59:00Z", EndedAt: "2026-03-28T18:59:30Z"},
		{Source: core.SourceClaudeCode, SessionID: "at-boundary", RepoPath: "/Users/test/proj", StartedAt: "2026-03-28T19:00:00Z", EndedAt: "2026-03-28T19:01:00Z"},
	}
	for _, session := range sessions {
		if err := db.UpsertSession(session, "2026-03-29T00:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", session.SessionID, err)
		}
		if err := db.InsertMessage(core.NormalizedMessage{
			Source:    session.Source,
			UUID:      session.SessionID + "-m1",
			SessionID: session.SessionID,
			Role:      "user",
			Content:   "hello",
			Timestamp: session.StartedAt,
		}); err != nil {
			t.Fatalf("InsertMessage(%s): %v", session.SessionID, err)
		}
	}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd([]string{"--since", "2026-03-29", "--day-boundary", "04:00"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	line := strings.TrimSpace(out.String())
	if strings.Contains(line, "before") {
		t.Fatalf("date-only --since should exclude the pre-boundary session:\n%s", line)
	}
	fields := strings.Split(line, "\t")
	if fields[0] != "at-boundary" {
		t.Fatalf("session column = %q, want at-boundary (line %q)", fields[0], line)
	}
	if fields[2] != "2026-03-29" {
		t.Errorf("LogicalDay column = %s, want 2026-03-29", fields[2])
	}
}

func TestSessionsCmd_TimeFilterBoundaryWithSecondsPrecisionStartedAt(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()

	newDBWithBoundarySession := func(t *testing.T) *core.DB {
		t.Helper()
		db, err := core.OpenDB(":memory:")
		if err != nil {
			t.Fatalf("OpenDB: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		if err := db.UpsertSession(core.SessionMeta{
			Source:    core.SourceClaudeCode,
			SessionID: "at-boundary",
			RepoPath:  "/Users/test/proj",
			StartedAt: "2026-03-28T10:00:00Z",
		}, "2026-03-28T12:00:00Z"); err != nil {
			t.Fatalf("UpsertSession: %v", err)
		}
		return db
	}

	t.Run("since includes equal boundary", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code, err := sessionsCmd([]string{"--since", "2026-03-28T10:00"}, staticDB(newDBWithBoundarySession(t)), config{}, &out, &errOut)
		if err != nil {
			t.Fatalf("sessionsCmd: %v", err)
		}
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
		}
		if !strings.HasPrefix(out.String(), "at-boundary\t") {
			t.Fatalf("equal seconds-precision started_at must match --since boundary:\n%s", out.String())
		}
	})

	t.Run("until excludes equal boundary", func(t *testing.T) {
		var out, errOut bytes.Buffer
		code, err := sessionsCmd([]string{"--until", "2026-03-28T10:00"}, staticDB(newDBWithBoundarySession(t)), config{}, &out, &errOut)
		if err != nil {
			t.Fatalf("sessionsCmd: %v", err)
		}
		if code != 0 {
			t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
		}
		if out.Len() != 0 {
			t.Fatalf("equal seconds-precision started_at must not match exclusive --until boundary:\n%s", out.String())
		}
	})
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
	if len(fields) != 9 {
		t.Fatalf("fields = %d, want 9: %q", len(fields), line)
	}
	if fields[7] != "2" {
		t.Errorf("NonCommandUserTurnCount column = %s, want 2", fields[7])
	}
	if fields[8] != "real work request" {
		t.Errorf("FirstNonCommandUserLine column = %q, want sanitized first non-command line", fields[8])
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

func TestSessionsCmd_InvalidDayBoundaryFailsBeforeOpeningDB(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid day boundary")
		return nil, nil
	}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd([]string{"--day-boundary", "99:00"}, openDB, config{}, &out, &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "invalid dayBoundary") {
		t.Errorf("err = %v, want invalid dayBoundary", err)
	}
}
