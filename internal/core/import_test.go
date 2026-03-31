package core

import (
	"os"
	"path/filepath"
	"testing"
)

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

	files, err := ScanJSONLFiles(dir)
	if err != nil {
		t.Fatalf("ScanJSONLFiles failed: %v", err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d: %+v", len(files), files)
	}

	// Check that ProjectDir and SessionID are correctly derived
	found := map[string]bool{}
	for _, f := range files {
		found[f.SessionID] = true
		if f.ProjectDir == "" {
			t.Errorf("empty ProjectDir for %s", f.Path)
		}
	}
	for _, sid := range []string{"sess1", "sess2", "sess3"} {
		if !found[sid] {
			t.Errorf("missing session %s", sid)
		}
	}
}

func TestScanJSONLFiles_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	files, err := ScanJSONLFiles(dir)
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

	jsonl := `{"type":"user","uuid":"u1","parentUuid":"p1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"hi there"}]}}
{"type":"custom-title","customTitle":"test session","sessionId":"s1"}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, ProjectDir: "-test", SessionID: "s1"}
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

func TestProcessFile_EmptyFile(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	path := filepath.Join(dir, "empty.jsonl")
	os.WriteFile(path, []byte(""), 0o644)

	file := JSONLFile{Path: path, ProjectDir: "-test", SessionID: "empty"}
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
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, ProjectDir: "-test", SessionID: "s1"}
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
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"tool_use","id":"t1","name":"Read","input":{}}]}}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, ProjectDir: "-test", SessionID: "s1"}
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

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":"   \n  "}}
`
	path := filepath.Join(dir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	file := JSONLFile{Path: path, ProjectDir: "-test", SessionID: "s1"}
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
	jsonl1 := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"first"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"reply1"}]}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl1), 0o644)

	res, err := Import(db, ImportOptions{ProjectsDir: dir})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if res.FilesImported != 1 {
		t.Errorf("expected 1 imported, got %d", res.FilesImported)
	}

	// Append 1 more message
	jsonl2 := jsonl1 + `{"type":"user","uuid":"u2","sessionId":"s1","timestamp":"2026-03-28T14:02:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"second"}}
`
	os.WriteFile(path, []byte(jsonl2), 0o644)

	res, err = Import(db, ImportOptions{ProjectsDir: dir})
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
	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"original"}}
{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:01:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"reply"}]}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	Import(db, ImportOptions{ProjectsDir: dir})

	// Shrink file (simulate truncate/recreate)
	smallJsonl := `{"type":"user","uuid":"u3","sessionId":"s1","timestamp":"2026-03-28T15:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"new"}}
`
	os.WriteFile(path, []byte(smallJsonl), 0o644)

	res, _ := Import(db, ImportOptions{ProjectsDir: dir})
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

	jsonl := `{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-03-28T14:00:00Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}
`
	path := filepath.Join(projDir, "s1.jsonl")
	os.WriteFile(path, []byte(jsonl), 0o644)

	// First import
	Import(db, ImportOptions{ProjectsDir: dir})

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 message after first import, got %d", count)
	}

	// Full re-import
	res, err := Import(db, ImportOptions{Full: true, ProjectsDir: dir})
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

func TestScanJSONLFiles_NonexistentDir(t *testing.T) {
	files, err := ScanJSONLFiles("/nonexistent/path")
	if err != nil {
		t.Fatalf("ScanJSONLFiles should not error on missing dir: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}
