package codex

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func TestFileHandler_HandleLineReturnsIgnoredOutcomeOnPersistError(t *testing.T) {
	wantErr := errors.New("write failed")
	h := &fileHandler{
		importedAt: "2026-07-12T00:00:00Z",
		path:       "/tmp/rollout.jsonl",
		hasMeta:    true,
		meta: sessionMetaCursor{
			SessionID: "s1",
			CWD:       "/repo",
			RepoPath:  "/repo",
			Timestamp: "2026-07-12T00:00:00Z",
		},
	}

	outcome, err := h.HandleLine(&failingTransaction{err: wantErr}, []byte(`{"timestamp":"2026-07-12T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`))

	if !errors.Is(err, wantErr) {
		t.Fatalf("HandleLine error = %v, want wrapping %v", err, wantErr)
	}
	if outcome != ingest.LineIgnored {
		t.Errorf("HandleLine outcome = %v, want %v", outcome, ingest.LineIgnored)
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
		ingest.File{Path: path},
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
	if result.NewOffset != int64(len(contents)) {
		t.Errorf("NewOffset = %d, want %d", result.NewOffset, len(contents))
	}
	if tx.messages != 1 {
		t.Errorf("InsertMessage calls = %d, want 1", tx.messages)
	}
	if tx.commits != 1 {
		t.Errorf("Commit calls = %d, want 1", tx.commits)
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

			outcome, err := h.HandleLine(&failingTransaction{}, []byte(tt.line))
			if err != nil {
				t.Fatalf("HandleLine error = %v, want nil", err)
			}
			if outcome != ingest.LineUnparsed {
				t.Errorf("HandleLine outcome = %v, want %v", outcome, ingest.LineUnparsed)
			}
			if h.UnparsedDiagnostic() == nil {
				t.Error("UnparsedDiagnostic = nil, want malformed payload diagnostic")
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
	messages int
	commits  int
}

func (*recordingTransaction) UpsertSession(ingest.SessionMeta, string) error { return nil }

func (t *recordingTransaction) InsertMessage(ingest.NormalizedMessage) error {
	t.messages++
	return nil
}

func (*recordingTransaction) UpsertImportState(ingest.ImportState) error { return nil }

func (t *recordingTransaction) Commit() error {
	t.commits++
	return nil
}

func (*recordingTransaction) Rollback() error { return nil }
