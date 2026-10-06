package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func TestImportSnapshotRejectsConcurrentAppend(t *testing.T) {
	for _, source := range []Source{SourceCodex, SourceClaudeCode} {
		for _, seeded := range []bool{false, true} {
			name := "initial"
			if seeded {
				name = "existing"
			}
			t.Run(string(source)+"/"+name, func(t *testing.T) {
				dir := testTempDir(t)
				root := filepath.Join(dir, "input")
				must(t, os.MkdirAll(root, 0755))
				project := filepath.Join(root, "project")
				must(t, os.MkdirAll(project, 0755))
				path := filepath.Join(project, "session.jsonl")
				line := func(body, id string) string {
					if source == SourceCodex {
						return `{"type":"response_item","timestamp":"2026-01-01T00:00:01Z","payload":{"type":"message","id":"` + id + `","role":"user","content":[{"type":"input_text","text":"` + body + `"}]}}` + "\n"
					}
					return `{"type":"user","uuid":"` + id + `","sessionId":"s1","cwd":"/test","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"` + body + `"}}` + "\n"
				}
				prefix := ""
				if source == SourceCodex {
					prefix = `{"type":"session_meta","payload":{"id":"s1","cwd":"/test"}}` + "\n"
				}
				prefix += line("original prefix", "prefix")
				must(t, os.WriteFile(path, []byte(prefix), 0600))
				dbPath := filepath.Join(dir, "import.db")
				db, err := OpenDB(dbPath)
				must(t, err)
				defer db.Close()
				other, err := OpenDB(dbPath)
				must(t, err)
				defer other.Close()
				inputID, err := db.EnsureInput(Input{Source: source, Root: root})
				must(t, err)
				run := func(db *DB, resolver ingest.RepoResolver, at string) (*ImportResult, error) {
					if source == SourceCodex {
						return importCodexGroups(db, inputID, root, codex.NewAdapter(resolver), at, false)
					}
					return importClaudeSnapshots(db, inputID, root, claudecode.NewAdapter(resolver), at, false)
				}
				checkSuccess := func(r *ImportResult, err error) {
					t.Helper()
					must(t, err)
					if r.FilesImported != 1 || r.FilesFailed != 0 || len(r.Errors) != 0 {
						t.Fatalf("import: %+v", r)
					}
				}
				resolver := func(string) string { return "" }
				if seeded {
					r, err := run(db, resolver, "2026-01-01T00:00:00Z")
					checkSuccess(r, err)
					// A must have a changed snapshot even when its expected cursor exists.
					prefix += line("first append", "first")
					must(t, os.WriteFile(path, []byte(prefix), 0600))
				}
				ready, release := make(chan struct{}), make(chan struct{})
				var resume, pause sync.Once
				defer resume.Do(func() { close(release) })
				type outcome struct {
					result *ImportResult
					err    error
				}
				done := make(chan outcome, 1)
				go func() {
					r, err := run(other, func(string) string {
						pause.Do(func() { close(ready); <-release })
						return ""
					}, "2026-01-01T00:00:01Z")
					done <- outcome{r, err}
				}()
				select {
				case <-ready:
				case <-time.After(10 * time.Second):
					t.Fatal("older import did not reach resolver after reading bytes")
				}
				must(t, os.WriteFile(path, []byte(prefix+line("newer tail", "tail")), 0600))
				r, err := run(db, resolver, "2026-01-01T00:00:02Z")
				checkSuccess(r, err)
				state, err := db.GetImportState(inputID, path)
				must(t, err)
				if state == nil || state.ImportedAt != "2026-01-01T00:00:02Z" || state.FileSize != int64(len(prefix+line("newer tail", "tail"))) {
					t.Fatalf("newer cursor: %+v", state)
				}
				readSaved := func() []string {
					t.Helper()
					rows, err := db.db.Query(`SELECT m.content, s.imported_at FROM messages m JOIN sessions s ON s.input_id=m.input_id AND s.identity=m.identity WHERE m.input_id=? ORDER BY m.number`, inputID)
					must(t, err)
					var saved []string
					for rows.Next() {
						var body, at string
						must(t, rows.Scan(&body, &at))
						saved = append(saved, body+"@"+at)
					}
					must(t, rows.Err())
					must(t, rows.Close())
					var at string
					must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE input_id=?`, inputID).Scan(&at))
					return append(saved, "session@"+at)
				}
				before := readSaved()
				want := []string{"original prefix@2026-01-01T00:00:02Z"}
				if seeded {
					want = append(want, "first append@2026-01-01T00:00:02Z")
				}
				want = append(want, "newer tail@2026-01-01T00:00:02Z", "session@2026-01-01T00:00:02Z")
				if !reflect.DeepEqual(before, want) {
					t.Fatalf("newer saved body/timestamps: %v, want %v", before, want)
				}
				resume.Do(func() { close(release) })
				var older outcome
				select {
				case older = <-done:
				case <-time.After(10 * time.Second):
					t.Fatal("older import did not finish")
				}
				must(t, older.err)
				if older.result.FilesFailed != 1 || older.result.FilesImported != 0 || len(older.result.Errors) != 1 || !strings.Contains(older.result.Errors[0].Error(), "import state changed during import") {
					t.Errorf("older snapshot must be rejected: %+v", older.result)
				}
				after, err := db.GetImportState(inputID, path)
				must(t, err)
				if !reflect.DeepEqual(state, after) {
					t.Errorf("cursor changed: before=%+v after=%+v", state, after)
				}
				if saved := readSaved(); !reflect.DeepEqual(before, saved) {
					t.Errorf("saved body/timestamps changed: before=%v after=%v", before, saved)
				}
			})
		}
	}
}
