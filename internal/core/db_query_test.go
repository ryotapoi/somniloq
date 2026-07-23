package core

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestQueryMethods_ClosedDatabaseErrorsIncludeOperationAndCause(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		query     func(*DB) error
	}{
		{
			name:      "GetImportState",
			operation: "get import state",
			query: func(db *DB) error {
				_, err := db.GetImportState("/tmp/session.jsonl")
				return err
			},
		},
		{
			name:      "ListSessions",
			operation: "list sessions",
			query: func(db *DB) error {
				_, err := db.ListSessions(SessionFilter{})
				return err
			},
		},
		{
			name:      "ListProjects",
			operation: "list projects",
			query: func(db *DB) error {
				_, err := db.ListProjects(SessionFilter{})
				return err
			},
		},
		{
			name:      "GetSession",
			operation: "get session",
			query: func(db *DB) error {
				_, err := db.GetSession(SourceClaudeCode, "session")
				return err
			},
		},
		{
			name:      "LookupSessionsByID",
			operation: "lookup sessions by ID",
			query: func(db *DB) error {
				_, err := db.LookupSessionsByID("session")
				return err
			},
		},
		{
			name:      "GetMessages",
			operation: "get messages",
			query: func(db *DB) error {
				_, err := db.GetMessages(SourceClaudeCode, "session")
				return err
			},
		},
		{
			name:      "GetSummaryMessages",
			operation: "get summary messages",
			query: func(db *DB) error {
				_, err := db.GetSummaryMessages(SourceClaudeCode, "session", 1, false)
				return err
			},
		},
		{
			name:      "SearchMessages",
			operation: "search messages",
			query: func(db *DB) error {
				_, err := db.SearchMessages(SessionFilter{}, "query")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			must(t, db.Close())

			err := tt.query(db)
			if err == nil {
				t.Fatal("expected query error from closed database")
			}
			if !strings.Contains(err.Error(), tt.operation) {
				t.Errorf("error %q does not identify operation %q", err, tt.operation)
			}
			if errors.Unwrap(err) == nil {
				t.Errorf("error %q does not retain its cause", err)
			}
		})
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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", StartedAt: "2026-03-28T14:00:00Z", EndedAt: "2026-03-28T14:30:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s2", Role: "user", Content: "hi", Timestamp: "2026-03-28T14:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s2", Role: "assistant", Content: "hey", Timestamp: "2026-03-28T14:01:00Z"}))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].SessionID != "s2" {
		t.Errorf("first row should be s2 (newer), got %s", rows[0].SessionID)
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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Session created via UpsertSession with no StartedAt, then title applied.
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s2", "title only", "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0].SessionID != "s1" {
		t.Errorf("first row should be s1 (has started_at), got %s", rows[0].SessionID)
	}
	if rows[1].SessionID != "s2" {
		t.Errorf("second row should be s2 (NULL started_at), got %s", rows[1].SessionID)
	}
}

func TestListSessions_SinceFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "old", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

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
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T14:10:45.977Z"}, "2026-03-28T15:00:00Z"))

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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-28T11:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Projects: []string{"Brimday"}})
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

func TestListSessions_RepoPath(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/Brimday" {
		t.Errorf("RepoPath: got %q, want %q", rows[0].RepoPath, "/Users/test/Brimday")
	}
}

func TestListSessions_RepoPath_NullReturnsEmpty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "" {
		t.Errorf("RepoPath should be empty for NULL, got %q", rows[0].RepoPath)
	}
}

func TestGetSession_RepoPath(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.RepoPath != "/Users/test/Brimday" {
		t.Errorf("RepoPath: got %q, want %q", got.RepoPath, "/Users/test/Brimday")
	}
}

func TestListSessions_ProjectFilter_LikeMetacharKnownLimitation(t *testing.T) {
	// Pin the documented Known limitation: LIKE wildcards in --project are not
	// escaped, so a literal "%" in the filter degenerates into a "match anything"
	// segment. This test catches a future change that decides to escape them.
	// Rename signal: if escape is added, rename/repurpose this test instead of
	// silently treating the new behavior as a regression.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "/Users/test/Brimday",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Projects: []string{"Brim%day"}})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (LIKE %% wildcard passthrough), got %d", len(rows))
	}
}

