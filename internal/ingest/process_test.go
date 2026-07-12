package ingest

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestProcessJSONL_HandlerErrorDiscardsOutcomeAndRollsBack(t *testing.T) {
	const prefix = "already-imported\n"
	const line = "misleading-outcome\n"

	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(prefix+line), 0o600); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("persist failed")
	tx := &processRecordingTx{}
	result, err := ProcessJSONL(
		func() (ImportTransaction, error) { return tx, nil },
		SourceClaudeCode,
		errorOutcomeHandler{err: wantErr},
		File{Path: path},
		int64(len(prefix)),
		int64(len(prefix+line)),
		"2026-07-12T00:00:00Z",
	)

	if !errors.Is(err, wantErr) {
		t.Fatalf("ProcessJSONL error = %v, want wrapping %v", err, wantErr)
	}
	if result.NewOffset != int64(len(prefix)) {
		t.Errorf("NewOffset = %d, want %d", result.NewOffset, len(prefix))
	}
	if result.UnparsedLines != 0 {
		t.Errorf("UnparsedLines = %d, want 0", result.UnparsedLines)
	}
	if tx.importStateWrites != 0 {
		t.Errorf("UpsertImportState calls = %d, want 0", tx.importStateWrites)
	}
	if tx.commits != 0 {
		t.Errorf("Commit calls = %d, want 0", tx.commits)
	}
	if tx.rollbacks != 1 {
		t.Errorf("Rollback calls = %d, want 1", tx.rollbacks)
	}
}

type errorOutcomeHandler struct {
	err error
}

func (h errorOutcomeHandler) Begin(string, int64) error { return nil }

func (h errorOutcomeHandler) HandleLine(ImportTransaction, []byte) (LineOutcome, error) {
	return LineWroteBody, h.err
}

func (h errorOutcomeHandler) Flush(ImportTransaction) error { return nil }

type processRecordingTx struct {
	importStateWrites int
	commits           int
	rollbacks         int
}

func (t *processRecordingTx) UpsertSession(SessionMeta, string) error { return nil }

func (t *processRecordingTx) InsertMessage(NormalizedMessage) error { return nil }

func (t *processRecordingTx) UpsertImportState(ImportState) error {
	t.importStateWrites++
	return nil
}

func (t *processRecordingTx) Commit() error {
	t.commits++
	return nil
}

func (t *processRecordingTx) Rollback() error {
	t.rollbacks++
	return nil
}

func TestProcessJSONL_TransactionCreationErrorKeepsOffset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("record\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("begin import failed")
	result, err := ProcessJSONL(
		func() (ImportTransaction, error) { return nil, wantErr },
		SourceClaudeCode,
		errorOutcomeHandler{},
		File{Path: path},
		0,
		int64(len("record\n")),
		"2026-07-12T00:00:00Z",
	)

	if !errors.Is(err, wantErr) {
		t.Fatalf("ProcessJSONL error = %v, want wrapping %v", err, wantErr)
	}
	if result.NewOffset != 0 {
		t.Errorf("NewOffset = %d, want 0", result.NewOffset)
	}
}
