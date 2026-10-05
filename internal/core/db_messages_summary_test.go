package core

import (
	"testing"
)

func TestMessagesWithoutNumberUseRowidAndPreserveRawTimestamp(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(testInput(t, db, SourceClaudeCode), SessionMeta{Source: SourceClaudeCode, SessionID: "instant-order"}, "2026-03-28T15:00:00Z"))
	for _, message := range []NormalizedMessage{
		{Source: SourceClaudeCode, UUID: "latest", SessionID: "instant-order", Role: "assistant", Content: "latest", Timestamp: "2026-03-28T08:00:00.2Z"},
		{Source: SourceClaudeCode, UUID: "tie-first", SessionID: "instant-order", Role: "user", Content: "tie first", Timestamp: "2026-03-28T09:00:00.100+01:00"},
		{Source: SourceClaudeCode, UUID: "tie-second", SessionID: "instant-order", Role: "user", Content: "tie second", Timestamp: "2026-03-28T08:00:00.1Z"},
		{Source: SourceClaudeCode, UUID: "earliest", SessionID: "instant-order", Role: "user", Content: "earliest", Timestamp: "2026-03-28T10:00:00+02:00"},
	} {
		must(t, db.InsertMessage(testInput(t, db, message.Source), message))
	}

	messages, err := db.GetMessages(testInput(t, db, SourceClaudeCode), SourceClaudeCode, "instant-order")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	want := []string{"latest", "tie-first", "tie-second", "earliest"}
	if len(messages) != len(want) {
		t.Fatalf("GetMessages rows = %d, want %d", len(messages), len(want))
	}
	for i, uuid := range want {
		if messages[i].UUID != uuid {
			t.Errorf("GetMessages[%d].UUID = %q, want %q", i, messages[i].UUID, uuid)
		}
	}
	if messages[0].Role != "assistant" || messages[0].Content != "latest" || messages[0].Timestamp != "2026-03-28T08:00:00.2Z" || messages[3].Role != "user" {
		t.Errorf("message fields = %+v, want raw role/content/timestamp", messages)
	}
	if messages[1].Timestamp != "2026-03-28T09:00:00.100+01:00" || messages[2].Timestamp != "2026-03-28T08:00:00.1Z" {
		t.Errorf("equal-instant stored strings changed: %q, %q", messages[1].Timestamp, messages[2].Timestamp)
	}

}

func TestGetMessages_IncludesOwnerSidechain(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(testInput(t, db, SourceClaudeCode), SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(testInput(t, db, SourceClaudeCode), NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(testInput(t, db, SourceClaudeCode), NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "sidechain thought", Timestamp: "2026-03-28T10:00:30Z", IsSidechain: true}))
	must(t, db.InsertMessage(testInput(t, db, SourceClaudeCode), NormalizedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s1", Role: "assistant", Content: "visible reply", Timestamp: "2026-03-28T10:01:00Z"}))

	msgs, err := db.GetMessages(testInput(t, db, SourceClaudeCode), SourceClaudeCode, "s1")
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0].UUID != "m1" || msgs[1].UUID != "m2" || msgs[2].UUID != "m3" {
		t.Errorf("expected m1, m2 and m3 (owner sidechain included), got %s and %s", msgs[0].UUID, msgs[1].UUID)
	}
}
