package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

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

func TestSessionsCmd_FormatJSON(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.UTC
	defer func() { time.Local = oldLocal }()
	db := newOutlineTestDB(t)
	if err := db.UpsertSession(testInputID(t, db, core.SourceClaudeCode), core.SessionMeta{Source: core.SourceClaudeCode, SessionID: "sess-1", EndedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.UpdateSessionTitle(testInputID(t, db, core.SourceClaudeCode), core.SourceClaudeCode, "sess-1", "Title\twith\nline", "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}

	insertOutlineMessage(t, db, "sess-1", "raw-first", "user", "first\tline\nmore", "2026-03-28T14:59:00Z", false)

	var out, errOut bytes.Buffer
	code, err := sessionsCmd([]string{"--format", "json"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1", len(got))
	}
	want := map[string]any{
		"source":       "claude_code",
		"sessionId":    "sess-1",
		"ref":          fixtureREF(core.SourceClaudeCode, "sess-1"),
		"project":      "/Users/test/proj",
		"title":        "Title\twith\nline",
		"startedAt":    "2026-03-28T15:00:00Z",
		"endedAt":      "2026-03-28T16:00:00Z",
		"logicalDay":   "2026-03-28",
		"messageCount": float64(5),
		"bodySize":     float64(102),
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[0][k], v)
		}
	}
	if len(got[0]) != len(want) {
		t.Errorf("fields = %d, want %d: %v", len(got[0]), len(want), got[0])
	}
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

func TestOutlineCmd_FormatJSON(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := outlineCmd([]string{"--format", "json", fixtureREF(core.SourceClaudeCode, "sess-1")}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("outlineCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 3 {
		t.Fatalf("entries = %d, want 3: %v", len(got), got)
	}
	if got[0]["turn"] != float64(1) || got[0]["bodySize"] != float64(36) || got[0]["firstLine"] != "first question" {
		t.Errorf("entry 0 = %v", got[0])
	}
	// Raw timestamp, not the local display format.
	if got[0]["timestamp"] != "2026-03-28T15:00:00Z" {
		t.Errorf("timestamp = %#v, want RFC3339 UTC", got[0]["timestamp"])
	}
	// Tabs survive: JSON escapes natively, no TSV sanitizing.
	if got[2]["firstLine"] != "second\tquestion after blank lines" {
		t.Errorf("entry 1 firstLine = %#v", got[2]["firstLine"])
	}
	if len(got[0]) != 4 {
		t.Errorf("fields = %d, want 4: %v", len(got[0]), got[0])
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
		{"turn", []string{"--turn", "3"}, 1, "\n\nsecond\tquestion after blank lines"},
		{"empty", []string{"--turn", "99"}, 0, ""},
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
func TestShowCmd_FormatJSON_EmptyBulk(t *testing.T) {
	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--format", "json", "--since", "2031-01-01"}, staticDB(newOutlineTestDB(t)), config{}, &out, &errOut)
	if code != 0 || err != nil || len(decodeShowItems(t, out.Bytes())) != 0 {
		t.Fatalf("%d %v %s", code, err, out.String())
	}
}

func TestSearchCmd_FormatJSON(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--format", "json", "second"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1: %v", len(got), got)
	}
	want := map[string]any{
		"source":    "claude_code",
		"sessionId": "sess-1", "ref": fixtureREF(core.SourceClaudeCode, "sess-1"),
		"turn":      float64(3),
		"timestamp": "2026-03-28T15:03:00Z",
		"project":   "/Users/test/proj",
		"snippet":   "second\tquestion after blank lines",
	}
	for k, v := range want {
		if got[0][k] != v {
			t.Errorf("%s = %#v, want %#v", k, got[0][k], v)
		}
	}
	if len(got[0]) != len(want) {
		t.Errorf("fields = %d, want %d: %v", len(got[0]), len(want), got[0])
	}
}

func TestSearchCmd_FormatJSON_Empty(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--format", "json", "no-such-text"}, staticDB(db), config{}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("searchCmd = %d, %v (stderr: %q)", code, err, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("output = %q, want []", out.String())
	}
}

func TestSearchCmd_FormatJSON_PreservesOrderSourceTurnsAndUnknownTimestamp(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, session := range []core.SessionMeta{
		{Source: core.SourceClaudeCode, SessionID: "shared", RepoPath: "/Users/test/claude", StartedAt: "2026-03-28T10:00:00Z"},
		{Source: core.SourceCursorAgent, SessionID: "shared", RepoPath: "/Users/test/cursor"},
	} {
		if err := db.UpsertSession(testInputID(t, db, session.Source), session, "2026-03-28T12:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", session.Source, err)
		}
	}
	for _, message := range []core.NormalizedMessage{
		{Source: core.SourceClaudeCode, UUID: "claude-1", SessionID: "shared", Role: "user", Content: "needle first", Timestamp: "2026-03-28T10:00:00Z"},
		{Source: core.SourceClaudeCode, UUID: "claude-2", SessionID: "shared", Role: "user", Content: "needle second", Timestamp: "2026-03-28T11:00:00Z"},
		{Source: core.SourceCursorAgent, UUID: "cursor-1", SessionID: "shared", Role: "user", Content: "needle unknown time", Timestamp: ""},
	} {
		if err := db.InsertMessage(testInputID(t, db, message.Source), message); err != nil {
			t.Fatalf("InsertMessage(%s): %v", message.UUID, err)
		}
	}

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--format", "json", "needle"}, staticDB(db), config{}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("searchCmd = %d, %v (stderr: %q)", code, err, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	want := []struct {
		source, timestamp string
		turn              float64
	}{
		{"claude_code", "2026-03-28T11:00:00Z", 2},
		{"claude_code", "2026-03-28T10:00:00Z", 1},
		{"cursor_agent", "", 1},
	}
	if len(got) != len(want) {
		t.Fatalf("entries = %d, want %d: %v", len(got), len(want), got)
	}
	for i, entry := range want {
		if got[i]["source"] != entry.source || got[i]["sessionId"] != "shared" || got[i]["timestamp"] != entry.timestamp || got[i]["turn"] != entry.turn {
			t.Errorf("entry %d = %v, want source=%q sessionId=shared timestamp=%q turn=%v", i, got[i], entry.source, entry.timestamp, entry.turn)
		}
	}
}

func TestFormatFlag_Unknown(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for an unknown format")
		return nil, nil
	}

	tests := []struct {
		name string
		run  func() (int, error)
	}{
		{"sessions", func() (int, error) {
			var out, errOut bytes.Buffer
			return sessionsCmd([]string{"--format", "xml"}, openDB, config{}, &out, &errOut)
		}},
		{"projects", func() (int, error) {
			var out, errOut bytes.Buffer
			return projectsCmd([]string{"--format", "xml"}, openDB, config{}, &out, &errOut)
		}},
		{"outline", func() (int, error) {
			var out, errOut bytes.Buffer
			return outlineCmd([]string{"--format", "xml", fixtureREF(core.SourceClaudeCode, "sess-1")}, openDB, config{}, &out, &errOut)
		}},
		{"show", func() (int, error) {
			var out, errOut bytes.Buffer
			return showCmd([]string{"--format", "xml", fixtureREF(core.SourceClaudeCode, "sess-1")}, openDB, config{}, &out, &errOut)
		}},
		{"search", func() (int, error) {
			var out, errOut bytes.Buffer
			return searchCmd([]string{"--format", "xml", "query"}, openDB, config{}, &out, &errOut)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, err := tt.run()
			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if err == nil || !strings.Contains(err.Error(), "unknown format") {
				t.Errorf("err = %v, want unknown format", err)
			}
		})
	}
}
