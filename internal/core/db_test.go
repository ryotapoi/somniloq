package core

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
}

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenDB_CreatesSchema(t *testing.T) {
	db := testDB(t)

	tables := []string{"sessions", "messages", "import_state"}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestOpenDB_HasRepoPathColumn(t *testing.T) {
	db := testDB(t)

	present, err := tableColumnPresent(db.db, "sessions", "repo_path")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if !present {
		t.Fatal("repo_path column missing from sessions table")
	}
}

// legacySessionsSchema is the v0.2.x sessions table shape: before the
// repo_path column was added and before project_dir was dropped. Used by
// migration tests that exercise ensureSessionsRepoPathColumn and
// ensureSessionsProjectDirColumnDropped against a DB that still has the old
// shape.
const legacySessionsSchema = `
CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY,
    project_dir TEXT NOT NULL,
    cwd TEXT,
    git_branch TEXT,
    custom_title TEXT,
    agent_name TEXT,
    version TEXT,
    started_at TEXT,
    ended_at TEXT,
    imported_at TEXT NOT NULL
);`

func openLegacyMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(legacySessionsSchema); err != nil {
		t.Fatalf("legacy schema exec failed: %v", err)
	}
	return db
}

func TestEnsureSessionsRepoPathColumn_AddsColumn(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsRepoPathColumn(db); err != nil {
		t.Fatalf("ensureSessionsRepoPathColumn failed: %v", err)
	}

	present, err := tableColumnPresent(db, "sessions", "repo_path")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if !present {
		t.Fatal("repo_path column should have been added")
	}
}

func TestEnsureSessionsRepoPathColumn_Idempotent(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsRepoPathColumn(db); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := ensureSessionsRepoPathColumn(db); err != nil {
		t.Fatalf("second call should be a no-op, got: %v", err)
	}
}

func TestEnsureSessionsProjectDirColumnDropped_DropsColumn(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		t.Fatalf("ensureSessionsProjectDirColumnDropped failed: %v", err)
	}

	present, err := tableColumnPresent(db, "sessions", "project_dir")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if present {
		t.Fatal("project_dir column should have been dropped")
	}
}

func TestEnsureSessionsProjectDirColumnDropped_Idempotent(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		t.Fatalf("second call should be a no-op, got: %v", err)
	}
}

func TestEnsureSessionsProjectDirColumnDropped_NoOpWhenAbsent(t *testing.T) {
	db := testDB(t)

	if err := ensureSessionsProjectDirColumnDropped(db.db); err != nil {
		t.Fatalf("call against fresh DB should be a no-op, got: %v", err)
	}
}

func TestOpenDB_HasNoProjectDirColumn(t *testing.T) {
	db := testDB(t)

	present, err := tableColumnPresent(db.db, "sessions", "project_dir")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if present {
		t.Fatal("project_dir column should not exist on a fresh DB")
	}
}

func TestUpsertSession(t *testing.T) {
	db := testDB(t)

	meta := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		CWD:       "/tmp",
		RepoPath:  "/Users/test",
		GitBranch: "main",
		Version:   "2.1.86",
		StartedAt: "2026-03-28T14:00:00Z",
		EndedAt:   "2026-03-28T14:10:00Z",
	}
	if err := db.UpsertSession(meta, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var sid, startedAt, repoPath string
	err := db.db.QueryRow("SELECT session_id, started_at, repo_path FROM sessions WHERE session_id='s1'").
		Scan(&sid, &startedAt, &repoPath)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if sid != "s1" || startedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("unexpected row: sid=%s startedAt=%s", sid, startedAt)
	}
	if repoPath != "/Users/test" {
		t.Errorf("repo_path: got %q, want %q", repoPath, "/Users/test")
	}

	// Second upsert with later ended_at
	meta2 := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		CWD:       "/tmp",
		RepoPath:  "/Users/test",
		StartedAt: "2026-03-28T14:05:00Z",
		EndedAt:   "2026-03-28T14:20:00Z",
	}
	if err := db.UpsertSession(meta2, "2026-03-28T15:01:00Z"); err != nil {
		t.Fatalf("UpsertSession (2nd) failed: %v", err)
	}

	var endedAt string
	err = db.db.QueryRow("SELECT started_at, ended_at FROM sessions WHERE session_id='s1'").
		Scan(&startedAt, &endedAt)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if startedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("started_at should be MIN: got %s", startedAt)
	}
	if endedAt != "2026-03-28T14:20:00Z" {
		t.Errorf("ended_at should be MAX: got %s", endedAt)
	}
}

