package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"
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

func TestParseMessagePreservesTextBlocks(t *testing.T) {
	rec := &RawRecord{UUID: "u1", SessionID: "s1", Message: json.RawMessage(`{"role":"user","content":[{"type":"text","text":" first "},{"type":"tool_result","content":"ignored"},{"type":"text","text":" second "}]}`)}
	msg, err := ParseMessage(rec)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != " first \n\n second " || len(msg.Blocks) != 2 || msg.Blocks[0] != " first " || msg.Blocks[1] != " second " || msg.Timestamp != "" {
		t.Fatalf("message = %+v", msg)
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

func TestNormalizeRecord_RequiredIDs(t *testing.T) {
	for _, field := range []string{"sessionId", "uuid"} {
		for _, value := range []string{"missing", `""`, "null"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				ids := map[string]json.RawMessage{
					"sessionId": json.RawMessage(`"s1"`),
					"uuid":      json.RawMessage(`"u1"`),
				}
				if value == "missing" {
					delete(ids, field)
				} else {
					ids[field] = json.RawMessage(value)
				}
				rawIDs, err := json.Marshal(ids)
				if err != nil {
					t.Fatal(err)
				}
				line := fmt.Sprintf(`{"type":"user","message":{"role":"user","content":"hello"},%s}`, rawIDs[1:len(rawIDs)-1])
				rec, err := ParseRecord([]byte(line))
				if err != nil {
					t.Fatal(err)
				}
				normalized, err := NormalizeRecord(rec, "")
				if err == nil || !strings.Contains(err.Error(), field) || normalized != nil {
					t.Fatalf("NormalizeRecord = %+v, %v; want error identifying %s", normalized, err, field)
				}
			})
		}
	}

	// Nonempty IDs are preserved without format validation or trimming.
	rec := &RawRecord{SessionID: " s1 ", UUID: " u1 ", Message: json.RawMessage(`{"role":"user","content":"hello"}`)}
	normalized, err := NormalizeRecord(rec, "")
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Session.SessionID != rec.SessionID || normalized.Message.SessionID != rec.SessionID || normalized.Message.UUID != rec.UUID {
		t.Fatalf("IDs changed: %+v", normalized)
	}
}
