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

func TestImportCmd_FullConfirmation(t *testing.T) {
	const prompt = "This will delete all data and re-import. Continue? [y/N] "
	const summary = "Imported 1 files (1 scanned, 0 skipped, 0 failed, 0 unparsed lines)\n"

	tests := []struct {
		name       string
		args       []string
		input      string
		isTTY      bool
		wantCode   int
		wantError  string
		wantOut    string
		wantErrOut string
		wantOpen   bool
		wantFull   bool
	}{
		{"non-TTY without yes", []string{"--full"}, "", false, 1, "--full requires confirmation; use --yes to skip in non-interactive mode", "", "", false, false},
		{"TTY rejects", []string{"--full"}, "n\n", true, 0, "", "", prompt, false, false},
		{"TTY confirms", []string{"--full"}, "y\n", true, 0, "", summary, prompt, true, true},
		{"non-TTY with yes", []string{"--full", "--yes"}, "", false, 0, "", summary, "", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			projectsDir := filepath.Join(dir, "projects")
			projectDir := filepath.Join(projectsDir, "-test-project")
			if err := os.MkdirAll(projectDir, 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			writeSession := func(sessionID, content string) string {
				path := filepath.Join(projectDir, sessionID+".jsonl")
				jsonl := `{"type":"user","uuid":"` + sessionID + `-u1","sessionId":"` + sessionID + `","timestamp":"2026-03-28T14:00:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"` + content + `"}}` + "\n"
				if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
					t.Fatalf("WriteFile(%s): %v", path, err)
				}
				return path
			}
			oldPath := writeSession("old", "old content")
			writeSession("kept", "kept content")
			dbPath := filepath.Join(dir, "somniloq.db")
			open := func() (*core.DB, error) { return core.OpenDB(dbPath) }

			var seedOut, seedErrOut bytes.Buffer
			code, err := importCmd([]string{"--source", "claude-code"}, open, projectsDir, filepath.Join(dir, "codex"), filepath.Join(dir, "cursor"), strings.NewReader(""), &seedOut, &seedErrOut, false)
			if code != 0 || err != nil {
				t.Fatalf("seed importCmd = (%d, %v), stdout = %q, stderr = %q", code, err, seedOut.String(), seedErrOut.String())
			}
			if got, want := seedOut.String(), "Imported 2 files (2 scanned, 0 skipped, 0 failed, 0 unparsed lines)\n"; got != want {
				t.Fatalf("seed stdout = %q, want %q", got, want)
			}
			if err := os.Remove(oldPath); err != nil {
				t.Fatalf("Remove(%s): %v", oldPath, err)
			}

			openCalls := 0
			countedOpen := func() (*core.DB, error) {
				openCalls++
				return open()
			}
			var out, errOut bytes.Buffer
			code, err = importCmd(append(tt.args, "--source", "claude-code"), countedOpen, projectsDir, filepath.Join(dir, "codex"), filepath.Join(dir, "cursor"), strings.NewReader(tt.input), &out, &errOut, tt.isTTY)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("error = %v, want nil", err)
				}
			} else if err == nil || err.Error() != tt.wantError {
				t.Errorf("error = %v, want %q", err, tt.wantError)
			}
			if got := out.String(); got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if got := errOut.String(); got != tt.wantErrOut {
				t.Errorf("stderr = %q, want %q", got, tt.wantErrOut)
			}
			wantOpenCalls := 0
			if tt.wantOpen {
				wantOpenCalls = 1
			}
			if openCalls != wantOpenCalls {
				t.Errorf("openDB calls = %d, want %d", openCalls, wantOpenCalls)
			}

			db, err := open()
			if err != nil {
				t.Fatalf("reopen DB: %v", err)
			}
			defer func() {
				if err := db.Close(); err != nil {
					t.Errorf("Close DB: %v", err)
				}
			}()
			oldSession, err := db.GetSession(core.SourceClaudeCode, "old")
			if err != nil {
				t.Fatalf("GetSession(old): %v", err)
			}
			wantOldPresent := !tt.wantFull
			if (oldSession != nil) != wantOldPresent {
				t.Errorf("old session = %v, want present = %t", oldSession, wantOldPresent)
			}
			oldMessages, err := db.GetMessages(core.SourceClaudeCode, "old")
			if err != nil {
				t.Fatalf("GetMessages(old): %v", err)
			}
			wantOld := 1
			if tt.wantFull {
				wantOld = 0
			}
			if len(oldMessages) != wantOld {
				t.Errorf("old messages = %v, want %d", oldMessages, wantOld)
			}
			keptMessages, err := db.GetMessages(core.SourceClaudeCode, "kept")
			if err != nil {
				t.Fatalf("GetMessages(kept): %v", err)
			}
			if len(keptMessages) != 1 || keptMessages[0].Content != "kept content" {
				t.Errorf("kept messages = %v, want one with original content", keptMessages)
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
	gotErr := errOut.String()
	errLines := strings.Split(gotErr, "\n")
	if len(errLines) != 2 || errLines[1] != "" {
		t.Fatalf("stderr = %q, want exactly one newline-terminated diagnostic line", gotErr)
	}
	wantErrPrefix := "  error: " + filepath.Join(projDir, "s1.jsonl") + ":2: "
	if !strings.HasPrefix(errLines[0], wantErrPrefix) {
		t.Errorf("stderr diagnostic = %q, want prefix %q", errLines[0], wantErrPrefix)
	} else if detail := strings.TrimPrefix(errLines[0], wantErrPrefix); detail == "" {
		t.Errorf("stderr diagnostic = %q, want non-empty detail after prefix %q", errLines[0], wantErrPrefix)
	}
}

func TestImportCmd_ErrorStderrWriteFailure(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	projectsPath := filepath.Join(dir, "projects")
	if err := os.WriteFile(projectsPath, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	code, err := importCmd([]string{"--source", "claude-code"}, staticDB(db), projectsPath, filepath.Join(dir, "codex"), filepath.Join(dir, "cursor"), strings.NewReader(""), &out, failWriter{}, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !errors.Is(err, errFailWriter) {
		t.Errorf("error = %v, want %v", err, errFailWriter)
	}
	if got, want := out.String(), "Imported 0 files (0 scanned, 0 skipped, 0 failed, 0 unparsed lines)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestImportCmd_UnparsedStderrWriteFailure(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	projDir := filepath.Join(dir, "project")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projDir, "session.jsonl"), []byte("{broken json\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	code, err := importCmd([]string{"--source", "claude-code"}, staticDB(db), dir, filepath.Join(dir, "codex"), filepath.Join(dir, "cursor"), strings.NewReader(""), &out, failWriter{}, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !errors.Is(err, errFailWriter) {
		t.Errorf("error = %v, want %v", err, errFailWriter)
	}
	if got, want := out.String(), "Imported 1 files (1 scanned, 0 skipped, 0 failed, 1 unparsed lines)\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
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
