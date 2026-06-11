package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func processCodexFile(db *DB, path, sessionID, jsonl string) (int64, error) {
	pr, err := codex.NewAdapter(ResolveRepoPath).ProcessFile(
		db,
		JSONLFile{Path: path, SessionID: sessionID},
		0,
		int64(len(jsonl)),
		"2026-05-01T00:10:00Z",
	)
	return pr.NewOffset, err
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

	files, errs := codex.NewAdapter(ResolveRepoPath).ScanFiles(root)
	if len(errs) != 0 {
		t.Fatalf("ScanFiles failed: %v", errs)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}
	if files[0].SessionID != "rollout-a" {
		t.Errorf("SessionID: got %q, want rollout-a", files[0].SessionID)
	}
}

func TestCodexScanFiles_UnreadableSubdirIsNonFatal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	root := t.TempDir()
	readable := filepath.Join(root, "2026", "05", "01")
	if err := os.MkdirAll(readable, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(readable, "rollout-a.jsonl"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	unreadable := filepath.Join(root, "2026", "05", "02")
	if err := os.MkdirAll(unreadable, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() { os.Chmod(unreadable, 0o755) })

	files, errs := codex.NewAdapter(ResolveRepoPath).ScanFiles(root)
	if len(errs) != 1 {
		t.Fatalf("got %d scan errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), unreadable) {
		t.Errorf("scan error should name the unreadable dir: %v", errs[0])
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1: %+v", len(files), files)
	}
	if files[0].SessionID != "rollout-a" {
		t.Errorf("SessionID: got %q, want rollout-a", files[0].SessionID)
	}
}

func TestCodexProcessFile_CountsUnparsedLines(t *testing.T) {
	db := testDB(t)
	dir := t.TempDir()

	// One broken JSON line and one message with a malformed content payload;
	// the non-message event_msg record is deliberately ignored.
	jsonl := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-session","cwd":"/nonexistent/codex-project","cli_version":"0.128.0","git":{"branch":"main"}}}
{broken json
{"timestamp":"2026-05-01T00:00:01.000Z","type":"event_msg","payload":{"type":"token_count"}}
{"timestamp":"2026-05-01T00:00:02.000Z","type":"response_item","payload":{"type":"message","role":"user","content":"not a block array"}}
{"timestamp":"2026-05-01T00:00:03.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	path := filepath.Join(dir, "rollout.jsonl")
	if err := os.WriteFile(path, []byte(jsonl), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	pr, err := codex.NewAdapter(ResolveRepoPath).ProcessFile(
		db,
		JSONLFile{Path: path, SessionID: "rollout"},
		0,
		int64(len(jsonl)),
		"2026-05-01T00:10:00Z",
	)
	if err != nil {
		t.Fatalf("ProcessFile failed: %v", err)
	}
	if pr.UnparsedLines != 2 {
		t.Errorf("UnparsedLines: got %d, want 2", pr.UnparsedLines)
	}

	var count int
	db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE session_id='codex-session'").Scan(&count)
	if count != 1 {
		t.Errorf("messages: got %d, want 1", count)
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
	if _, err := importWithAdapter(db, root, codex.NewAdapter(ResolveRepoPath)); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	second := first + `{"timestamp":"2026-05-01T00:00:02.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}}
`
	if err := os.WriteFile(path, []byte(second), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := importWithAdapter(db, root, codex.NewAdapter(ResolveRepoPath)); err != nil {
		t.Fatalf("second import failed: %v", err)
	}

	// Pin the UUIDs, not just the count: they are derived from path + line
	// number, so a regression in prefix-replay line counting would silently
	// shift them (or collide with already-imported rows).
	rows, err := db.db.Query("SELECT uuid FROM messages WHERE source='codex' AND session_id='codex-incremental' ORDER BY timestamp")
	if err != nil {
		t.Fatalf("messages query failed: %v", err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var uuid string
		if err := rows.Scan(&uuid); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		got = append(got, uuid)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Rows failed: %v", err)
	}
	want := []string{codexMessageUUID(path, 2), codexMessageUUID(path, 3)}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("uuids:\ngot  %q\nwant %q", got, want)
	}
}

// codexMessageUUID mirrors the UUID derivation in internal/ingest/codex so the
// test pins the on-disk identity of imported messages.
func codexMessageUUID(path string, lineNumber int) string {
	sum := sha256.Sum256([]byte(path + "\x00" + strconv.Itoa(lineNumber)))
	return "codex:" + hex.EncodeToString(sum[:])
}

func TestImport_SourceCodexUsesCodexAdapter(t *testing.T) {
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

	result, err := Import(db, ImportOptions{CodexSessionsDir: root, Source: ImportSourceCodex})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.FilesImported != 1 || result.FilesScanned != 1 || len(result.Errors) != 0 {
		t.Fatalf("Import result: %+v", result)
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