func TestUpsertSession_RepoPath(t *testing.T) {
	db := testDB(t)

	// Use distinct values for every text column so that any order mismatch
	// between the Go args and the SQL placeholders is immediately visible.
	meta := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s-map",
		CWD:       "cwd-val",
		RepoPath:  "repo-val",
		GitBranch: "branch-val",
		Version:   "version-val",
		StartedAt: "started-val",
		EndedAt:   "ended-val",
	}
	if err := db.UpsertSession(meta, "imported-val"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var cwd, repoPath, branch, version, startedAt, endedAt string
	err := db.db.QueryRow(`SELECT cwd, repo_path, git_branch, version, started_at, ended_at FROM sessions WHERE session_id='s-map'`).
		Scan(&cwd, &repoPath, &branch, &version, &startedAt, &endedAt)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	checks := []struct {
		label, got, want string
	}{
		{"cwd", cwd, "cwd-val"},
		{"repo_path", repoPath, "repo-val"},
		{"git_branch", branch, "branch-val"},
		{"version", version, "version-val"},
		{"started_at", startedAt, "started-val"},
		{"ended_at", endedAt, "ended-val"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.label, c.got, c.want)
		}
	}
}

func TestUpsertSession_RepoPath_EmptyInsertsNull(t *testing.T) {
	db := testDB(t)

	meta := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "",
	}
	if err := db.UpsertSession(meta, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var isNull int
	if err := db.db.QueryRow("SELECT repo_path IS NULL FROM sessions WHERE session_id='s1'").Scan(&isNull); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if isNull != 1 {
		t.Errorf("repo_path should be NULL for empty RepoPath on insert")
	}
}

func TestUpsertSession_RepoPath_EmptyDoesNotOverwrite(t *testing.T) {
	db := testDB(t)

	if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/proj"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("first UpsertSession failed: %v", err)
	}
	if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: ""}, "2026-03-28T15:01:00Z"); err != nil {
		t.Fatalf("second UpsertSession failed: %v", err)
	}

	var repoPath string
	if err := db.db.QueryRow("SELECT repo_path FROM sessions WHERE session_id='s1'").Scan(&repoPath); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if repoPath != "/Users/test/proj" {
		t.Errorf("repo_path should not be overwritten by empty value: got %q", repoPath)
	}
}

func TestUpsertSession_RepoPath_AfterUpdateSessionTitle(t *testing.T) {
	db := testDB(t)

	// UpdateSessionTitle on an existing row only touches custom_title/imported_at,
	// not repo_path.
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/proj"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s1", "title", "2026-03-28T15:01:00Z"))

	var repoPath string
	if err := db.db.QueryRow("SELECT repo_path FROM sessions WHERE session_id='s1'").Scan(&repoPath); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if repoPath != "/Users/test/proj" {
		t.Errorf("repo_path: got %q, want %q", repoPath, "/Users/test/proj")
	}

	var title string
	if err := db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "title" {
		t.Errorf("custom_title: got %q, want %q", title, "title")
	}
}

func TestOpenDB_ReopenIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test.db")

	db1, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("first OpenDB failed: %v", err)
	}
	if err := db1.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	db2, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("second OpenDB failed: %v", err)
	}
	t.Cleanup(func() { db2.Close() })

	// Second open must hit the "column already present" fast path without error.
	if present, err := tableColumnPresent(db2.db, "sessions", "repo_path"); err != nil || !present {
		t.Fatalf("repo_path not present after reopen: present=%v err=%v", present, err)
	}

	rows, err := db2.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 || rows[0].SessionID != "s1" {
		t.Errorf("session not retained across reopen: %+v", rows)
	}
}

