package ingest

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestPersistMessage_UpsertsSessionThenInsertsNonEmptyMessage(t *testing.T) {
	tx := &recordingTx{}
	record := normalizedRecord("hello")

	if err := PersistMessage(tx, &record, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("PersistMessage failed: %v", err)
	}

	if diff := cmpCalls(tx.calls, []string{"upsert:s1:2026-03-28T15:00:00Z", "insert:m1"}); diff != "" {
		t.Fatal(diff)
	}
	if !reflect.DeepEqual(tx.upserted, record.Session) {
		t.Fatalf("upserted session = %#v, want %#v", tx.upserted, record.Session)
	}
	if !reflect.DeepEqual(tx.inserted, record.Message) {
		t.Fatalf("inserted message = %#v, want %#v", tx.inserted, record.Message)
	}
}

func TestPersistMessage_EmptyContentUpsertsSessionOnly(t *testing.T) {
	for _, content := range []string{"", "  \n\t"} {
		t.Run("content="+content, func(t *testing.T) {
			tx := &recordingTx{}

			record := normalizedRecord(content)

			if err := PersistMessage(tx, &record, "2026-03-28T15:00:00Z"); err != nil {
				t.Fatalf("PersistMessage failed: %v", err)
			}

			if diff := cmpCalls(tx.calls, []string{"upsert:s1:2026-03-28T15:00:00Z"}); diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestPersistMessage_ReturnsUpsertErrorBeforeInsert(t *testing.T) {
	wantErr := errors.New("boom")
	tx := &recordingTx{upsertErr: wantErr}

	record := normalizedRecord("hello")

	err := PersistMessage(tx, &record, "2026-03-28T15:00:00Z")
	if !errors.Is(err, wantErr) {
		t.Fatalf("PersistMessage error = %v, want wrapping %v", err, wantErr)
	}
	if diff := cmpCalls(tx.calls, []string{"upsert:s1:2026-03-28T15:00:00Z"}); diff != "" {
		t.Fatal(diff)
	}
}

func TestPersistMessage_ReturnsInsertErrorAfterUpsert(t *testing.T) {
	wantErr := errors.New("boom")
	tx := &recordingTx{insertErr: wantErr}

	record := normalizedRecord("hello")

	err := PersistMessage(tx, &record, "2026-03-28T15:00:00Z")
	if !errors.Is(err, wantErr) {
		t.Fatalf("PersistMessage error = %v, want wrapping %v", err, wantErr)
	}
	if diff := cmpCalls(tx.calls, []string{"upsert:s1:2026-03-28T15:00:00Z", "insert:m1"}); diff != "" {
		t.Fatal(diff)
	}
}

func normalizedRecord(content string) NormalizedRecord {
	return NormalizedRecord{
		Session: SessionMeta{
			Source:    SourceClaudeCode,
			SessionID: "s1",
			CWD:       "/tmp/project",
			StartedAt: "2026-03-28T14:00:00Z",
			EndedAt:   "2026-03-28T14:00:00Z",
		},
		Message: NormalizedMessage{
			UUID:      "m1",
			Source:    SourceClaudeCode,
			SessionID: "s1",
			Role:      "user",
			Content:   content,
			Timestamp: "2026-03-28T14:00:00Z",
		},
	}
}

type recordingTx struct {
	calls     []string
	upserted  SessionMeta
	inserted  NormalizedMessage
	upsertErr error
	insertErr error
}

func (t *recordingTx) UpsertSession(meta SessionMeta, importedAt string) error {
	t.calls = append(t.calls, "upsert:"+meta.SessionID+":"+importedAt)
	t.upserted = meta
	return t.upsertErr
}

func (t *recordingTx) InsertMessage(msg NormalizedMessage) error {
	t.calls = append(t.calls, "insert:"+msg.UUID)
	t.inserted = msg
	return t.insertErr
}

func (t *recordingTx) UpsertImportState(state ImportState) error {
	return nil
}

func (t *recordingTx) Commit() error {
	return nil
}

func (t *recordingTx) Rollback() error {
	return nil
}

func cmpCalls(got, want []string) string {
	if reflect.DeepEqual(got, want) {
		return ""
	}
	return fmt.Sprintf("calls = %#v, want %#v", got, want)
}
