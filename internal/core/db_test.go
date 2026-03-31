package core

import (
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}
}

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

func TestListSessions_Empty(t *testing.T) {
	db := testDB(t)

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestListSessions_OrderAndCount(t *testing.T) {
	db := testDB(t)

	// Older session with 1 message
	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-proj1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	// Newer session with 2 messages
	must(t, db.UpsertSession(SessionMeta{SessionID: "s2", ProjectDir: "-Users-test-proj2", StartedAt: "2026-03-28T14:00:00Z", EndedAt: "2026-03-28T14:30:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m2", SessionID: "s2", Role: "user", Content: "hi", Timestamp: "2026-03-28T14:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m3", SessionID: "s2", Role: "assistant", Content: "hey", Timestamp: "2026-03-28T14:01:00Z"}))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Newer session first (DESC order)
	if rows[0].SessionID != "s2" {
		t.Errorf("first row should be s2 (newer), got %s", rows[0].SessionID)
	}
	if rows[0].ProjectDir != "-Users-test-proj2" {
		t.Errorf("s2 project_dir: got %s, want -Users-test-proj2", rows[0].ProjectDir)
	}
	if rows[0].StartedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("s2 started_at: got %s, want 2026-03-28T14:00:00Z", rows[0].StartedAt)
	}
	if rows[0].MessageCount != 2 {
		t.Errorf("s2 message count: got %d, want 2", rows[0].MessageCount)
	}
	if rows[1].SessionID != "s1" {
		t.Errorf("second row should be s1 (older), got %s", rows[1].SessionID)
	}
	if rows[1].MessageCount != 1 {
		t.Errorf("s1 message count: got %d, want 1", rows[1].MessageCount)
	}
}

func TestListSessions_ZeroMessages(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].MessageCount != 0 {
		t.Errorf("message count: got %d, want 0", rows[0].MessageCount)
	}
}

func TestListSessions_NullTitle(t *testing.T) {
	db := testDB(t)

	// Session with no custom_title set
	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if rows[0].CustomTitle != "" {
		t.Errorf("custom_title should be empty string, got %q", rows[0].CustomTitle)
	}
}

func TestListSessions_NullStartedAt(t *testing.T) {
	db := testDB(t)

	// Session with started_at (normal)
	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Session created via title-only update (no started_at)
	must(t, db.UpdateSessionTitle("s2", "-test", "title only", "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Normal session first, NULL started_at at the end
	if rows[0].SessionID != "s1" {
		t.Errorf("first row should be s1 (has started_at), got %s", rows[0].SessionID)
	}
	if rows[1].SessionID != "s2" {
		t.Errorf("second row should be s2 (NULL started_at), got %s", rows[1].SessionID)
	}
}

func TestListSessions_SinceFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "old", ProjectDir: "-test", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "new", ProjectDir: "-test", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T00:00:00Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "new" {
		t.Errorf("expected session 'new', got %s", rows[0].SessionID)
	}
}

func TestListSessions_SinceFilter_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	// Real JSONL timestamps have milliseconds (e.g. "2026-03-28T14:10:45.977Z")
	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T14:10:45.977Z"}, "2026-03-28T15:00:00Z"))

	// Since filter with millisecond precision (as generated by cmd layer)
	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T14:10:45.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (ms timestamp should match), got %d", len(rows))
	}
}

func TestListSessions_ProjectFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "s2", ProjectDir: "-Users-test-somniloq", StartedAt: "2026-03-28T11:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Project: "Brimday"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "s1" {
		t.Errorf("expected s1, got %s", rows[0].SessionID)
	}
}

func TestListSessions_CombinedFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "old-brim", ProjectDir: "-Users-test-Brimday", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "new-brim", ProjectDir: "-Users-test-Brimday", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "new-somniloq", ProjectDir: "-Users-test-somniloq", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T00:00:00Z", Project: "Brimday"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "new-brim" {
		t.Errorf("expected new-brim, got %s", rows[0].SessionID)
	}
}

func TestListSessions_UntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "early", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "late", ProjectDir: "-test", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "early" {
		t.Errorf("expected 'early', got %s", rows[0].SessionID)
	}
}

func TestListSessions_SinceAndUntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "s2", ProjectDir: "-test", StartedAt: "2026-03-28T12:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "s3", ProjectDir: "-test", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T11:00:00.000Z", Until: "2026-03-28T13:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SessionID != "s2" {
		t.Errorf("expected 's2', got %s", rows[0].SessionID)
	}
}

func TestListSessions_UntilFilter_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T12:00:00.500Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (ms timestamp 500ms > 000ms), got %d", len(rows))
	}
}

