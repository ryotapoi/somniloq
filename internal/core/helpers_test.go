package core

import (
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

func testInput(t *testing.T, db *DB, source Source) int64 {
	t.Helper()
	id, err := db.EnsureInput(Input{Source: source, Root: "/test-input/" + string(source)})
	must(t, err)
	return id
}

func testOnlyInput(t *testing.T, db *DB) int64 {
	t.Helper()
	var id int64
	var count int
	must(t, db.db.QueryRow(`SELECT COUNT(*), COALESCE(MIN(id), 0) FROM inputs`).Scan(&count, &id))
	if count == 0 {
		return testInput(t, db, SourceClaudeCode)
	}
	if count != 1 {
		t.Fatalf("expected one input, got %d", count)
	}
	return id
}

func testTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	return path
}