func TestOpenDB_MigratesLegacyFile(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "legacy.db")

	// Create a legacy-shaped DB file directly, without going through OpenDB.
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	if _, err := raw.Exec(legacySessionsSchema); err != nil {
		raw.Close()
		t.Fatalf("legacy schema exec failed: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO sessions (session_id, project_dir, imported_at) VALUES (?, ?, ?)`,
		"legacy-s1", "-test", "2026-03-28T15:00:00Z",
	); err != nil {
		raw.Close()
		t.Fatalf("legacy INSERT failed: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close failed: %v", err)
	}

	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB on legacy file failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	present, err := tableColumnPresent(db.db, "sessions", "repo_path")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if !present {
		t.Fatal("repo_path column should have been added to legacy DB")
	}

	projectDirPresent, err := tableColumnPresent(db.db, "sessions", "project_dir")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if projectDirPresent {
		t.Fatal("project_dir column should have been dropped from legacy DB")
	}

	// OpenDB must NOT add the v0.4 source column on a legacy file: the
	// v0.3 → v0.4 migration is gated on `somniloq backfill`, not OpenDB.
	// If this assertion ever fails, OpenDB has started doing migration work
	// it shouldn't (see ADR 0004).
	sourcePresent, err := tableColumnPresent(db.db, "sessions", "source")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if sourcePresent {
		t.Fatal("source column must not be added by OpenDB; it is added by Backfill (v0.4 migration)")
	}

	var (
		importedAt string
		isNull     int
	)
	if err := db.db.QueryRow(
		"SELECT imported_at, repo_path IS NULL FROM sessions WHERE session_id='legacy-s1'",
	).Scan(&importedAt, &isNull); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if importedAt != "2026-03-28T15:00:00Z" {
		t.Errorf("imported_at mutated by migration: got %q", importedAt)
	}
	if isNull != 1 {
		t.Errorf("existing row should have repo_path IS NULL after migration")
	}
}

func TestInsertMessage(t *testing.T) {
	db := testDB(t)

	// Need a session first
	if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	parent := "p1"
	msg := ParsedMessage{
		Source:      SourceClaudeCode,
		UUID:        "m1",
		ParentUUID:  &parent,
		SessionID:   "s1",
		Role:        "user",
		Content:     "hello",
		Timestamp:   "2026-03-28T14:00:00Z",
		IsSidechain: false,
	}
	if err := db.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	var uuid, role, content string
	err := db.db.QueryRow("SELECT uuid, role, content FROM messages WHERE uuid='m1'").
		Scan(&uuid, &role, &content)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if role != "user" || content != "hello" {
		t.Errorf("unexpected: role=%s content=%s", role, content)
	}

	// Duplicate insert should not error
	if err := db.InsertMessage(msg); err != nil {
		t.Fatalf("duplicate InsertMessage should not error: %v", err)
	}
}

func TestUpdateSessionTitle(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"))
	if err := db.UpdateSessionTitle(SourceClaudeCode, "s1", "my title", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle failed: %v", err)
	}

	var title string
	err := db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "my title" {
		t.Errorf("got %q, want %q", title, "my title")
	}
}

func TestUpdateSessionTitle_NoRow_IsNoop(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateSessionTitle(SourceClaudeCode, "ghost", "title", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle should not error on missing row: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id='ghost'").Scan(&count); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if count != 0 {
		t.Errorf("UpdateSessionTitle should not create rows; got %d", count)
	}
}

func TestUpsertImportState(t *testing.T) {
	db := testDB(t)

	state := ImportState{
		JSONLPath:  "/path/to/file.jsonl",
		Source:     SourceClaudeCode,
		FileSize:   1000,
		LastOffset: 500,
		ImportedAt: "2026-03-28T15:00:00Z",
	}
	if err := db.UpsertImportState(state); err != nil {
		t.Fatalf("UpsertImportState failed: %v", err)
	}

	got, err := db.GetImportState("/path/to/file.jsonl")
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.FileSize != 1000 || got.LastOffset != 500 {
		t.Errorf("unexpected state: %+v", got)
	}

	// Update
	state.FileSize = 2000
	state.LastOffset = 1500
	if err := db.UpsertImportState(state); err != nil {
		t.Fatalf("UpsertImportState (update) failed: %v", err)
	}
	got, _ = db.GetImportState("/path/to/file.jsonl")
	if got.FileSize != 2000 || got.LastOffset != 1500 {
		t.Errorf("update failed: %+v", got)
	}
}

