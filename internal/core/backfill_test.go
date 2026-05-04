package core

import (
	"database/sql"
	"os/exec"
	"path/filepath"
	"testing"
)

// insertLegacySession inserts a sessions row directly, bypassing upsertSession,
// so tests can construct pre-v0.3 state (repo_path NULL with cwd present, or
// cwd itself NULL/empty). A nil cwd argument stores SQL NULL; a non-nil value
// is stored verbatim (including the empty string).
//
// Stamps source = 'claude_code' so the row satisfies the v0.4 NOT NULL
// constraint. v0.4 schema is what OpenDB creates and what every CRUD path
// expects; tests that need to construct v0.3-shaped rows for migration
// testing use setupV03DB instead.
func insertLegacySession(t *testing.T, db *DB, sessionID string, cwd *string) {
	t.Helper()
	var cwdArg any
	if cwd != nil {
		cwdArg = *cwd
	}
	if _, err := db.db.Exec(
		`INSERT INTO sessions (source, session_id, cwd, repo_path, imported_at)
		 VALUES ('claude_code', ?, ?, NULL, '2026-03-28T15:00:00Z')`,
		sessionID, cwdArg,
	); err != nil {
		t.Fatalf("insertLegacySession: %v", err)
	}
}

// insertLegacyMessage inserts a single messages row tied to sessionID, so the
// session is no longer an orphan. The uuid must be unique within the test DB.
func insertLegacyMessage(t *testing.T, db *DB, sessionID, uuid string) {
	t.Helper()
	if _, err := db.db.Exec(
		`INSERT INTO messages (uuid, source, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		 VALUES (?, 'claude_code', ?, NULL, 'user', '{}', '2026-03-28T15:00:00Z', 0)`,
		uuid, sessionID,
	); err != nil {
		t.Fatalf("insertLegacyMessage: %v", err)
	}
}

func strptr(s string) *string { return &s }

func queryRepoPath(t *testing.T, db *DB, sessionID string) (sql.NullString, error) {
	t.Helper()
	var s sql.NullString
	err := db.db.QueryRow(
		`SELECT repo_path FROM sessions WHERE session_id = ?`, sessionID,
	).Scan(&s)
	return s, err
}

func sessionExists(t *testing.T, db *DB, sessionID string) bool {
	t.Helper()
	var n int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE session_id = ?`, sessionID,
	).Scan(&n); err != nil {
		t.Fatalf("sessionExists(%s): %v", sessionID, err)
	}
	return n > 0
}

// initGitRepo creates a git repository at dir and returns the EvalSymlinks'd
// path that git rev-parse --show-toplevel would report.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-c", "safe.directory=*", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	return resolved
}

func TestBackfill_FillsWorktreeCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", strptr("/Users/test/proj/.claude/worktrees/feature"))
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 1 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:1 Unresolved:0}", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != "/Users/test/proj" {
		t.Errorf("repo_path = %+v, want {/Users/test/proj, valid}", got)
	}
}

func TestBackfill_SkipsNullCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", nil)
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want zero", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if got.Valid {
		t.Errorf("repo_path = %+v, want NULL", got)
	}
}

func TestBackfill_SkipsEmptyCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", strptr(""))
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want zero", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if got.Valid {
		t.Errorf("repo_path = %+v, want NULL", got)
	}
}

// TestBackfill_FillsNonGitCWDVerbatim は仕様 4（git 失敗時は cwd を
// そのまま返す）により、git 配下外の cwd でも repo_path が cwd 自体で埋まることを
// 担保する。
func TestBackfill_FillsNonGitCWDVerbatim(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	insertLegacySession(t, db, "s1", &missing)
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 1 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:1 Unresolved:0}", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != missing {
		t.Errorf("repo_path = %+v, want {%q, valid}", got, missing)
	}
}

func TestBackfill_LeavesFilledSessions(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := db.db.Exec(
		`INSERT INTO sessions (source, session_id, cwd, repo_path, imported_at)
		 VALUES ('claude_code', 's1', '/Users/test/existing/.claude/worktrees/x', '/Users/test/existing', '2026-03-28T15:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want zero", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != "/Users/test/existing" {
		t.Errorf("repo_path = %+v, want {/Users/test/existing, valid}", got)
	}
}

