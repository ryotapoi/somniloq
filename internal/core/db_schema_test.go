package core

import (
	"path/filepath"
	"testing"
)

func TestOpenDB_ReopenPreservesSchemaAndMessageRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.db")
	db, err := OpenDB(path)
	must(t, err)
	inputID := testInput(t, db, SourceCodex)
	must(t, db.UpsertSession(inputID, SessionMeta{Source: SourceCodex, SessionID: "s1"}, "before"))
	message := NormalizedMessage{Source: SourceCodex, SessionID: "s1", UUID: "first", Role: "user", Content: "original", Timestamp: ""}
	must(t, db.InsertMessage(inputID, message))
	var beforeRowID, beforeVersion int
	must(t, db.db.QueryRow(`SELECT rowid FROM messages WHERE uuid='first'`).Scan(&beforeRowID))
	must(t, db.db.QueryRow(`PRAGMA schema_version`).Scan(&beforeVersion))
	var revision, receipts int
	must(t, db.db.QueryRow(`PRAGMA user_version`).Scan(&revision))
	must(t, db.db.QueryRow(`SELECT COUNT(*) FROM migration_origin`).Scan(&receipts))
	if revision != 1 || receipts != 0 {
		t.Fatalf("revision/receipts = %d/%d", revision, receipts)
	}
	must(t, db.Close())
	for i := 0; i < 2; i++ {
		db, err = OpenDB(path)
		must(t, err)
		message.Content = "replacement"
		must(t, db.InsertMessage(inputID, message))
		var rowID, version int
		must(t, db.db.QueryRow(`SELECT rowid FROM messages WHERE uuid='first'`).Scan(&rowID))
		must(t, db.db.QueryRow(`PRAGMA schema_version`).Scan(&version))
		if rowID != beforeRowID || version != beforeVersion {
			t.Fatalf("row/schema changed: %d/%d", rowID, version)
		}
		messages, err := db.GetMessages(inputID, SourceCodex, "s1")
		must(t, err)
		if len(messages) != 1 || messages[0].Content != "original" {
			t.Fatalf("messages = %+v", messages)
		}
		must(t, db.Close())
	}
}
