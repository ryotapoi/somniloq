package core

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

// The adapter boundaries let separate processes pause without shipping hooks.
type pausedImportAdapter struct {
	ingest.Adapter
	phase string
	wait  func()
}

func (a pausedImportAdapter) ScanFiles(root string) ([]string, []error) {
	if a.phase == "full" {
		a.wait() // Import has already committed DeleteInputs.
	}
	return a.Adapter.ScanFiles(root)
}

func (a pausedImportAdapter) ProcessFile(newTx ingest.NewImportTransaction, path string, offset, size int64, at string) (ingest.ProcessResult, error) {
	if a.phase == "before" {
		a.wait() // core has selected the old offset, but has not begun saving.
	}
	if a.phase == "after" {
		original := newTx
		newTx = func() (ingest.ImportTransaction, error) {
			tx, err := original()
			if err == nil {
				a.wait() // The transaction has validated its state snapshot.
			}
			return tx, err
		}
	}
	return a.Adapter.ProcessFile(newTx, path, offset, size, at)
}

type concurrentImportOutcome struct {
	Imported int
	Skipped  int
	Failed   int
	Errors   []string
}

func TestConcurrentImportProcess(t *testing.T) {
	phase := os.Getenv("SOMNILOQ_IMPORT_TEST_PHASE")
	if phase == "" {
		return
	}
	db, err := OpenDB(os.Getenv("SOMNILOQ_IMPORT_TEST_DB"))
	must(t, err)
	defer db.Close()
	importSourceSpecs = []importSourceSpec{{
		source: ImportSourceClaudeCode,
		newAdapter: func() ingest.Adapter {
			return pausedImportAdapter{Adapter: claudecode.NewAdapter(ResolveRepoPath), phase: phase, wait: func() {
				fmt.Println("ready")
				var signal [1]byte
				_, err := io.ReadFull(os.Stdin, signal[:])
				must(t, err)
			}}
		},
	}}
	r, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceClaudeCode, Root: os.Getenv("SOMNILOQ_IMPORT_TEST_ROOT")}}, Full: phase == "full", Source: ImportSourceClaudeCode})
	must(t, err)
	outcome := concurrentImportOutcome{Imported: r.FilesImported, Skipped: r.FilesSkipped, Failed: r.FilesFailed}
	for _, err := range r.Errors {
		outcome.Errors = append(outcome.Errors, err.Error())
	}
	must(t, json.NewEncoder(os.Stdout).Encode(outcome))
}

type concurrentImportProcess struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	output *bufio.Scanner
	stderr bytes.Buffer
}

func startConcurrentImport(t *testing.T, dbPath, root, phase string) *concurrentImportProcess {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	p := &concurrentImportProcess{cmd: exec.CommandContext(ctx, os.Args[0], "-test.run=^TestConcurrentImportProcess$")}
	p.cmd.Env = append(os.Environ(), "SOMNILOQ_IMPORT_TEST_PHASE="+phase, "SOMNILOQ_IMPORT_TEST_DB="+dbPath, "SOMNILOQ_IMPORT_TEST_ROOT="+root)
	p.cmd.Stderr = &p.stderr
	var err error
	p.input, err = p.cmd.StdinPipe()
	must(t, err)
	output, err := p.cmd.StdoutPipe()
	must(t, err)
	p.output = bufio.NewScanner(output)
	must(t, p.cmd.Start())
	t.Cleanup(func() {
		cancel()
		p.input.Close()
		if p.cmd.ProcessState == nil {
			_ = p.cmd.Wait() // Reap only this helper, including on a failed assertion.
		}
	})
	if !p.output.Scan() || p.output.Text() != "ready" {
		t.Fatalf("%s did not reach checkpoint: %q (%v)", phase, p.output.Text(), p.output.Err())
	}
	return p
}

