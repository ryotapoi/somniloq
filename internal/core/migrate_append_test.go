package core

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
	"modernc.org/sqlite"
)

func appendMigrationMessage(path, id string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, `{"type":"response_item","ordinal":100,"payload":{"id":%q,"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`+"\n", id, id)
	return err
}

func TestMigrateAppendsRecoveredByImportMatchStaticInput(t *testing.T) {
	f := setupMigrationFixture(t, readMigrationOracle(t), migrationOracleCase{Files: []string{"01-child.jsonl", "02-multi.jsonl", "03-multi.jsonl"}})
	db, digest, _, err := prepareMigration(f.from, f.destination)
	must(t, err)
	defer db.Close()
	adapter := codex.NewMigrationAdapter(ResolveRepoPath)
	files, errs := adapter.ScanFiles(f.root)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	groups, errs := adapter.BuildMigrationIndex(f.root, files, "now")
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	// This append occurs after metadata discovery but before the owner's full read.
	child := filepath.Join(f.root, "01-child.jsonl")
	multi := filepath.Join(f.root, "02-multi.jsonl")
	must(t, appendMigrationMessage(multi, "after-index"))
	fn := fmt.Sprintf("append_migration_%d", time.Now().UnixNano())
	must(t, sqlite.RegisterScalarFunction(fn, 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		id := args[0].(string)
		if id == "child" {
			return int64(0), appendMigrationMessage(child, "after-body")
		}
		// The previous owner has committed; a new child was absent from discovery.
		if err := appendMigrationMessage(child, "while-next-owner"); err != nil {
			return nil, err
		}
		data := `{"type":"session_meta","payload":{"id":"later-child","source":{"subagent":{"thread_spawn":{"parent_thread_id":"child"}}}}}` + "\n" + `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"new child"}]}}` + "\n"
		return int64(0), os.WriteFile(filepath.Join(f.root, "later-child.jsonl"), []byte(data), 0600)
	}))
	must(t, db.Close())
	db, err = OpenDB(f.destination)
	must(t, err)
	defer db.Close()
	_, err = db.db.Exec("CREATE TEMP TRIGGER append_during_save AFTER INSERT ON sessions BEGIN SELECT " + fn + "(NEW.session_id); END")
	must(t, err)
	result := &MigrationResult{SnapshotSHA256: digest}
	must(t, replaceMigrationGroups(db, []migrationInput{{Input: f.inputs[0], Groups: groups}}, adapter, "now", result))
	if result.GroupsFailed != 0 || result.GroupsReplaced != 2 {
		t.Fatalf("%+v", result)
	}
	_, err = db.db.Exec("DROP TRIGGER append_during_save")
	must(t, err)
	imported, err := Import(db, ImportOptions{Inputs: f.inputs})
	must(t, err)
	if imported.FilesFailed != 0 || imported.FilesImported != 2 {
		t.Fatalf("followup=%+v", imported)
	}
	static, err := OpenDB(filepath.Join(t.TempDir(), "static.db"))
	must(t, err)
	defer static.Close()
	staticResult, err := Import(static, ImportOptions{Inputs: f.inputs})
	must(t, err)
	if staticResult.FilesFailed != 0 {
		t.Fatalf("static=%+v", staticResult)
	}
	for _, q := range []string{
		`SELECT printf('%s:%d:%s:%s:%s:%d',identity,number,content,membership,origin_path,origin_line) FROM messages ORDER BY identity,membership,origin_path,origin_line`,
		`SELECT printf('%s:%s:%s',identity,session_id,parent_session_id) FROM sessions ORDER BY identity`,
		`SELECT printf('%s:%d:%d:%s',jsonl_path,file_size,last_offset,content_hash) FROM import_state ORDER BY jsonl_path`,
		`SELECT printf('%s:%s:%s',identity,parent_identity,root_identity) FROM sessions ORDER BY identity`,
	} {
		got, want := migrationQueryStrings(t, db, q), migrationQueryStrings(t, static, q)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s\ngot=%v\nwant=%v", q, got, want)
		}
	}
	again, err := Import(db, ImportOptions{Inputs: f.inputs})
	must(t, err)
	if again.FilesSkipped != 4 || again.FilesFailed != 0 {
		t.Fatalf("repeat=%+v", again)
	}
}

func TestMigrateRejectsDestructiveChangesAfterBodyRead(t *testing.T) {
	for _, mutation := range []string{"delete", "move", "truncate", "replace", "same-size-edit"} {
		t.Run(mutation, func(t *testing.T) {
			f := setupMigrationFixture(t, readMigrationOracle(t), migrationOracleCase{Files: []string{"02-multi.jsonl", "03-multi.jsonl"}})
			db, digest, _, err := prepareMigration(f.from, f.destination)
			must(t, err)
			defer db.Close()
			path := filepath.Join(f.root, "02-multi.jsonl")
			original, err := os.ReadFile(path)
			must(t, err)
			fn := fmt.Sprintf("mutate_migration_%d", time.Now().UnixNano())
			must(t, sqlite.RegisterScalarFunction(fn, 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
				var e error
				switch mutation {
				case "delete":
					e = os.Remove(path)
				case "move":
					e = os.Rename(path, path+".moved")
				case "truncate":
					e = os.WriteFile(path, original[:len(original)/2], 0600)
				case "replace":
					e = os.Rename(path, path+".old")
					if e == nil {
						e = os.WriteFile(path, original, 0600)
					}
				case "same-size-edit":
					e = os.WriteFile(path, []byte(strings.Replace(string(original), "first", "other", 1)), 0600)
					if e == nil {
						e = os.Chtimes(path, time.Now(), time.Now().Add(time.Second))
					}
				}
				return int64(0), e
			}))
			must(t, db.Close())
			db, err = OpenDB(f.destination)
			must(t, err)
			defer db.Close()
			_, err = db.db.Exec("CREATE TEMP TRIGGER mutate_during_save AFTER INSERT ON sessions BEGIN SELECT " + fn + "(); END")
			must(t, err)
			adapter := codex.NewMigrationAdapter(ResolveRepoPath)
			files, errs := adapter.ScanFiles(f.root)
			if len(errs) != 0 {
				t.Fatal(errs)
			}
			groups, errs := adapter.BuildMigrationIndex(f.root, files, "now")
			if len(errs) != 0 {
				t.Fatal(errs)
			}
			result := &MigrationResult{SnapshotSHA256: digest}
			must(t, replaceMigrationGroups(db, []migrationInput{{Input: f.inputs[0], Groups: groups}}, adapter, "now", result))
			if result.GroupsFailed != 1 || result.GroupsReplaced != 0 {
				t.Fatalf("%+v", result)
			}
			if mutation == "delete" || mutation == "move" {
				if !errors.Is(result.Errors[0], os.ErrNotExist) {
					t.Fatalf("not missing-file failure: %v", result.Errors)
				}
			} else if !strings.Contains(result.Errors[0].Error(), "rollout changed during migration") {
				t.Fatalf("not changed-file failure: %v", result.Errors)
			}
			for _, table := range []string{"messages", "sessions", "import_state"} {
				var n int
				must(t, db.db.QueryRow("SELECT count(*) FROM "+table).Scan(&n))
				if n != 0 {
					t.Fatalf("partial %s=%d", table, n)
				}
			}
			var retained int
			must(t, db.db.QueryRow(`SELECT count(*) FROM legacy_messages WHERE session_id='multi'`).Scan(&retained))
			if retained != 2 {
				t.Fatalf("legacy=%d", retained)
			}
		})
	}
}
