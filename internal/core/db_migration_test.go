package core

import (
	"database/sql"
	"path/filepath"
	"testing"
)

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
