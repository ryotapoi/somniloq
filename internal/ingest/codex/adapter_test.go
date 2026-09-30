package codex

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func TestFileHandler_HandleLineReturnsPersistenceError(t *testing.T) {
	wantErr := errors.New("write failed")
	h := &fileHandler{
		importedAt: "2026-07-12T00:00:00Z",
		path:       "/tmp/rollout.jsonl",
		meta: &ingest.SessionMeta{
			Source:    ingest.SourceCodex,
			SessionID: "s1",
			CWD:       "/repo",
			RepoPath:  "/repo",
			StartedAt: "2026-07-12T00:00:00Z",
			EndedAt:   "2026-07-12T00:00:00Z",
		},
	}

	_, err := h.HandleLine(&failingTransaction{err: wantErr}, []byte(`{"timestamp":"2026-07-12T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`))

	if !errors.Is(err, wantErr) {
		t.Fatalf("HandleLine error = %v, want wrapping %v", err, wantErr)
	}
}

func TestAdapter_ProcessFileMalformedSessionMetaContinues(t *testing.T) {
	const contents = `{"type":"session_meta","payload":[]}
{"timestamp":"2026-07-12T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}
{"timestamp":"2026-07-12T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	tx := &recordingTransaction{}
	result, err := NewAdapter(func(string) string { return "/repo" }).ProcessFile(
		func() (ingest.ImportTransaction, error) { return tx, nil },
		path,
		0,
		int64(len(contents)),
		"2026-07-12T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("ProcessFile error = %v, want nil", err)
	}
	if result.UnparsedLines != 1 {
		t.Errorf("UnparsedLines = %d, want 1", result.UnparsedLines)
	}
	if len(tx.messages) != 1 {
		t.Errorf("InsertMessage calls = %d, want 1", len(tx.messages))
	}
	if tx.commits != 1 {
		t.Errorf("Commit calls = %d, want 1", tx.commits)
	}
}

func TestAdapter_RejectsEmptySessionIDsAcrossRollouts(t *testing.T) {
	const message = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	tx := &recordingTransaction{}
	adapter := NewAdapter(func(string) string { return "/repo" })
	for _, payload := range []string{`{"cwd":"/repo"}`, `{"id":""}`, `{"id":null}`, `{"id":"valid"}`} {
		path := filepath.Join(t.TempDir(), "rollout.jsonl")
		contents := "\n" + `{"type":"session_meta","payload":` + payload + "}\n" + message
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		result, err := adapter.ProcessFile(func() (ingest.ImportTransaction, error) { return tx, nil }, path, 0, int64(len(contents)), "2026-07-12T00:00:00Z")
		if err != nil {
			t.Fatal(err)
		}
		if payload == `{"id":"valid"}` {
			if result.UnparsedLines != 0 {
				t.Errorf("valid metadata: unparsed = %d, want 0", result.UnparsedLines)
			}
			continue
		}
		if result.UnparsedLines != 1 || len(result.UnparsedDiagnostics) != 1 {
			t.Fatalf("payload %s: unparsed = %d, diagnostics = %v", payload, result.UnparsedLines, result.UnparsedDiagnostics)
		}
		if got, want := result.UnparsedDiagnostics[0].Error(), path+":2: session_meta payload.id is missing or empty"; got != want {
			t.Errorf("diagnostic = %q, want %q", got, want)
		}
		if len(tx.sessions) != 0 || len(tx.messages) != 0 {
			t.Fatalf("invalid rollout persisted sessions/messages: %v / %v", tx.sessions, tx.messages)
		}
	}
	if len(tx.sessions) != 1 || tx.sessions[0].SessionID != "valid" || len(tx.messages) != 1 || tx.messages[0].SessionID != "valid" {
		t.Fatalf("persisted sessions/messages = %v / %v, want only valid session", tx.sessions, tx.messages)
	}
}

func TestAdapter_InvalidSessionMetaPrefixRecoversAtValidMetadata(t *testing.T) {
	const prefix = `{"type":"session_meta","payload":{"id":""}}
`
	const suffix = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"ignored"}]}}
{"type":"session_meta","payload":{"id":"valid"}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"kept"}]}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(prefix+suffix), 0o600); err != nil {
		t.Fatal(err)
	}
	tx := &recordingTransaction{}
	result, err := NewAdapter(func(string) string { return "/repo" }).ProcessFile(func() (ingest.ImportTransaction, error) { return tx, nil }, path, int64(len(prefix)), int64(len(prefix+suffix)), "2026-07-12T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if result.UnparsedLines != 0 || len(result.UnparsedDiagnostics) != 0 {
		t.Errorf("prefix metadata diagnosed again: %+v", result)
	}
	if len(tx.sessions) != 1 || tx.sessions[0].SessionID != "valid" || len(tx.messages) != 1 || tx.messages[0].Content != "kept" || tx.messages[0].SessionID != "valid" {
		t.Fatalf("persisted sessions/messages = %v / %v, want only message after valid metadata", tx.sessions, tx.messages)
	}
}

func TestAdapter_UsesOriginalSessionTimestampAfterTimestampedMessage(t *testing.T) {
	const sessionTimestamp = "2026-07-12T00:00:00Z"
	const contents = `{"timestamp":"2026-07-12T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}
{"timestamp":"2026-07-12T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	tx := &recordingTransaction{}
	result, err := NewAdapter(func(string) string { return "/repo" }).ProcessFile(
		func() (ingest.ImportTransaction, error) { return tx, nil },
		path,
		0,
		int64(len(contents)),
		"2026-07-12T01:00:00Z",
	)
	if err != nil {
		t.Fatalf("ProcessFile error = %v", err)
	}
	if result.UnparsedLines != 0 {
		t.Errorf("UnparsedLines = %d, want 0", result.UnparsedLines)
	}
	if len(tx.messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(tx.messages))
	}
	if got, want := tx.messages[0].Timestamp, "2026-07-12T00:00:01Z"; got != want {
		t.Errorf("first message timestamp = %q, want %q", got, want)
	}
	if got := tx.messages[1].Timestamp; got != sessionTimestamp {
		t.Errorf("second message timestamp = %q, want original session metadata timestamp %q", got, sessionTimestamp)
	}
}

func TestFileHandler_HandleLineReportsMalformedPayloads(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "session meta payload",
			line: `{"type":"session_meta","payload":[]}`,
		},
		{
			name: "response item payload",
			line: `{"type":"response_item","payload":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &fileHandler{
				path:            "/tmp/rollout.jsonl",
				resolveRepoPath: func(string) string { return "/repo" },
			}

			result, err := h.HandleLine(&failingTransaction{}, []byte(tt.line))
			if err != nil {
				t.Fatalf("HandleLine error = %v, want nil", err)
			}
			if result.Outcome != ingest.LineUnparsed {
				t.Errorf("HandleLine outcome = %v, want %v", result.Outcome, ingest.LineUnparsed)
			}
			if result.Diagnostic == nil {
				t.Error("HandleLine diagnostic = nil, want malformed payload diagnostic")
			} else if !strings.HasPrefix(result.Diagnostic.Error(), h.path+":1:") {
				t.Errorf("HandleLine diagnostic = %q, want path and physical line prefix", result.Diagnostic)
			}
		})
	}
}

type failingTransaction struct {
	err error
}

func (t *failingTransaction) UpsertSession(ingest.SessionMeta, string) error { return t.err }

func (t *failingTransaction) InsertMessage(ingest.NormalizedMessage) error { return nil }

func (t *failingTransaction) UpsertImportState(ingest.ImportState) error { return nil }

func (t *failingTransaction) Commit() error { return nil }

func (t *failingTransaction) Rollback() error { return nil }

type recordingTransaction struct {
	sessions []ingest.SessionMeta
	messages []ingest.NormalizedMessage
	commits  int
}

func (t *recordingTransaction) UpsertSession(meta ingest.SessionMeta, _ string) error {
	t.sessions = append(t.sessions, meta)
	return nil
}

func (t *recordingTransaction) InsertMessage(message ingest.NormalizedMessage) error {
	t.messages = append(t.messages, message)
	return nil
}

func (*recordingTransaction) UpsertImportState(ingest.ImportState) error { return nil }

func (t *recordingTransaction) Commit() error {
	t.commits++
	return nil
}

func (*recordingTransaction) Rollback() error { return nil }
