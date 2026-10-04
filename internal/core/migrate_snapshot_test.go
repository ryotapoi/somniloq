package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeLegacySnapshot(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(script); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPrepareMigrationCopyAndRetry(t *testing.T) {
	script, err := os.ReadFile("../ingest/testdata/v0.14.0-migration/legacy.sql")
	if err != nil {
		t.Fatal(err)
	}
	from := makeLegacySnapshot(t, string(script))
	before, _ := os.ReadFile(from)
	destination := filepath.Join(t.TempDir(), "new.db")
	db, digest, copied, err := prepareMigration(from, destination)
	if err != nil {
		t.Fatal(err)
	}
	if !copied || len(digest) != 64 {
		t.Fatalf("copy=%v digest=%q", copied, digest)
	}
	var sessions, messages, cursors int
	db.db.QueryRow("SELECT COUNT(*) FROM legacy_sessions").Scan(&sessions)
	db.db.QueryRow("SELECT COUNT(*) FROM legacy_messages").Scan(&messages)
	db.db.QueryRow("SELECT COUNT(*) FROM import_state").Scan(&cursors)
	if sessions != 6 || messages != 8 || cursors != 0 {
		t.Fatalf("counts %d %d %d", sessions, messages, cursors)
	}
	var number int
	var timestamp, blocks sql.NullString
	var provenance string
	if err = db.db.QueryRow("SELECT number,timestamp,blocks_json,provenance FROM legacy_messages WHERE legacy_rowid=8").Scan(&number, &timestamp, &blocks, &provenance); err != nil {
		t.Fatal(err)
	}
	if number != 1 || timestamp.Valid || blocks.Valid || provenance != "legacy_saved" {
		t.Fatalf("legacy values %d %v %v %s", number, timestamp, blocks, provenance)
	}
	db.Close()
	copiedPath := filepath.Join(t.TempDir(), "same.db")
	os.WriteFile(copiedPath, before, 0600)
	db, again, copied, err := prepareMigration(copiedPath, destination)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if copied || again != digest {
		t.Fatal("retry copied")
	}
	after, _ := os.ReadFile(from)
	if string(before) != string(after) {
		t.Fatal("snapshot changed")
	}
	other := makeLegacySnapshot(t, legacySnapshotSchema+`INSERT INTO sessions(source,session_id,imported_at) VALUES('codex','new','');`)
	if _, _, _, err = prepareMigration(other, destination); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("other snapshot: %v", err)
	}
}

func TestPrepareMigrationSafetyBoundaries(t *testing.T) {
	for _, kind := range []string{"same", "symlink", "hardlink", "wal", "journal", "shm", "shape", "fk", "identity", "ordinary", "receipt"} {
		t.Run(kind, func(t *testing.T) {
			from := makeLegacySnapshot(t, legacySnapshotSchema)
			destination := filepath.Join(t.TempDir(), "target.db")
			switch kind {
			case "same":
				destination = from
			case "symlink":
				if err := os.Symlink(from, destination); err != nil {
					t.Fatal(err)
				}
			case "hardlink":
				if err := os.Link(from, destination); err != nil {
					t.Fatal(err)
				}
			case "wal", "journal", "shm":
				os.WriteFile(from+"-"+kind, nil, 0600)
			case "shape", "fk", "identity":
				db, _ := sql.Open("sqlite", from)
				statement := `ALTER TABLE sessions ADD COLUMN extra TEXT`
				if kind == "fk" {
					statement = `INSERT INTO messages(uuid,source,session_id,role,content,timestamp) VALUES('u','codex','missing','user','','')`
				}
				if kind == "identity" {
					statement = `INSERT INTO sessions(source,session_id,imported_at) VALUES('codex','','')`
				}
				if _, err := db.Exec(statement); err != nil {
					t.Fatal(err)
				}
				db.Close()
			case "ordinary":
				db, err := OpenDB(destination)
				if err != nil {
					t.Fatal(err)
				}
				db.Close()
			case "receipt":
				db, _, _, err := prepareMigration(from, destination)
				if err != nil {
					t.Fatal(err)
				}
				db.db.Exec("DELETE FROM migration_origin")
				db.Close()
			}
			before, _ := os.ReadFile(from)
			db, _, _, err := prepareMigration(from, destination)
			if db != nil {
				db.Close()
			}
			if err == nil {
				t.Fatal("accepted unsafe migration")
			}
			var schemaErr *SchemaError
			if errors.As(err, &schemaErr) != (kind == "shape") {
				t.Fatalf("error classification: %v", err)
			}
			after, _ := os.ReadFile(from)
			if string(before) != string(after) {
				t.Fatal("source modified")
			}
		})
	}
}