func TestListSessions_ProjectFilter_SlashSpan(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "/Users/ryota/Sources/ryotapoi/somniloq",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Projects: []string{"Sources/ryot"}})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row matching slash-span, got %d", len(rows))
	}
	if rows[0].SessionID != "s1" {
		t.Errorf("expected s1, got %s", rows[0].SessionID)
	}
}

// Multiple patterns come from project-alias expansion: a row matches when
// ANY pattern matches (OR), not when all do.
func TestListSessions_MultipleProjectsMatchAny(t *testing.T) {
	db := testDB(t)

	for i, repoPath := range []string{"/Users/test/somniloq", "/Users/test/Brimday", "/Users/test/other"} {
		must(t, db.UpsertSession(SessionMeta{
			Source:    SourceClaudeCode,
			SessionID: fmt.Sprintf("s%d", i+1),
			RepoPath:  repoPath,
			StartedAt: "2026-03-28T10:00:00Z",
		}, "2026-03-28T15:00:00Z"))
	}

	rows, err := db.ListSessions(SessionFilter{Projects: []string{"somniloq", "Brimday"}})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (either project matches), got %d", len(rows))
	}
	for _, r := range rows {
		if r.SessionID == "s3" {
			t.Errorf("s3 (other) must not match")
		}
	}
}

func TestListSessions_ProjectFilter_RepoPathOnly(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "repo-only",
		RepoPath:  "/Users/test/UniqRepo",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "projdir-only",
		RepoPath:  "/Users/other/baz",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "both",
		RepoPath:  "/Users/test/Common",
		StartedAt: "2026-03-28T12:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	t.Run("repo-only", func(t *testing.T) {
		rows, err := db.ListSessions(SessionFilter{Projects: []string{"UniqRepo"}})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if rows[0].SessionID != "repo-only" {
			t.Errorf("got %s, want repo-only", rows[0].SessionID)
		}
	})
	t.Run("both", func(t *testing.T) {
		rows, err := db.ListSessions(SessionFilter{Projects: []string{"Common"}})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("expected 1 row, got %d", len(rows))
		}
		if rows[0].SessionID != "both" {
			t.Errorf("got %s, want both", rows[0].SessionID)
		}
	})
	t.Run("unrelated repo_path", func(t *testing.T) {
		rows, err := db.ListSessions(SessionFilter{Projects: []string{"UniqProj"}})
		if err != nil {
			t.Fatalf("ListSessions failed: %v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("expected 0 rows (no repo_path matches), got %d", len(rows))
		}
	})
}

func TestListSessions_CombinedFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "old-brim", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new-brim", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new-somniloq", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Since: "2026-03-28T00:00:00Z", Projects: []string{"Brimday"}})
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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "early", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "late", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", StartedAt: "2026-03-28T12:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s3", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T12:00:00.500Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows (ms timestamp 500ms > 000ms), got %d", len(rows))
	}
}

func TestListSessions_EndedAt(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EndedAt != "2026-03-28T10:30:00Z" {
		t.Errorf("EndedAt: got %q, want %q", rows[0].EndedAt, "2026-03-28T10:30:00Z")
	}
}

func TestListSessions_EndedAt_Null(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].EndedAt != "" {
		t.Errorf("EndedAt: got %q, want empty string", rows[0].EndedAt)
	}
}

