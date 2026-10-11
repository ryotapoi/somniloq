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

func TestCodexScanFiles_Recursive(t *testing.T) {
	root := testTempDir(t)
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
	if filepath.Base(files[0]) != "rollout-a.jsonl" {
		t.Errorf("path: got %q, want rollout-a.jsonl", files[0])
	}
}

func TestCodexScanFiles_UnreadableRootIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	root := testTempDir(t)
	if err := os.Chmod(root, 0o000); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	t.Cleanup(func() { os.Chmod(root, 0o755) })

	files, errs := codex.NewAdapter(ResolveRepoPath).ScanFiles(root)
	if len(errs) != 1 {
		t.Fatalf("got %d scan errors, want 1: %v", len(errs), errs)
	}
	if !strings.Contains(errs[0].Error(), root) {
		t.Errorf("scan error should name the unreadable root: %v", errs[0])
	}
	if len(files) != 0 {
		t.Fatalf("got %d files, want 0: %+v", len(files), files)
	}
}

func TestCodexImport_ReportsDiagnosticOnUnterminatedPrefixLine(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	nested := filepath.Join(root, "2026", "05", "01")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "rollout.jsonl")
	first := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-session","cwd":"/nonexistent/codex-project","cli_version":"0.128.0","git":{"branch":"main"}}}` + "\n" +
		`{"timestamp":"2026-05-01T00:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}, Source: ImportSourceCodex}); err != nil {
		t.Fatalf("initial Import failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(first+"{broken json\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}, Source: ImportSourceCodex})
	if err != nil {
		t.Fatalf("incremental Import failed: %v", err)
	}
	if res.UnparsedLines != 1 || len(res.UnparsedDiagnostics) != 1 {
		t.Fatalf("incremental diagnostics: lines=%d diagnostics=%v", res.UnparsedLines, res.UnparsedDiagnostics)
	}
	if got, wantPrefix := res.UnparsedDiagnostics[0].Error(), path+":2: "; !strings.HasPrefix(got, wantPrefix) {
		t.Errorf("diagnostic = %q, want prefix %q", got, wantPrefix)
	}
}

func TestCodexImport_IncrementalPreservesMetadataAndPhysicalLines(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	nested := filepath.Join(root, "2026", "05", "01")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	path := filepath.Join(nested, "rollout-incremental.jsonl")

	first := `{"timestamp":"2026-05-01T00:00:00.000Z","type":"session_meta","payload":{"id":"codex-incremental","timestamp":"2026-05-01T00:00:00.000Z","cwd":"/nonexistent/codex-incremental","cli_version":"0.128.0","git":{"branch":"main"}}}

{"timestamp":"2026-05-01T00:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]}}
{"type":"session_meta","payload":[]}
`
	if err := os.WriteFile(path, []byte(first), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if _, err := importCodexGroups(db, testInput(t, db, SourceCodex), root, codex.NewAdapter(ResolveRepoPath), "2026-03-28T15:00:00Z", false); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	second := first + `{"timestamp":"2026-05-01T00:00:02.000Z","type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}}
`
	if err := os.WriteFile(path, []byte(second), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	result, err := importCodexGroups(db, testInput(t, db, SourceCodex), root, codex.NewAdapter(ResolveRepoPath), "2026-03-28T15:00:00Z", false)
	if err != nil {
		t.Fatalf("second import failed: %v", err)
	}
	if result.UnparsedLines != 0 {
		t.Errorf("second import unparsed lines = %d, want 0", result.UnparsedLines)
	}

	// Pin the UUIDs, not just the count: they are derived from path + line
	// number, so a regression in snapshot line counting would silently
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
	want := []string{codexMessageUUID(path, 3), codexMessageUUID(path, 5)}
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
