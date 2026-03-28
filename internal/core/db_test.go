package core

import (
	"testing"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenDB_CreatesSchema(t *testing.T) {
	db := testDB(t)

	tables := []string{"sessions", "messages", "import_state"}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestUpsertSession(t *testing.T) {
	db := testDB(t)

	meta := SessionMeta{
		SessionID:  "s1",
		ProjectDir: "-Users-test",
		CWD:        "/tmp",
		GitBranch:  "main",
		Version:    "2.1.86",
		StartedAt:  "2026-03-28T14:00:00Z",
		EndedAt:    "2026-03-28T14:10:00Z",
	}
	if err := db.UpsertSession(meta, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var sid, projDir, startedAt string
	err := db.db.QueryRow("SELECT session_id, project_dir, started_at FROM sessions WHERE session_id='s1'").
		Scan(&sid, &projDir, &startedAt)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if sid != "s1" || projDir != "-Users-test" || startedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("unexpected row: sid=%s projDir=%s startedAt=%s", sid, projDir, startedAt)
	}

	// Second upsert with later ended_at
	meta2 := SessionMeta{
		SessionID:  "s1",
		ProjectDir: "-Users-test",
		CWD:        "/tmp",
		StartedAt:  "2026-03-28T14:05:00Z",
		EndedAt:    "2026-03-28T14:20:00Z",
	}
	if err := db.UpsertSession(meta2, "2026-03-28T15:01:00Z"); err != nil {
		t.Fatalf("UpsertSession (2nd) failed: %v", err)
	}

	var endedAt string
	err = db.db.QueryRow("SELECT started_at, ended_at FROM sessions WHERE session_id='s1'").
		Scan(&startedAt, &endedAt)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if startedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("started_at should be MIN: got %s", startedAt)
	}
	if endedAt != "2026-03-28T14:20:00Z" {
		t.Errorf("ended_at should be MAX: got %s", endedAt)
	}
}

func TestInsertMessage(t *testing.T) {
	db := testDB(t)

	// Need a session first
	if err := db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	parent := "p1"
	msg := ParsedMessage{
		UUID:        "m1",
		ParentUUID:  &parent,
		SessionID:   "s1",
		Role:        "user",
		Content:     "hello",
		Timestamp:   "2026-03-28T14:00:00Z",
		IsSidechain: false,
	}
	if err := db.InsertMessage(msg); err != nil {
		t.Fatalf("InsertMessage failed: %v", err)
	}

	var uuid, role, content string
	err := db.db.QueryRow("SELECT uuid, role, content FROM messages WHERE uuid='m1'").
		Scan(&uuid, &role, &content)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if role != "user" || content != "hello" {
		t.Errorf("unexpected: role=%s content=%s", role, content)
	}

	// Duplicate insert should not error
	if err := db.InsertMessage(msg); err != nil {
		t.Fatalf("duplicate InsertMessage should not error: %v", err)
	}
}

func TestUpdateSessionTitle(t *testing.T) {
	db := testDB(t)

	// UpdateSessionTitle on non-existent session should create minimal row
	if err := db.UpdateSessionTitle("s1", "-test", "my title", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle failed: %v", err)
	}

	var title string
	err := db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "my title" {
		t.Errorf("got %q, want %q", title, "my title")
	}
}

func TestUpsertImportState(t *testing.T) {
	db := testDB(t)

	state := ImportState{
		JSONLPath:  "/path/to/file.jsonl",
		FileSize:   1000,
		LastOffset: 500,
		ImportedAt: "2026-03-28T15:00:00Z",
	}
	if err := db.UpsertImportState(state); err != nil {
		t.Fatalf("UpsertImportState failed: %v", err)
	}

	got, err := db.GetImportState("/path/to/file.jsonl")
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil state")
	}
	if got.FileSize != 1000 || got.LastOffset != 500 {
		t.Errorf("unexpected state: %+v", got)
	}

	// Update
	state.FileSize = 2000
	state.LastOffset = 1500
	if err := db.UpsertImportState(state); err != nil {
		t.Fatalf("UpsertImportState (update) failed: %v", err)
	}
	got, _ = db.GetImportState("/path/to/file.jsonl")
	if got.FileSize != 2000 || got.LastOffset != 1500 {
		t.Errorf("update failed: %+v", got)
	}
}

func TestGetImportState_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetImportState("/nonexistent")
	if err != nil {
		t.Fatalf("GetImportState failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestUpdateSessionAgentName(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateSessionAgentName("s1", "-test", "agent1", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionAgentName failed: %v", err)
	}

	var name string
	err := db.db.QueryRow("SELECT agent_name FROM sessions WHERE session_id='s1'").Scan(&name)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if name != "agent1" {
		t.Errorf("got %q, want %q", name, "agent1")
	}
}
