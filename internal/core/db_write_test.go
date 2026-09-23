package core

import (
	"database/sql"
	"testing"
)

func TestUpsertSession(t *testing.T) {
	db := testDB(t)

	meta := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		CWD:       "/tmp",
		RepoPath:  "/Users/test",
		GitBranch: "main",
		Version:   "2.1.86",
		StartedAt: "2026-03-28T14:00:00Z",
		EndedAt:   "2026-03-28T14:10:00Z",
	}
	if err := db.UpsertSession(meta, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var sid, startedAt, repoPath string
	err := db.db.QueryRow("SELECT session_id, started_at, repo_path FROM sessions WHERE session_id='s1'").
		Scan(&sid, &startedAt, &repoPath)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if sid != "s1" || startedAt != "2026-03-28T14:00:00Z" {
		t.Errorf("unexpected row: sid=%s startedAt=%s", sid, startedAt)
	}
	if repoPath != "/Users/test" {
		t.Errorf("repo_path: got %q, want %q", repoPath, "/Users/test")
	}

	meta2 := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		CWD:       "/tmp",
		RepoPath:  "/Users/test",
		StartedAt: "2026-03-28T14:05:00Z",
		EndedAt:   "2026-03-28T14:20:00Z",
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

func TestUpsertSession_ChoosesInstantsAndPreservesUnknowns(t *testing.T) {
	db := testDB(t)
	upsert := func(startedAt, endedAt string) {
		t.Helper()
		if err := db.UpsertSession(SessionMeta{
			Source: SourceClaudeCode, SessionID: "instant-upsert", StartedAt: startedAt, EndedAt: endedAt,
		}, "2026-03-28T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%q, %q): %v", startedAt, endedAt, err)
		}
	}
	read := func() (string, string) {
		t.Helper()
		var startedAt, endedAt string
		if err := db.db.QueryRow("SELECT started_at, ended_at FROM sessions WHERE session_id='instant-upsert'").Scan(&startedAt, &endedAt); err != nil {
			t.Fatalf("read session endpoints: %v", err)
		}
		return startedAt, endedAt
	}

	upsert("2026-03-28T08:30:00Z", "2026-03-28T09:00:00Z")
	upsert("2026-03-28T10:00:00+02:00", "2026-03-28T10:30:00+02:00")
	if startedAt, endedAt := read(); startedAt != "2026-03-28T10:00:00+02:00" || endedAt != "2026-03-28T09:00:00Z" {
		t.Fatalf("after earlier offset values: started_at=%q ended_at=%q", startedAt, endedAt)
	}
	upsert("2026-03-28T08:00:00.5Z", "2026-03-28T11:00:00.5+02:00")
	if startedAt, endedAt := read(); startedAt != "2026-03-28T10:00:00+02:00" || endedAt != "2026-03-28T11:00:00.5+02:00" {
		t.Fatalf("after later offset/fraction values: started_at=%q ended_at=%q", startedAt, endedAt)
	}
	upsert("", "")
	if startedAt, endedAt := read(); startedAt != "2026-03-28T10:00:00+02:00" || endedAt != "2026-03-28T11:00:00.5+02:00" {
		t.Fatalf("empty values erased known endpoints: started_at=%q ended_at=%q", startedAt, endedAt)
	}

	for _, sessionID := range []string{"empty-endpoints", "null-endpoints"} {
		if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: sessionID}, "2026-03-28T15:00:00Z"); err != nil {
			t.Fatalf("insert %s: %v", sessionID, err)
		}
	}
	if _, err := db.db.Exec("UPDATE sessions SET started_at = NULL, ended_at = NULL WHERE session_id='null-endpoints'"); err != nil {
		t.Fatalf("set null endpoints: %v", err)
	}
	for _, sessionID := range []string{"empty-endpoints", "null-endpoints"} {
		var startedAt, endedAt sql.NullString
		if err := db.db.QueryRow("SELECT started_at, ended_at FROM sessions WHERE session_id=?", sessionID).Scan(&startedAt, &endedAt); err != nil {
			t.Fatalf("read unknown endpoints for %s: %v", sessionID, err)
		}
		if sessionID == "empty-endpoints" && (startedAt.String != "" || endedAt.String != "") {
			t.Fatalf("empty endpoints changed before update: %q, %q", startedAt.String, endedAt.String)
		}
		if sessionID == "null-endpoints" && (startedAt.Valid || endedAt.Valid) {
			t.Fatalf("NULL endpoints changed before update: %+v, %+v", startedAt, endedAt)
		}
		if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: sessionID}, "2026-03-28T15:01:00Z"); err != nil {
			t.Fatalf("empty update for %s: %v", sessionID, err)
		}
	}
	var nullStart, nullEnd sql.NullString
	if err := db.db.QueryRow("SELECT started_at, ended_at FROM sessions WHERE session_id='null-endpoints'").Scan(&nullStart, &nullEnd); err != nil {
		t.Fatalf("read null endpoints after empty update: %v", err)
	}
	if nullStart.Valid || nullEnd.Valid {
		t.Fatalf("unknown NULL endpoints were replaced with values: %+v, %+v", nullStart, nullEnd)
	}

	for _, sessionID := range []string{"empty-endpoints", "null-endpoints"} {
		if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: sessionID, StartedAt: "2026-03-28T08:00:00.1Z", EndedAt: "2026-03-28T09:00:00.100+01:00"}, "2026-03-28T15:02:00Z"); err != nil {
			t.Fatalf("fill endpoints for %s: %v", sessionID, err)
		}
		var startedAt, endedAt string
		if err := db.db.QueryRow("SELECT started_at, ended_at FROM sessions WHERE session_id=?", sessionID).Scan(&startedAt, &endedAt); err != nil {
			t.Fatalf("read filled endpoints for %s: %v", sessionID, err)
		}
		if startedAt != "2026-03-28T08:00:00.1Z" || endedAt != "2026-03-28T09:00:00.100+01:00" {
			t.Errorf("filled endpoints for %s = %q, %q", sessionID, startedAt, endedAt)
		}
	}
}

