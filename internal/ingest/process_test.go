package ingest

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessJSONL_NoBodyDoesNotAdvanceOffsetOrCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte("metadata\\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	tx := &processRecordingTx{}
	result, err := ProcessJSONL(
		func() (ImportTransaction, error) { return tx, nil },
		SourceClaudeCode,
		ignoredHandler{},
		File{Path: path},
		0,
		int64(len("metadata\\n")),
		"2026-07-12T00:00:00Z",
	)
	if err != nil {
		t.Fatalf("ProcessJSONL error = %v, want nil", err)
	}
	if result.NewOffset != 0 {
		t.Errorf("NewOffset = %d, want 0", result.NewOffset)
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

func TestProcessJSONL_BeginErrorKeepsOffsetWithoutStartingTransaction(t *testing.T) {
	const offset = 17
	wantErr := errors.New("restore state failed")
	transactions := 0
	result, err := ProcessJSONL(
		func() (ImportTransaction, error) {
			transactions++
			return &processRecordingTx{}, nil
		},
		SourceClaudeCode,
		beginErrorHandler{err: wantErr},
		File{Path: "unused.jsonl"},
		offset,
		offset,
		"2026-07-12T00:00:00Z",
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("ProcessJSONL error = %v, want wrapping %v", err, wantErr)
	}
	if result.NewOffset != offset {
		t.Errorf("NewOffset = %d, want %d", result.NewOffset, offset)
	}
	if transactions != 0 {
		t.Errorf("newTransaction calls = %d, want 0", transactions)
	}
}

func TestProcessJSONL_ReadErrorAfterBodyKeepsOffsetAndRollsBack(t *testing.T) {
	const line = "body\\n"
	wantErr := errors.New("read failed")
	reader := &errorAfterReader{data: strings.NewReader(line), err: wantErr}
	tx := &processRecordingTx{}
	handler := &bodyHandler{}
	result, err := processJSONL(
		func() (ImportTransaction, error) { return tx, nil },
		SourceClaudeCode,
		handler,
		File{Path: "session.jsonl"},
		0,
		int64(len(line)),
		"2026-07-12T00:00:00Z",
		func(string) (readSeekCloser, error) { return reader, nil },
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("processJSONL error = %v, want wrapping %v", err, wantErr)
	}
	if handler.lines != 1 {
		t.Errorf("HandleLine calls = %d, want 1", handler.lines)
	}
	if result.NewOffset != 0 {
		t.Errorf("NewOffset = %d, want 0", result.NewOffset)
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
	if !reader.closed {
		t.Error("reader Close was not called")
	}
}

type ignoredHandler struct{}

func (ignoredHandler) Begin(string, int64) error { return nil }

func (ignoredHandler) HandleLine(ImportTransaction, []byte) (LineOutcome, error) {
	return LineIgnored, nil
}

func (ignoredHandler) Flush(ImportTransaction) error { return nil }

type beginErrorHandler struct{ err error }

func (h beginErrorHandler) Begin(string, int64) error { return h.err }

func (beginErrorHandler) HandleLine(ImportTransaction, []byte) (LineOutcome, error) {
	return LineIgnored, nil
}

func (beginErrorHandler) Flush(ImportTransaction) error { return nil }

type bodyHandler struct{ lines int }

func (*bodyHandler) Begin(string, int64) error { return nil }

func (h *bodyHandler) HandleLine(ImportTransaction, []byte) (LineOutcome, error) {
	h.lines++
	return LineWroteBody, nil
}

func (*bodyHandler) Flush(ImportTransaction) error { return nil }

type failingPersistenceHandler struct {
	bodyHandler
	flushErr error
	flushes  int
}

func (h *failingPersistenceHandler) Flush(ImportTransaction) error {
	h.flushes++
	return h.flushErr
}

type errorAfterReader struct {
	data   *strings.Reader
	err    error
	failed bool
	closed bool
}

func (r *errorAfterReader) Read(p []byte) (int, error) {
	if r.failed {
		return 0, r.err
	}
	n, err := r.data.Read(p)
	if err == io.EOF {
		r.failed = true
		return n, nil
	}
	return n, err
}

func (r *errorAfterReader) Seek(offset int64, whence int) (int64, error) {
	return r.data.Seek(offset, whence)
}

func (r *errorAfterReader) Close() error {
	r.closed = true
	return nil
}

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
	upsertErr         error
	commitErr         error
}

func (t *processRecordingTx) UpsertSession(SessionMeta, string) error { return nil }

func (t *processRecordingTx) InsertMessage(NormalizedMessage) error { return nil }

func (t *processRecordingTx) UpsertImportState(ImportState) error {
	t.importStateWrites++
	return t.upsertErr
}

func (t *processRecordingTx) Commit() error {
	t.commits++
	return t.commitErr
}

func (t *processRecordingTx) Rollback() error {
	t.rollbacks++
	return nil
}

func TestProcessJSONL_PersistenceFailureKeepsOffsetAndRollsBack(t *testing.T) {
	const prefix = "prior-body\n"
	const line = "body\n"

	path := filepath.Join(t.TempDir(), "session.jsonl")
	if err := os.WriteFile(path, []byte(prefix+line), 0o600); err != nil {
		t.Fatal(err)
	}
	offset := int64(len(prefix))

	tests := []struct {
		name             string
		configure        func(*failingPersistenceHandler, *processRecordingTx, error)
		wantFlushes      int
		wantImportStates int
		wantCommits      int
	}{
		{
			name: "flush",
			configure: func(handler *failingPersistenceHandler, _ *processRecordingTx, wantErr error) {
				handler.flushErr = wantErr
			},
			wantFlushes:      1,
			wantImportStates: 0,
			wantCommits:      0,
		},
		{
			name: "upsert import state",
			configure: func(_ *failingPersistenceHandler, tx *processRecordingTx, wantErr error) {
				tx.upsertErr = wantErr
			},
			wantFlushes:      1,
			wantImportStates: 1,
			wantCommits:      0,
		},
		{
			name: "commit",
			configure: func(_ *failingPersistenceHandler, tx *processRecordingTx, wantErr error) {
				tx.commitErr = wantErr
			},
			wantFlushes:      1,
			wantImportStates: 1,
			wantCommits:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantErr := errors.New(tt.name + " failed")
			handler := &failingPersistenceHandler{}
			tx := &processRecordingTx{}
			tt.configure(handler, tx, wantErr)

			result, err := ProcessJSONL(
				func() (ImportTransaction, error) { return tx, nil },
				SourceClaudeCode,
				handler,
				File{Path: path},
				offset,
				int64(len(prefix+line)),
				"2026-07-12T00:00:00Z",
			)

			if !errors.Is(err, wantErr) {
				t.Fatalf("ProcessJSONL error = %v, want wrapping %v", err, wantErr)
			}
			if result.NewOffset != offset {
				t.Errorf("NewOffset = %d, want %d", result.NewOffset, offset)
			}
			if handler.flushes != tt.wantFlushes {
				t.Errorf("Flush calls = %d, want %d", handler.flushes, tt.wantFlushes)
			}
			if tx.importStateWrites != tt.wantImportStates {
				t.Errorf("UpsertImportState calls = %d, want %d", tx.importStateWrites, tt.wantImportStates)
			}
			if tx.commits != tt.wantCommits {
				t.Errorf("Commit calls = %d, want %d", tx.commits, tt.wantCommits)
			}
			if tx.rollbacks != 1 {
				t.Errorf("Rollback calls = %d, want 1", tx.rollbacks)
			}
		})
	}
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
