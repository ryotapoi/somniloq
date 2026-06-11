package core

import "testing"

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

	// Second upsert with later ended_at
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

	// Need a session first
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

	// Duplicate insert should not error
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