func TestBackfill_Idempotent(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "ok", strptr("/Users/test/proj/.claude/worktrees/feature"))
	insertLegacyMessage(t, db, "ok", "m-ok")
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	insertLegacySession(t, db, "bad", &missing)
	insertLegacyMessage(t, db, "bad", "m-bad")
	insertLegacySession(t, db, "orphan", strptr("/Users/test/proj"))

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("first Backfill: %v", err)
	}
	if result.Resolved != 2 || result.Unresolved != 0 || result.Deleted != 1 {
		t.Errorf("first result = %+v, want {Deleted:1 Resolved:2 Unresolved:0}", result)
	}

	result, err = Backfill(db)
	if err != nil {
		t.Fatalf("second Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("second result = %+v, want zero", result)
	}
}

func TestBackfill_MultipleSessionsSameCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	cwd := "/Users/test/proj/.claude/worktrees/feature"
	insertLegacySession(t, db, "s1", &cwd)
	insertLegacyMessage(t, db, "s1", "m1")
	insertLegacySession(t, db, "s2", &cwd)
	insertLegacyMessage(t, db, "s2", "m2")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 2 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:2 Unresolved:0}", result)
	}

	for _, id := range []string{"s1", "s2"} {
		got, err := queryRepoPath(t, db, id)
		if err != nil {
			t.Fatalf("queryRepoPath(%s): %v", id, err)
		}
		if !got.Valid || got.String != "/Users/test/proj" {
			t.Errorf("repo_path[%s] = %+v, want {/Users/test/proj, valid}", id, got)
		}
	}
}

// TestBackfill_GitToplevel verifies the git path resolution is exercised
// (not just the worktree string match), by using a real temp git repo as cwd.
func TestBackfill_GitToplevel(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	want := initGitRepo(t, dir)
	insertLegacySession(t, db, "s1", &dir)
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 1 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:1 Unresolved:0}", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != want {
		t.Errorf("repo_path = %+v, want %q", got, want)
	}
}

func TestBackfill_DeletesOrphanSessions(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "orphan", strptr("/Users/test/proj"))

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Deleted != 1 || result.Resolved != 0 || result.Unresolved != 0 {
		t.Errorf("result = %+v, want {Deleted:1 Resolved:0 Unresolved:0}", result)
	}
	if sessionExists(t, db, "orphan") {
		t.Errorf("orphan session not deleted")
	}
}

