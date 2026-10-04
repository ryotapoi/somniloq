package cursoragent

import (
	"encoding/json"
	"testing"
)

func TestExtractTextRejectsNonStringTextWithoutPartialContent(t *testing.T) {
	for _, content := range []json.RawMessage{
		json.RawMessage(`[{"type":"text","text":"kept?"},{"type":"text","text":42}]`),
		json.RawMessage(`[{"type":"text","text":null}]`),
		json.RawMessage(`null`),
	} {
		if got, err := extractText(content); err == nil || got != "" {
			t.Errorf("extractText(%s) = %q, %v; want empty content and error", content, got, err)
		}
	}
}

func TestNormalizeRecordPreservesBlocksAndUnknownMetadata(t *testing.T) {
	record, err := parseRecord([]byte(`{"role":"assistant","message":{"content":[{"type":"text","text":" first "},{"type":"tool_use","id":"tool"},{"type":"text","text":" second "}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := normalizeRecord(record, "session", "/logs/session.jsonl", 8)
	if err != nil {
		t.Fatal(err)
	}
	msg := got.Message
	if msg.Content != " first \n\n second " || len(msg.Blocks) != 2 || msg.Blocks[0] != " first " || msg.Blocks[1] != " second " || msg.Timestamp != "" || msg.OriginPath != "/logs/session.jsonl" || msg.OriginLine != 8 || got.Session.RepoPath != "" {
		t.Fatalf("normalized = %+v", got)
	}
}
