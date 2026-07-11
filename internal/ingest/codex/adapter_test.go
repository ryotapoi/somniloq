package codex

import (
	"errors"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func TestFileHandler_HandleLineReturnsIgnoredOutcomeOnPersistError(t *testing.T) {
	wantErr := errors.New("write failed")
	h := &fileHandler{
		importedAt: "2026-07-12T00:00:00Z",
		path:       "/tmp/rollout.jsonl",
		hasMeta:    true,
		meta: SessionMeta{
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

type failingTransaction struct {
	err error
}

func (t *failingTransaction) UpsertSession(ingest.SessionMeta, string) error { return t.err }

func (t *failingTransaction) InsertMessage(ingest.NormalizedMessage) error { return nil }

func (t *failingTransaction) UpsertImportState(ingest.ImportState) error { return nil }

func (t *failingTransaction) Commit() error { return nil }

func (t *failingTransaction) Rollback() error { return nil }
