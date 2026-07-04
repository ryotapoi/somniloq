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

func TestSessionsCmd_FormatJSON(t *testing.T) {
	db := newOutlineTestDB(t)

	// Read before sessionsCmd, which closes the DB on exit.
	rows, err := db.ListSessions(core.SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows))
	}

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
		"project":      rows[0].RepoPath,
		"title":        "",
		"startedAt":    "2026-03-28T15:00:00Z",
		"endedAt":      rows[0].EndedAt,
		"messageCount": float64(rows[0].MessageCount),
		"bodySize":     float64(rows[0].BodySize),
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
	if _, ok := got[0]["project"]; !ok {
		t.Errorf("project field missing: %v", got[0])
	}
}

func TestOutlineCmd_FormatJSON(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := outlineCmd([]string{"--format", "json", "sess-1"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("outlineCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2: %v", len(got), got)
	}
	if got[0]["turn"] != float64(1) || got[0]["firstLine"] != "first question" {
		t.Errorf("entry 0 = %v", got[0])
	}
	// Raw timestamp, not the local display format.
	if got[0]["timestamp"] != "2026-03-28T15:00:00Z" {
		t.Errorf("timestamp = %#v, want RFC3339 UTC", got[0]["timestamp"])
	}
	// Tabs survive: JSON escapes natively, no TSV sanitizing.
	if got[1]["firstLine"] != "second\tquestion after blank lines" {
		t.Errorf("entry 1 firstLine = %#v", got[1]["firstLine"])
	}
}

func TestShowCmd_FormatJSON_SingleSession(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--format", "json", "sess-1"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 1 {
		t.Fatalf("entries = %d, want 1 (single session still wrapped in an array)", len(got))
	}
	if got[0]["sessionId"] != "sess-1" || got[0]["source"] != "claude_code" {
		t.Errorf("session header = %v", got[0])
	}
	msgs, ok := got[0]["messages"].([]any)
	if !ok {
		t.Fatalf("messages is %T, want array", got[0]["messages"])
	}
	if len(msgs) != 3 {
		t.Fatalf("messages = %d, want 3 (sidechain excluded)", len(msgs))
	}
	first := msgs[0].(map[string]any)
	if first["role"] != "user" || first["content"] != "first question\nwith detail" || first["timestamp"] != "2026-03-28T15:00:00Z" {
		t.Errorf("message 0 = %v", first)
	}
	for _, m := range msgs {
		if m.(map[string]any)["content"] == "sidechain prompt" {
			t.Error("sidechain message leaked into JSON output")
		}
	}
}

func TestShowCmd_FormatJSON_TurnFilter(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--format", "json", "--turn", "2", "sess-1"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	msgs := got[0]["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages = %d, want 1 (turn 2 only)", len(msgs))
	}
	if c := msgs[0].(map[string]any)["content"]; c != "\n\nsecond\tquestion after blank lines" {
		t.Errorf("content = %#v", c)
	}
}

func TestShowCmd_FormatJSON_EmptyBulk(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--format", "json", "--since", "2031-01-01"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("output = %q, want []", out.String())
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
			return outlineCmd([]string{"--format", "xml", "sess-1"}, openDB, &out, &errOut)
		}},
		{"show", func() (int, error) {
			var out, errOut bytes.Buffer
			return showCmd([]string{"--format", "xml", "sess-1"}, openDB, config{}, &out, &errOut)
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