func TestGetMessages_Empty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	msgs, err := db.GetMessages(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetMessages_OrderByTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Insert in reverse order
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "world", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	msgs, err := db.GetMessages(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

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

	if msgs[1].UUID != "m2" {
		t.Errorf("second message UUID: got %s, want m2", msgs[1].UUID)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("second message Role: got %s, want assistant", msgs[1].Role)
	}
}

func TestListSessions_BodySizeCountsBytesExcludingSidechain(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	// "héllo" is 5 runes / 6 bytes: BodySize must count bytes, not runes.
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "héllo", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "abcd", Timestamp: "2026-03-28T10:01:00Z"}))
	// Sidechain content must not count toward BodySize (show excludes it)
	// even though MessageCount includes the row.
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s1", Role: "assistant", Content: "sidechain", Timestamp: "2026-03-28T10:02:00Z", IsSidechain: true}))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].BodySize != 10 {
		t.Errorf("BodySize: got %d, want 10 (6 + 4 bytes)", rows[0].BodySize)
	}
	if rows[0].MessageCount != 3 {
		t.Errorf("MessageCount: got %d, want 3 (sidechain still counted)", rows[0].MessageCount)
	}
}

func TestListSessions_BodySizeZeroWithoutMessages(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].BodySize != 0 {
		t.Errorf("BodySize: got %d, want 0", rows[0].BodySize)
	}
}

func TestGetMessages_EqualTimestampsKeepInsertionOrder(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceCodex, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	// Old-format Codex rollouts inherit the session_meta timestamp for every
	// record, so all rows tie on timestamp. Insertion (JSONL line) order must
	// win deterministically: turn numbering is derived from this order.
	const ts = "2026-03-28T10:00:00Z"
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m1", SessionID: "s1", Role: "user", Content: "first", Timestamp: ts}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "reply", Timestamp: ts}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m3", SessionID: "s1", Role: "user", Content: "second", Timestamp: ts}))

	msgs, err := db.GetMessages(SourceCodex, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	for i, want := range []string{"m1", "m2", "m3"} {
		if msgs[i].UUID != want {
			t.Errorf("message[%d] UUID: got %s, want %s", i, msgs[i].UUID, want)
		}
	}
}

func TestGetSummaryMessages_EqualTimestampsKeepInsertionOrder(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceCodex, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	const ts = "2026-03-28T10:00:00Z"
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m1", SessionID: "s1", Role: "user", Content: "first", Timestamp: ts}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m2", SessionID: "s1", Role: "user", Content: "second", Timestamp: ts}))

	msgs, err := db.GetSummaryMessages(SourceCodex, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m1" {
		t.Errorf("message UUID: got %s, want m1", msgs[0].UUID)
	}
}

func TestGetMessages_ExcludesSidechain(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "sidechain thought", Timestamp: "2026-03-28T10:00:30Z", IsSidechain: true}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s1", Role: "assistant", Content: "visible reply", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetMessages(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].UUID != "m1" || msgs[1].UUID != "m3" {
		t.Errorf("expected m1 and m3 (sidechain m2 excluded), got %s and %s", msgs[0].UUID, msgs[1].UUID)
	}
}

