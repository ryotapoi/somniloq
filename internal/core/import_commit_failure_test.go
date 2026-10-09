package core

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"modernc.org/sqlite"
)

func writeCommitFixture(t *testing.T, root, id, parent, body string) string {
	t.Helper()
	path := filepath.Join(root, id+".jsonl")
	data := fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":"/test","source":{"subagent":{"thread_spawn":{"parent_thread_id":%q}}}}}
{"type":"response_item","payload":{"type":"message","id":%q,"role":"user","content":[{"type":"input_text","text":%q}]}}
`, id, parent, id+"-message", body)
	must(t, os.WriteFile(path, []byte(data), 0600))
	return path
}

// The trigger synchronizes the reader lock with the second writer group,
// after the first independent group has committed, without timing assumptions.
func TestCodexCommitBusyPreservesGroupsAndConnection(t *testing.T) {
	root := testTempDir(t)
	path := filepath.Join(testTempDir(t), "import.db")
	var reader *sql.DB
	var readTx *sql.Tx
	fn := fmt.Sprintf("lock_reader_%d", time.Now().UnixNano())
	must(t, sqlite.RegisterScalarFunction(fn, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		if readTx == nil {
			var e error
			readTx, e = reader.Begin()
			if e != nil {
				return nil, e
			}
			var n int
			e = readTx.QueryRow("SELECT count(*) FROM messages").Scan(&n)
			if e != nil {
				return nil, e
			}
		}
		return int64(0), nil
	}))
	db, err := OpenDB(path)
	must(t, err)
	defer db.Close()
	bPath := writeCommitFixture(t, root, "b", "old-parent", "old body")
	runCodexImport(t, db, root, false)
	inputID, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	must(t, err)
	oldState, err := db.GetImportState(inputID, bPath)
	must(t, err)
	oldMessages, err := db.GetIdentityMessages(inputID, SourceCodex, rootIdentity("b"))
	must(t, err)
	writeCommitFixture(t, root, "a", "", "first committed")
	writeCommitFixture(t, root, "b", "new-parent", "uncommitted body")
	writeCommitFixture(t, root, "c", "", "later body")
	reader, err = sql.Open("sqlite", path)
	must(t, err)
	defer reader.Close()
	defer func() {
		if readTx != nil {
			readTx.Rollback()
		}
	}()

	_, err = db.db.Exec("CREATE TEMP TRIGGER lock_reader AFTER INSERT ON sessions WHEN NEW.session_id='b' BEGIN SELECT " + fn + "(); END")
	must(t, err)
	r, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}})
	must(t, err)
	if r.FilesImported != 1 || r.FilesFailed != 2 || len(r.Errors) != 2 {
		t.Fatalf("partial import: %+v", r)
	}
	for i, id := range []string{"b", "c"} {
		if !strings.Contains(r.Errors[i].Error(), id+":") || !strings.Contains(r.Errors[i].Error(), "SQLITE_BUSY") || strings.Contains(r.Errors[i].Error(), "within a transaction") {
			t.Errorf("failure diagnostic: %v", r.Errors[i])
		}
	}
	state, err := db.GetImportState(inputID, bPath)
	must(t, err)
	messages, err := db.GetIdentityMessages(inputID, SourceCodex, rootIdentity("b"))
	must(t, err)
	if !reflect.DeepEqual(oldState, state) || !reflect.DeepEqual(oldMessages, messages) {
		t.Fatal("failed group changed body or cursor")
	}
	var parent string
	must(t, db.db.QueryRow("SELECT parent_session_id FROM sessions WHERE session_id='b'").Scan(&parent))
	if parent != "old-parent" {
		t.Fatalf("failed group changed parent: %q", parent)
	}
	var a, c int
	must(t, db.db.QueryRow("SELECT count(*) FROM messages WHERE session_id='a'").Scan(&a))
	must(t, db.db.QueryRow("SELECT count(*) FROM messages WHERE session_id='c'").Scan(&c))
	if a != 1 || c != 0 {
		t.Fatalf("saved groups: a=%d c=%d", a, c)
	}
	must(t, readTx.Rollback())
	readTx = nil
	_, err = db.db.Exec("DROP TRIGGER lock_reader")
	must(t, err)
	retry := runCodexImport(t, db, root, false)
	if retry.FilesImported != 2 || retry.FilesSkipped != 1 {
		t.Fatalf("retry: %+v", retry)
	}
	repeat := runCodexImport(t, db, root, false)
	if repeat.FilesSkipped != 3 {
		t.Fatalf("repeat: %+v", repeat)
	}
	messages, err = db.GetIdentityMessages(inputID, SourceCodex, rootIdentity("b"))
	must(t, err)
	if len(messages) != 1 || messages[0].Content != "uncommitted body" || messages[0].Number != 1 {
		t.Fatalf("retry body: %+v", messages)
	}
}

func TestImportRetainsEarlierInputsAndFatalPartialResult(t *testing.T) {
	var db *DB
	fn := fmt.Sprintf("close_writer_%d", time.Now().UnixNano())
	must(t, sqlite.RegisterScalarFunction(fn, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		return int64(0), db.Close()
	}))
	db = testDB(t)
	first, second := testTempDir(t), testTempDir(t)
	writeCommitFixture(t, first, "a", "", "first input")
	writeCommitFixture(t, second, "b", "", "failed second input group")
	writeCommitFixture(t, second, "c", "", "second input committed")
	writeCommitFixture(t, second, "d", "", "not started")

	_, err := db.db.Exec("CREATE TEMP TRIGGER close_writer AFTER INSERT ON sessions WHEN NEW.session_id='c' BEGIN SELECT " + fn + "(); END")
	must(t, err)
	_, err = db.db.Exec("CREATE TEMP TRIGGER reject_group BEFORE INSERT ON sessions WHEN NEW.session_id='b' BEGIN SELECT RAISE(ABORT, 'initial group failure'); END")
	must(t, err)
	r, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: first}, {Source: SourceCodex, Root: second}}})
	if err == nil || !strings.Contains(err.Error(), "d: begin: sql: database is closed") {
		t.Fatalf("fatal: %v", err)
	}
	if r == nil || r.FilesImported != 2 || r.FilesScanned != 4 || r.FilesFailed != 2 || len(r.Errors) != 1 || !strings.Contains(r.Errors[0].Error(), "b: constraint failed: initial group failure") {
		t.Fatalf("partial result lost: %+v", r)
	}
}

func TestWriteTransactionRetainsMemoryAndForeignKeys(t *testing.T) {
	db := testDB(t)
	tx, err := db.Begin()
	must(t, err)
	_, err = tx.Exec("INSERT INTO inputs(input_key, source, root) VALUES('memory', 'codex', '/memory')")
	must(t, err)
	must(t, tx.Commit())
	tx, err = db.Begin()
	must(t, err)
	_, err = tx.Exec("DELETE FROM inputs")
	must(t, err)
	must(t, tx.Rollback())
	var n, fk int
	must(t, db.db.QueryRow("SELECT count(*) FROM inputs").Scan(&n))
	must(t, db.db.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	if n != 1 || fk != 1 {
		t.Fatalf("memory/fk: rows=%d fk=%d", n, fk)
	}
	// Exercise a replacement connection on a file database as well.
	fileDB, err := OpenDB(filepath.Join(testTempDir(t), "fk.db"))
	must(t, err)
	defer fileDB.Close()
	fileDB.db.SetMaxIdleConns(0)
	tx, err = fileDB.Begin()
	must(t, err)
	must(t, tx.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	must(t, tx.Rollback())
	if fk != 1 {
		t.Fatal("replacement connection disabled foreign keys")
	}
}