func TestGetMessages_Empty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	msgs, err := db.GetMessages("s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetMessages_OrderByTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Insert in reverse order
	must(t, db.InsertMessage(ParsedMessage{UUID: "m2", SessionID: "s1", Role: "assistant", Content: "world", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: false}))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z", IsSidechain: true}))

	msgs, err := db.GetMessages("s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Should be in timestamp ASC order
	if msgs[0].UUID != "m1" {
		t.Errorf("first message UUID: got %s, want m1", msgs[0].UUID)
	}
	if msgs[0].Role != "user" {
		t.Errorf("first message Role: got %s, want user", msgs[0].Role)
	}
	if msgs[0].Content != "hello" {
		t.Errorf("first message Content: got %s, want hello", msgs[0].Content)
	}
	if msgs[0].Timestamp != "2026-03-28T10:00:00Z" {
		t.Errorf("first message Timestamp: got %s, want 2026-03-28T10:00:00Z", msgs[0].Timestamp)
	}
	if msgs[0].IsSidechain != true {
		t.Errorf("first message IsSidechain: got %v, want true", msgs[0].IsSidechain)
	}

	if msgs[1].UUID != "m2" {
		t.Errorf("second message UUID: got %s, want m2", msgs[1].UUID)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("second message Role: got %s, want assistant", msgs[1].Role)
	}
	if msgs[1].IsSidechain != false {
		t.Errorf("second message IsSidechain: got %v, want false", msgs[1].IsSidechain)
	}
}

func TestGetSession_Found(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle("s1", "-Users-test-proj", "my session", "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	got, err := db.GetSession("s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID: got %s, want s1", got.SessionID)
	}
	if got.ProjectDir != "-Users-test-proj" {
		t.Errorf("ProjectDir: got %s, want -Users-test-proj", got.ProjectDir)
	}
	if got.StartedAt != "2026-03-28T10:00:00Z" {
		t.Errorf("StartedAt: got %s, want 2026-03-28T10:00:00Z", got.StartedAt)
	}
	if got.CustomTitle != "my session" {
		t.Errorf("CustomTitle: got %q, want %q", got.CustomTitle, "my session")
	}
	if got.MessageCount != 1 {
		t.Errorf("MessageCount: got %d, want 1", got.MessageCount)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetSession("nonexistent")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestListProjects_Empty(t *testing.T) {
	db := testDB(t)

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestListProjects_GroupByProject(t *testing.T) {
	db := testDB(t)

	// Project A: 2 sessions
	must(t, db.UpsertSession(SessionMeta{SessionID: "a1", ProjectDir: "-Users-test-projA", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "a2", ProjectDir: "-Users-test-projA", StartedAt: "2026-03-28T11:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Project B: 1 session
	must(t, db.UpsertSession(SessionMeta{SessionID: "b1", ProjectDir: "-Users-test-projB", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Project B first (latest started_at is 14:00, A's latest is 11:00)
	if rows[0].ProjectDir != "-Users-test-projB" {
		t.Errorf("first row: got %s, want -Users-test-projB", rows[0].ProjectDir)
	}
	if rows[0].SessionCount != 1 {
		t.Errorf("projB session count: got %d, want 1", rows[0].SessionCount)
	}
	if rows[1].ProjectDir != "-Users-test-projA" {
		t.Errorf("second row: got %s, want -Users-test-projA", rows[1].ProjectDir)
	}
	if rows[1].SessionCount != 2 {
		t.Errorf("projA session count: got %d, want 2", rows[1].SessionCount)
	}
}

func TestListProjects_SinceFilter(t *testing.T) {
	db := testDB(t)

	// Old project (only old sessions)
	must(t, db.UpsertSession(SessionMeta{SessionID: "old1", ProjectDir: "-Users-test-old", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// New project
	must(t, db.UpsertSession(SessionMeta{SessionID: "new1", ProjectDir: "-Users-test-new", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Since: "2026-03-28T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ProjectDir != "-Users-test-new" {
		t.Errorf("expected -Users-test-new, got %s", rows[0].ProjectDir)
	}
}

func TestListProjects_UntilFilter(t *testing.T) {
	db := testDB(t)

	// Early project
	must(t, db.UpsertSession(SessionMeta{SessionID: "early1", ProjectDir: "-Users-test-early", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Late project (only late sessions)
	must(t, db.UpsertSession(SessionMeta{SessionID: "late1", ProjectDir: "-Users-test-late", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ProjectDir != "-Users-test-early" {
		t.Errorf("expected -Users-test-early, got %s", rows[0].ProjectDir)
	}
}

func TestListProjects_SinceAndUntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-old", StartedAt: "2026-03-28T08:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "s2", ProjectDir: "-Users-test-mid", StartedAt: "2026-03-28T12:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{SessionID: "s3", ProjectDir: "-Users-test-new", StartedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Since: "2026-03-28T10:00:00.000Z", Until: "2026-03-28T14:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].ProjectDir != "-Users-test-mid" {
		t.Errorf("expected -Users-test-mid, got %s", rows[0].ProjectDir)
	}
}

func TestListProjects_NullStartedAt(t *testing.T) {
	db := testDB(t)

	// Normal session
	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-normal", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Session with NULL started_at (title-only)
	must(t, db.UpdateSessionTitle("s2", "-Users-test-titleonly", "title", "2026-03-28T15:00:00Z"))

	// No filter: both projects should appear
	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Normal project first, NULL started_at project last
	if rows[0].ProjectDir != "-Users-test-normal" {
		t.Errorf("first row: got %s, want -Users-test-normal", rows[0].ProjectDir)
	}
	if rows[1].ProjectDir != "-Users-test-titleonly" {
		t.Errorf("second row: got %s, want -Users-test-titleonly", rows[1].ProjectDir)
	}

	// With filter: NULL started_at excluded
	rows, err = db.ListProjects(SessionFilter{Since: "2026-03-28T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects with Since failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row with Since filter, got %d", len(rows))
	}
	if rows[0].ProjectDir != "-Users-test-normal" {
		t.Errorf("expected -Users-test-normal, got %s", rows[0].ProjectDir)
	}
}

func TestListProjects_CWD(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		SessionID:  "s1",
		ProjectDir: "-Users-test-projA",
		CWD:        "/Users/test/projA",
		StartedAt:  "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CWD != "/Users/test/projA" {
		t.Errorf("CWD: got %q, want %q", rows[0].CWD, "/Users/test/projA")
	}
}

func TestListProjects_CWD_LatestSession(t *testing.T) {
	db := testDB(t)

	// Same project, different cwds; older session has alphabetically later cwd
	must(t, db.UpsertSession(SessionMeta{
		SessionID:  "s1",
		ProjectDir: "-Users-test-proj",
		CWD:        "/Users/test/proj/sub",
		StartedAt:  "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		SessionID:  "s2",
		ProjectDir: "-Users-test-proj",
		CWD:        "/Users/test/proj",
		StartedAt:  "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// Latest session (s2, started at 11:00) has cwd "/Users/test/proj"
	if rows[0].CWD != "/Users/test/proj" {
		t.Errorf("CWD: got %q, want %q", rows[0].CWD, "/Users/test/proj")
	}
}

func TestListProjects_CWD_NullReturnsEmpty(t *testing.T) {
	db := testDB(t)

	// Session with no CWD (empty string → stored as NULL or empty)
	must(t, db.UpsertSession(SessionMeta{
		SessionID:  "s1",
		ProjectDir: "-Users-test-proj",
		StartedAt:  "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CWD != "" {
		t.Errorf("CWD: got %q, want empty string", rows[0].CWD)
	}
}

func TestGetSummaryMessages_Empty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	msgs, err := db.GetSummaryMessages("s1")
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_ReturnsFirstUserMessage(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m1", SessionID: "s1", Role: "user", Content: "fix the bug", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m2", SessionID: "s1", Role: "assistant", Content: "done", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m3", SessionID: "s1", Role: "user", Content: "thanks", Timestamp: "2026-03-28T10:02:00Z"}))

	msgs, err := db.GetSummaryMessages("s1")
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m1" {
		t.Errorf("UUID: got %s, want m1", msgs[0].UUID)
	}
	if msgs[0].Role != "user" {
		t.Errorf("Role: got %s, want user", msgs[0].Role)
	}
	if msgs[0].Content != "fix the bug" {
		t.Errorf("Content: got %s, want 'fix the bug'", msgs[0].Content)
	}
	if msgs[0].Timestamp != "2026-03-28T10:00:00Z" {
		t.Errorf("Timestamp: got %s, want 2026-03-28T10:00:00Z", msgs[0].Timestamp)
	}
	if msgs[0].IsSidechain != false {
		t.Errorf("IsSidechain: got %v, want false", msgs[0].IsSidechain)
	}
}

func TestGetSummaryMessages_SkipsSidechain(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m1", SessionID: "s1", Role: "user", Content: "sidechain msg", Timestamp: "2026-03-28T10:00:00Z", IsSidechain: true}))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m2", SessionID: "s1", Role: "user", Content: "real msg", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: false}))

	msgs, err := db.GetSummaryMessages("s1")
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m2" {
		t.Errorf("expected m2 (non-sidechain), got %s", msgs[0].UUID)
	}
}

func TestGetSummaryMessages_NoUserMessages(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(ParsedMessage{UUID: "m1", SessionID: "s1", Role: "assistant", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	msgs, err := db.GetSummaryMessages("s1")
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
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

func TestListSessions_CWD(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-proj", CWD: "/Users/test/proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CWD != "/Users/test/proj" {
		t.Errorf("CWD: got %q, want %q", rows[0].CWD, "/Users/test/proj")
	}
}

func TestGetSession_CWD(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{SessionID: "s1", ProjectDir: "-Users-test-proj", CWD: "/Users/test/proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession("s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.CWD != "/Users/test/proj" {
		t.Errorf("CWD: got %q, want %q", got.CWD, "/Users/test/proj")
	}
}

func TestListSessions_CWD_Null(t *testing.T) {
	db := testDB(t)

	// Session with no CWD (title-only)
	must(t, db.UpdateSessionTitle("s1", "-test", "title", "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].CWD != "" {
		t.Errorf("CWD should be empty for NULL cwd, got %q", rows[0].CWD)
	}
}
