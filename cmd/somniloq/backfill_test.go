package main

import (
	"bytes"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

// insertOrphanSession registers a session with no messages (repo_path NULL,
// cwd populated). Backfill should DELETE this row.
func insertOrphanSession(t *testing.T, db *core.DB, sessionID, cwd string) {
	t.Helper()
	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: sessionID,
		CWD:       cwd,
		StartedAt: "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession(%s): %v", sessionID, err)
	}
}

// insertSessionWithMessage registers a session with one message attached.
func insertSessionWithMessage(t *testing.T, db *core.DB, sessionID, cwd, uuid string) {
	t.Helper()
	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: sessionID,
		CWD:       cwd,
		StartedAt: "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession(%s): %v", sessionID, err)
	}
	if err := db.InsertMessage(core.ParsedMessage{
		UUID:      uuid,
		Source:    core.SourceClaudeCode,
		SessionID: sessionID,
		Role:      "user",
		Content:   "{}",
		Timestamp: "2026-03-28T15:00:00Z",
	}); err != nil {
		t.Fatalf("InsertMessage(%s): %v", uuid, err)
	}
}

// staticDB returns an opener that hands back the same *core.DB every call.
// backfillCmd Closes the DB it opens. A second Close from the test's defer
// is harmless because tests do not query the DB after backfillCmd returns.
func staticDB(db *core.DB) func() (*core.DB, error) {
	return func() (*core.DB, error) { return db, nil }
}

func TestBackfillCmd_NonInteractiveOrphanRequiresYes(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, staticDB(db), strings.NewReader(""), &out, &errOut, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "backfill requires confirmation when deleting sessions") {
		t.Errorf("err = %v, want one mentioning 'backfill requires confirmation when deleting sessions'", err)
	}
	if strings.Contains(out.String(), "Backfilled") {
		t.Errorf("stdout must not contain 'Backfilled' on non-interactive failure, got %q", out.String())
	}
}

func TestBackfillCmd_NonInteractiveYesSucceeds(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"--yes"}, staticDB(db), strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "deleted=1") {
		t.Errorf("stdout = %q, want it to contain 'deleted=1'", out.String())
	}
}

func TestBackfillCmd_InteractiveYesSkipsPrompt(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"--yes"}, staticDB(db), strings.NewReader(""), &out, &errOut, true)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Errorf("--yes must skip the confirmation prompt, but stderr = %q", errOut.String())
	}
}

func TestBackfillCmd_InteractiveDeclineDoesNothing(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, staticDB(db), strings.NewReader("n\n"), &out, &errOut, true)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(out.String(), "Backfilled") {
		t.Errorf("stdout must not contain 'Backfilled' after decline, got %q", out.String())
	}
}

func TestBackfillCmd_NoOrphanRunsResolveOnly(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	// Session has a message attached, so it is not an orphan. cwd is set,
	// repo_path stays NULL until backfill resolves it.
	insertSessionWithMessage(t, db, "kept", "/Users/test/proj/.claude/worktrees/feature", "m1")

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, staticDB(db), strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Errorf("must not prompt when there are no orphans, stderr = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "resolved=1") {
		t.Errorf("stdout = %q, want it to contain 'resolved=1'", out.String())
	}
}

// TestBackfillCmd_HelpDoesNotOpenDB ensures that --help short-circuits before
// the openDB callback runs. Otherwise `somniloq backfill -h` would create the
// DB directory / migrate the schema just to print usage.
func TestBackfillCmd_HelpDoesNotOpenDB(t *testing.T) {
	called := false
	open := func() (*core.DB, error) {
		called = true
		return nil, errors.New("openDB must not be called for --help")
	}
	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"-h"}, open, strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if called {
		t.Errorf("openDB was invoked for --help; flag parsing must short-circuit first")
	}
	if !strings.Contains(errOut.String(), "Correct legacy session data") {
		t.Errorf("usage missing from stderr: %q", errOut.String())
	}
}

// TestBackfillCmd_UnexpectedArgsDoesNotOpenDB makes sure positional-arg
// validation also short-circuits before openDB.
func TestBackfillCmd_UnexpectedArgsDoesNotOpenDB(t *testing.T) {
	called := false
	open := func() (*core.DB, error) {
		called = true
		return nil, errors.New("openDB must not be called when args are invalid")
	}
	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"extra"}, open, strings.NewReader(""), &out, &errOut, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Errorf("err = %v, want 'unexpected arguments'", err)
	}
	if called {
		t.Errorf("openDB was invoked for invalid args; validation must short-circuit first")
	}
}