func TestGetImportState_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetImportState("/nonexistent")
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestListSessions_Empty(t *testing.T) {
	db := testDB(t)

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestListSessions_OrderAndCount(t *testing.T) {
	db := testDB(t)

	// Older session with 1 message
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	// Newer session with 2 messages
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", StartedAt: "2026-03-28T14:00:00Z", EndedAt: "2026-03-28T14:30:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s2", Role: "user", Content: "hi", Timestamp: "2026-03-28T14:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s2", Role: "assistant", Content: "hey", Timestamp: "2026-03-28T14:01:00Z"}))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Newer session first (DESC order)
	if rows[0].SessionID != "s2" {
		t.Errorf("first row should be s2 (newer), got %s", rows[0].SessionID)
	}
	if rows[0].StartedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("s2 started_at: got %s, want 2026-03-28T14:00:00Z", rows[0].StartedAt)
	}
	if rows[0].MessageCount != 2 {
		t.Errorf("s2 message count: got %d, want 2", rows[0].MessageCount)
	}
	if rows[1].SessionID != "s1" {
		t.Errorf("second row should be s1 (older), got %s", rows[1].SessionID)
	}
	if rows[1].MessageCount != 1 {
		t.Errorf("s1 message count: got %d, want 1", rows[1].MessageCount)
	}
}

func TestListSessions_ZeroMessages(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].MessageCount != 0 {
		t.Errorf("message count: got %d, want 0", rows[0].MessageCount)
	}
}

func TestListSessions_NullTitle(t *testing.T) {
	db := testDB(t)

	// Session with no custom_title set
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if rows[0].CustomTitle != "" {
		t.Errorf("custom_title should be empty string, got %q", rows[0].CustomTitle)
	}
}

func TestListSessions_NullStartedAt(t *testing.T) {
	db := testDB(t)

	// Session with started_at (normal)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Session created via UpsertSession with no StartedAt, then title applied.
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s2", "title only", "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Normal session first, NULL started_at at the end
	if rows[0].SessionID != "s1" {
		t.Errorf("first row should be s1 (has started_at), got %s", rows[0].SessionID)
	}
	if rows[1].SessionID != "s2" {
		t.Errorf("second row should be s2 (NULL started_at), got %s", rows[1].SessionID)
	}
}

func TestListSessions_SinceFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "old", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T00:00:00Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "new" {
		t.Errorf("expected session 'new', got %s", rows[0].SessionID)
	}
}

func TestListSessions_SinceFilter_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	// Real JSONL timestamps have milliseconds (e.g. "2026-03-28T14:10:45.977Z")
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T14:10:45.977Z"}, "2026-03-28T15:00:00Z"))

	// Since filter with millisecond precision (as generated by cmd layer)
	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T14:10:45.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (ms timestamp should match), got %d", len(rows))
	}
}

func TestListSessions_ProjectFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-28T11:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Project: "Brimday"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "s1" {
		t.Errorf("expected s1, got %s", rows[0].SessionID)
	}
}

func TestListSessions_RepoPath(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/Brimday" {
		t.Errorf("RepoPath: got %q, want %q", rows[0].RepoPath, "/Users/test/Brimday")
	}
}

func TestListSessions_RepoPath_NullReturnsEmpty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "" {
		t.Errorf("RepoPath should be empty for NULL, got %q", rows[0].RepoPath)
	}
}

func TestGetSession_RepoPath(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.RepoPath != "/Users/test/Brimday" {
		t.Errorf("RepoPath: got %q, want %q", got.RepoPath, "/Users/test/Brimday")
	}
}

func TestListSessions_ProjectFilter_LikeMetacharKnownLimitation(t *testing.T) {
	// Pin the documented Known limitation: LIKE wildcards in --project are not
	// escaped, so a literal "%" in the filter degenerates into a "match anything"
	// segment. This test catches a future change that decides to escape them.
	// Rename signal: if escape is added, rename/repurpose this test instead of
	// silently treating the new behavior as a regression.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "/Users/test/Brimday",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Project: "Brim%day"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (LIKE %% wildcard passthrough), got %d", len(rows))
	}
}

func TestListSessions_ProjectFilter_SlashSpan(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "/Users/ryota/Sources/ryotapoi/somniloq",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Project: "Sources/ryot"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row matching slash-span, got %d", len(rows))
	}
	if rows[0].SessionID != "s1" {
		t.Errorf("expected s1, got %s", rows[0].SessionID)
	}
}