func (p *concurrentImportProcess) finish(t *testing.T) concurrentImportOutcome {
	t.Helper()
	_, err := p.input.Write([]byte{1})
	must(t, err)
	if !p.output.Scan() {
		t.Fatalf("helper did not return outcome: %v", p.output.Err())
	}
	var outcome concurrentImportOutcome
	must(t, json.Unmarshal(p.output.Bytes(), &outcome))
	for p.output.Scan() {
		// Drain the test runner's trailing PASS before Wait closes the pipe.
	}
	if err := p.cmd.Wait(); err != nil {
		t.Fatalf("helper failed: %v; stderr: %s", err, &p.stderr)
	}
	t.Logf("helper outcome: %+v", outcome)
	return outcome
}

func TestImportConcurrentFull(t *testing.T) {
	for _, phase := range []string{"before", "after"} {
		t.Run(phase, func(t *testing.T) {
			dir := testTempDir(t)
			dbPath := filepath.Join(dir, "import.db")
			root := filepath.Join(dir, "projects")
			project := filepath.Join(root, "project")
			must(t, os.MkdirAll(project, 0o755))
			path := filepath.Join(project, "session.jsonl")
			prefix := `{"type":"user","uuid":"prefix-id","sessionId":"s1","timestamp":"2026-01-01T00:00:00Z","message":{"role":"user","content":"original prefix"}}` + "\n"
			tail := `{"type":"assistant","uuid":"tail-id","sessionId":"s1","timestamp":"2026-01-01T00:00:01Z","message":{"role":"assistant","content":[{"type":"text","text":"appended tail"}]}}` + "\n"
			must(t, os.WriteFile(path, []byte(prefix), 0o644))
			db, err := OpenDB(dbPath)
			must(t, err)
			defer db.Close()
			if phase == "after" {
				// WAL permits a committed deletion after the reader's snapshot;
				// the before case exercises the default rollback journal.
				_, err = db.db.Exec("PRAGMA journal_mode=WAL")
				must(t, err)
			}
			opts := ImportOptions{Inputs: []Input{{Source: SourceClaudeCode, Root: root}}, Source: ImportSourceClaudeCode}
			r, err := Import(db, opts)
			must(t, err)
			if r.FilesImported != 1 || len(r.Errors) != 0 {
				t.Fatalf("seed import: %+v", r)
			}
			must(t, os.WriteFile(path, []byte(prefix+tail), 0o644))
			delta := startConcurrentImport(t, dbPath, root, phase)
			full := startConcurrentImport(t, dbPath, root, "full")
			deltaResult := delta.finish(t)
			state, err := db.GetImportState(testOnlyInput(t, db), path)
			must(t, err)
			if state != nil {
				t.Errorf("stale delta committed state after deletion: %+v", state)
			}
			fullResult := full.finish(t)
			assertBodies := func() {
				t.Helper()
				rows, err := db.db.Query("SELECT uuid, content FROM messages ORDER BY timestamp, rowid")
				must(t, err)
				defer rows.Close()
				var got []string
				for rows.Next() {
					var id, body string
					must(t, rows.Scan(&id, &body))
					got = append(got, id+":"+body)
				}
				must(t, rows.Err())
				t.Logf("DB bodies: %v", got)
				want := []string{"prefix-id:original prefix", "tail-id:appended tail"}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("bodies: got %v, want %v", got, want)
				}
			}
			assertBodies()
			r, err = Import(db, opts)
			must(t, err)
			t.Logf("next incremental canonicalizes the test wrapper cursor: %+v", r)
			assertBodies()
			if deltaResult.Failed != 1 || deltaResult.Imported != 0 || len(deltaResult.Errors) != 1 {
				t.Errorf("stale delta must report failure: %+v", deltaResult)
			} else if phase == "before" && !strings.Contains(deltaResult.Errors[0], "import state changed") {
				t.Errorf("unexpected stale-state error: %v", deltaResult.Errors)
			}
			if fullResult.Imported != 1 || fullResult.Failed != 0 || len(fullResult.Errors) != 0 || r.FilesImported != 1 || len(r.Errors) != 0 {
				t.Errorf("full / next import: %+v / %+v", fullResult, r)
			}
		})
	}
}