func TestBackfill_KeepsSessionsWithMessages(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := db.db.Exec(
		`INSERT INTO sessions (source, session_id, cwd, repo_path, imported_at)
		 VALUES ('claude_code', 's1', '/Users/test/proj', '/Users/test/proj', '2026-03-28T15:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("result.Deleted = %d, want 0", result.Deleted)
	}
	if !sessionExists(t, db, "s1") {
		t.Errorf("session s1 must remain")
	}
}

// TestBackfill_DeletePlusResolveCombined verifies a single Backfill call can
// both delete orphans and resolve repo_path on the same DB without
// double-counting the deleted session in Resolved.
func TestBackfill_DeletePlusResolveCombined(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// orphan: messages 0 件かつ repo_path NULL — DELETE 対象
	insertLegacySession(t, db, "orphan", strptr("/Users/test/proj"))

	// kept: messages 1 件、repo_path NULL — UPDATE 対象（resolve）
	insertLegacySession(t, db, "kept", strptr("/Users/test/proj/.claude/worktrees/feature"))
	insertLegacyMessage(t, db, "kept", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Deleted != 1 || result.Resolved != 1 || result.Unresolved != 0 {
		t.Errorf("result = %+v, want {Deleted:1 Resolved:1 Unresolved:0}", result)
	}
	if sessionExists(t, db, "orphan") {
		t.Errorf("orphan session not deleted")
	}
	if !sessionExists(t, db, "kept") {
		t.Errorf("kept session must remain")
	}
}

func TestCountOrphanSessions(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "orphan-a", strptr("/x"))
	insertLegacySession(t, db, "orphan-b", strptr("/y"))
	insertLegacySession(t, db, "kept", strptr("/z"))
	insertLegacyMessage(t, db, "kept", "m1")

	count, err := CountOrphanSessions(db)
	if err != nil {
		t.Fatalf("CountOrphanSessions: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

// v03Schema is the pre-v0.4 (i.e. v0.3) shape of the three tables. Used by
// migration tests that exercise MigrateToV04IfNeeded against a DB that still
// has the old layout (no source column, sessions PK on session_id alone).
const v03Schema = `
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

// setupV03DB returns a *DB whose underlying sqlite is on the v0.3 schema
// (no source column, sessions PK on session_id alone). Bypasses OpenDB so
// the v0.4 schema constant does not get applied.
//
// SetMaxOpenConns(1) is required because modernc.org/sqlite treats each
// connection to ":memory:" as a separate DB instance.
func setupV03DB(t *testing.T) *DB {
	t.Helper()
	rawDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	rawDB.SetMaxOpenConns(1)
	t.Cleanup(func() { rawDB.Close() })
	if _, err := rawDB.Exec(v03Schema); err != nil {
		t.Fatalf("v03Schema exec: %v", err)
	}
	return &DB{db: rawDB}
}

// insertV03Session inserts a row using the v0.3 column set (no source).
func insertV03Session(t *testing.T, db *DB, sessionID, cwd, repoPath string) {
	t.Helper()
	if _, err := db.db.Exec(
		`INSERT INTO sessions (session_id, cwd, repo_path, imported_at)
		 VALUES (?, ?, NULLIF(?, ''), '2026-03-28T15:00:00Z')`,
		sessionID, cwd, repoPath,
	); err != nil {
		t.Fatalf("insertV03Session: %v", err)
	}
}

// insertV03Message inserts a v0.3 messages row tied to sessionID.
func insertV03Message(t *testing.T, db *DB, sessionID, uuid string) {
	t.Helper()
	if _, err := db.db.Exec(
		`INSERT INTO messages (uuid, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		 VALUES (?, ?, NULL, 'user', '{}', '2026-03-28T15:00:00Z', 0)`,
		uuid, sessionID,
	); err != nil {
		t.Fatalf("insertV03Message: %v", err)
	}
}

// insertV03ImportState inserts a v0.3 import_state row.
func insertV03ImportState(t *testing.T, db *DB, path string) {
	t.Helper()
	if _, err := db.db.Exec(
		`INSERT INTO import_state (jsonl_path, file_size, last_offset, imported_at)
		 VALUES (?, 100, 50, '2026-03-28T15:00:00Z')`,
		path,
	); err != nil {
		t.Fatalf("insertV03ImportState: %v", err)
	}
}

func TestBackfill_MigratesV03ToV04(t *testing.T) {
	unsetAllGitEnv(t)

	db := setupV03DB(t)

	// Sample data on v0.3 schema.
	insertV03Session(t, db, "s1", "/Users/test/proj", "/Users/test/proj")
	insertV03Session(t, db, "s2", "/tmp", "")
	insertV03Message(t, db, "s1", "m1")
	insertV03Message(t, db, "s1", "m2")
	insertV03ImportState(t, db, "/path/to/a.jsonl")
	insertV03ImportState(t, db, "/path/to/b.jsonl")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if result.MigratedSessions != 2 || result.MigratedMessages != 2 || result.MigratedImportStates != 2 {
		t.Errorf("migrated counts = {sessions:%d messages:%d import_states:%d}, want 2/2/2",
			result.MigratedSessions, result.MigratedMessages, result.MigratedImportStates)
	}

	// Source column exists.
	present, err := tableColumnPresent(db.db, "sessions", "source")
	if err != nil || !present {
		t.Fatalf("sessions.source missing after migration: present=%v err=%v", present, err)
	}

	// Composite PK on (source, session_id): two rows in PRAGMA table_info should
	// have pk > 0.
	pkCount := 0
	rows, err := db.db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	for rows.Next() {
		var (
			cid     int
			name    string
			colType string
			notnull int
			dflt    sql.NullString
			pk      int
		)
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dflt, &pk); err != nil {
			rows.Close()
			t.Fatalf("scan PRAGMA: %v", err)
		}
		if pk > 0 {
			pkCount++
		}
	}
	rows.Close()
	if pkCount != 2 {
		t.Errorf("composite PK columns: got %d, want 2", pkCount)
	}

	// Round-trip data: existing rows are stamped source='claude_code'.
	var src, sid, cwd string
	if err := db.db.QueryRow(
		`SELECT source, session_id, cwd FROM sessions WHERE session_id='s1'`,
	).Scan(&src, &sid, &cwd); err != nil {
		t.Fatalf("SELECT s1: %v", err)
	}
	if src != "claude_code" || sid != "s1" || cwd != "/Users/test/proj" {
		t.Errorf("s1 round-trip mismatch: src=%q sid=%q cwd=%q", src, sid, cwd)
	}
}

func TestBackfill_MigrationIdempotent(t *testing.T) {
	unsetAllGitEnv(t)

	db := setupV03DB(t)
	insertV03Session(t, db, "s1", "/proj", "/proj")
	insertV03Message(t, db, "s1", "m1")

	first, err := Backfill(db)
	if err != nil {
		t.Fatalf("first Backfill: %v", err)
	}
	if first.MigratedSessions == 0 {
		t.Errorf("first run: MigratedSessions = 0, want > 0")
	}

	second, err := Backfill(db)
	if err != nil {
		t.Fatalf("second Backfill: %v", err)
	}
	if second.MigratedSessions != 0 || second.MigratedMessages != 0 || second.MigratedImportStates != 0 {
		t.Errorf("second run migrated counts = {%d %d %d}, want all 0",
			second.MigratedSessions, second.MigratedMessages, second.MigratedImportStates)
	}
}

func TestBackfill_MigrationWithOrphans(t *testing.T) {
	unsetAllGitEnv(t)

	db := setupV03DB(t)

	// orphan: no messages, repo_path NULL → should be deleted by Backfill.
	insertV03Session(t, db, "orphan", "/Users/test/proj", "")
	// kept: has message, repo_path NULL but cwd is a worktree path → resolve.
	insertV03Session(t, db, "kept", "/Users/test/proj/.claude/worktrees/feature", "")
	insertV03Message(t, db, "kept", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	if result.MigratedSessions != 2 || result.MigratedMessages != 1 || result.MigratedImportStates != 0 {
		t.Errorf("migrated counts = {%d %d %d}, want 2/1/0",
			result.MigratedSessions, result.MigratedMessages, result.MigratedImportStates)
	}
	if result.Deleted != 1 || result.Resolved != 1 || result.Unresolved != 0 {
		t.Errorf("cleanup counts = {Deleted:%d Resolved:%d Unresolved:%d}, want 1/1/0",
			result.Deleted, result.Resolved, result.Unresolved)
	}
}

func TestBackfill_MigrationOnV04DB_NoOp(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.MigratedSessions != 0 || result.MigratedMessages != 0 || result.MigratedImportStates != 0 {
		t.Errorf("v0.4 DB migrated counts = {%d %d %d}, want all 0",
			result.MigratedSessions, result.MigratedMessages, result.MigratedImportStates)
	}
}

func TestBackfill_MigrationCompositeFK(t *testing.T) {
	unsetAllGitEnv(t)

	db := setupV03DB(t)
	insertV03Session(t, db, "s1", "/proj", "/proj")
	insertV03Message(t, db, "s1", "m1")

	if _, err := Backfill(db); err != nil {
		t.Fatalf("Backfill: %v", err)
	}

	// FK list should mention composite (source, session_id).
	rows, err := db.db.Query("PRAGMA foreign_key_list(messages)")
	if err != nil {
		t.Fatalf("PRAGMA foreign_key_list: %v", err)
	}
	defer rows.Close()
	fkColumns := map[string]string{}
	for rows.Next() {
		var (
			id, seq                   int
			table, from, to, onUpdate string
			onDelete, match           string
		)
		if err := rows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			t.Fatalf("scan PRAGMA: %v", err)
		}
		if table != "sessions" {
			t.Errorf("FK references %q, want sessions", table)
		}
		fkColumns[from] = to
	}
	if fkColumns["source"] != "source" || fkColumns["session_id"] != "session_id" {
		t.Errorf("composite FK missing: got %+v", fkColumns)
	}

	// With foreign_keys = ON, inserting a messages row referencing a session
	// that does not exist should fail.
	if _, err := db.db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	defer db.db.Exec("PRAGMA foreign_keys = OFF")

	_, err = db.db.Exec(`INSERT INTO messages (uuid, source, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		 VALUES ('m-bad', 'codex', 'nonexistent', NULL, 'user', '', '2026-03-28T15:00:00Z', 0)`)
	if err == nil {
		t.Errorf("INSERT with missing FK reference should fail under foreign_keys=ON")
	}
}