func TestGetSession_Found(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s1", "my session", "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.SessionID != "s1" {
		t.Errorf("SessionID: got %s, want s1", got.SessionID)
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

func TestGetSession_EndedAt(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z", EndedAt: "2026-03-28T10:30:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.EndedAt != "2026-03-28T10:30:00Z" {
		t.Errorf("EndedAt: got %q, want %q", got.EndedAt, "2026-03-28T10:30:00Z")
	}
}

func TestGetSession_EndedAt_Null(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil session")
	}
	if got.EndedAt != "" {
		t.Errorf("EndedAt: got %q, want empty string", got.EndedAt)
	}
}

func TestGetSession_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.GetSession(SourceClaudeCode, "nonexistent")
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestLookupSessionsByID_CrossSource(t *testing.T) {
	db := testDB(t)

	for _, source := range []Source{SourceClaudeCode, SourceCodex} {
		must(t, db.UpsertSession(SessionMeta{Source: source, SessionID: "same-id", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	}
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "codex-m1", SessionID: "same-id", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	got, err := db.LookupSessionsByID("same-id")
	if err != nil {
		t.Fatalf("LookupSessionsByID failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got): got %d, want 2", len(got))
	}
	if got[0].Source != SourceClaudeCode || got[1].Source != SourceCodex {
		t.Errorf("sources: got %s, %s; want %s, %s", got[0].Source, got[1].Source, SourceClaudeCode, SourceCodex)
	}
	if got[1].MessageCount != 1 {
		t.Errorf("codex MessageCount: got %d, want 1", got[1].MessageCount)
	}
}

func TestLookupSessionsByID_NotFound(t *testing.T) {
	db := testDB(t)

	got, err := db.LookupSessionsByID("nonexistent")
	if err != nil {
		t.Fatalf("LookupSessionsByID failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("len(got): got %d, want 0", len(got))
	}
}

func TestSessionRowQueryPaths_ReturnAllFields(t *testing.T) {
	db := testDB(t)

	want := SessionRow{
		Source:       SourceClaudeCode,
		SessionID:    "s1",
		CWD:          "/Users/test/project",
		RepoPath:     "/Users/test/project",
		StartedAt:    "2026-03-28T10:00:00Z",
		EndedAt:      "2026-03-28T10:30:00Z",
		CustomTitle:  "session title",
		MessageCount: 1,
		BodySize:     7,
	}
	must(t, db.UpsertSession(SessionMeta{
		Source:    want.Source,
		SessionID: want.SessionID,
		CWD:       want.CWD,
		RepoPath:  want.RepoPath,
		StartedAt: want.StartedAt,
		EndedAt:   want.EndedAt,
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(want.Source, want.SessionID, want.CustomTitle, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{
		Source:    want.Source,
		UUID:      "m1",
		SessionID: want.SessionID,
		Role:      "user",
		Content:   "visible",
		Timestamp: want.StartedAt,
	}))

	rows, err := db.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("ListSessions result: got %+v, want %+v", rows, want)
	}

	got, err := db.GetSession(want.Source, want.SessionID)
	if err != nil {
		t.Fatalf("GetSession failed: %v", err)
	}
	if got == nil || *got != want {
		t.Fatalf("GetSession result: got %+v, want %+v", got, want)
	}

	rows, err = db.LookupSessionsByID(want.SessionID)
	if err != nil {
		t.Fatalf("LookupSessionsByID failed: %v", err)
	}
	if len(rows) != 1 || rows[0] != want {
		t.Fatalf("LookupSessionsByID result: got %+v, want %+v", rows, want)
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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "a1", RepoPath: "/Users/test/projA", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "a2", RepoPath: "/Users/test/projA", StartedAt: "2026-03-28T11:00:00Z"}, "2026-03-28T15:00:00Z"))

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "b1", RepoPath: "/Users/test/projB", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Project B first (latest started_at is 14:00, A's latest is 11:00)
	if rows[0].RepoPath != "/Users/test/projB" {
		t.Errorf("first row: got %s, want /Users/test/projB", rows[0].RepoPath)
	}
	if rows[0].SessionCount != 1 {
		t.Errorf("projB session count: got %d, want 1", rows[0].SessionCount)
	}
	if rows[1].RepoPath != "/Users/test/projA" {
		t.Errorf("second row: got %s, want /Users/test/projA", rows[1].RepoPath)
	}
	if rows[1].SessionCount != 2 {
		t.Errorf("projA session count: got %d, want 2", rows[1].SessionCount)
	}
}

func TestListProjects_SinceFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "old1", RepoPath: "/Users/test/old", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "new1", RepoPath: "/Users/test/new", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Since: "2026-03-28T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/new" {
		t.Errorf("expected /Users/test/new, got %s", rows[0].RepoPath)
	}
}

func TestListProjects_UntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "early1", RepoPath: "/Users/test/early", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "late1", RepoPath: "/Users/test/late", StartedAt: "2026-03-28T14:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Until: "2026-03-28T12:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/early" {
		t.Errorf("expected /Users/test/early, got %s", rows[0].RepoPath)
	}
}

func TestListProjects_SinceAndUntilFilter(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/old", StartedAt: "2026-03-28T08:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", RepoPath: "/Users/test/mid", StartedAt: "2026-03-28T12:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s3", RepoPath: "/Users/test/new", StartedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{Since: "2026-03-28T10:00:00.000Z", Until: "2026-03-28T14:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/mid" {
		t.Errorf("expected /Users/test/mid, got %s", rows[0].RepoPath)
	}
}

func TestListProjects_GroupByRepoPath(t *testing.T) {
	// Worktree and body sessions share the same repo_path; they must collapse
	// into one row.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "body",
		RepoPath:  "/Users/test/Brimday",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "worktree",
		RepoPath:  "/Users/test/Brimday",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 aggregated row, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/Brimday" {
		t.Errorf("RepoPath: got %q, want %q", rows[0].RepoPath, "/Users/test/Brimday")
	}
	if rows[0].SessionCount != 2 {
		t.Errorf("SessionCount: got %d, want 2", rows[0].SessionCount)
	}
}

