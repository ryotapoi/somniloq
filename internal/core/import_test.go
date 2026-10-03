package core

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

func scanJSONLFiles(projectsDir string) ([]string, []error) {
	return claudecode.NewAdapter(ResolveRepoPath).ScanFiles(projectsDir)
}

func newImportTransaction(db *DB) ingest.NewImportTransaction {
	return func() (ingest.ImportTransaction, error) {
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		return importTx{tx: tx}, nil
	}
}

type failingFileAdapter struct {
	files     []string
	failPath  string
	failErr   error
	processed []string
}

func (a *failingFileAdapter) ScanFiles(string) ([]string, []error) {
	return a.files, nil
}

func (a *failingFileAdapter) ProcessFile(_ ingest.NewImportTransaction, path string, _, _ int64, _ string) (ingest.ProcessResult, error) {
	a.processed = append(a.processed, path)
	if path == a.failPath {
		return ingest.ProcessResult{}, a.failErr
	}
	return ingest.ProcessResult{}, nil
}

func TestImportWithAdapter_StatFailureContinues(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	missingPath := filepath.Join(dir, "missing.jsonl")
	successPath := filepath.Join(dir, "success.jsonl")
	if err := os.WriteFile(successPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := &failingFileAdapter{files: []string{missingPath, successPath}}

	result, err := importWithAdapter(db, dir, adapter)
	if err != nil {
		t.Fatalf("importWithAdapter failed: %v", err)
	}
	if result.FilesScanned != 2 || result.FilesFailed != 1 || result.FilesImported != 1 {
		t.Errorf("counts: got scanned=%d failed=%d imported=%d, want 2, 1, 1", result.FilesScanned, result.FilesFailed, result.FilesImported)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors: got %d, want 1: %v", len(result.Errors), result.Errors)
	}
	if !strings.Contains(result.Errors[0].Error(), missingPath+": stat:") || !errors.Is(result.Errors[0], os.ErrNotExist) {
		t.Errorf("stat error should retain path and cause: %v", result.Errors[0])
	}
	if !reflect.DeepEqual(adapter.processed, []string{successPath}) {
		t.Errorf("processed paths: got %v, want [%s]", adapter.processed, successPath)
	}
}

func TestImportWithAdapter_ProcessFileFailureContinues(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	failPath := filepath.Join(dir, "fail.jsonl")
	successPath := filepath.Join(dir, "success.jsonl")
	for _, path := range []string{failPath, successPath} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	processErr := errors.New("process failure")
	adapter := &failingFileAdapter{files: []string{failPath, successPath}, failPath: failPath, failErr: processErr}

	result, err := importWithAdapter(db, dir, adapter)
	if err != nil {
		t.Fatalf("importWithAdapter failed: %v", err)
	}
	if result.FilesScanned != 2 || result.FilesFailed != 1 || result.FilesImported != 1 {
		t.Errorf("counts: got scanned=%d failed=%d imported=%d, want 2, 1, 1", result.FilesScanned, result.FilesFailed, result.FilesImported)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("Errors: got %d, want 1: %v", len(result.Errors), result.Errors)
	}
	if !strings.Contains(result.Errors[0].Error(), failPath+":") || !errors.Is(result.Errors[0], processErr) {
		t.Errorf("process error should retain path and cause: %v", result.Errors[0])
	}
	if !reflect.DeepEqual(adapter.processed, []string{failPath, successPath}) {
		t.Errorf("processed paths: got %v, want [%s %s]", adapter.processed, failPath, successPath)
	}
}

func processFile(db *DB, path string, offset, fileSize int64, importedAt string) error {
	_, err := claudecode.NewAdapter(ResolveRepoPath).ProcessFile(newImportTransaction(db), path, offset, fileSize, importedAt)
	return err
}

func TestScanJSONLFiles(t *testing.T) {
	dir := t.TempDir()

	projA := filepath.Join(dir, "-Users-test-projA")
	projB := filepath.Join(dir, "-Users-test-projB")
	os.MkdirAll(projA, 0o755)
	os.MkdirAll(projB, 0o755)

	os.WriteFile(filepath.Join(projA, "sess1.jsonl"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(projA, "sess2.jsonl"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(projB, "sess3.jsonl"), []byte("{}"), 0o644)

	// Non-JSONL files should be excluded
	os.WriteFile(filepath.Join(projA, "notes.txt"), []byte("hi"), 0o644)

	// memory/ directory should be excluded
	memDir := filepath.Join(projA, "memory")
	os.MkdirAll(memDir, 0o755)
	os.WriteFile(filepath.Join(memDir, "data.md"), []byte("x"), 0o644)

	files, errs := scanJSONLFiles(dir)
	if len(errs) != 0 {
		t.Fatalf("ScanJSONLFiles failed: %v", errs)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(files), files)
	}

	found := map[string]bool{}
	for _, path := range files {
		found[filepath.Base(path)] = true
	}
	for _, name := range []string{"sess1.jsonl", "sess2.jsonl", "sess3.jsonl"} {
		if !found[name] {
			t.Errorf("missing file %s", name)
		}
	}
}

func TestProcessFile(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"user","uuid":"u1","parentUuid":"p1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"hi there"}]}}
{"type":"custom-title","customTitle":"test session","sessionId":"s1"}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	err := processFile(db, path, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}
	var title string
	err = db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	if title != "test session" {
		t.Errorf("title: got %q, want %q", title, "test session")
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&count); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if count != 2 {
		t.Errorf("messages: got %d, want 2", count)
	}
}

func TestProcessFile_ResolvesRepoPath(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/Users/test/projA/.claude/worktrees/feature-x","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"user","uuid":"u2","sessionId":"s2","timestamp":"2026-03-28T14:01:00Z","cwd":"/Users/test/projB/.claude/worktrees/feature-y","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"world"}}
{"type":"user","uuid":"u3","sessionId":"s3","timestamp":"2026-03-28T14:02:00Z","cwd":"","gitBranch":"","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
`
	path := filepath.Join(dir, "s.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	if err := processFile(db, path, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	for _, c := range []struct {
		sessionID string
		want      string
	}{
		{"s1", "/Users/test/projA"},
		{"s2", "/Users/test/projB"},
	} {
		var repoPath string
		if err := db.db.QueryRow("SELECT COALESCE(repo_path, '') FROM sessions WHERE session_id=?", c.sessionID).Scan(&repoPath); err != nil {
			t.Fatalf("%s SELECT failed: %v", c.sessionID, err)
		}
		if repoPath != c.want {
			t.Errorf("%s repo_path: got %q, want %q", c.sessionID, repoPath, c.want)
		}
	}

	var isNull int
	if err := db.db.QueryRow("SELECT repo_path IS NULL FROM sessions WHERE session_id='s3'").Scan(&isNull); err != nil {
		t.Fatalf("s3 SELECT failed: %v", err)
	}
	if isNull != 1 {
		t.Errorf("s3 repo_path should be NULL for empty cwd, got non-NULL")
	}
}

func TestImport_CountsUnparsedLines(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-Users-test-proj")
	os.MkdirAll(projDir, 0o755)

	// One valid record, one broken JSON line, one record with a malformed
	// message envelope, and one deliberately ignored record type.
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
{broken json
{"type":"user","uuid":"u2","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":"not an envelope object"}
{"type":"summary","summary":"ignored record type"}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if res.FilesImported != 1 {
		t.Fatalf("FilesImported: got %d, want 1", res.FilesImported)
	}
	if res.UnparsedLines != 2 {
		t.Errorf("UnparsedLines: got %d, want 2", res.UnparsedLines)
	}
	if len(res.UnparsedDiagnostics) != 2 {
		t.Fatalf("UnparsedDiagnostics: got %d, want 2: %v", len(res.UnparsedDiagnostics), res.UnparsedDiagnostics)
	}
	for i, wantPrefix := range []string{
		path + ":2: ",
		path + ":3: ",
	} {
		got := res.UnparsedDiagnostics[i].Error()
		if !strings.HasPrefix(got, wantPrefix) {
			t.Errorf("UnparsedDiagnostics[%d] = %q, want prefix %q", i, got, wantPrefix)
			continue
		}
		if detail := strings.TrimPrefix(got, wantPrefix); detail == "" {
			t.Errorf("UnparsedDiagnostics[%d] = %q, want non-empty detail after prefix %q", i, got, wantPrefix)
		}
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&count); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if count != 1 {
		t.Errorf("messages: got %d, want 1", count)
	}
}

func TestImportResultAdd_CapsDiagnosticsAcrossBatches(t *testing.T) {
	diagnostics := []error{
		errors.New("first"), errors.New("second"), errors.New("third"),
		errors.New("fourth"), errors.New("fifth"), errors.New("sixth"),
		errors.New("seventh"), errors.New("eighth"),
	}
	want := []error{diagnostics[0], diagnostics[1], diagnostics[2], diagnostics[3], diagnostics[4]}
	var result ImportResult

	result.add(&ImportResult{UnparsedDiagnostics: diagnostics[:2]})
	result.add(&ImportResult{UnparsedDiagnostics: diagnostics[2:6]})
	if !reflect.DeepEqual(result.UnparsedDiagnostics, want) {
		t.Fatalf("diagnostics at cap = %v, want %v", result.UnparsedDiagnostics, want)
	}

	result.add(&ImportResult{UnparsedDiagnostics: diagnostics[6:]})
	if !reflect.DeepEqual(result.UnparsedDiagnostics, want) {
		t.Errorf("diagnostics after cap = %v, want %v", result.UnparsedDiagnostics, want)
	}
}

func TestImport_ReportsClaudeCodeDiagnosticLineAfterOffset(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projDir, "s1.jsonl")
	first := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}` + "\n"
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("initial Import failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(first+"{broken json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("incremental Import failed: %v", err)
	}
	if res.UnparsedLines != 1 || len(res.UnparsedDiagnostics) != 1 {
		t.Fatalf("incremental diagnostics: lines=%d diagnostics=%v", res.UnparsedLines, res.UnparsedDiagnostics)
	}
	got := res.UnparsedDiagnostics[0].Error()
	wantPrefix := path + ":2: "
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("diagnostic = %q, want prefix %q", got, wantPrefix)
	} else if detail := strings.TrimPrefix(got, wantPrefix); detail == "" {
		t.Errorf("diagnostic = %q, want non-empty detail after prefix %q", got, wantPrefix)
	}
}

func TestImport_ReportsClaudeCodeDiagnosticOnUnterminatedPrefixLine(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-Users-test-proj")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projDir, "s1.jsonl")
	first := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}`
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("initial Import failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(first+"{broken json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("incremental Import failed: %v", err)
	}
	if res.UnparsedLines != 1 || len(res.UnparsedDiagnostics) != 1 {
		t.Fatalf("incremental diagnostics: lines=%d diagnostics=%v", res.UnparsedLines, res.UnparsedDiagnostics)
	}
	got := res.UnparsedDiagnostics[0].Error()
	wantPrefix := path + ":1: "
	if !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("diagnostic = %q, want prefix %q", got, wantPrefix)
	} else if detail := strings.TrimPrefix(got, wantPrefix); detail == "" {
		t.Errorf("diagnostic = %q, want non-empty detail after prefix %q", got, wantPrefix)
	}
}

func TestProcessFile_EmptyFile(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	err := processFile(db, path, 0, 0, "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}
	state, err := db.GetImportState(path)
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if state != nil {
		t.Fatalf("empty file must not create import_state, got %+v", state)
	}
}

func TestProcessFile_SkipsEmptyContent(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	// tool_use only message: ExtractText returns "" for this content
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	err := processFile(db, path, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	// Only the user message should be saved (tool_use-only assistant message skipped)
	var msgCount int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&msgCount); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if msgCount != 1 {
		t.Errorf("messages: got %d, want 1 (empty content skipped)", msgCount)
	}

	// Session should still be created (upsertSession called for all messages)
	var sessCount int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id='s1'").Scan(&sessCount); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if sessCount != 1 {
		t.Errorf("session should exist even for empty content messages")
	}
}

func TestProcessFile_SkipsWhitespaceOnlyContent(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":"   \n  "}}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	err := processFile(db, path, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	var msgCount int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&msgCount); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if msgCount != 1 {
		t.Errorf("messages: got %d, want 1 (whitespace-only content skipped)", msgCount)
	}
}

func TestImport_FileShrink(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-test-proj")
	os.MkdirAll(projDir, 0o755)

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"original"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("first Import failed: %v", err)
	}

	// Shrink file (simulate truncate/recreate)
	smallJsonl := `{"type":"user","uuid":"u3","sessionId":"s1","timestamp":"2026-03-28T15:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"new"}}
`
	os.WriteFile(path, []byte(smallJsonl), 0o644)

	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("Import after shrink failed: %v", err)
	}
	if res.FilesImported != 1 {
		t.Errorf("shrunk file should be re-imported, got imported=%d", res.FilesImported)
	}

	// Should have 3 messages total (2 old + 1 new, old not deleted)
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 messages (orphans retained), got %d", count)
	}

	state, err := db.GetImportState(path)
	if err != nil {
		t.Fatalf("GetImportState: %v", err)
	}
	if state == nil {
		t.Fatal("import_state missing after shrink import")
	}
	if state.FileSize != int64(len(smallJsonl)) || state.LastOffset != int64(len(smallJsonl)) {
		t.Errorf("import_state = {FileSize:%d LastOffset:%d}, want both %d", state.FileSize, state.LastOffset, len(smallJsonl))
	}
}

func TestImport_AllSources(t *testing.T) {
	db := testDB(t)
	claudeRoot := t.TempDir()
	codexRoot := t.TempDir()
	cursorRoot := t.TempDir()

	claudeProjectDir := filepath.Join(claudeRoot, "-test-claude")
	if err := os.MkdirAll(claudeProjectDir, 0o755); err != nil {
		t.Fatalf("MkdirAll Claude dir failed: %v", err)
	}
	claudeJSONL := `{"type":"user","uuid":"claude-u1","sessionId":"claude-s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/claude","gitBranch":"main","version":"2.1.86","message":{"role":"user","content":"hello claude"}}
`
	if err := os.WriteFile(filepath.Join(claudeProjectDir, "claude-s1.jsonl"), []byte(claudeJSONL), 0o644); err != nil {
		t.Fatalf("WriteFile Claude JSONL failed: %v", err)
	}

	codexDir := filepath.Join(codexRoot, "2026", "05", "01")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		t.Fatalf("MkdirAll Codex dir failed: %v", err)
	}
	codexJSONL := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-s1","timestamp":"2026-05-01T00:00:00.000Z","cwd":"/nonexistent/codex","cli_version":"0.128.0"}}
{"timestamp":"2026-05-01T00:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello codex"}]}}
`
	if err := os.WriteFile(filepath.Join(codexDir, "rollout-codex-s1.jsonl"), []byte(codexJSONL), 0o644); err != nil {
		t.Fatalf("WriteFile Codex JSONL failed: %v", err)
	}
	cursorDir := filepath.Join(cursorRoot, "project", "agent-transcripts", "cursor-s1")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		t.Fatalf("MkdirAll Cursor dir failed: %v", err)
	}
	cursorJSONL := `{"role":"user","message":{"content":[{"type":"text","text":"hello cursor"}]}}
`
	if err := os.WriteFile(filepath.Join(cursorDir, "cursor-s1.jsonl"), []byte(cursorJSONL), 0o644); err != nil {
		t.Fatalf("WriteFile Cursor JSONL failed: %v", err)
	}

	result, err := Import(db, ImportOptions{
		ProjectsDir:       claudeRoot,
		CodexSessionsDir:  codexRoot,
		CursorProjectsDir: cursorRoot,
		Source:            ImportSourceAll,
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.FilesImported != 3 || result.FilesScanned != 3 || len(result.Errors) != 0 {
		t.Fatalf("Import result: %+v", result)
	}

	for _, c := range []struct {
		source    Source
		sessionID string
	}{
		{SourceClaudeCode, "claude-s1"},
		{SourceCodex, "codex-s1"},
		{SourceCursorAgent, "cursor-s1"},
	} {
		var count int
		if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE source=? AND session_id=?", c.source, c.sessionID).Scan(&count); err != nil {
			t.Fatalf("COUNT %s/%s failed: %v", c.source, c.sessionID, err)
		}
		if count != 1 {
			t.Errorf("%s/%s messages: got %d, want 1", c.source, c.sessionID, count)
		}
	}
}

func TestImport_InvalidSourceDoesNotDelete(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "kept"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{
		Source:    SourceClaudeCode,
		UUID:      "kept-message",
		SessionID: "kept",
		Role:      "user",
		Content:   "keep me",
		Timestamp: "2026-03-28T15:00:00Z",
	}))

	if _, err := Import(db, ImportOptions{Full: true, Source: ImportSource("bad")}); err == nil {
		t.Fatal("Import should reject an unknown source")
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE uuid='kept-message'").Scan(&count); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if count != 1 {
		t.Errorf("message count after failed import: got %d, want 1", count)
	}
}

func TestImportSourceChoicesMatchValidSources(t *testing.T) {
	choices := ImportSourceChoices()
	if len(choices) != len(importSourceSpecs)+1 {
		t.Fatalf("choices length: got %d (%v), want %d specs plus all", len(choices), choices, len(importSourceSpecs))
	}
	if choices[0] != string(ImportSourceAll) {
		t.Fatalf("choices[0]: got %q, want %q", choices[0], ImportSourceAll)
	}
	seen := map[string]bool{choices[0]: true}
	for i, spec := range importSourceSpecs {
		choice := choices[i+1]
		if choice != string(spec.source) {
			t.Fatalf("choices[%d]: got %q, want %q", i+1, choice, spec.source)
		}
		if !ImportSource(choice).Valid() {
			t.Fatalf("choice %q must be valid", choice)
		}
		if seen[choice] {
			t.Fatalf("choice %q appears more than once", choice)
		}
		seen[choice] = true
	}
}

func TestScanJSONLFiles_NonexistentDir(t *testing.T) {
	files, errs := scanJSONLFiles("/nonexistent/path")
	if len(errs) != 0 {
		t.Fatalf("ScanJSONLFiles should not error on missing dir: %v", errs)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestScanJSONLFiles_UnreadableProjectDirIsNonFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	dir := t.TempDir()
	projA := filepath.Join(dir, "-Users-test-projA")
	os.MkdirAll(projA, 0o755)
	os.WriteFile(filepath.Join(projA, "sess1.jsonl"), []byte("{}"), 0o644)
	projBad := filepath.Join(dir, "-Users-test-projBad")
	os.MkdirAll(projBad, 0o755)
	if err := os.Chmod(projBad, 0o000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() { os.Chmod(projBad, 0o755) })

	files, errs := scanJSONLFiles(dir)
	if len(errs) != 1 {
		t.Fatalf("got %d scan errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), projBad) {
		t.Errorf("scan error should name the unreadable dir: %v", errs[0])
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}
	if filepath.Base(files[0]) != "sess1.jsonl" {
		t.Errorf("path: got %q, want sess1.jsonl", files[0])
	}
}

func TestImport_ScanErrorsAreNonFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	db := testDB(t)
	projectsDir := t.TempDir()
	projA := filepath.Join(projectsDir, "-Users-test-projA")
	os.MkdirAll(projA, 0o755)
	jsonl := `{"type":"user","uuid":"u1","parentUuid":null,"sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
`
	os.WriteFile(filepath.Join(projA, "s1.jsonl"), []byte(jsonl), 0o644)
	projBad := filepath.Join(projectsDir, "-Users-test-projBad")
	os.MkdirAll(projBad, 0o755)
	if err := os.Chmod(projBad, 0o000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() { os.Chmod(projBad, 0o755) })

	result, err := Import(db, ImportOptions{
		ProjectsDir:      projectsDir,
		CodexSessionsDir: filepath.Join(projectsDir, "no-codex-sessions"),
		Source:           ImportSourceAll,
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.FilesImported != 1 {
		t.Errorf("FilesImported: got %d, want 1", result.FilesImported)
	}
	if len(result.Errors) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(result.Errors), result.Errors)
	}
	if !strings.Contains(result.Errors[0].Error(), projBad) {
		t.Errorf("error should name the unreadable dir: %v", result.Errors[0])
	}
}

func TestProcessFile_MetaOnly_NoSessionRow(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"custom-title","customTitle":"meta only","sessionId":"meta1"}
`
	path := filepath.Join(dir, "meta1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	if err := processFile(db, path, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if count != 0 {
		t.Errorf("meta-only JSONL should not create a session row; got %d", count)
	}
}

func TestProcessFile_AgentNameOnly_NoSessionRow(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"agent-name","agentName":"orphan","sessionId":"meta1"}
`
	path := filepath.Join(dir, "meta1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	if err := processFile(db, path, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&count); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if count != 0 {
		t.Errorf("agent-name-only JSONL should not create a session row; got %d", count)
	}
}

func TestImport_MetaBeforeBody_AcrossInvocations(t *testing.T) {
	// First Import sees only meta records; the file's import_state must NOT
	// advance, so a later Import that sees a body record can re-read the meta
	// from offset 0 and apply the title/agent_name to the freshly created row.
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-test-proj")
	os.MkdirAll(projDir, 0o755)

	metaOnly := `{"type":"custom-title","customTitle":"early title","sessionId":"s1"}
{"type":"agent-name","agentName":"early-agent","sessionId":"s1"}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(metaOnly), 0o644)

	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("first Import failed: %v", err)
	}

	// import_state for a meta-only file must remain unset, so the next Import
	// re-reads from offset 0 once the body is appended.
	state, err := db.GetImportState(path)
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if state != nil {
		t.Fatalf("import_state should be nil after meta-only Import, got %+v", state)
	}

	appended := metaOnly + `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
`
	os.WriteFile(path, []byte(appended), 0o644)

	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("second Import failed: %v", err)
	}

	var title, name string
	if err := db.db.QueryRow("SELECT COALESCE(custom_title, ''), COALESCE(agent_name, '') FROM sessions WHERE session_id='s1'").Scan(&title, &name); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "early title" {
		t.Errorf("custom_title: got %q, want %q", title, "early title")
	}
	if name != "early-agent" {
		t.Errorf("agent_name: got %q, want %q", name, "early-agent")
	}
}

func TestImport_MetaAfterBody_AcrossInvocations(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-test-proj")
	os.MkdirAll(projDir, 0o755)

	body := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(body), 0o644)

	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("first Import failed: %v", err)
	}

	appended := body + `{"type":"custom-title","customTitle":"later title","sessionId":"s1"}
{"type":"agent-name","agentName":"later-agent","sessionId":"s1"}
`
	os.WriteFile(path, []byte(appended), 0o644)

	if _, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode}); err != nil {
		t.Fatalf("second Import failed: %v", err)
	}

	var title, name string
	if err := db.db.QueryRow("SELECT COALESCE(custom_title, ''), COALESCE(agent_name, '') FROM sessions WHERE session_id='s1'").Scan(&title, &name); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "later title" {
		t.Errorf("custom_title: got %q, want %q", title, "later title")
	}
	if name != "later-agent" {
		t.Errorf("agent_name: got %q, want %q", name, "later-agent")
	}
}

func TestImport_LinkedWorktreeProjects(t *testing.T) {
	repo, worktree := linkedWorktree(t)
	foreignRepo, foreignWorktree := linkedWorktree(t)
	foreignGitDir, err := exec.Command("git", "-C", foreignWorktree, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	db := testDB(t)
	claudeRoot, codexRoot := t.TempDir(), t.TempDir()
	project := filepath.Join(claudeRoot, "project")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	for i, cwd := range []string{sub, worktree, other} {
		claude := fmt.Sprintf(`{"type":"user","uuid":"u%d","sessionId":"claude-%d","timestamp":"2026-05-01T00:00:00Z","cwd":%q,"message":{"role":"user","content":"hello"}}`+"\n", i, i, cwd)
		codex := fmt.Sprintf(`{"timestamp":"2026-05-01T00:00:00Z","type":"session_meta","payload":{"id":"codex-%d","cwd":%q}}
{"timestamp":"2026-05-01T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`, i, cwd)
		for path, body := range map[string]string{
			filepath.Join(project, fmt.Sprintf("claude-%d.jsonl", i)):    claude,
			filepath.Join(codexRoot, fmt.Sprintf("rollout-%d.jsonl", i)): codex,
		} {
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Discovery must use the log cwd even when the caller targets another repo.
	t.Setenv("GIT_DIR", strings.TrimRight(string(foreignGitDir), "\r\n"))
	t.Setenv("GIT_WORK_TREE", foreignWorktree)
	t.Setenv("GIT_COMMON_DIR", filepath.Join(foreignRepo, ".git"))
	result, err := Import(db, ImportOptions{ProjectsDir: claudeRoot, CodexSessionsDir: codexRoot, CursorProjectsDir: t.TempDir(), Source: ImportSourceAll})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesImported != 6 || len(result.Errors) != 0 || result.UnparsedLines != 0 {
		t.Fatalf("Import: %+v", result)
	}
	stored, err := db.ListSessions(SessionFilter{})
	if err != nil || len(stored) != 6 {
		t.Fatalf("stored sessions: %+v, %v", stored, err)
	}
	for _, session := range stored {
		want := repo
		if session.SessionID == "claude-2" || session.SessionID == "codex-2" {
			want = other
		}
		if session.RepoPath != want {
			t.Errorf("%s repo_path = %q, want %q", session.SessionID, session.RepoPath, want)
		}
	}
	// Old non-NULL identities survive unchanged differential import.
	if _, err := db.db.Exec("UPDATE sessions SET repo_path=? WHERE session_id IN ('claude-1', 'codex-1')", foreignRepo); err != nil {
		t.Fatal(err)
	}
	opts := ImportOptions{ProjectsDir: claudeRoot, CodexSessionsDir: codexRoot, CursorProjectsDir: t.TempDir(), Source: ImportSourceAll}
	result, err = Import(db, opts)
	if err != nil || result.FilesSkipped != 6 {
		t.Fatalf("unchanged import: %+v, %v", result, err)
	}
	old, err := db.ListSessions(SessionFilter{Projects: []string{foreignRepo}})
	if err != nil || len(old) != 2 {
		t.Fatalf("old identities: %+v, %v", old, err)
	}
	// Existing full import rebuilds the saved identity from the surviving logs.
	opts.Full = true
	result, err = Import(db, opts)
	if err != nil || result.FilesImported != 6 {
		t.Fatalf("full import: %+v, %v", result, err)
	}
	projects, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, p := range projects {
		counts[p.RepoPath] = p.SessionCount
	}
	if !reflect.DeepEqual(counts, map[string]int{repo: 4, other: 2}) {
		t.Fatalf("projects = %v", counts)
	}
	sessions, err := db.ListSessions(SessionFilter{Projects: []string{repo}})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, session := range sessions {
		if session.RepoPath != repo {
			t.Errorf("%s repo_path = %q", session.SessionID, session.RepoPath)
		}
		ids[session.SessionID] = true
	}
	if !reflect.DeepEqual(ids, map[string]bool{"claude-0": true, "claude-1": true, "codex-0": true, "codex-1": true}) {
		t.Fatalf("filtered sessions = %v", ids)
	}
	for project, want := range map[string]map[string]bool{
		other:       {"claude-2": true, "codex-2": true},
		foreignRepo: {},
	} {
		sessions, err := db.ListSessions(SessionFilter{Projects: []string{project}})
		if err != nil {
			t.Fatal(err)
		}
		ids := map[string]bool{}
		for _, session := range sessions {
			ids[session.SessionID] = true
		}
		if !reflect.DeepEqual(ids, want) {
			t.Errorf("project %q sessions = %v, want %v", project, ids, want)
		}
	}
}

func TestImport_ClaudeCodeMissingIDs(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-test-proj")
	must(t, os.MkdirAll(projDir, 0o755))
	path := filepath.Join(projDir, "s1.jsonl")
	before := `{"type":"user","uuid":"u1","sessionId":"s1","cwd":"/nonexistent/valid","gitBranch":"main","version":"good","timestamp":"2026-03-28T14:00:00Z","message":{"role":"user","content":"before"}}` + "\n"
	after := `{"type":"assistant","uuid":"a1","sessionId":"s1","cwd":"/nonexistent/valid","gitBranch":"main","version":"good","timestamp":"2026-03-28T14:01:00Z","message":{"role":"assistant","content":"after"}}` + "\n"
	jsonl := before + `{"type":"user","uuid":"bad-user","message":{"role":"user","content":"bad"}}
{"type":"assistant","sessionId":"s1","cwd":"/nonexistent/invalid","gitBranch":"bad","version":"bad","timestamp":"2099-01-01T00:00:00Z","message":{"role":"assistant","content":"bad"}}
{"type":"user","sessionId":"new-session","uuid":"","message":{"role":"user","content":""}}
{"type":"assistant","sessionId":null,"uuid":"bad-assistant","message":{"role":"assistant","content":"bad"}}
` + after
	must(t, os.WriteFile(path, []byte(jsonl), 0o644))
	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	must(t, err)
	if res.FilesImported != 1 || res.FilesFailed != 0 || len(res.Errors) != 0 || res.UnparsedLines != 4 || len(res.UnparsedDiagnostics) != 4 {
		t.Fatalf("Import result: %+v", res)
	}
	for i, field := range []string{"sessionId", "uuid", "uuid", "sessionId"} {
		got := res.UnparsedDiagnostics[i].Error()
		if !strings.HasPrefix(got, fmt.Sprintf("%s:%d: ", path, i+2)) || !strings.Contains(got, field) {
			t.Errorf("diagnostic %d = %q; want physical line and %s", i, got, field)
		}
	}

	var sessionID, cwd, branch, version, started, ended string
	must(t, db.db.QueryRow("SELECT session_id, cwd, git_branch, version, started_at, ended_at FROM sessions").Scan(&sessionID, &cwd, &branch, &version, &started, &ended))
	if sessionID != "s1" || cwd != "/nonexistent/valid" || branch != "main" || version != "good" || started != "2026-03-28T14:00:00Z" || ended != "2026-03-28T14:01:00Z" {
		t.Fatalf("session was contaminated: %q %q %q %q %q %q", sessionID, cwd, branch, version, started, ended)
	}
	var sessions int
	must(t, db.db.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&sessions))
	if sessions != 1 {
		t.Fatalf("sessions = %d, want 1", sessions)
	}

	assertMessages := func(want []string) {
		t.Helper()
		rows, err := db.db.Query("SELECT uuid, session_id, role, content FROM messages ORDER BY rowid")
		must(t, err)
		defer rows.Close()
		var got []string
		for rows.Next() {
			var uuid, sid, role, content string
			must(t, rows.Scan(&uuid, &sid, &role, &content))
			got = append(got, strings.Join([]string{uuid, sid, role, content}, ":"))
		}
		must(t, rows.Err())
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("messages = %v, want %v", got, want)
		}
	}
	assertMessages([]string{"u1:s1:user:before", "a1:s1:assistant:after"})

	// Append a duplicate with changed content to prove INSERT OR IGNORE is
	// exercised, followed by a new message to prove incremental continuation.
	duplicate := strings.Replace(before, `"before"`, `"replacement"`, 1)
	appended := jsonl + duplicate + `{"type":"user","uuid":"u2","sessionId":"s1","timestamp":"2026-03-28T14:02:00Z","message":{"role":"user","content":"appended"}}` + "\n"
	must(t, os.WriteFile(path, []byte(appended), 0o644))
	res, err = Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	must(t, err)
	if res.FilesImported != 1 || res.FilesFailed != 0 || len(res.Errors) != 0 || res.UnparsedLines != 0 {
		t.Fatalf("incremental Import result: %+v", res)
	}
	assertMessages([]string{"u1:s1:user:before", "a1:s1:assistant:after", "u2:s1:user:appended"})
}

func TestImport_ClaudeCodeInvalidIDsOnly(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()
	projDir := filepath.Join(dir, "-test-proj")
	must(t, os.MkdirAll(projDir, 0o755))
	path := filepath.Join(projDir, "invalid.jsonl")
	must(t, os.WriteFile(path, []byte(`{"type":"user","sessionId":"new-session","message":{"role":"user","content":""}}
`), 0o644))
	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	must(t, err)
	if res.UnparsedLines != 1 || res.FilesFailed != 0 || len(res.Errors) != 0 {
		t.Fatalf("Import result: %+v", res)
	}
	for _, table := range []string{"sessions", "messages"} {
		var count int
		must(t, db.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		if count != 0 {
			t.Errorf("%s = %d, want 0", table, count)
		}
	}
	state, err := db.GetImportState(path)
	must(t, err)
	if state != nil {
		t.Fatalf("invalid-only file advanced body save boundary: %+v", state)
	}
}
