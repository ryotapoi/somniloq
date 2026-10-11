package core

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func TestMigrationLockProcessHelper(t *testing.T) {
	destination := os.Getenv("SOMNILOQ_MIGRATION_LOCK_DB")
	if destination == "" {
		return
	}
	timeNow = func() string {
		fmt.Fprintln(os.Stdout, "ready")
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			t.Fatal(err)
		}
		return "2026-01-01T00:00:00Z"
	}
	if _, err := Migrate(os.Getenv("SOMNILOQ_MIGRATION_LOCK_FROM"), destination, nil); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationOperationLockAcrossProcesses(t *testing.T) {
	script, err := os.ReadFile("../ingest/testdata/v0.14.0-migration/legacy.sql")
	must(t, err)
	from := makeLegacySnapshot(t, string(script))
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_snapshot_retry=%t", retry), func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "new", "history.db")
			if retry {
				_, err := Migrate(from, destination, nil)
				must(t, err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestMigrationLockProcessHelper$")
			cmd.Env = append(os.Environ(), "SOMNILOQ_MIGRATION_LOCK_DB="+destination, "SOMNILOQ_MIGRATION_LOCK_FROM="+from)
			stdin, err := cmd.StdinPipe()
			must(t, err)
			stdout, err := cmd.StdoutPipe()
			must(t, err)
			cmd.Stderr = os.Stderr
			must(t, cmd.Start())
			defer func() {
				stdin.Close()
				if cmd.ProcessState == nil {
					cmd.Process.Kill()
					cmd.Wait()
				}
			}()
			ready := make(chan string, 1)
			go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
			select {
			case line := <-ready:
				if line != "ready\n" {
					t.Fatalf("readiness: %q", line)
				}
			case <-time.After(10 * time.Second):
				t.Fatal("migration did not reach input boundary")
			}
			if _, err := LockOperation(destination); !errors.Is(err, ErrOperationBusy) {
				t.Fatalf("import lock: %v", err)
			}
			if result, err := Migrate("missing", destination, nil); result != nil || !errors.Is(err, ErrOperationBusy) {
				t.Fatalf("migration: %+v %v", result, err)
			}
			fmt.Fprintln(stdin, "finish")
			stdin.Close()
			must(t, cmd.Wait())
			_, err = Migrate(from, destination, nil)
			must(t, err)
		})
	}
	// Canonical aliases share the lock, including a not-yet-created DB below a symlink.
	real := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(real, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	lock, err := LockOperation(filepath.Join(real, "nested", "new.db"))
	must(t, err)
	defer lock.Close()
	if _, err := LockOperation(filepath.Join(alias, "nested", "new.db")); !errors.Is(err, ErrOperationBusy) {
		t.Fatalf("alias lock: %v", err)
	}
}