func TestLegacySnapshotSQLSpellingAndConstraint(t *testing.T) {
	from := makeLegacySnapshot(t, strings.ToLower(legacySnapshotSchema))
	db, _, _, err := prepareMigration(from, filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	from = makeLegacySnapshot(t, strings.Replace(legacySnapshotSchema, "CHECK(source <> '')", "CHECK(source != '')", 1))
	_, _, _, err = prepareMigration(from, filepath.Join(t.TempDir(), "target.db"))
	var schemaErr *SchemaError
	if !errors.As(err, &schemaErr) {
		t.Fatalf("constraint change: %v", err)
	}
}

func TestSnapshotCopyRollbackAndRetry(t *testing.T) {
	data, err := os.ReadFile("../ingest/testdata/v0.14.0-migration/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Cases []struct {
			Name     string
			Expected struct {
				Sessions int    `json:"committed_sessions"`
				Complete bool   `json:"copy_complete"`
				Retry    string `json:"retry_mode"`
			}
		}
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range oracle.Cases {
		if item.Name == "initial_copy_interrupt" {
			found = true
			if item.Expected.Sessions != 0 || item.Expected.Complete || item.Expected.Retry != "initial_copy" {
				t.Fatal("unexpected interruption oracle")
			}
		}
	}
	if !found {
		t.Fatal("missing interruption oracle")
	}

	sourcePath := makeLegacySnapshot(t, legacySnapshotSchema+`INSERT INTO sessions(source,session_id,imported_at) VALUES('codex','s',''); INSERT INTO messages(uuid,source,session_id,role,content,timestamp) VALUES('u','codex','s','user','saved','');`)
	source, _ := sql.Open("sqlite", sourcePath)
	defer source.Close()
	destination := filepath.Join(t.TempDir(), "target.db")
	target, _ := sql.Open("sqlite", destination)
	tx, _ := target.Begin()
	if _, err := tx.Exec(schema + `CREATE TRIGGER fail_copy BEFORE INSERT ON legacy_messages BEGIN SELECT RAISE(ABORT,'copy interrupted'); END;`); err != nil {
		t.Fatal(err)
	}
	if err := copySnapshotRows(source, tx, strings.Repeat("a", 64)); err == nil {
		t.Fatal("copy did not fail")
	}
	tx.Rollback()
	empty, err := inspectSchema(target)
	if err != nil || !empty {
		t.Fatalf("rollback left schema: %v %v", empty, err)
	}
	target.Close()
	db, _, copied, err := prepareMigration(sourcePath, destination)
	if err != nil || !copied {
		t.Fatalf("retry %v %v", copied, err)
	}
	db.Close()
}

func TestSnapshotFixtureRejections(t *testing.T) {
	data, err := os.ReadFile("../ingest/testdata/v0.14.0-migration/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Cases []struct {
			Name     string
			Mutation struct {
				SQL     string `json:"sql"`
				Session string `json:"legacy_session_id"`
			}
			Expected struct {
				Exit      int    `json:"exit_code"`
				Unchanged bool   `json:"destination_unchanged"`
				Created   bool   `json:"destination_created"`
				Reason    string `json:"reason"`
			}
		}
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, item := range oracle.Cases {
		if item.Name != "different_snapshot_retry" && item.Name != "ordinary_new_destination" && item.Name != "unknown_schema" {
			continue
		}
		found++
		t.Run(item.Name, func(t *testing.T) {
			from := makeLegacySnapshot(t, legacySnapshotSchema)
			destination := filepath.Join(t.TempDir(), "target.db")
			switch item.Name {
			case "different_snapshot_retry":
				db, _, _, err := prepareMigration(from, destination)
				if err != nil {
					t.Fatal(err)
				}
				db.Close()
				source, _ := sql.Open("sqlite", from)
				_, err = source.Exec("INSERT INTO sessions(source,session_id,imported_at) VALUES('codex',?,'')", item.Mutation.Session)
				source.Close()
				if err != nil {
					t.Fatal(err)
				}
			case "ordinary_new_destination":
				db, err := OpenDB(destination)
				if err != nil {
					t.Fatal(err)
				}
				db.Close()
			case "unknown_schema":
				source, _ := sql.Open("sqlite", from)
				_, err = source.Exec(item.Mutation.SQL)
				source.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			before, _ := os.ReadFile(destination)
			_, _, _, err := prepareMigration(from, destination)
			if err == nil {
				t.Fatal("accepted rejected fixture")
			}
			var schemaErr *SchemaError
			exit := 1
			if errors.As(err, &schemaErr) {
				exit = 2
			}
			if exit != item.Expected.Exit {
				t.Fatalf("exit %d want %d", exit, item.Expected.Exit)
			}
			if item.Expected.Unchanged {
				after, _ := os.ReadFile(destination)
				if string(before) != string(after) {
					t.Fatal("destination changed")
				}
			}
			if item.Name == "unknown_schema" {
				_, err := os.Stat(destination)
				if !os.IsNotExist(err) || item.Expected.Created {
					t.Fatal("destination created")
				}
			}
			reason := map[string]string{"snapshot_digest_mismatch": "snapshot digest mismatch", "missing_copy_receipt": "destination has no completed copy receipt"}[item.Expected.Reason]
			if reason != "" && !strings.Contains(err.Error(), reason) {
				t.Fatalf("reason %v", err)
			}
		})
	}
	if found != 3 {
		t.Fatalf("oracle cases %d", found)
	}
}
