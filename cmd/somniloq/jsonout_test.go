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
	if err := db.UpsertSession(core.SessionMeta{Source: core.SourceClaudeCode, SessionID: "sess-1", EndedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.UpdateSessionTitle(core.SourceClaudeCode, "sess-1", "Title\twith\nline", "2026-03-28T16:00:00Z"); err != nil {
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
		"project":      "/Users/test/proj",
		"title":        "Title\twith\nline",
		"startedAt":    "2026-03-28T15:00:00Z",
		"endedAt":      "2026-03-28T16:00:00Z",
		"logicalDay":   "2026-03-28",
		"messageCount": float64(5),
		"bodySize":     float64(86),
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
	code, err := outlineCmd([]string{"--format", "json", "sess-1"}, staticDB(db), config{}, &out, &errOut)
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
	if got[0]["turn"] != float64(1) || got[0]["bodySize"] != float64(36) || got[0]["firstLine"] != "first question" {
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
	if len(got[0]) != 4 {
		t.Errorf("fields = %d, want 4: %v", len(got[0]), got[0])
	}
}

func TestShowCmd_FormatJSON_SingleSession(t *testing.T) {
	db := newOutlineTestDB(t)
	if err := db.UpsertSession(core.SessionMeta{Source: core.SourceClaudeCode, SessionID: "sess-1", EndedAt: "2026-03-28T16:00:00Z"}, "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}

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
	wantHeader := map[string]any{
		"sessionId": "sess-1", "source": "claude_code", "project": "/Users/test/proj",
		"title": "", "startedAt": "2026-03-28T15:00:00Z", "endedAt": "2026-03-28T16:00:00Z",
	}
	for key, want := range wantHeader {
		if got[0][key] != want {
			t.Errorf("%s = %#v, want %#v", key, got[0][key], want)
		}
	}
	if len(got[0]) != len(wantHeader)+1 {
		t.Errorf("session fields = %v, want header plus messages", got[0])
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
	if second := msgs[1].(map[string]any); second["role"] != "assistant" || second["content"] != "answer one" || second["timestamp"] != "2026-03-28T15:01:00Z" {
		t.Errorf("message 1 = %v, want raw assistant fields", second)
	}
	for _, m := range msgs {
		message := m.(map[string]any)
		if len(message) != 3 {
			t.Errorf("message fields = %v, want only role/content/timestamp", message)
		}
		for _, key := range []string{"role", "content", "timestamp"} {
			if _, ok := message[key]; !ok {
				t.Errorf("message missing %q: %v", key, message)
			}
		}
		if message["content"] == "sidechain prompt" {
			t.Error("sidechain message leaked into JSON output")
		}
	}
}

func TestShowCmd_FormatJSON_TurnFilter(t *testing.T) {
	db := newOutlineTestDB(t)
	if err := db.UpdateSessionTitle(core.SourceClaudeCode, "sess-1", "Title\twith\nline", "2026-03-28T16:00:00Z"); err != nil {
		t.Fatalf("UpdateSessionTitle: %v", err)
	}

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--format", "json", "--turn", "2", "sess-1"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if got[0]["title"] != "Title\twith\nline" {
		t.Errorf("title = %#v, want raw custom title", got[0]["title"])
	}
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
		"sessionId": "sess-1",
		"turn":      float64(2),
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
		if err := db.UpsertSession(session, "2026-03-28T12:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", session.Source, err)
		}
	}
	for _, message := range []core.NormalizedMessage{
		{Source: core.SourceClaudeCode, UUID: "claude-1", SessionID: "shared", Role: "user", Content: "needle first", Timestamp: "2026-03-28T10:00:00Z"},
		{Source: core.SourceClaudeCode, UUID: "claude-2", SessionID: "shared", Role: "user", Content: "needle second", Timestamp: "2026-03-28T11:00:00Z"},
		{Source: core.SourceCursorAgent, UUID: "cursor-1", SessionID: "shared", Role: "user", Content: "needle unknown time", Timestamp: ""},
	} {
		if err := db.InsertMessage(message); err != nil {
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
			return outlineCmd([]string{"--format", "xml", "sess-1"}, openDB, config{}, &out, &errOut)
		}},
		{"show", func() (int, error) {
			var out, errOut bytes.Buffer
			return showCmd([]string{"--format", "xml", "sess-1"}, openDB, config{}, &out, &errOut)
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
