package claudecode

import (
	"errors"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func TestFileHandler_HandleLineReturnsIgnoredOutcomeOnPersistError(t *testing.T) {
	wantErr := errors.New("write failed")
	h := &fileHandler{
		resolveRepoPath: func(string) string { return "/repo" },
		importedAt:      "2026-07-12T00:00:00Z",
		repoCache:       map[string]string{},
		titles:          map[string]string{},
		agentNames:      map[string]string{},
	}

	outcome, err := h.HandleLine(&failingTransaction{err: wantErr}, []byte(`{"type":"user","uuid":"u1","sessionId":"s1","timestamp":"2026-07-12T00:00:00Z","cwd":"/repo","message":{"role":"user","content":"hello"}}`))

	if !errors.Is(err, wantErr) {
		t.Fatalf("HandleLine error = %v, want wrapping %v", err, wantErr)
	}
	if outcome != ingest.LineIgnored {
		t.Errorf("HandleLine outcome = %v, want %v", outcome, ingest.LineIgnored)
	}
}

func TestFileHandler_FlushErrorsWhenTransactionLacksSessionMetaWriter(t *testing.T) {
	h := &fileHandler{
		importedAt: "2026-07-12T00:00:00Z",
		titles:     map[string]string{"s1": "title"},
		agentNames: map[string]string{},
	}

	err := h.Flush(&failingTransaction{})
	if err == nil {
		t.Fatal("Flush error = nil, want SessionMetaWriter assertion failure")
	}
	if !strings.Contains(err.Error(), "does not implement claudecode.SessionMetaWriter") {
		t.Errorf("Flush error = %v, want SessionMetaWriter assertion failure", err)
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
