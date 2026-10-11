package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func TestAdapter_BuildGroupsMalformedSessionMetaContinues(t *testing.T) {
	const contents = `{"type":"session_meta","payload":[]}
{"timestamp":"2026-07-12T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}
{"timestamp":"2026-07-12T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	groups, errs := NewAdapter(func(string) string { return "/repo" }).BuildGroups(filepath.Dir(path), []string{path}, "2026-07-12T00:00:00Z")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Err != nil {
		t.Fatalf("groups=%+v errors=%v", groups, errs)
	}
	g := groups[0]
	if len(g.Reports) != 1 || len(g.Reports[0].Failures) != 1 {
		t.Fatalf("reports=%+v, want one malformed metadata failure", g.Reports)
	}
	if g.Session.SessionID != "s1" || len(g.Messages) != 1 || g.Messages[0].Content != "hello" {
		t.Fatalf("group=%+v, want recovered owner and message", g)
	}
}

func TestAdapter_RejectsEmptySessionIDsAcrossRollouts(t *testing.T) {
	const message = `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	adapter := NewAdapter(func(string) string { return "/repo" })
	for _, payload := range []string{`{"cwd":"/repo"}`, `{"id":""}`, `{"id":null}`, `{"id":"valid"}`} {
		path := filepath.Join(t.TempDir(), "rollout.jsonl")
		contents := "\n" + `{"type":"session_meta","payload":` + payload + "}\n" + message
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		groups, errs := adapter.BuildGroups(filepath.Dir(path), []string{path}, "2026-07-12T00:00:00Z")
		if len(errs) != 0 || len(groups) != 1 || groups[0].Err != nil {
			t.Fatalf("groups=%+v errors=%v", groups, errs)
		}
		g := groups[0]
		if payload == `{"id":"valid"}` {
			if g.Session.SessionID != "valid" || len(g.Messages) != 1 || g.Messages[0].SessionID != "valid" || len(g.Reports[0].Failures) != 0 {
				t.Fatalf("valid group=%+v", g)
			}
			continue
		}
		if len(g.Reports) != 1 || len(g.Reports[0].Failures) != 1 {
			t.Fatalf("payload %s: reports=%+v", payload, g.Reports)
		}
		if got, want := g.Reports[0].Failures[0].Diagnostic.Error(), path+":2: session_meta payload.id is missing or empty"; got != want {
			t.Errorf("diagnostic = %q, want %q", got, want)
		}
		if g.Session.SessionID != "" || len(g.Messages) != 0 {
			t.Fatalf("invalid rollout has owner/messages: %+v", g)
		}
	}
}

func TestAdapter_MissingRecordTimestampRemainsUnknown(t *testing.T) {
	const contents = `{"timestamp":"2026-07-12T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/repo"}}
{"timestamp":"2026-07-12T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"first"}]}}
{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":"second"}]}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	groups, errs := NewAdapter(func(string) string { return "/repo" }).BuildGroups(filepath.Dir(path), []string{path}, "2026-07-12T01:00:00Z")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Err != nil {
		t.Fatalf("groups=%+v errors=%v", groups, errs)
	}
	messages := groups[0].Messages
	if len(groups[0].Reports[0].Failures) != 0 || len(messages) != 2 {
		t.Fatalf("group=%+v, want two parsed messages", groups[0])
	}
	if got, want := messages[0].Timestamp, "2026-07-12T00:00:01Z"; got != want {
		t.Errorf("first message timestamp = %q, want %q", got, want)
	}
	if got := messages[1].Timestamp; got != "" {
		t.Errorf("second message timestamp = %q, want unknown", got)
	}
}

func TestFileHandler_HandleLineReportsMalformedPayloads(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "session meta payload",
			line: `{"type":"session_meta","payload":[]}`,
		},
		{
			name: "response item payload",
			line: `{"type":"response_item","payload":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &fileHandler{
				path:            "/tmp/rollout.jsonl",
				resolveRepoPath: func(string) string { return "/repo" },
			}

			result, err := h.HandleLine(&collector{}, []byte(tt.line))
			if err != nil {
				t.Fatalf("HandleLine error = %v, want nil", err)
			}
			if result.Outcome != ingest.LineUnparsed {
				t.Errorf("HandleLine outcome = %v, want %v", result.Outcome, ingest.LineUnparsed)
			}
			if result.Diagnostic == nil {
				t.Error("HandleLine diagnostic = nil, want malformed payload diagnostic")
			} else if !strings.HasPrefix(result.Diagnostic.Error(), h.path+":1:") {
				t.Errorf("HandleLine diagnostic = %q, want path and physical line prefix", result.Diagnostic)
			}
		})
	}
}
