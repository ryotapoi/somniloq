package codex

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
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

func TestNormalizeMessageTextBlocks(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content json.RawMessage
		blocks  []string
		text    string
		wantErr bool
	}{
		{name: "blocks", content: json.RawMessage(`[{"type":"input_text","text":" first "},{"type":"tool_call"},{"type":"text","text":""}]`), blocks: []string{" first ", ""}, text: " first \n\n"},
		{name: "null", content: json.RawMessage(`null`)},
		{name: "empty array", content: json.RawMessage(`[]`)},
		{name: "malformed", content: json.RawMessage(`{`), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := &ResponseItemPayload{Role: "user", Content: tc.content}
			record, err := normalizeMessage(&RawRecord{}, payload, ingest.SessionMeta{SessionID: "owner"}, "rollout.jsonl", 1)
			if (err != nil) != tc.wantErr {
				t.Fatalf("normalizeMessage error = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				if record != nil {
					t.Fatalf("record = %+v, want nil", record)
				}
				return
			}
			if !reflect.DeepEqual(record.Message.Blocks, tc.blocks) || record.Message.Content != tc.text {
				t.Fatalf("blocks = %#v, content = %q; want %#v, %q", record.Message.Blocks, record.Message.Content, tc.blocks, tc.text)
			}
		})
	}
}
