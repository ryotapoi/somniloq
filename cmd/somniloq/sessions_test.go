package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSessionsCmd_OutputColumns(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()
	db := newOutlineTestDB(t)
	if err := db.UpsertSession(core.SessionMeta{Source: core.SourceClaudeCode, SessionID: "sess-1", EndedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.UpdateSessionTitle(core.SourceClaudeCode, "sess-1", "Title\twith\nline", "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}
	insertOutlineMessage(t, db, "sess-1", "raw-first", "user", "first\tline\nmore", "2026-03-28T14:59:00Z", false)

	var out, errOut bytes.Buffer
	code, err := sessionsCmd(nil, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	const want = "sess-1\t2026-03-28 15:00 ~ 2026-03-28 16:00\t2026-03-28\t/Users/test/proj\tTitle with line\t5\t86\tclaude_code\n"
	if got := out.String(); got != want {
		t.Errorf("TSV = %q, want %q", got, want)
	}
}

func TestSessionsCmd_ReturnsTSVWriteError(t *testing.T) {
	db := newOutlineTestDB(t)

	code, err := sessionsCmd(nil, staticDB(db), config{}, failWriter{}, &bytes.Buffer{})
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err != errFailWriter {
		t.Errorf("error = %v, want %v", err, errFailWriter)
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

func TestSessionsCmd_ImportedSinceFiltersUnknownStartedAt(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()

	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSession(core.SessionMeta{Source: core.SourceCursorAgent, SessionID: "unknown-start"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd([]string{"--imported-since", "2026-03-28T15:00"}, staticDB(db), config{}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("sessionsCmd = %d, %v (stderr: %q)", code, err, errOut.String())
	}
	if !strings.HasPrefix(out.String(), "unknown-start\t") || !strings.HasSuffix(out.String(), "\tcursor_agent\n") {
		t.Fatalf("output = %q, want source/session pair for the unknown-started session", out.String())
	}
}

func TestSessionsCmdAt_RelativeFiltersShareSubsecondNow(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()

	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, session := range []struct {
		id, startedAt, importedAt string
	}{
		{"included", "2026-03-29T11:59:00.600Z", "2026-03-29T12:00:01Z"},
		{"before-since", "2026-03-29T11:59:00.499Z", "2026-03-29T12:00:01Z"},
		{"at-until", "2026-03-29T12:00:00.500Z", "2026-03-29T12:00:01Z"},
		{"before-imported-since", "2026-03-29T11:59:00.600Z", "2026-03-29T12:00:00Z"},
	} {
		if err := db.UpsertSession(core.SessionMeta{
			Source:    core.SourceClaudeCode,
			SessionID: session.id,
			StartedAt: session.startedAt,
		}, session.importedAt); err != nil {
			t.Fatalf("UpsertSession(%s): %v", session.id, err)
		}
	}

	var out, errOut bytes.Buffer
	now := time.Date(2026, 3, 29, 12, 0, 0, 500_000_000, time.UTC)
	code, err := sessionsCmdAt(now, []string{"--since", "1m", "--until", "0m", "--imported-since", "0m"}, staticDB(db), config{}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("sessionsCmdAt = %d, %v (stderr: %q)", code, err, errOut.String())
	}
	if got, want := out.String(), "included\t2026-03-29 11:59 ~\t2026-03-29\t\t\t0\t0\tclaude_code\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func newSessionUserMessageExclusionDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceCodex,
		SessionID: "exclude-all",
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
			SessionID:   "exclude-all",
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

func TestSessionsCmd_DoesNotFilterRowsByUserMessageExclusions(t *testing.T) {
	db := newSessionUserMessageExclusionDB(t)
	cfg := config{ExcludeUserMessagePatterns: []string{`.*`}}

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
	if fields[0] != "exclude-all" || fields[7] != "codex" {
		t.Errorf("session columns = %v, want included row with source in final column", fields)
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

func TestSessionsCmd_InvalidImportedSinceFailsBeforeOpeningDB(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid imported-since")
		return nil, nil
	}

	for _, args := range [][]string{{"--imported-since", ""}, {"--imported-since", "not-a-time"}} {
		var out, errOut bytes.Buffer
		code, err := sessionsCmd(args, openDB, config{}, &out, &errOut)
		if code != 1 || err == nil || out.Len() != 0 {
			t.Errorf("sessionsCmd(%v) = (%d, %v, stdout %q), want validation error before DB open", args, code, err, out.String())
		}
	}
}

func TestSessionsCmd_RejectsUnexpectedArgumentsBeforeOpeningDB(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"positional", []string{"unexpected"}},
		{"positional before filter", []string{"unexpected", "--project", "x"}},
		{"after separator", []string{"--", "unexpected"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open := func() (*core.DB, error) {
				t.Fatal("DB must not be opened for unexpected arguments")
				return nil, nil
			}
			var out, errOut bytes.Buffer
			code, err := sessionsCmd(tt.args, open, config{}, &out, &errOut)
			if code != 1 || err != nil {
				t.Fatalf("sessionsCmd = (%d, %v), want (1, nil)", code, err)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want empty", out.String())
			}
			if !strings.Contains(errOut.String(), "unexpected arguments") || !strings.Contains(errOut.String(), "usage: somniloq sessions") {
				t.Errorf("stderr = %q, want argument diagnostic and usage", errOut.String())
			}
		})
	}
}

func staticDB(db *core.DB) func() (*core.DB, error) {
	return func() (*core.DB, error) { return db, nil }
}
