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

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/cursoragent"
	"modernc.org/sqlite"
)

func writeCursorFullFixture(t *testing.T, root, id, body string) string {
	t.Helper()
	path := filepath.Join(root, "project", "agent-transcripts", id, id+".jsonl")
	must(t, os.MkdirAll(filepath.Dir(path), 0700))
	must(t, os.WriteFile(path, []byte(fmt.Sprintf("{\"role\":\"user\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":%q}]}}\n", body)), 0600))
	return path
}

// Compare every saved column, including session metadata and all file cursors.
func cursorFullSnapshot(t *testing.T, db *DB, inputID int64) map[string][][]any {
	t.Helper()
	snapshot := map[string][][]any{}
	for _, table := range []string{"messages", "sessions", "import_state"} {
		rows, err := db.db.Query("SELECT * FROM "+table+" WHERE input_id=? ORDER BY 1,2", inputID)
		must(t, err)
		columns, err := rows.Columns()
		must(t, err)
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			must(t, rows.Scan(pointers...))
			snapshot[table] = append(snapshot[table], values)
		}
		must(t, rows.Err())
		must(t, rows.Close())
	}
	return snapshot
}

type cursorFullFailureAdapter struct {
	cursoragent.Adapter
	scan          func(string) ([]string, []error)
	beforeProcess func(string)
}

func (a cursorFullFailureAdapter) ScanFiles(root string) ([]string, []error) {
	if a.scan != nil {
		return a.scan(root)
	}
	return a.Adapter.ScanFiles(root)
}

func (a cursorFullFailureAdapter) ProcessFile(newTx ingest.NewImportTransaction, path string, offset, size int64, at string) (ingest.ProcessResult, error) {
	if a.beforeProcess != nil {
		a.beforeProcess(path)
	}
	return a.Adapter.ProcessFile(newTx, path, offset, size, at)
}

func TestCursorFullFailurePreservesInput(t *testing.T) {
	for _, failure := range []string{"scan", "stat", "read", "session", "message", "cursor", "delete", "commit"} {
		t.Run(failure, func(t *testing.T) {
			root := testTempDir(t)
			dbPath := filepath.Join(testTempDir(t), "full.db")
			var reader *sql.DB
			var readTx *sql.Tx
			fn := fmt.Sprintf("cursor_lock_%d", time.Now().UnixNano())
			if failure == "commit" {
				must(t, sqlite.RegisterScalarFunction(fn, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
					var err error
					readTx, err = reader.Begin()
					if err != nil {
						return nil, err
					}
					var n int
					err = readTx.QueryRow("SELECT count(*) FROM messages").Scan(&n)
					return int64(0), err
				}))
			}
			db, err := OpenDB(dbPath)
			must(t, err)
			defer db.Close()
			a := writeCursorFullFixture(t, root, "a", "old a")
			b := writeCursorFullFixture(t, root, "b", "old b")
			opts := ImportOptions{Inputs: []Input{{Source: SourceCursorAgent, Root: root}}}
			r, err := Import(db, opts)
			must(t, err)
			if r.FilesImported != 2 || len(r.Errors) != 0 {
				t.Fatalf("initial: %+v", r)
			}
			id, err := db.EnsureInput(opts.Inputs[0])
			must(t, err)
			before := cursorFullSnapshot(t, db, id)
			writeCursorFullFixture(t, root, "a", "new a")
			writeCursorFullFixture(t, root, "b", "new b")
			adapter := cursorFullFailureAdapter{}
			switch failure {
			case "scan":
				adapter.scan = func(string) ([]string, []error) { return []string{a}, []error{fmt.Errorf("scan failed")} }
			case "stat":
				adapter.scan = func(string) ([]string, []error) { return []string{a, b + "-missing"}, nil }
			case "read":
				adapter.beforeProcess = func(path string) {
					if path == b {
						must(t, os.Remove(b))
						must(t, os.Mkdir(b, 0700))
					}
				}
			case "session", "message", "cursor", "delete":
				table, event, condition := "sessions", "INSERT", "NEW.session_id='b'"
				if failure == "message" {
					table = "messages"
				}
				if failure == "cursor" {
					table, condition = "import_state", "NEW.jsonl_path="+fmt.Sprintf("'%s'", b)
				}
				if failure == "delete" {
					table, event, condition = "sessions", "DELETE", "OLD.session_id='b'"
				}
				_, err := db.db.Exec("CREATE TEMP TRIGGER reject_full BEFORE " + event + " ON " + table + " WHEN " + condition + " BEGIN SELECT RAISE(ABORT, 'full failure'); END")
				must(t, err)
			case "commit":
				reader, err = sql.Open("sqlite", dbPath)
				must(t, err)
				defer reader.Close()
				defer func() {
					if readTx != nil {
						readTx.Rollback()
					}
				}()
				_, err = db.db.Exec("CREATE TEMP TRIGGER reject_full AFTER INSERT ON sessions WHEN NEW.session_id='b' BEGIN SELECT " + fn + "(); END")
				must(t, err)
			}
			if failure == "scan" || failure == "stat" || failure == "read" {
				r, err = replaceCursorInput(db, id, root, adapter, "2026-10-12T00:00:00Z")
			} else {
				opts.Full = true
				r, err = Import(db, opts)
			}
			must(t, err)
			if r.FilesImported != 0 || len(r.Errors) == 0 {
				t.Fatalf("failed full: %+v", r)
			}
			if failure == "commit" && !strings.Contains(r.Errors[0].Error(), "SQLITE_BUSY") {
				t.Fatalf("unexpected commit failure: %v", r.Errors)
			}
			if after := cursorFullSnapshot(t, db, id); !reflect.DeepEqual(before, after) {
				t.Fatalf("%s changed old input", failure)
			}
			if readTx != nil {
				must(t, readTx.Rollback())
				readTx = nil
			}
			if failure == "read" {
				must(t, os.Remove(b))
				writeCursorFullFixture(t, root, "b", "new b")
			}
			if failure == "session" || failure == "message" || failure == "cursor" || failure == "delete" || failure == "commit" {
				_, err = db.db.Exec("DROP TRIGGER reject_full")
				must(t, err)
			}
			opts.Full = true
			r, err = Import(db, opts)
			must(t, err)
			if r.FilesImported != 2 || len(r.Errors) != 0 {
				t.Fatalf("retry connection: %+v", r)
			}
		})
	}
}

