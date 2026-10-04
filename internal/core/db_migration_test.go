package core

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenDB_ReturnsErrorForUnopenablePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing", "sessions.db")
	db, err := OpenDB(path)
	if db != nil {
		db.Close()
		t.Fatal("returned DB for missing parent")
	}
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenDB_RejectsUnsupportedWithoutMutation(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"legacy", `CREATE TABLE sessions(session_id TEXT PRIMARY KEY, project_dir TEXT, imported_at TEXT); INSERT INTO sessions VALUES('kept','project','before')`},
		{"unknown", `CREATE TABLE unrelated(body TEXT); INSERT INTO unrelated VALUES('kept')`},
		{"future", `PRAGMA user_version=2; CREATE TABLE future(body TEXT); INSERT INTO future VALUES('kept')`},
		{"unknown_revision_one", `PRAGMA user_version=1; CREATE TABLE unrelated(body TEXT)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sessions.db")
			raw, err := sql.Open("sqlite", path)
			must(t, err)
			_, err = raw.Exec(tc.sql)
			must(t, err)
			must(t, raw.Close())
			before, err := os.ReadFile(path)
			must(t, err)
			for _, open := range []func(string) (*DB, error){OpenDB, OpenDBRead} {
				db, err := open(path)
				if db != nil {
					db.Close()
					t.Fatal("unsupported DB was opened")
				}
				var schemaErr *SchemaError
				if !errors.As(err, &schemaErr) {
					t.Fatalf("error = %v, want SchemaError", err)
				}
				after, err := os.ReadFile(path)
				must(t, err)
				if !bytes.Equal(before, after) {
					t.Fatal("rejected DB bytes changed")
				}
			}
		})
	}
}

func TestOpenDBRead_MissingDoesNotCreate(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "missing")
	path := filepath.Join(parent, "sessions.db")
	db, err := OpenDBRead(path)
	if db != nil {
		db.Close()
		t.Fatal("missing DB was opened")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("missing parent was created: %v", err)
	}
}