func TestListProjects_EmptyRepoPathGroup(t *testing.T) {
	// When all sessions have empty repo_path, the group surfaces with
	// RepoPath: "" and a correct count.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s1",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "s2",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].RepoPath != "" {
		t.Errorf("RepoPath: got %q, want empty", rows[0].RepoPath)
	}
	if rows[0].SessionCount != 2 {
		t.Errorf("SessionCount: got %d, want 2", rows[0].SessionCount)
	}
}

func TestListProjects_NullRepoPathCollapsesAcrossProjects(t *testing.T) {
	// Sessions with empty repo_path collapse into a single group.
	// Documented as a Known limitation in rules/scope.md.
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "a1",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "b1",
		StartedAt: "2026-03-28T11:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 collapsed row, got %d", len(rows))
	}
	if rows[0].RepoPath != "" {
		t.Errorf("RepoPath: got %q, want empty", rows[0].RepoPath)
	}
	if rows[0].SessionCount != 2 {
		t.Errorf("SessionCount: got %d, want 2", rows[0].SessionCount)
	}
}

func TestListProjects_GroupByRepoPath_OrderByLatest(t *testing.T) {
	// Two repo_path groups; the one whose latest session is newer must come first.
	db := testDB(t)

	// Body session for repo A, older.
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "a-body",
		RepoPath:  "/Users/test/RepoA",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	// Worktree session for repo A, newer than any repo B session.
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "a-wt",
		RepoPath:  "/Users/test/RepoA",
		StartedAt: "2026-03-28T16:00:00Z",
	}, "2026-03-28T15:00:00Z"))
	// Repo B session in between.
	must(t, db.UpsertSession(SessionMeta{
		Source:    SourceClaudeCode,
		SessionID: "b1",
		RepoPath:  "/Users/test/RepoB",
		StartedAt: "2026-03-28T12:00:00Z",
	}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/RepoA" {
		t.Errorf("first row RepoPath: got %q, want %q", rows[0].RepoPath, "/Users/test/RepoA")
	}
	if rows[1].RepoPath != "/Users/test/RepoB" {
		t.Errorf("second row RepoPath: got %q, want %q", rows[1].RepoPath, "/Users/test/RepoB")
	}
}

func TestListProjects_NullStartedAt(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/normal", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s2", RepoPath: "/Users/test/titleonly"}, "2026-03-28T15:00:00Z"))

	rows, err := db.ListProjects(SessionFilter{})
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/normal" {
		t.Errorf("first row: got %s, want /Users/test/normal", rows[0].RepoPath)
	}
	if rows[1].RepoPath != "/Users/test/titleonly" {
		t.Errorf("second row: got %s, want /Users/test/titleonly", rows[1].RepoPath)
	}

	rows, err = db.ListProjects(SessionFilter{Since: "2026-03-28T00:00:00.000Z"})
	if err != nil {
		t.Fatalf("ListProjects with Since failed: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row with Since filter, got %d", len(rows))
	}
	if rows[0].RepoPath != "/Users/test/normal" {
		t.Errorf("expected /Users/test/normal, got %s", rows[0].RepoPath)
	}
}