func TestListSessions_ProjectFilter_RepoPathOnly(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "repo-only",
		RepoPath:  "/Users/test/UniqRepo",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "projdir-only",
		RepoPath:  "/Users/other/baz",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "both",
		RepoPath:  "/Users/test/Common",
		StartedAt: "2026-03-28T12:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	t.Run("repo-only", func(t *testing.T) {
		rows, err := db.ListSessions(SessionFilter{Project: "UniqRepo"})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if rows[0].SessionID != "repo-only" {
			t.Errorf("got %s, want repo-only", rows[0].SessionID)
		}
	})
	t.Run("both", func(t *testing.T) {
		rows, err := db.ListSessions(SessionFilter{Project: "Common"})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if rows[0].SessionID != "both" {
			t.Errorf("got %s, want both", rows[0].SessionID)
		}
	})
	t.Run("unrelated repo_path", func(t *testing.T) {
		rows, err := db.ListSessions(SessionFilter{Project: "UniqProj"})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 rows (no repo_path matches), got %d", len(rows))
		}
	})
}

func TestListSessions_CombinedFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "old-brim", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new-brim", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new-somniloq", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T00:00:00Z", Project: "Brimday"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "new-brim" {
		t.Errorf("expected new-brim, got %s", rows[0].SessionID)
	}
}

func TestListSessions_UntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "early", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "late", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "early" {
		t.Errorf("expected 'early', got %s", rows[0].SessionID)
	}
}

func TestListSessions_SinceAndUntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", StartedAt: "2026-03-28T12:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s3", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T11:00:00.000Z", Until: "2026-03-28T13:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "s2" {
		t.Errorf("expected 's2', got %s", rows[0].SessionID)
	}
}

func TestListSessions_UntilFilter_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T12:00:00.500Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (ms timestamp 500ms > 000ms), got %d", len(rows))
	}
}

func TestListSessions_EndedAt(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EndedAt != "2026-03-28T10:30:00Z" {
		t.Errorf("EndedAt: got %q, want %q", rows[0].EndedAt, "2026-03-28T10:30:00Z")
	}
}

func TestListSessions_EndedAt_Null(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EndedAt != "" {
		t.Errorf("EndedAt: got %q, want empty string", rows[0].EndedAt)
	}
}

func TestGetMessages_Empty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	msgs, err := db.GetMessages(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetMessages_OrderByTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Insert in reverse order
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "world", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: false}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z", IsSidechain: true}))

	msgs, err := db.GetMessages(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Should be in timestamp ASC order
	if msgs[0].UUID != "m1" {
		t.Errorf("first message UUID: got %s, want m1", msgs[0].UUID)
	}
	if msgs[0].Role != "user" {
		t.Errorf("first message Role: got %s, want user", msgs[0].Role)
	}
	if msgs[0].Content != "hello" {
		t.Errorf("first message Content: got %s, want hello", msgs[0].Content)
	}
	if msgs[0].Timestamp != "2026-03-28T10:00:00Z" {
		t.Errorf("first message Timestamp: got %s, want 2026-03-28T10:00:00Z", msgs[0].Timestamp)
	}
	if msgs[0].IsSidechain != true {
		t.Errorf("first message IsSidechain: got %v, want true", msgs[0].IsSidechain)
	}

	if msgs[1].UUID != "m2" {
		t.Errorf("second message UUID: got %s, want m2", msgs[1].UUID)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("second message Role: got %s, want assistant", msgs[1].Role)
	}
	if msgs[1].IsSidechain != false {
		t.Errorf("second message IsSidechain: got %v, want false", msgs[1].IsSidechain)
	}
}

func TestGetSession_Found(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s1", "my session", "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID: got %s, want s1", got.SessionID)
	}
	if got.StartedAt != "2026-03-28T10:00:00Z" {
		t.Errorf("StartedAt: got %s, want 2026-03-28T10:00:00Z", got.StartedAt)
	}
	if got.CustomTitle != "my session" {
		t.Errorf("CustomTitle: got %q, want %q", got.CustomTitle, "my session")
	}
	if got.MessageCount != 1 {
		t.Errorf("MessageCount: got %d, want 1", got.MessageCount)
	}
}

func TestGetSession_EndedAt(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.EndedAt != "2026-03-28T10:30:00Z" {
		t.Errorf("EndedAt: got %q, want %q", got.EndedAt, "2026-03-28T10:30:00Z")
	}
}

