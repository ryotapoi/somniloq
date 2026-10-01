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