func TestUpsertSession_RepoPath(t *testing.T) {
	db := testDB(t)

	// Use distinct values for every text column so that any order mismatch
	// between the Go args and the SQL placeholders is immediately visible.
	meta := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s-map",
		CWD:       "cwd-val",
		RepoPath:  "repo-val",
		GitBranch: "branch-val",
		Version:   "version-val",
		StartedAt: "started-val",
		EndedAt:   "ended-val",
	}
	if err := db.UpsertSession(meta, "imported-val"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var cwd, repoPath, branch, version, startedAt, endedAt string
	err := db.db.QueryRow(`SELECT cwd, repo_path, git_branch, version, started_at, ended_at FROM sessions WHERE session_id='s-map'`).
		Scan(&cwd, &repoPath, &branch, &version, &startedAt, &endedAt)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	checks := []struct {
		label, got, want string
	}{
		{"cwd", cwd, "cwd-val"},
		{"repo_path", repoPath, "repo-val"},
		{"git_branch", branch, "branch-val"},
		{"version", version, "version-val"},
		{"started_at", startedAt, "started-val"},
		{"ended_at", endedAt, "ended-val"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.label, c.got, c.want)
		}
	}
}

func TestUpsertSession_RepoPath_EmptyInsertsNull(t *testing.T) {
	db := testDB(t)

	meta := SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "",
	}
	if err := db.UpsertSession(meta, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	var isNull int
	if err := db.db.QueryRow("SELECT repo_path IS NULL FROM sessions WHERE session_id='s1'").Scan(&isNull); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if isNull != 1 {
		t.Errorf("repo_path should be NULL for empty RepoPath on insert")
	}
}

func TestUpsertSession_RepoPath_EmptyDoesNotOverwrite(t *testing.T) {
	db := testDB(t)

	if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/proj"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("first UpsertSession failed: %v", err)
	}
	if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: ""}, "2026-03-28T15:01:00Z"); err != nil {
		t.Fatalf("second UpsertSession failed: %v", err)
	}

	var repoPath string
	if err := db.db.QueryRow("SELECT repo_path FROM sessions WHERE session_id='s1'").Scan(&repoPath); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if repoPath != "/Users/test/proj" {
		t.Errorf("repo_path should not be overwritten by empty value: got %q", repoPath)
	}
}

func TestUpsertSession_RepoPath_AfterUpdateSessionTitle(t *testing.T) {
	db := testDB(t)

	// UpdateSessionTitle on an existing row only touches custom_title/imported_at,
	// not repo_path.
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/proj"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s1", "title", "2026-03-28T15:01:00Z"))

	var repoPath string
	if err := db.db.QueryRow("SELECT repo_path FROM sessions WHERE session_id='s1'").Scan(&repoPath); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if repoPath != "/Users/test/proj" {
		t.Errorf("repo_path: got %q, want %q", repoPath, "/Users/test/proj")
	}

	var title string
	if err := db.db.QueryRow("SELECT custom_title FROM sessions WHERE session_id='s1'").Scan(&title); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if title != "title" {
		t.Errorf("custom_title: got %q, want %q", title, "title")
	}
}

