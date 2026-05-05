package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func processCodexFile(db *DB, path, sessionID, jsonl string) (int64, error) {
	return codex.NewAdapter(ResolveRepoPath).ProcessFile(
		db,
		JSONLFile{Path: path, SessionID: sessionID},
		0,
		int64(len(jsonl)),
		"2026-05-01T00:10:00Z",
	)
}

func TestCodexScanFiles_Recursive(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "2026", "05", "01")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "rollout-a.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "notes.txt"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	files, err := codex.NewAdapter(ResolveRepoPath).ScanFiles(root)
	if err != nil {
		t.Fatalf("ScanFiles failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}
	if files[0].SessionID != "rollout-a" {
		t.Errorf("SessionID: got %q, want rollout-a", files[0].SessionID)
	}
}

func TestCodexProcessFile_ImportsConversationMessages(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-session","timestamp":"2026-05-01T00:00:00.000Z","cwd":"/nonexistent/codex-project","cli_version":"0.128.0","git":{"branch":"main"}}}
{"timestamp":"2026-05-01T00:00:01.000Z","type":"event_msg","payload":{"type":"token_count"}}
{"timestamp":"2026-05-01T00:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
{"timestamp":"2026-05-01T00:00:03.000Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{}"}}
{"timestamp":"2026-05-01T00:00:04.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hi there"}]}}
{"timestamp":"2026-05-01T00:00:05.000Z","type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"text","text":"skip"}]}}
`
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	newOffset, err := processCodexFile(db, path, "rollout", jsonl)
	if err != nil {
		t.Fatalf("processCodexFile failed: %v", err)
	}
	if newOffset != int64(len(jsonl)) {
		t.Errorf("offset: got %d, want %d", newOffset, len(jsonl))
	}

	var source, repoPath, branch, version string
	if err := db.db.QueryRow(
		"SELECT source, repo_path, git_branch, version FROM sessions WHERE source='codex' AND session_id='codex-session'",
	).Scan(&source, &repoPath, &branch, &version); err != nil {
		t.Fatalf("session not found: %v", err)
	}
	if source != "codex" || repoPath != "/nonexistent/codex-project" || branch != "main" || version != "0.128.0" {
		t.Errorf("session: source=%q repo=%q branch=%q version=%q", source, repoPath, branch, version)
	}

	rows, err := db.db.Query("SELECT uuid, role, content FROM messages WHERE source='codex' AND session_id='codex-session' ORDER BY timestamp")
	if err != nil {
		t.Fatalf("messages query failed: %v", err)
	}
	defer rows.Close()

	var got []string
	for rows.Next() {
		var uuid, role, content string
		if err := rows.Scan(&uuid, &role, &content); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		if !strings.HasPrefix(uuid, "codex:") {
			t.Errorf("uuid should be codex-derived, got %q", uuid)
		}
		got = append(got, role+":"+content)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Rows failed: %v", err)
	}
	want := []string{"user:hello", "assistant:hi there"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("messages:\ngot  %q\nwant %q", got, want)
	}

	state, err := db.GetImportState(path)
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if state == nil || state.Source != SourceCodex || state.LastOffset != int64(len(jsonl)) {
		t.Fatalf("import_state: %+v", state)
	}
}

func TestCodexImport_IncrementalUsesSessionMetaBeforeOffset(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	nested := filepath.Join(root, "2026", "05", "01")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	path := filepath.Join(nested, "rollout-incremental.jsonl")

	first := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-incremental","timestamp":"2026-05-01T00:00:00.000Z","cwd":"/nonexistent/codex-incremental","cli_version":"0.128.0","git":{"branch":"main"}}}
{"timestamp":"2026-05-01T00:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]}}
`
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := importWithAdapter(db, false, root, codex.NewAdapter(ResolveRepoPath)); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	second := first + `{"timestamp":"2026-05-01T00:00:02.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}}
`
	if err := os.WriteFile(path, []byte(second), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := importWithAdapter(db, false, root, codex.NewAdapter(ResolveRepoPath)); err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE source='codex' AND session_id='codex-incremental'").Scan(&count); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if count != 2 {
		t.Errorf("messages: got %d, want 2", count)
	}
}

func TestImportCodex_UsesCodexAdapter(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	nested := filepath.Join(root, "2026", "05", "01")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}

	jsonl := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-public","timestamp":"2026-05-01T00:00:00.000Z","cwd":"/nonexistent/codex-public","cli_version":"0.128.0"}}
{"timestamp":"2026-05-01T00:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	if err := os.WriteFile(filepath.Join(nested, "rollout-public.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	result, err := ImportCodex(db, ImportOptions{ProjectsDir: root})
	if err != nil {
		t.Fatalf("ImportCodex failed: %v", err)
	}
	if result.FilesImported != 1 || result.FilesScanned != 1 || len(result.Errors) != 0 {
		t.Fatalf("ImportCodex result: %+v", result)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE source='codex' AND session_id='codex-public'").Scan(&count); err != nil {
		t.Fatalf("COUNT failed: %v", err)
	}
	if count != 1 {
		t.Errorf("messages: got %d, want 1", count)
	}
}

func TestCodexProcessFile_MetaOnlyDoesNotAdvanceImportState(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	jsonl := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"meta-only","timestamp":"2026-05-01T00:00:00.000Z","cwd":"/nonexistent/meta-only","cli_version":"0.128.0"}}
`
	path := filepath.Join(dir, "rollout-meta-only.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	newOffset, err := processCodexFile(db, path, "rollout-meta-only", jsonl)
	if err != nil {
		t.Fatalf("processCodexFile failed: %v", err)
	}
	if newOffset != 0 {
		t.Errorf("offset: got %d, want 0", newOffset)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE source='codex'").Scan(&count); err != nil {
		t.Fatalf("COUNT sessions failed: %v", err)
	}
	if count != 0 {
		t.Errorf("sessions: got %d, want 0", count)
	}
	state, err := db.GetImportState(path)
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if state != nil {
		t.Fatalf("import_state should be nil for meta-only file, got %+v", state)
	}
}
