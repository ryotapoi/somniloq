package core

import (
	"reflect"
	"strings"
	"testing"
)

func TestMessagesAndSummaryOrderByInstantAndKeepRowidTies(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "instant-order"}, "2026-03-28T15:00:00Z"))
	for _, message := range []NormalizedMessage{
		{Source: SourceClaudeCode, UUID: "latest", SessionID: "instant-order", Role: "assistant", Content: "latest", Timestamp: "2026-03-28T08:00:00.2Z"},
		{Source: SourceClaudeCode, UUID: "tie-first", SessionID: "instant-order", Role: "user", Content: "tie first", Timestamp: "2026-03-28T09:00:00.100+01:00"},
		{Source: SourceClaudeCode, UUID: "tie-second", SessionID: "instant-order", Role: "user", Content: "tie second", Timestamp: "2026-03-28T08:00:00.1Z"},
		{Source: SourceClaudeCode, UUID: "earliest", SessionID: "instant-order", Role: "user", Content: "earliest", Timestamp: "2026-03-28T10:00:00+02:00"},
	} {
		must(t, db.InsertMessage(message))
	}

	messages, err := db.GetMessages(SourceClaudeCode, "instant-order")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	want := []string{"earliest", "tie-first", "tie-second", "latest"}
	if len(messages) != len(want) {
		t.Fatalf("GetMessages rows = %d, want %d", len(messages), len(want))
	}
	for i, uuid := range want {
		if messages[i].UUID != uuid {
			t.Errorf("GetMessages[%d].UUID = %q, want %q", i, messages[i].UUID, uuid)
		}
	}
	if messages[0].Role != "user" || messages[0].Content != "earliest" || messages[0].Timestamp != "2026-03-28T10:00:00+02:00" || messages[3].Role != "assistant" {
		t.Errorf("message fields = %+v, want raw role/content/timestamp", messages)
	}
	if messages[1].Timestamp != "2026-03-28T09:00:00.100+01:00" || messages[2].Timestamp != "2026-03-28T08:00:00.1Z" {
		t.Errorf("equal-instant stored strings changed: %q, %q", messages[1].Timestamp, messages[2].Timestamp)
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

func TestGetTurnMessages_NumberingPopulationWithoutBodies(t *testing.T) {
	db := testDB(t)
	for _, source := range []Source{SourceClaudeCode, SourceCodex, SourceCursorAgent} {
		must(t, db.UpsertSession(SessionMeta{Source: source, SessionID: "shared"}, "2026-03-28T15:00:00Z"))
	}
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "other"}, "2026-03-28T15:00:00Z"))
	for _, message := range []NormalizedMessage{
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "latest", Role: "assistant", Timestamp: "2026-03-28T08:00:00.100000001Z"},
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "tie-user", Role: "user", Timestamp: "2026-03-28T09:00:00.100+01:00"},
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "tie-reply", Role: "assistant", Timestamp: "2026-03-28T08:00:00.1Z"},
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "unknown", Role: "assistant"},
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "invalid", Role: "user", Timestamp: "invalid"},
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "earliest", Role: "user", Timestamp: "2026-03-28T10:00:00+02:00"},
		{Source: SourceClaudeCode, SessionID: "shared", UUID: "sidechain", Role: "user", IsSidechain: true},
		{Source: SourceClaudeCode, SessionID: "other", UUID: "other", Role: "user"},
		{Source: SourceCodex, SessionID: "shared", UUID: "codex", Role: "user"},
		{Source: SourceCursorAgent, SessionID: "shared", UUID: "cursor", Role: "assistant"},
	} {
		message.Content = strings.Repeat("large body", 1000)
		must(t, db.InsertMessage(message))
	}
	for _, tt := range []struct {
		source Source
		want   []MessageRow
	}{
		{SourceClaudeCode, []MessageRow{{UUID: "unknown", Role: "assistant"}, {UUID: "invalid", Role: "user"}, {UUID: "earliest", Role: "user"}, {UUID: "tie-user", Role: "user"}, {UUID: "tie-reply", Role: "assistant"}, {UUID: "latest", Role: "assistant"}}},
		{SourceCodex, []MessageRow{{UUID: "codex", Role: "user"}}},
		{SourceCursorAgent, []MessageRow{{UUID: "cursor", Role: "assistant"}}},
	} {
		got, err := db.GetTurnMessages(tt.source, "shared")
		must(t, err)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("GetTurnMessages(%s) = %+v, want %+v", tt.source, got, tt.want)
		}
	}
}