func TestCursorFullReplacementAndSelection(t *testing.T) {
	db := testDB(t)
	root, other := testTempDir(t), testTempDir(t)
	a := writeCursorFullFixture(t, root, "a", "old a")
	b := writeCursorFullFixture(t, root, "b", "old b")
	d := writeCursorFullFixture(t, root, "d", "removed")
	writeCursorFullFixture(t, other, "outside", "outside")
	claude := testTempDir(t)
	opts := ImportOptions{Inputs: []Input{{Source: SourceCursorAgent, Root: root}, {Source: SourceCursorAgent, Root: other}, {Source: SourceClaudeCode, Root: claude}}}
	r, err := Import(db, opts)
	must(t, err)
	if len(r.Errors) != 0 {
		t.Fatalf("initial: %+v", r)
	}
	id, err := db.EnsureInput(opts.Inputs[0])
	must(t, err)
	otherID, err := db.EnsureInput(opts.Inputs[1])
	must(t, err)
	claudeID, err := db.EnsureInput(opts.Inputs[2])
	must(t, err)
	must(t, db.UpsertSession(claudeID, SessionMeta{Source: SourceClaudeCode, SessionID: "outside"}, "old"))
	outside := cursorFullSnapshot(t, db, otherID)
	outsideClaude := cursorFullSnapshot(t, db, claudeID)
	writeCursorFullFixture(t, root, "a", "new a") // Same size must still be replaced.
	must(t, os.Remove(b))
	must(t, os.Remove(d))
	writeCursorFullFixture(t, root, "c", "new c")
	empty := writeCursorFullFixture(t, root, "b", "ignored")
	must(t, os.WriteFile(empty, []byte("{\"role\":\"tool\"}\n"), 0600))
	opts.Full, opts.Source, opts.InputPaths = true, ImportSourceCursorAgent, []string{root, claude}
	r, err = Import(db, opts)
	must(t, err)
	if r.FilesImported != 3 || len(r.Errors) != 0 {
		t.Fatalf("full: %+v", r)
	}
	for inputID, before := range map[int64]map[string][][]any{otherID: outside, claudeID: outsideClaude} {
		if !reflect.DeepEqual(before, cursorFullSnapshot(t, db, inputID)) {
			t.Fatal("selected-out input changed")
		}
	}
	messages, err := db.GetIdentityMessages(id, SourceCursorAgent, rootIdentity("a"))
	must(t, err)
	if len(messages) != 1 || messages[0].Content != "new a" {
		t.Fatalf("replacement: %+v", messages)
	}
	var n int
	must(t, db.db.QueryRow("SELECT count(*) FROM sessions WHERE input_id=? AND session_id='b'", id).Scan(&n))
	if n != 0 {
		t.Fatal("missing body remained")
	}
	must(t, db.db.QueryRow("SELECT count(*) FROM sessions WHERE input_id=? AND session_id='d'", id).Scan(&n))
	if n != 0 {
		t.Fatal("removed log session remained")
	}
	for _, path := range []string{b, d} {
		state, err := db.GetImportState(id, path)
		must(t, err)
		if state != nil {
			t.Fatalf("removed body cursor remained: %+v", state)
		}
	}
	f, err := os.OpenFile(a, os.O_APPEND|os.O_WRONLY, 0600)
	must(t, err)
	_, err = f.WriteString("\n{\"role\":\"assistant\",\"message\":{\"content\":[{\"type\":\"text\",\"text\":\"appended\"}]}}\n")
	must(t, err)
	must(t, f.Close())
	opts.Full = false
	r, err = Import(db, opts)
	must(t, err)
	if len(r.Errors) != 0 {
		t.Fatalf("append: %+v", r)
	}
	messages, err = db.GetIdentityMessages(id, SourceCursorAgent, rootIdentity("a"))
	must(t, err)
	if len(messages) != 2 || messages[1].Content != "appended" || messages[1].Number != 2 || messages[1].OriginLine != 3 {
		t.Fatalf("append identity/order: %+v", messages)
	}
	// An empty or absent root is a successful empty replacement.
	must(t, os.RemoveAll(filepath.Join(root, "project")))
	opts.Full = true
	for i := 0; i < 2; i++ {
		r, err = Import(db, opts)
		must(t, err)
		if r.FilesImported != 0 || len(r.Errors) != 0 || len(cursorFullSnapshot(t, db, id)) != 0 {
			t.Fatalf("empty full: %+v", r)
		}
		must(t, os.RemoveAll(root))
	}
}
