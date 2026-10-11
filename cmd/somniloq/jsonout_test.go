package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

// decodeJSONArray pins the wire format: unmarshalling into maps catches
// wrong/missing field names that a struct round-trip would hide.
func decodeJSONArray(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var got []map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, data)
	}
	return got
}

func TestProjectsCmd_FormatJSON(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := projectsCmd([]string{"--format", "json"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("projectsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	if got[0]["sessionCount"] != float64(1) {
		t.Errorf("sessionCount = %#v, want 1", got[0]["sessionCount"])
	}
	if got[0]["project"] != "/Users/test/proj" || len(got[0]) != 2 {
		t.Errorf("project schema/value = %v, want only project and sessionCount", got[0])
	}
}

func decodeShowItems(t *testing.T, data []byte) []any {
	t.Helper()
	var envelope map[string]any
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	items, ok := envelope["items"].([]any)
	if !ok {
		t.Fatalf("items must be array: %s", data)
	}
	if len(envelope) != 7 || envelope["total"] != float64(len(items)) || envelope["count"] != float64(len(items)) || envelope["limit"] != nil || envelope["offset"] != float64(0) || envelope["hasMore"] != false || envelope["nextOffset"] != nil {
		t.Fatalf("envelope: %s", data)
	}
	return items
}
func TestShowCmd_FormatJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []string
		count int
		text  string
	}{
		{"owner", nil, 4, "first question\nwith detail"},
		{"turn", []string{"--messages", "4:4"}, 1, "\n\nsecond\tquestion after blank lines"},
		{"empty", []string{"--messages", "99:"}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			args := append([]string{"--format", "json"}, tc.args...)
			args = append(args, fixtureREF(core.SourceClaudeCode, "sess-1"))
			code, err := showCmd(args, staticDB(newOutlineTestDB(t)), config{}, &out, &errOut)
			if code != 0 || err != nil {
				t.Fatalf("%d %v", code, err)
			}
			items := decodeShowItems(t, out.Bytes())
			if len(items) != tc.count {
				t.Fatalf("items: %v", items)
			}
			for _, raw := range items {
				item := raw.(map[string]any)
				for _, key := range []string{"ref", "messageNumber", "role", "timestamp", "text", "blocks", "parentRef", "rootRef", "provenance"} {
					if _, ok := item[key]; !ok {
						t.Fatalf("missing %s: %v", key, item)
					}
				}
				if len(item) != 9 || item["provenance"] != "source_record" {
					t.Fatalf("item: %v", item)
				}
			}
			if tc.count > 0 && items[0].(map[string]any)["text"] != tc.text {
				t.Fatalf("text: %v", items)
			}
		})
	}
}
func TestProjectsRejectsUnknownFormatBeforeDB(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := projectsCmd([]string{"--format", "xml"}, func() (*core.DB, error) {
		t.Fatal("DB opened for unknown format")
		return nil, nil
	}, config{}, &out, &errOut)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "unknown format") || out.Len() != 0 {
		t.Fatalf("unknown format: %d %v %q", code, err, out.String())
	}
}