func TestClaudeFullOperationLockPreservesLaterAppend(t *testing.T) {
	dir, err := CanonicalPath(t.TempDir(), "")
	must(t, err)
	root := filepath.Join(dir, "input")
	project := filepath.Join(root, "project")
	must(t, os.MkdirAll(project, 0700))
	path := filepath.Join(project, "session.jsonl")
	line := func(id, body string) string {
		return `{"type":"user","uuid":"` + id + `","sessionId":"s1","cwd":"/test","timestamp":"2026-01-01T00:00:01Z","message":{"role":"user","content":"` + body + `"}}` + "\n"
	}
	prefix := line("prefix", "original")
	tail := line("tail", "later")
	must(t, os.WriteFile(path, []byte(prefix), 0600))
	destination := filepath.Join(dir, "history.db")
	lock, err := LockOperation(destination)
	must(t, err)
	defer lock.Close()
	db, err := OpenDB(destination)
	must(t, err)
	defer db.Close()
	input := Input{Source: SourceClaudeCode, Root: root}
	id, err := db.EnsureInput(input)
	must(t, err)
	ready, release := make(chan struct{}), make(chan struct{})
	var resume sync.Once
	defer resume.Do(func() { close(release) })
	done := make(chan error, 1)
	go func() {
		result, err := importClaudeSnapshots(db, id, root, claudecode.NewAdapter(func(string) string { close(ready); <-release; return "" }), "2026-01-01T00:00:02Z", true)
		if err == nil && (result.FilesImported != 1 || len(result.Errors) > 0) {
			err = fmt.Errorf("full import: %+v", result)
		}
		done <- err
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("full did not read snapshot")
	}
	must(t, os.WriteFile(path, []byte(prefix+tail), 0600))
	if _, err := LockOperation(destination); !errors.Is(err, ErrOperationBusy) {
		t.Fatalf("later import: %v", err)
	}
	resume.Do(func() { close(release) })
	must(t, <-done)
	must(t, lock.Close())
	lock, err = LockOperation(destination)
	must(t, err)
	defer lock.Close()
	result, err := Import(db, ImportOptions{Inputs: []Input{input}})
	must(t, err)
	if result.FilesImported != 1 || len(result.Errors) > 0 {
		t.Fatalf("later import: %+v", result)
	}
	rows, err := db.db.Query(`SELECT content FROM messages ORDER BY number`)
	must(t, err)
	var bodies []string
	for rows.Next() {
		var body string
		must(t, rows.Scan(&body))
		bodies = append(bodies, body)
	}
	must(t, rows.Err())
	must(t, rows.Close())
	if !reflect.DeepEqual(bodies, []string{"original", "later"}) {
		t.Fatalf("bodies: %v", bodies)
	}
	state, err := db.GetImportState(id, path)
	must(t, err)
	if state == nil || state.FileSize != int64(len(prefix+tail)) || state.LastOffset != int64(len(prefix+tail)) {
		t.Fatalf("cursor: %+v", state)
	}
	var at string
	must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE input_id=?`, id).Scan(&at))
	if at != state.ImportedAt || strings.TrimSpace(at) == "" || at == "2026-01-01T00:00:02Z" {
		t.Fatalf("metadata time %q cursor %+v", at, state)
	}
}

func TestCodexFullOperationLockPreservesLaterRollout(t *testing.T) {
	dir, err := CanonicalPath(t.TempDir(), "")
	must(t, err)
	root := filepath.Join(dir, "input")
	must(t, os.MkdirAll(root, 0700))
	rollout := func(id, body string) string {
		return `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"/test"}}` + "\n" +
			`{"timestamp":"2026-01-01T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + body + `"}]}}` + "\n"
	}
	must(t, os.WriteFile(filepath.Join(root, "rollout-original.jsonl"), []byte(rollout("original", "original")), 0600))
	destination := filepath.Join(dir, "history.db")
	lock, err := LockOperation(destination)
	must(t, err)
	defer lock.Close()
	db, err := OpenDB(destination)
	must(t, err)
	defer db.Close()
	input := Input{Source: SourceCodex, Root: root}
	id, err := db.EnsureInput(input)
	must(t, err)
	ready, release := make(chan struct{}), make(chan struct{})
	var resume sync.Once
	defer resume.Do(func() { close(release) })
	done := make(chan error, 1)
	go func() {
		// Repository resolution occurs during body loading, after the full file list and owner index.
		result, err := importCodexGroups(db, id, root, codex.NewAdapter(func(string) string { close(ready); <-release; return "" }), "2026-01-01T00:00:02Z", true)
		if err == nil && (result.FilesScanned != 1 || result.FilesImported != 1 || len(result.Errors) > 0) {
			err = fmt.Errorf("full import: %+v", result)
		}
		done <- err
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("full did not reach body loading after scan")
	}
	path := filepath.Join(root, "rollout-later.jsonl")
	later := rollout("later", "later")
	must(t, os.WriteFile(path, []byte(later), 0600))
	if _, err := LockOperation(destination); !errors.Is(err, ErrOperationBusy) {
		t.Fatalf("later import: %v", err)
	}
	resume.Do(func() { close(release) })
	must(t, <-done)
	state, err := db.GetImportState(id, path)
	must(t, err)
	if state != nil {
		t.Fatalf("full imported rollout outside its scan: %+v", state)
	}
	must(t, lock.Close())
	lock, err = LockOperation(destination)
	must(t, err)
	defer lock.Close()
	result, err := Import(db, ImportOptions{Inputs: []Input{input}, Source: ImportSourceCodex})
	must(t, err)
	if result.FilesImported != 1 || result.FilesSkipped != 1 || len(result.Errors) > 0 {
		t.Fatalf("later import: %+v", result)
	}
	rows, err := db.db.Query(`SELECT content FROM messages ORDER BY content`)
	must(t, err)
	var bodies []string
	for rows.Next() {
		var body string
		must(t, rows.Scan(&body))
		bodies = append(bodies, body)
	}
	must(t, rows.Err())
	must(t, rows.Close())
	if !reflect.DeepEqual(bodies, []string{"later", "original"}) {
		t.Fatalf("bodies: %v", bodies)
	}
	state, err = db.GetImportState(id, path)
	must(t, err)
	hash, err := codex.HashFile(path, make([]byte, 4096))
	must(t, err)
	if state == nil || state.FileSize != int64(len(later)) || state.LastOffset != int64(len(later)) || state.ContentHash != hash {
		t.Fatalf("cursor: %+v", state)
	}
}
