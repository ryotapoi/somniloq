package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestImportCmd_ConfirmationIOErrorDoesNotOpenDB(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name string
		in   io.Reader
		err  io.Writer
		want error
	}{
		{"prompt write", strings.NewReader("y\\n"), failWriter{}, errFailWriter},
		{"read", &readError{err: readErr}, &bytes.Buffer{}, readErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opened := false
			open := func() (*core.DB, error) {
				opened = true
				return nil, errors.New("openDB must not be called after confirmation I/O error")
			}
			code, err := importCmd([]string{"--full"}, open, "", "", "", tt.in, &bytes.Buffer{}, tt.err, true)
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if opened {
				t.Error("openDB was called after confirmation I/O error")
			}
		})
	}
}

// Pins the summary line scripts parse, including the unparsed-lines counter.
func TestImportCmd_OutputIncludesUnparsedLines(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	projDir := filepath.Join(dir, "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
{broken json
`
	if err := os.WriteFile(filepath.Join(projDir, "s1.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out, errOut bytes.Buffer
	code, err := importCmd([]string{"--source", "claude-code"}, staticDB(db), dir, filepath.Join(dir, "codex"), filepath.Join(dir, "cursor"), strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("importCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	want := "Imported 1 files (1 scanned, 0 skipped, 0 failed, 1 unparsed lines)\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	wantErr := "  error: " + filepath.Join(projDir, "s1.jsonl") + ":2: invalid character 'b' looking for beginning of object key string\n"
	if errOut.String() != wantErr {
		t.Errorf("stderr = %q, want %q", errOut.String(), wantErr)
	}
}

func TestImportCmd_CursorAgentRootWiring(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	cursorRoot := filepath.Join(dir, "cursor")
	path := filepath.Join(cursorRoot, "project", "agent-transcripts", "session", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"role":"user","message":{"content":[{"type":"text","text":"hello"}]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errOut bytes.Buffer
	code, err := importCmd([]string{"--source", "cursor-agent"}, staticDB(db), filepath.Join(dir, "claude"), filepath.Join(dir, "codex"), cursorRoot, strings.NewReader(""), &out, &errOut, false)
	if err != nil || code != 0 {
		t.Fatalf("importCmd = %d, %v (stderr: %q)", code, err, errOut.String())
	}
	if got, want := out.String(), "Imported 1 files (1 scanned, 0 skipped, 0 failed, 0 unparsed lines)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

// Pins the CLI contract for non-fatal scan failures: discovered files are
// still imported, the error goes to stderr, and the exit code is 1.
func TestImportCmd_ScanErrorExitsNonZero(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	projDir := filepath.Join(dir, "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
`
	if err := os.WriteFile(filepath.Join(projDir, "s1.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	badDir := filepath.Join(dir, "-Users-test-bad")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { os.Chmod(badDir, 0o755) })

	var out, errOut bytes.Buffer
	code, err := importCmd([]string{"--source", "claude-code"}, staticDB(db), dir, filepath.Join(dir, "codex"), filepath.Join(dir, "cursor"), strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("importCmd: %v", err)
	}
	if code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr: %q)", code, errOut.String())
	}
	want := "Imported 1 files (1 scanned, 0 skipped, 0 failed, 0 unparsed lines)\n"
	if out.String() != want {
		t.Errorf("stdout = %q, want %q", out.String(), want)
	}
	if !strings.Contains(errOut.String(), "error: scan "+badDir) {
		t.Errorf("stderr should report the scan error: %q", errOut.String())
	}
}