func TestGetSession_EndedAt_Null(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.EndedAt != "" {
		t.Errorf("EndedAt: got %q, want empty string", got.EndedAt)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetSession(SourceClaudeCode, "nonexistent")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestLookupSessionsByID_CrossSource(t *testing.T) {
	db := testDB(t)

	for _, source := range []Source{SourceClaudeCode, SourceCodex} {
		must(t, db.UpsertSession(SessionMeta{Source: source, SessionID: "same-id", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	}
	must(t, db.InsertMessage(ParsedMessage{Source: SourceCodex, UUID: "codex-m1", SessionID: "same-id", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	got, err := db.LookupSessionsByID("same-id")
	if err != nil {
		t.Fatalf("LookupSessionsByID failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got): got %d, want 2", len(got))
	}
	if got[0].Source != SourceClaudeCode || got[1].Source != SourceCodex {
		t.Errorf("sources: got %s, %s; want %s, %s", got[0].Source, got[1].Source, SourceClaudeCode, SourceCodex)
	}
	if got[1].MessageCount != 1 {
		t.Errorf("codex MessageCount: got %d, want 1", got[1].MessageCount)
	}
}

func TestLookupSessionsByID_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.LookupSessionsByID("nonexistent")
	if err != nil {
		t.Fatalf("LookupSessionsByID failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got): got %d, want 0", len(got))
	}
}

func TestListProjects_Empty(t *testing.T) {
	db := testDB(t)

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestListProjects_GroupByProject(t *testing.T) {
	db := testDB(t)

	// Project A: 2 sessions
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "a1", RepoPath: "/Users/test/projA", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "a2", RepoPath: "/Users/test/projA", StartedAt: "2026-03-28T11:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Project B: 1 session
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "b1", RepoPath: "/Users/test/projB", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Project B first (latest started_at is 14:00, A's latest is 11:00)
	if rows[0].RepoPath != "/Users/test/projB" {
		t.Errorf("first row: got %s, want /Users/test/projB", rows[0].RepoPath)
	}
	if rows[0].SessionCount != 1 {
		t.Errorf("projB session count: got %d, want 1", rows[0].SessionCount)
	}
	if rows[1].RepoPath != "/Users/test/projA" {
		t.Errorf("second row: got %s, want /Users/test/projA", rows[1].RepoPath)
	}
	if rows[1].SessionCount != 2 {
		t.Errorf("projA session count: got %d, want 2", rows[1].SessionCount)
	}
}

func TestListProjects_SinceFilter(t *testing.T) {
	db := testDB(t)

	// Old project (only old sessions)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "old1", RepoPath: "/Users/test/old", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// New project
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new1", RepoPath: "/Users/test/new", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Since: "2026-03-28T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/new" {
		t.Errorf("expected /Users/test/new, got %s", rows[0].RepoPath)
	}
}

func TestListProjects_UntilFilter(t *testing.T) {
	db := testDB(t)

	// Early project
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "early1", RepoPath: "/Users/test/early", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Late project (only late sessions)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "late1", RepoPath: "/Users/test/late", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/early" {
		t.Errorf("expected /Users/test/early, got %s", rows[0].RepoPath)
	}
}

func TestListProjects_SinceAndUntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/old", StartedAt: "2026-03-28T08:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", RepoPath: "/Users/test/mid", StartedAt: "2026-03-28T12:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s3", RepoPath: "/Users/test/new", StartedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Since: "2026-03-28T10:00:00.000Z", Until: "2026-03-28T14:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/mid" {
		t.Errorf("expected /Users/test/mid, got %s", rows[0].RepoPath)
	}
}

func TestListProjects_GroupByRepoPath(t *testing.T) {
	// Worktree and body sessions share the same repo_path; they must collapse
	// into one row.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "body",
		RepoPath:  "/Users/test/Brimday",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "worktree",
		RepoPath:  "/Users/test/Brimday",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 aggregated row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/Brimday" {
		t.Errorf("RepoPath: got %q, want %q", rows[0].RepoPath, "/Users/test/Brimday")
	}
	if rows[0].SessionCount != 2 {
		t.Errorf("SessionCount: got %d, want 2", rows[0].SessionCount)
	}
}

func TestListProjects_EmptyRepoPathGroup(t *testing.T) {
	// When all sessions have empty repo_path, the group surfaces with
	// RepoPath: "" and a correct count.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s2",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "" {
		t.Errorf("RepoPath: got %q, want empty", rows[0].RepoPath)
	}
	if rows[0].SessionCount != 2 {
		t.Errorf("SessionCount: got %d, want 2", rows[0].SessionCount)
	}
}

func TestListProjects_NullRepoPathCollapsesAcrossProjects(t *testing.T) {
	// Sessions with empty repo_path collapse into a single group.
	// Documented as a Known limitation in rules/scope.md.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "a1",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "b1",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 collapsed row, got %d", len(rows))
	}
	if rows[0].RepoPath != "" {
		t.Errorf("RepoPath: got %q, want empty", rows[0].RepoPath)
	}
	if rows[0].SessionCount != 2 {
		t.Errorf("SessionCount: got %d, want 2", rows[0].SessionCount)
	}
}

func TestListProjects_GroupByRepoPath_OrderByLatest(t *testing.T) {
	// Two repo_path groups; the one whose latest session is newer must come first.
	db := testDB(t)

	// Body session for repo A, older.
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "a-body",
		RepoPath:  "/Users/test/RepoA",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	// Worktree session for repo A, newer than any repo B session.
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "a-wt",
		RepoPath:  "/Users/test/RepoA",
		StartedAt: "2026-03-28T16:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	// Repo B session in between.
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "b1",
		RepoPath:  "/Users/test/RepoB",
		StartedAt: "2026-03-28T12:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/RepoA" {
		t.Errorf("first row RepoPath: got %q, want %q", rows[0].RepoPath, "/Users/test/RepoA")
	}
	if rows[1].RepoPath != "/Users/test/RepoB" {
		t.Errorf("second row RepoPath: got %q, want %q", rows[1].RepoPath, "/Users/test/RepoB")
	}
}

func TestListProjects_NullStartedAt(t *testing.T) {
	db := testDB(t)

	// Normal session
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/normal", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Session with NULL started_at
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", RepoPath: "/Users/test/titleonly"}, "2026-03-28T15:00:00Z"))

	// No filter: both projects should appear
	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Normal project first, NULL started_at project last
	if rows[0].RepoPath != "/Users/test/normal" {
		t.Errorf("first row: got %s, want /Users/test/normal", rows[0].RepoPath)
	}
	if rows[1].RepoPath != "/Users/test/titleonly" {
		t.Errorf("second row: got %s, want /Users/test/titleonly", rows[1].RepoPath)
	}

	// With filter: NULL started_at excluded
	rows, err = db.ListProjects(SessionFilter{Since: "2026-03-28T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects with Since failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row with Since filter, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/normal" {
		t.Errorf("expected /Users/test/normal, got %s", rows[0].RepoPath)
	}
}

func TestGetSummaryMessages_Empty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_ReturnsFirstUserMessage(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "fix the bug", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "done", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s1", Role: "user", Content: "thanks", Timestamp: "2026-03-28T10:02:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m1" {
		t.Errorf("UUID: got %s, want m1", msgs[0].UUID)
	}
	if msgs[0].Role != "user" {
		t.Errorf("Role: got %s, want user", msgs[0].Role)
	}
	if msgs[0].Content != "fix the bug" {
		t.Errorf("Content: got %s, want 'fix the bug'", msgs[0].Content)
	}
	if msgs[0].Timestamp != "2026-03-28T10:00:00Z" {
		t.Errorf("Timestamp: got %s, want 2026-03-28T10:00:00Z", msgs[0].Timestamp)
	}
	if msgs[0].IsSidechain != false {
		t.Errorf("IsSidechain: got %v, want false", msgs[0].IsSidechain)
	}
}

func TestGetSummaryMessages_SkipsSidechain(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "sidechain msg", Timestamp: "2026-03-28T10:00:00Z", IsSidechain: true}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "real msg", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: false}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m2" {
		t.Errorf("expected m2 (non-sidechain), got %s", msgs[0].UUID)
	}
}

