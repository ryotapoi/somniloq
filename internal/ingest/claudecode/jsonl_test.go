package claudecode

import (
	"encoding/json"
	"testing"
)

func TestExtractText_String(t *testing.T) {
	raw := json.RawMessage(`"hello world"`)
	got, err := ExtractText(raw)
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}
	if got != "hello world" {
		t.Errorf("got %q, want %q", got, "hello world")
	}
}

func TestExtractText_ContentBlocks(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"response"},{"type":"tool_use","id":"t1","name":"Read","input":{}}]`)
	got, err := ExtractText(raw)
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}
	if got != "response" {
		t.Errorf("got %q, want %q", got, "response")
	}
}

func TestExtractText_MultipleTextBlocks(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"A"},{"type":"text","text":"B"}]`)
	got, err := ExtractText(raw)
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}
	want := "A\n\nB"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExtractText_NoTextBlocks(t *testing.T) {
	raw := json.RawMessage(`[{"type":"tool_use","id":"t1","name":"Read","input":{}}]`)
	got, err := ExtractText(raw)
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestParseRecord_User(t *testing.T) {
	line := []byte(`{"type":"user","uuid":"u1","parentUuid":"p1","sessionId":"s1","timestamp":"2026-03-28T14:10:45.977Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"user","content":"hello"}}`)
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord failed: %v", err)
	}
	if rec.Type != "user" || rec.UUID != "u1" || rec.SessionID != "s1" {
		t.Errorf("unexpected record: %+v", rec)
	}

	msg, err := ParseMessage(rec)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}
	if msg.Role != "user" || msg.Content != "hello" || msg.UUID != "u1" {
		t.Errorf("unexpected message: %+v", msg)
	}
	if msg.ParentUUID == nil || *msg.ParentUUID != "p1" {
		t.Errorf("expected parentUuid p1, got %v", msg.ParentUUID)
	}
}

func TestParseRecord_Assistant(t *testing.T) {
	line := []byte(`{"type":"assistant","uuid":"a1","sessionId":"s1","timestamp":"2026-03-28T14:10:53.874Z","cwd":"/tmp","gitBranch":"main","version":"2.1.86","isSidechain":false,"message":{"role":"assistant","content":[{"type":"text","text":"response"}]}}`)
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord failed: %v", err)
	}

	msg, err := ParseMessage(rec)
	if err != nil {
		t.Fatalf("ParseMessage failed: %v", err)
	}
	if msg.Role != "assistant" || msg.Content != "response" {
		t.Errorf("unexpected message: %+v", msg)
	}
	if msg.ParentUUID != nil {
		t.Errorf("expected nil parentUuid, got %v", msg.ParentUUID)
	}
}

func TestParseRecord_CustomTitle(t *testing.T) {
	line := []byte(`{"type":"custom-title","customTitle":"my session","sessionId":"s1"}`)
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord failed: %v", err)
	}
	if rec.Type != "custom-title" || rec.CustomTitle != "my session" {
		t.Errorf("unexpected record: %+v", rec)
	}
}

func TestParseRecord_InvalidJSON(t *testing.T) {
	_, err := ParseRecord([]byte(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseRecord_UnknownType(t *testing.T) {
	line := []byte(`{"type":"progress","data":"streaming"}`)
	rec, err := ParseRecord(line)
	if err != nil {
		t.Fatalf("ParseRecord failed: %v", err)
	}
	if rec.Type != "progress" {
		t.Errorf("expected type progress, got %s", rec.Type)
	}
}

func TestExtractText_EmptyArray(t *testing.T) {
	raw := json.RawMessage(`[]`)
	got, err := ExtractText(raw)
	if err != nil {
		t.Fatalf("ExtractText failed: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
