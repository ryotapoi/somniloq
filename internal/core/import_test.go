package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

type JSONLFile = ingest.File

func scanJSONLFiles(projectsDir string) ([]JSONLFile, error) {
	return claudecode.NewAdapter(ResolveRepoPath).ScanFiles(projectsDir)
}

func processFile(db *DB, file JSONLFile, offset, fileSize int64, importedAt string) (int64, error) {
	return claudecode.NewAdapter(ResolveRepoPath).ProcessFile(db, file, offset, fileSize, importedAt)
}

func TestScanJSONLFiles(t *testing.T) {
	dir := t.TempDir()

	// Create project dirs with JSONL files
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

	files, err := scanJSONLFiles(dir)
	if err != nil {
		t.Fatalf("ScanJSONLFiles failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(files), files)
	}

	// Check that SessionID is correctly derived
	found := map[string]bool{}
	for _, f := range files {
		found[f.SessionID] = true
	}
	for _, sid := range []string{"sess1", "sess2", "sess3"} {
		if !found[sid] {
			t.Errorf("missing session %s", sid)
		}
	}
}

func TestScanJSONLFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	files, err := scanJSONLFiles(dir)
	if err != nil {
		t.Fatalf("ScanJSONLFiles failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
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

	file := JSONLFile{Path: path, SessionID: "s1"}
	newOffset, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}
	if newOffset != int64(len(jsonl)) {
		t.Errorf("offset: got %d, want %d", newOffset, len(jsonl))
	}

	// Check sessions
	var title string
	err = db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title)
	if err != nil {
		t.Fatalf("session not found: %v", err)
	}
	if title != "test session" {
		t.Errorf("title: got %q, want %q", title, "test session")
	}

	// Check messages
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&count)
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

	file := JSONLFile{Path: path, SessionID: "s"}
	if _, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
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

