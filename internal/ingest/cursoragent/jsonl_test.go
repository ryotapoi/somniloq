package cursoragent

import (
	"encoding/json"
	"testing"
)

func TestExtractText(t *testing.T) {
	content := json.RawMessage(`[{"type":"text","text":"first"},{"type":"tool_use","name":"Read"},{"type":"text","text":"second"}]`)
	got, err := extractText(content)
	if err != nil {
		t.Fatalf("extractText failed: %v", err)
	}
	if want := "first\n\nsecond"; got != want {
		t.Errorf("extractText = %q, want %q", got, want)
	}
}

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

func TestMessageUUIDIncludesPhysicalLine(t *testing.T) {
	path := "/cursor/project/agent-transcripts/s/s.jsonl"
	if messageUUID(path, 1) == messageUUID(path, 2) {
		t.Error("message UUID must distinguish physical lines")
	}
}
