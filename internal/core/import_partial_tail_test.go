package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestImport_RetriesUnfinishedFinalLine(t *testing.T) {
	for _, source := range []ImportSource{ImportSourceClaudeCode, ImportSourceCodex, ImportSourceCursorAgent} {
		for _, existingState := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing-state=%t", source, existingState), func(t *testing.T) {
				db := testDB(t)
				root := t.TempDir()
				path := filepath.Join(root, "project", "agent-transcripts", "session", "session.jsonl")
				if source == ImportSourceClaudeCode {
					path = filepath.Join(root, "project", "session.jsonl")
				}
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				opts := ImportOptions{Source: source, ProjectsDir: root, CodexSessionsDir: root, CursorProjectsDir: root}
				record := func(text string) string {
					switch source {
					case ImportSourceClaudeCode:
						return fmt.Sprintf(`{"type":"user","uuid":%q,"sessionId":"session","timestamp":"2026-10-01T00:00:00Z","message":{"content":%q}}`, text, text)
					case ImportSourceCodex:
						return fmt.Sprintf(`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`, text)
					default:
						return fmt.Sprintf(`{"role":"user","message":{"content":[{"type":"text","text":%q}]}}`, text)
					}
				}
				prefix := ""
				bodyLine := 1
				if source == ImportSourceCodex {
					prefix = `{"timestamp":"2026-10-01T00:00:00Z","type":"session_meta","payload":{"id":"session","timestamp":"2026-10-01T00:00:00Z","cwd":"/nonexistent/sample","cli_version":"0.128.0","git":{"branch":"main"}}}` + "\n"
					bodyLine++
				}
				prefix += record("first") + "\n\n"
				tailLine := bodyLine + 2
				write := func(contents string) {
					t.Helper()
					if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				run := func() *ImportResult {
					t.Helper()
					result, err := Import(db, opts)
					if err != nil || result.FilesFailed != 0 || len(result.Errors) != 0 {
						t.Fatalf("Import = %+v, %v", result, err)
					}
					return result
				}
				if existingState {
					write(prefix)
					run()
				}
				second := record("second")
				partial := prefix + second[:len(second)-2]
				write(partial)
				result := run()
				if result.UnparsedLines != 1 || len(result.UnparsedDiagnostics) != 1 || !strings.HasPrefix(result.UnparsedDiagnostics[0].Error(), fmt.Sprintf("%s:%d: ", path, tailLine)) {
					t.Fatalf("partial diagnostics = %+v; want physical line %d", result, tailLine)
				}
				var offset, size int64
				if err := db.db.QueryRow("SELECT last_offset, file_size FROM import_state WHERE jsonl_path = ?", path).Scan(&offset, &size); err != nil {
					t.Fatal(err)
				}
				if offset != int64(len(prefix)) || size != int64(len(partial)) {
					t.Fatalf("partial state = (%d, %d), want (%d, %d)", offset, size, len(prefix), len(partial))
				}
				if result := run(); result.FilesSkipped != 1 || result.UnparsedLines != 0 {
					t.Fatalf("unchanged partial import = %+v, want skipped", result)
				}

				// Complete the same physical line without a newline. It must be saved now.
				write(prefix + second)
				if result := run(); result.UnparsedLines != 0 {
					t.Fatalf("completed line = %+v", result)
				}
				type saved struct{ uuid, content, timestamp string }
				readMessages := func() []saved {
					t.Helper()
					rows, err := db.db.Query("SELECT uuid, content, timestamp FROM messages ORDER BY rowid")
					if err != nil {
						t.Fatal(err)
					}
					defer rows.Close()
					var got []saved
					for rows.Next() {
						var message saved
						if err := rows.Scan(&message.uuid, &message.content, &message.timestamp); err != nil {
							t.Fatal(err)
						}
						got = append(got, message)
					}
					if err := rows.Err(); err != nil {
						t.Fatal(err)
					}
					return got
				}
				completed := readMessages()
				if len(completed) != 2 || completed[0].content != "first" || completed[1].content != "second" {
					t.Fatalf("completed messages = %+v", completed)
				}
				if result := run(); result.FilesSkipped != 1 {
					t.Fatalf("unchanged complete import = %+v", result)
				}

				// Adding LF finishes the prior physical line; a terminated invalid line
				// must be consumed so the following message and diagnostic keep their lines.
				final := prefix + second + "\n{broken\n" + record("third") + "\n{broken\n"
				write(final)
				result = run()
				if result.UnparsedLines != 2 || len(result.UnparsedDiagnostics) != 2 {
					t.Fatalf("later diagnostics = %+v", result)
				}
				for i, line := range []int{tailLine + 1, tailLine + 3} {
					if !strings.HasPrefix(result.UnparsedDiagnostics[i].Error(), fmt.Sprintf("%s:%d: ", path, line)) {
						t.Errorf("diagnostic = %v, want physical line %d", result.UnparsedDiagnostics[i], line)
					}
				}
				incremental := readMessages()
				if len(incremental) != 3 || incremental[2].content != "third" || !reflect.DeepEqual(incremental[:2], completed) {
					t.Fatalf("incremental messages = %+v", incremental)
				}
				for _, message := range incremental {
					wantTimestamp := "2026-10-01T00:00:00Z"
					if source == ImportSourceCursorAgent {
						wantTimestamp = ""
					}
					if message.timestamp != wantTimestamp {
						t.Errorf("timestamp = %q, want %q", message.timestamp, wantTimestamp)
					}
				}
				if result := run(); result.FilesSkipped != 1 {
					t.Fatalf("terminated invalid tail did not advance: %+v", result)
				}
				if source == ImportSourceCodex {
					var cwd, branch, version string
					if err := db.db.QueryRow("SELECT cwd, git_branch, version FROM sessions WHERE session_id = 'session'").Scan(&cwd, &branch, &version); err != nil {
						t.Fatal(err)
					}
					if cwd != "/nonexistent/sample" || branch != "main" || version != "0.128.0" {
						t.Fatalf("restored metadata = %q, %q, %q", cwd, branch, version)
					}
				}
				// A full replay derives the same identities from physical line numbers.
				opts.Full = true
				run()
				if got := readMessages(); !reflect.DeepEqual(got, incremental) {
					t.Fatalf("full replay = %+v, want %+v", got, incremental)
				}
			})
		}
	}
}