func TestImport_FillsRepoPath(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-Users-test-proj")
	os.MkdirAll(projDir, 0o755)

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/Users/test/proj/.claude/worktrees/feature","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
`
	os.WriteFile(filepath.Join(projDir, "s1.jsonl"), []byte(jsonl), 0o644)

	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if res.FilesImported != 1 || len(res.Errors) != 0 {
		t.Fatalf("Import result: imported=%d errors=%v", res.FilesImported, res.Errors)
	}

	var repoPath string
	if err := db.db.QueryRow("SELECT COALESCE(repo_path, '') FROM sessions WHERE session_id='s1'").Scan(&repoPath); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if repoPath != "/Users/test/proj" {
		t.Errorf("repo_path: got %q, want /Users/test/proj", repoPath)
	}
}

func TestProcessFile_EmptyFile(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	file := JSONLFile{Path: path, SessionID: "empty"}
	newOffset, err := processFile(db, file, 0, 0, "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}
	if newOffset != 0 {
		t.Errorf("offset should be 0 for empty file, got %d", newOffset)
	}
}

func TestProcessFile_NoTrailingNewline(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	// No trailing newline
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, SessionID: "s1"}
	newOffset, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}
	if newOffset != int64(len(jsonl)) {
		t.Errorf("offset: got %d, want %d", newOffset, len(jsonl))
	}

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 message, got %d", count)
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

	file := JSONLFile{Path: path, SessionID: "s1"}
	_, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	// Only the user message should be saved (tool_use-only assistant message skipped)
	var msgCount int
	db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&msgCount)
	if msgCount != 1 {
		t.Errorf("messages: got %d, want 1 (empty content skipped)", msgCount)
	}

	// Session should still be created (upsertSession called for all messages)
	var sessCount int
	db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id='s1'").Scan(&sessCount)
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

	file := JSONLFile{Path: path, SessionID: "s1"}
	_, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z")
	if err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	var msgCount int
	db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&msgCount)
	if msgCount != 1 {
		t.Errorf("messages: got %d, want 1 (whitespace-only content skipped)", msgCount)
	}
}

func TestImport_Incremental(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-test-proj")
	os.MkdirAll(projDir, 0o755)

	// First import: 2 messages
	jsonl1 := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"first"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"reply1"}]}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl1), 0o644)

	res, err := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if res.FilesImported != 1 {
		t.Errorf("expected 1 imported, got %d", res.FilesImported)
	}

	// Append 1 more message
	jsonl2 := jsonl1 + `{"type":"user","uuid":"u2","sessionId":"s1","timestamp":"2026-03-28T14:02:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"second"}}
`
	os.WriteFile(path, []byte(jsonl2), 0o644)

	res, err = Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("Import (2nd) failed: %v", err)
	}
	if res.FilesImported != 1 {
		t.Errorf("expected 1 imported (incremental), got %d", res.FilesImported)
	}

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='s1'").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 messages, got %d", count)
	}
}

func TestImport_FileShrink(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-test-proj")
	os.MkdirAll(projDir, 0o755)

	// First import with larger file
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"original"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})

	// Shrink file (simulate truncate/recreate)
	smallJsonl := `{"type":"user","uuid":"u3","sessionId":"s1","timestamp":"2026-03-28T15:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"new"}}
`
	os.WriteFile(path, []byte(smallJsonl), 0o644)

	res, _ := Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if res.FilesImported != 1 {
		t.Errorf("shrunk file should be re-imported, got imported=%d", res.FilesImported)
	}

	// Should have 3 messages total (2 old + 1 new, old not deleted)
	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if count != 3 {
		t.Errorf("expected 3 messages (orphans retained), got %d", count)
	}
}

func TestImport_Full(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	projDir := filepath.Join(dir, "-test-proj")
	os.MkdirAll(projDir, 0o755)

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	// First import
	Import(db, ImportOptions{ProjectsDir: dir, Source: ImportSourceClaudeCode})

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 message after first import, got %d", count)
	}

	// Full re-import
	res, err := Import(db, ImportOptions{Full: true, ProjectsDir: dir, Source: ImportSourceClaudeCode})
	if err != nil {
		t.Fatalf("Import --full failed: %v", err)
	}
	if res.FilesImported != 1 {
		t.Errorf("expected 1 imported, got %d", res.FilesImported)
	}

	db.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 message after full re-import, got %d", count)
	}
}

func TestImport_AllSources(t *testing.T) {
	db := testDB(t)
	claudeRoot := t.TempDir()
	codexRoot := t.TempDir()

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

	result, err := Import(db, ImportOptions{
		ProjectsDir:      claudeRoot,
		CodexSessionsDir: codexRoot,
		Source:           ImportSourceAll,
	})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.FilesImported != 2 || result.FilesScanned != 2 || len(result.Errors) != 0 {
		t.Fatalf("Import result: %+v", result)
	}

	for _, c := range []struct {
		source    Source
		sessionID string
	}{
		{SourceClaudeCode, "claude-s1"},
		{SourceCodex, "codex-s1"},
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
	must(t, db.InsertMessage(ParsedMessage{
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

func TestScanJSONLFiles_NonexistentDir(t *testing.T) {
	files, err := scanJSONLFiles("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanJSONLFiles should not error on missing dir: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestProcessFile_MetaOnly_NoSessionRow(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"custom-title","customTitle":"meta only","sessionId":"meta1"}
`
	path := filepath.Join(dir, "meta1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, SessionID: "meta1"}
	if _, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
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

	file := JSONLFile{Path: path, SessionID: "meta1"}
	if _, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
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

func TestProcessFile_MetaBeforeBody(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"custom-title","customTitle":"top title","sessionId":"s1"}
{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"yo"}]}}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, SessionID: "s1"}
	if _, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	var title string
	if err := db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "top title" {
		t.Errorf("custom_title: got %q, want %q", title, "top title")
	}
}

func TestProcessFile_AgentNameBeforeBody(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"type":"agent-name","agentName":"first-agent","sessionId":"s1"}
{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/nonexistent/not-a-repo","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hi"}}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, SessionID: "s1"}
	if _, err := processFile(db, file, 0, int64(len(jsonl)), "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("processFile failed: %v", err)
	}

	var name string
	if err := db.db.QueryRow("SELECT agent_name FROM sessions WHERE session_id='s1'").Scan(&name); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if name != "first-agent" {
		t.Errorf("agent_name: got %q, want %q", name, "first-agent")
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
