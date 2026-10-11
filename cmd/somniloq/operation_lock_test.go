package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

// The child pauses at the real DB-open boundary, after import has acquired its lock.
func TestImportLockProcessHelper(t *testing.T) {
	path := os.Getenv("SOMNILOQ_IMPORT_LOCK_DB")
	if path == "" {
		return
	}
	open := func() (*core.DB, error) {
		fmt.Fprintln(os.Stdout, "ready")
		if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
			return nil, err
		}
		return openDB(path)
	}
	code, err := importConfiguredCmd(nil, open, config{DB: path}, strings.NewReader(""), io.Discard, io.Discard, false)
	if code != 0 || err != nil {
		t.Fatalf("import: %d %v", code, err)
	}
}

func startImportLockProcess(t *testing.T, path string) (*exec.Cmd, io.WriteCloser) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestImportLockProcessHelper$")
	cmd.Env = append(os.Environ(), "SOMNILOQ_IMPORT_LOCK_DB="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stdin.Close()
		if cmd.ProcessState == nil {
			cmd.Process.Kill()
			cmd.Wait()
		}
	})
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && line != "ready\n" {
			err = fmt.Errorf("unexpected readiness: %q", line)
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			t.Fatalf("child readiness: %v %s", err, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("child did not reach DB open")
	}
	return cmd, stdin
}

func TestImportOperationLockBeforeOpenAcrossProcesses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "history.db")
	cmd, release := startImportLockProcess(t, path)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("DB already touched: %v", err)
	}
	for _, args := range [][]string{nil, {"--source", "codex"}, {"--source", "cursor-agent", "--full", "--yes"}, {"--source", "claude-code", "--full", "--yes"}} {
		opened := false
		code, err := importConfiguredCmd(args, func() (*core.DB, error) { opened = true; return nil, errors.New("must not open") }, config{DB: path}, strings.NewReader(""), io.Discard, io.Discard, false)
		if code != 1 || !errors.Is(err, core.ErrOperationBusy) || opened {
			t.Fatalf("args=%v code=%d err=%v opened=%v", args, code, err, opened)
		}
	}
	code, err := migrateCmd([]string{"--from", filepath.Join(t.TempDir(), "missing")}, config{DB: path}, io.Discard, io.Discard)
	if code != 1 || !errors.Is(err, core.ErrOperationBusy) {
		t.Fatalf("migrate busy: %d %v", code, err)
	}
	other := filepath.Join(t.TempDir(), "other.db")
	code, err = importConfiguredCmd(nil, func() (*core.DB, error) { return openDB(other) }, config{DB: other}, strings.NewReader(""), io.Discard, io.Discard, false)
	if code != 0 || err != nil {
		t.Fatalf("other DB: %d %v", code, err)
	}
	fmt.Fprintln(release, "finish")
	release.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	code, err = importConfiguredCmd(nil, func() (*core.DB, error) { return openDB(path) }, config{DB: path}, strings.NewReader(""), io.Discard, io.Discard, false)
	if code != 0 || err != nil {
		t.Fatalf("after success: %d %v", code, err)
	}
	cmd, _ = startImportLockProcess(t, path)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	cmd.Wait()
	lock, err := core.LockOperation(path)
	if err != nil {
		t.Fatalf("after kill: %v", err)
	}
	lock.Close()
	code, err = importConfiguredCmd(nil, func() (*core.DB, error) { return nil, errors.New("open failed") }, config{DB: path}, strings.NewReader(""), io.Discard, io.Discard, false)
	if code != 1 || err == nil {
		t.Fatalf("open failure: %d %v", code, err)
	}
	lock, err = core.LockOperation(path)
	if err != nil {
		t.Fatalf("after open failure: %v", err)
	}
	lock.Close()
}