func TestInsertMessage(t *testing.T) {
	db := testDB(t)

	if err := db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}

	parent := "p1"
	msg := NormalizedMessage{
		Source:      SourceClaudeCode,
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

	if err := db.InsertMessage(msg); err != nil {
		t.Fatalf("duplicate InsertMessage should not error: %v", err)
	}
}

func TestUpdateSessionTitle(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"))
	if err := db.UpdateSessionTitle(SourceClaudeCode, "s1", "my title", "2026-03-28T15:00:00Z"); err != nil {
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

func TestUpdateSessionTitle_NoRow_IsNoop(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateSessionTitle(SourceClaudeCode, "ghost", "title", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle should not error on missing row: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id='ghost'").Scan(&count); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if count != 0 {
		t.Errorf("UpdateSessionTitle should not create rows; got %d", count)
	}
}

func TestUpsertImportState(t *testing.T) {
	db := testDB(t)

	state := ImportState{
		JSONLPath:  "/path/to/file.jsonl",
		Source:     SourceClaudeCode,
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

func TestDeleteAll_RollsBackOnDeleteFailure(t *testing.T) {
	db := testDB(t)
	if err := db.DeleteAll(); err != nil {
		t.Fatalf("DeleteAll on empty database: %v", err)
	}
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "session-before"}, "imported-before"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "message-before", SessionID: "session-before", Role: "user", Content: "content-before", Timestamp: "timestamp-before"}))
	must(t, db.UpsertImportState(ImportState{JSONLPath: "path-before", Source: SourceClaudeCode, FileSize: 42, LastOffset: 21, ImportedAt: "state-before"}))
	if _, err := db.db.Exec(`CREATE TRIGGER fail_import_state_delete BEFORE DELETE ON import_state BEGIN SELECT RAISE(ABORT, 'delete failed'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := db.DeleteAll(); err == nil {
		t.Fatal("DeleteAll succeeded despite the import_state trigger")
	}

	var sessionID, importedAt string
	if err := db.db.QueryRow(`SELECT session_id, imported_at FROM sessions`).Scan(&sessionID, &importedAt); err != nil {
		t.Fatalf("read sessions after failed delete: %v", err)
	}
	if sessionID != "session-before" || importedAt != "imported-before" {
		t.Errorf("sessions changed after failed delete: %q, %q", sessionID, importedAt)
	}
	var uuid, content string
	if err := db.db.QueryRow(`SELECT uuid, content FROM messages`).Scan(&uuid, &content); err != nil {
		t.Fatalf("read messages after failed delete: %v", err)
	}
	if uuid != "message-before" || content != "content-before" {
		t.Errorf("messages changed after failed delete: %q, %q", uuid, content)
	}
	var path string
	var fileSize, lastOffset int64
	if err := db.db.QueryRow(`SELECT jsonl_path, file_size, last_offset FROM import_state`).Scan(&path, &fileSize, &lastOffset); err != nil {
		t.Fatalf("read import_state after failed delete: %v", err)
	}
	if path != "path-before" || fileSize != 42 || lastOffset != 21 {
		t.Errorf("import_state changed after failed delete: %q, %d, %d", path, fileSize, lastOffset)
	}

	if _, err := db.db.Exec(`DROP TRIGGER fail_import_state_delete`); err != nil {
		t.Fatalf("drop trigger: %v", err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		if err := db.DeleteAll(); err != nil {
			t.Fatalf("DeleteAll attempt %d: %v", attempt, err)
		}
		for _, table := range []string{"messages", "sessions", "import_state"} {
			var count int
			if err := db.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
				t.Fatalf("count %s after attempt %d: %v", table, attempt, err)
			}
			if count != 0 {
				t.Errorf("%s count after attempt %d = %d, want 0", table, attempt, count)
			}
		}
	}
}

func TestUpdateSessionAgentName(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"))
	if err := db.UpdateSessionAgentName(SourceClaudeCode, "s1", "agent1", "2026-03-28T15:00:00Z"); err != nil {
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

func TestUpdateSessionAgentName_NoRow_IsNoop(t *testing.T) {
	db := testDB(t)

	if err := db.UpdateSessionAgentName(SourceClaudeCode, "ghost", "agent", "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionAgentName should not error on missing row: %v", err)
	}

	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM sessions WHERE session_id='ghost'").Scan(&count); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if count != 0 {
		t.Errorf("UpdateSessionAgentName should not create rows; got %d", count)
	}
}
