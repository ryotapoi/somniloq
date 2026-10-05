package main

import (
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func insertOutlineMessage(t *testing.T, db *core.DB, sessionID, uuid, role, content, timestamp string, sidechain bool) {
	t.Helper()
	if err := db.InsertMessage(testInputID(t, db,

		core.SourceClaudeCode), core.NormalizedMessage{
		UUID:        uuid,
		Number:      map[string]int{"u1": 1, "a1": 2, "s1": 3, "u2": 4}[uuid],
		Source:      core.SourceClaudeCode,
		SessionID:   sessionID,
		Role:        role,
		Content:     content,
		Timestamp:   timestamp,
		IsSidechain: sidechain,
	}); err != nil {
		t.Fatalf("InsertMessage(%s): %v", uuid, err)
	}
}

func newOutlineTestDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "sess-1",
		CWD:       "/Users/test/proj",
		RepoPath:  "/Users/test/proj",
		StartedAt: "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

	insertOutlineMessage(t, db, "sess-1", "u1", "user", "first question\nwith detail", "2026-03-28T15:00:00Z", false)
	insertOutlineMessage(t, db, "sess-1", "a1", "assistant", "answer one", "2026-03-28T15:01:00Z", false)
	insertOutlineMessage(t, db, "sess-1", "s1", "user", "sidechain prompt", "2026-03-28T15:02:00Z", true)
	insertOutlineMessage(t, db, "sess-1", "u2", "user", "\n\nsecond\tquestion after blank lines", "2026-03-28T15:03:00Z", false)
	return db
}

func staticDB(db *core.DB) func() (*core.DB, error) {
	return func() (*core.DB, error) { return db, nil }
}