// cmdV03Schema mirrors the pre-v0.4 schema so cmd-layer tests can exercise
// migration without depending on internal/core test helpers.
const cmdV03Schema = `
CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY,
    cwd TEXT,
    repo_path TEXT,
    git_branch TEXT,
    custom_title TEXT,
    agent_name TEXT,
    version TEXT,
    started_at TEXT,
    ended_at TEXT,
    imported_at TEXT NOT NULL
);
CREATE TABLE messages (
    uuid TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    parent_uuid TEXT,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    is_sidechain BOOLEAN DEFAULT FALSE,
    UNIQUE(uuid)
);
CREATE TABLE import_state (
    jsonl_path TEXT PRIMARY KEY,
    file_size INTEGER,
    last_offset INTEGER,
    imported_at TEXT NOT NULL
);
`

// setupV03DBFile creates a v0.3-shaped sqlite file at a temp path and returns
// the path. Used by cmd-layer tests that need to exercise the migration
// preflight in backfillCmd. Going through a file (not :memory:) lets us
// hand the path to backfillCmd's openDB factory like the real CLI does.
func setupV03DBFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dsn := dir + "/v03.db"

	rawDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	t.Cleanup(func() { rawDB.Close() })
	if _, err := rawDB.Exec(cmdV03Schema); err != nil {
		t.Fatalf("cmdV03Schema exec: %v", err)
	}
	return dsn
}

// insertV03Row helpers for cmd-layer migration tests.
func insertV03SessionAt(t *testing.T, dsn, sessionID, cwd string, hasMessages bool) {
	t.Helper()
	rawDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	defer rawDB.Close()

	if _, err := rawDB.Exec(
		`INSERT INTO sessions (session_id, cwd, repo_path, imported_at)
		 VALUES (?, ?, NULL, '2026-03-28T15:00:00Z')`,
		sessionID, cwd,
	); err != nil {
		t.Fatalf("insertV03Session(%s): %v", sessionID, err)
	}
	if hasMessages {
		if _, err := rawDB.Exec(
			`INSERT INTO messages (uuid, session_id, parent_uuid, role, content, timestamp, is_sidechain)
			 VALUES (?, ?, NULL, 'user', '{}', '2026-03-28T15:00:00Z', 0)`,
			"m-"+sessionID, sessionID,
		); err != nil {
			t.Fatalf("insertV03Message(%s): %v", sessionID, err)
		}
	}
}

// TestBackfillCmd_OutputIncludesMigratedCounts verifies that backfillCmd
// prints the "Migrated to v0.4: ..." line with non-zero counts when the DB
// is on v0.3.
func TestBackfillCmd_OutputIncludesMigratedCounts(t *testing.T) {
	dsn := setupV03DBFile(t)
	insertV03SessionAt(t, dsn, "s1", "/Users/test/proj", true)

	open := func() (*core.DB, error) { return core.OpenDB(dsn) }

	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"--yes"}, open, strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Migrated to v0.4: sessions=1 messages=1 import_states=0") {
		t.Errorf("stdout = %q, want to contain 'Migrated to v0.4: sessions=1 messages=1 import_states=0'", out.String())
	}
	if !strings.Contains(out.String(), "Backfilled:") {
		t.Errorf("stdout = %q, want to also contain 'Backfilled:' line", out.String())
	}
}

// TestBackfillCmd_NonInteractiveOrphanError_StillEmitsMigrationLine verifies
// that the migration line is emitted before the confirmation-required error,
// so the user sees the migration result even when the run aborts.
func TestBackfillCmd_NonInteractiveOrphanError_StillEmitsMigrationLine(t *testing.T) {
	dsn := setupV03DBFile(t)
	insertV03SessionAt(t, dsn, "orphan", "/Users/test/proj", false) // no messages -> orphan

	open := func() (*core.DB, error) { return core.OpenDB(dsn) }

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, open, strings.NewReader(""), &out, &errOut, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "backfill requires confirmation") {
		t.Errorf("err = %v, want one mentioning 'backfill requires confirmation'", err)
	}
	if !strings.Contains(out.String(), "Migrated to v0.4: sessions=1 messages=0 import_states=0") {
		t.Errorf("stdout = %q, want 'Migrated to v0.4: ...' even on confirmation error", out.String())
	}
	if strings.Contains(out.String(), "Backfilled:") {
		t.Errorf("stdout = %q, must not contain 'Backfilled:' since cleanup did not run", out.String())
	}
}

// TestBackfillCmd_InteractiveDeclineEmitsMigrationLine: interactive decline
// path also keeps the migration line, but does not emit the cleanup line.
func TestBackfillCmd_InteractiveDeclineEmitsMigrationLine(t *testing.T) {
	dsn := setupV03DBFile(t)
	insertV03SessionAt(t, dsn, "orphan", "/Users/test/proj", false)

	open := func() (*core.DB, error) { return core.OpenDB(dsn) }

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, open, strings.NewReader("n\n"), &out, &errOut, true)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "Migrated to v0.4: sessions=1 messages=0 import_states=0") {
		t.Errorf("stdout = %q, want 'Migrated to v0.4: ...' before decline", out.String())
	}
	if strings.Contains(out.String(), "Backfilled:") {
		t.Errorf("stdout = %q, must not contain 'Backfilled:' after decline", out.String())
	}
}
