package codex

import (
	"encoding/json"
	"testing"
)

func TestExtractText_CodexContentBlocks(t *testing.T) {
	raw := json.RawMessage(`[{"type":"input_text","text":"question"},{"type":"output_text","text":"answer"},{"type":"tool_call","name":"exec"}]`)
	got, err := ExtractText(raw)
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}
	want := "question\n\nanswer"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsMessageRecord(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "user message",
			line: `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}}`,
			want: true,
		},
		{
			name: "assistant message",
			line: `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}}`,
			want: true,
		},
		{
			name: "function call",
			line: `{"type":"response_item","payload":{"type":"function_call","name":"exec_command"}}`,
			want: false,
		},
		{
			name: "system role",
			line: `{"type":"response_item","payload":{"type":"message","role":"system","content":[{"type":"text","text":"skip"}]}}`,
			want: false,
		},
		{
			name: "session meta",
			line: `{"type":"session_meta","payload":{"id":"s1","cwd":"/tmp"}}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := ParseRecord([]byte(tt.line))
			if err != nil {
				t.Fatalf("ParseRecord failed: %v", err)
			}
			got, _, err := IsMessageRecord(rec)
			if err != nil {
				t.Fatalf("IsMessageRecord failed: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeMessage_UsesRolloutPathAndLineNumberUUID(t *testing.T) {
	meta := SessionMeta{
		SessionID: "s1",
		CWD:       "/tmp/project",
		RepoPath:  "/tmp/project",
		GitBranch: "main",
		Version:   "0.128.0",
		Timestamp: "2026-05-01T00:00:00.000Z",
	}
	rec, err := ParseRecord([]byte(`{"timestamp":"2026-05-01T00:00:01.000Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}`))
	if err != nil {
		t.Fatalf("ParseRecord failed: %v", err)
	}

	got, err := NormalizeMessage(rec, meta, "/tmp/rollout.jsonl", 7)
	if err != nil {
		t.Fatalf("NormalizeMessage failed: %v", err)
	}

	if got.Message.UUID != messageUUID("/tmp/rollout.jsonl", 7) {
		t.Errorf("uuid: got %q, want %q", got.Message.UUID, messageUUID("/tmp/rollout.jsonl", 7))
	}
	if got.Message.UUID == messageUUID("/tmp/rollout.jsonl", 8) {
		t.Errorf("uuid should include line number, got %q", got.Message.UUID)
	}
	if got.Message.Role != "user" || got.Message.Content != "hello" {
		t.Errorf("message: %+v", got.Message)
	}
}