func TestGetSummaryMessages_NoUserMessages(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "assistant", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_LimitN(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "u1", SessionID: "s1", Role: "user", Content: "one", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "u2", SessionID: "s1", Role: "user", Content: "two", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "a1", SessionID: "s1", Role: "assistant", Content: "reply", Timestamp: "2026-03-28T10:02:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "u3", SessionID: "s1", Role: "user", Content: "three", Timestamp: "2026-03-28T10:03:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "u4", SessionID: "s1", Role: "user", Content: "four", Timestamp: "2026-03-28T10:04:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "u5", SessionID: "s1", Role: "user", Content: "five", Timestamp: "2026-03-28T10:05:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 3, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	wantUUIDs := []string{"u1", "u2", "u3"}
	for i, w := range wantUUIDs {
		if msgs[i].UUID != w {
			t.Errorf("msgs[%d].UUID: got %s, want %s", i, msgs[i].UUID, w)
		}
	}
}

func TestGetSummaryMessages_SkipsClearPrefix(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "<command-name>/clear</command-name>\n<command-message>clear</command-message>", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "real question", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m2" {
		t.Errorf("expected m2 (/clear skipped), got %s", msgs[0].UUID)
	}
}

func TestGetSummaryMessages_SkipsCaveatPrefix(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "<local-command-caveat>Caveat: ...</local-command-caveat>", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "real question", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m2" {
		t.Errorf("expected m2 (caveat skipped), got %s", msgs[0].UUID)
	}
}

