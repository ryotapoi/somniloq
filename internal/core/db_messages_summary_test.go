package core

import (
	"errors"
	"strings"
	"testing"
)

func TestMessagesSummaryQueryMethods_ClosedDatabaseErrorsIncludeOperationAndCause(t *testing.T) {
	tests := []struct {
		name, operation string
		query           func(*DB) error
	}{
		{"GetMessages", "get messages", func(db *DB) error { _, err := db.GetMessages(SourceClaudeCode, "session"); return err }},
		{"GetSummaryMessages", "get summary messages", func(db *DB) error { _, err := db.GetSummaryMessages(SourceClaudeCode, "session", 1, false); return err }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := testDB(t)
			must(t, db.Close())
			err := tt.query(db)
			if err == nil || !strings.Contains(err.Error(), tt.operation) || errors.Unwrap(err) == nil {
				t.Errorf("error = %v, want wrapped %q query error", err, tt.operation)
			}
		})
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
