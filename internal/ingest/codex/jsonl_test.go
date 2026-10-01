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