func TestGetSummaryMessages_IncludeClear(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_clear", SessionID: "s1", Role: "user", Content: "<command-name>/clear</command-name>\nmore", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_caveat", SessionID: "s1", Role: "user", Content: "<local-command-caveat>note</local-command-caveat>", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_assistant", SessionID: "s1", Role: "assistant", Content: "reply", Timestamp: "2026-03-28T10:02:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_sidechain", SessionID: "s1", Role: "user", Content: "sidechain", Timestamp: "2026-03-28T10:03:00Z", IsSidechain: true}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_real", SessionID: "s1", Role: "user", Content: "real question", Timestamp: "2026-03-28T10:04:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 3, true)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	wantUUIDs := []string{"m_clear", "m_caveat", "m_real"}
	for i, w := range wantUUIDs {
		if msgs[i].UUID != w {
			t.Errorf("msgs[%d].UUID: got %s, want %s", i, msgs[i].UUID, w)
		}
	}
	if msgs[0].Content != "<command-name>/clear</command-name>\nmore" {
		t.Errorf("msgs[0].Content: got %q", msgs[0].Content)
	}
	if msgs[1].Content != "<local-command-caveat>note</local-command-caveat>" {
		t.Errorf("msgs[1].Content: got %q", msgs[1].Content)
	}
}

func TestGetSummaryMessages_AllSkipped(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "<command-name>/clear</command-name>", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "<local-command-caveat>x</local-command-caveat>", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_LimitExceedsAvailable(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "one", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "two", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 5, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_LimitZeroReturnsError(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	_, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 0, false)
	if err == nil {
		t.Fatal("expected error for limit=0, got nil")
	}
}

func TestGetSummaryMessages_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00.000Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_late", SessionID: "s1", Role: "user", Content: "later", Timestamp: "2026-03-28T10:00:00.200Z"}))
	must(t, db.InsertMessage(ParsedMessage{Source: SourceClaudeCode, UUID: "m_early", SessionID: "s1", Role: "user", Content: "earlier", Timestamp: "2026-03-28T10:00:00.100Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m_early" {
		t.Errorf("expected m_early (100ms), got %s", msgs[0].UUID)
	}
}

func TestUpdateSessionAgentName(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"))
	if err := db.UpdateSessionAgentName(SourceClaudeCode, "s1", "agent1", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionAgentName failed: %v", err)
	}

	var name string
	err := db.db.QueryRow("SELECT agent_name FROM sessions WHERE session_id='s1'").Scan(&name)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if name != "agent1" {
		t.Errorf("got %q, want %q", name, "agent1")
	}
}

func TestUpdateSessionAgentName_NoRow_IsNoop(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateSessionAgentName(SourceClaudeCode, "ghost", "agent", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionAgentName should not error on missing row: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id='ghost'").Scan(&count); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if count != 0 {
		t.Errorf("UpdateSessionAgentName should not create rows; got %d", count)
	}
}

func TestListSessions_CWD(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", CWD: "/Users/test/proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CWD != "/Users/test/proj" {
		t.Errorf("CWD: got %q, want %q", rows[0].CWD, "/Users/test/proj")
	}
}

func TestGetSession_CWD(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", CWD: "/Users/test/proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.CWD != "/Users/test/proj" {
		t.Errorf("CWD: got %q, want %q", got.CWD, "/Users/test/proj")
	}
}

func TestListSessions_CWD_Null(t *testing.T) {
	db := testDB(t)

	// Session with no CWD
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s1", "title", "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CWD != "" {
		t.Errorf("CWD should be empty for NULL cwd, got %q", rows[0].CWD)
	}
}