func TestGetSummaryMessages_Empty(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_ReturnsFirstUserMessage(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "fix the bug", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "done", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s1", Role: "user", Content: "thanks", Timestamp: "2026-03-28T10:02:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
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
}

func TestGetSummaryMessages_SkipsSidechain(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "sidechain msg", Timestamp: "2026-03-28T10:00:00Z", IsSidechain: true}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "real msg", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: false}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "assistant", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_LimitN(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "u1", SessionID: "s1", Role: "user", Content: "one", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "u2", SessionID: "s1", Role: "user", Content: "two", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "a1", SessionID: "s1", Role: "assistant", Content: "reply", Timestamp: "2026-03-28T10:02:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "u3", SessionID: "s1", Role: "user", Content: "three", Timestamp: "2026-03-28T10:03:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "u4", SessionID: "s1", Role: "user", Content: "four", Timestamp: "2026-03-28T10:04:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "u5", SessionID: "s1", Role: "user", Content: "five", Timestamp: "2026-03-28T10:05:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 3, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	wantUUIDs := []string{"u1", "u2", "u3"}
	for i, w := range wantUUIDs {
		if msgs[i].UUID != w {
			t.Errorf("msgs[%d].UUID: got %s, want %s", i, msgs[i].UUID, w)
		}
	}
}

func TestGetSummaryMessages_SkipsClearPrefix(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "<command-name>/clear</command-name>\n<command-message>clear</command-message>", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "real question", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m2" {
		t.Errorf("expected m2 (/clear skipped), got %s", msgs[0].UUID)
	}
}

func TestGetSummaryMessages_SkipsCaveatPrefix(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "<local-command-caveat>Caveat: ...</local-command-caveat>", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "real question", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m2" {
		t.Errorf("expected m2 (caveat skipped), got %s", msgs[0].UUID)
	}
}

func TestGetSummaryMessages_IncludeClear(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_clear", SessionID: "s1", Role: "user", Content: "<command-name>/clear</command-name>\nmore", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_caveat", SessionID: "s1", Role: "user", Content: "<local-command-caveat>note</local-command-caveat>", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_assistant", SessionID: "s1", Role: "assistant", Content: "reply", Timestamp: "2026-03-28T10:02:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_sidechain", SessionID: "s1", Role: "user", Content: "sidechain", Timestamp: "2026-03-28T10:03:00Z", IsSidechain: true}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_real", SessionID: "s1", Role: "user", Content: "real question", Timestamp: "2026-03-28T10:04:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 3, true)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	wantUUIDs := []string{"m_clear", "m_caveat", "m_real"}
	for i, w := range wantUUIDs {
		if msgs[i].UUID != w {
			t.Errorf("msgs[%d].UUID: got %s, want %s", i, msgs[i].UUID, w)
		}
	}
	if msgs[0].Content != "<command-name>/clear</command-name>\nmore" {
		t.Errorf("msgs[0].Content: got %q", msgs[0].Content)
	}
	if msgs[1].Content != "<local-command-caveat>note</local-command-caveat>" {
		t.Errorf("msgs[1].Content: got %q", msgs[1].Content)
	}
}

func TestGetSummaryMessages_AllSkipped(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "<command-name>/clear</command-name>", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "<local-command-caveat>x</local-command-caveat>", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_LimitExceedsAvailable(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "one", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "user", Content: "two", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 5, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
}

func TestGetSummaryMessages_LimitZeroReturnsError(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	_, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 0, false)
	if err == nil {
		t.Fatal("expected error for limit=0, got nil")
	}
}

func TestGetSummaryMessages_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00.000Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_late", SessionID: "s1", Role: "user", Content: "later", Timestamp: "2026-03-28T10:00:00.200Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m_early", SessionID: "s1", Role: "user", Content: "earlier", Timestamp: "2026-03-28T10:00:00.100Z"}))

	msgs, err := db.GetSummaryMessages(SourceClaudeCode, "s1", 1, false)
	if err != nil {
		t.Fatalf("GetSummaryMessages failed: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].UUID != "m_early" {
		t.Errorf("expected m_early (100ms), got %s", msgs[0].UUID)
	}
}

func TestListSessions_CWD(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", CWD: "/Users/test/proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", CWD: "/Users/test/proj", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))

	got, err := db.GetSession(SourceClaudeCode, "s1")
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

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpdateSessionTitle(SourceClaudeCode, "s1", "title", "2026-03-28T15:00:00Z"))

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
