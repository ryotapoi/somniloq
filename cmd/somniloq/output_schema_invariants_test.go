package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

// TestOutputSchemaInvariant_* keep the independently formatted text and JSON
// surfaces aligned without coupling their production implementations. ADR 0012
// intentionally keeps JSON raw (RFC3339 timestamps and unsanitized strings),
// while TSV/Markdown are display formats; each conversion is explicit below.
func TestOutputSchemaInvariant_Sessions(t *testing.T) {
	tsv := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return sessionsCmd(nil, staticDB(db), config{}, out, errOut)
	})
	jsonOut := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return sessionsCmd([]string{"--format", "json"}, staticDB(db), config{}, out, errOut)
	})

	columns := strings.Split(strings.TrimSuffix(tsv, "\n"), "\t")
	if len(columns) != 9 {
		t.Fatalf("TSV columns = %d, want 9: %q", len(columns), tsv)
	}
	entry := singleSchemaInvariantEntry(t, jsonOut)
	assertSchemaInvariantKeys(t, entry, []string{
		"source", "sessionId", "project", "title", "startedAt", "endedAt", "logicalDay", "messageCount", "bodySize", "nonCommandUserTurnCount", "firstNonCommandUserLine",
	})

	// source is JSON-only. The other fields correspond to the TSV columns in
	// this order; time_range deliberately expands into raw startedAt/endedAt.
	want := []string{
		entry["sessionId"].(string),
		formatTimeRange(entry["startedAt"].(string), entry["endedAt"].(string), time.Local),
		entry["logicalDay"].(string),
		entry["project"].(string),
		sanitizeTSV(entry["title"].(string)),
		fmt.Sprint(entry["messageCount"]),
		fmt.Sprint(entry["bodySize"]),
		fmt.Sprint(entry["nonCommandUserTurnCount"]),
		sanitizeTSV(entry["firstNonCommandUserLine"].(string)),
	}
	assertSchemaInvariantValues(t, columns, want)
}

func TestOutputSchemaInvariant_Projects(t *testing.T) {
	tsv := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return projectsCmd(nil, staticDB(db), config{}, out, errOut)
	})
	jsonOut := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return projectsCmd([]string{"--format", "json"}, staticDB(db), config{}, out, errOut)
	})

	columns := strings.Split(strings.TrimSuffix(tsv, "\n"), "\t")
	if len(columns) != 2 {
		t.Fatalf("TSV columns = %d, want 2: %q", len(columns), tsv)
	}
	entry := singleSchemaInvariantEntry(t, jsonOut)
	assertSchemaInvariantKeys(t, entry, []string{"project", "sessionCount"})
	assertSchemaInvariantValues(t, columns, []string{entry["project"].(string), fmt.Sprint(entry["sessionCount"])})
}

func TestOutputSchemaInvariant_Outline(t *testing.T) {
	tsv := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return outlineCmd([]string{"schema-1"}, staticDB(db), out, errOut)
	})
	jsonOut := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return outlineCmd([]string{"--format", "json", "schema-1"}, staticDB(db), out, errOut)
	})

	columns := strings.Split(strings.TrimSuffix(tsv, "\n"), "\t")
	if len(columns) != 4 {
		t.Fatalf("TSV columns = %d, want 4: %q", len(columns), tsv)
	}
	entry := singleSchemaInvariantEntry(t, jsonOut)
	assertSchemaInvariantKeys(t, entry, []string{"turn", "timestamp", "bodySize", "firstLine"})
	assertSchemaInvariantValues(t, columns, []string{
		fmt.Sprint(entry["turn"]),
		sanitizeTSV(formatLocalTime(entry["timestamp"].(string), time.Local)),
		fmt.Sprint(entry["bodySize"]),
		sanitizeTSV(entry["firstLine"].(string)),
	})
}

func TestOutputSchemaInvariant_Show(t *testing.T) {
	markdown := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return showCmd([]string{"schema-1"}, staticDB(db), config{}, out, errOut)
	})
	jsonOut := runSchemaInvariantCommand(t, func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return showCmd([]string{"--format", "json", "schema-1"}, staticDB(db), config{}, out, errOut)
	})

	entry := singleSchemaInvariantEntry(t, jsonOut)
	assertSchemaInvariantKeys(t, entry, []string{"source", "sessionId", "project", "title", "startedAt", "endedAt", "messages"})
	messages, ok := entry["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("messages = %#v, want two entries", entry["messages"])
	}
	for _, message := range messages {
		assertSchemaInvariantKeys(t, message.(map[string]any), []string{"role", "content", "timestamp"})
	}
	if got := messages[0].(map[string]any)["timestamp"]; got != "2026-03-28T15:00:00Z" {
		t.Errorf("messages[0].timestamp = %#v, want raw RFC3339 fixture value", got)
	}
	if got := messages[1].(map[string]any)["timestamp"]; got != "2026-03-28T15:01:00Z" {
		t.Errorf("messages[1].timestamp = %#v, want raw RFC3339 fixture value", got)
	}

	// source and messages[].timestamp are JSON-only. Markdown represents
	// sessionId, project and the two session timestamps as metadata; title is
	// display-sanitized and message role becomes a heading while content stays raw.
	want := fmt.Sprintf("## %s\n\n- **Session**: `%s`\n- **Project**: `%s`\n- **Started**: `%s`\n\n### User\n\n%s\n\n### Assistant\n\n%s\n",
		titleSanitizer.Replace(entry["title"].(string)),
		entry["sessionId"].(string),
		entry["project"].(string),
		formatTimeRange(entry["startedAt"].(string), entry["endedAt"].(string), time.Local),
		messages[0].(map[string]any)["content"].(string),
		messages[1].(map[string]any)["content"].(string),
	)
	if markdown != want {
		t.Errorf("Markdown output = %q, want %q", markdown, want)
	}
}

func TestOutputSchemaInvariant_ShowEmptyTitleFallback(t *testing.T) {
	markdown := runSchemaInvariantCommandWithDB(t, newSchemaInvariantDBWithTitle(""), func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return showCmd([]string{"schema-1"}, staticDB(db), config{}, out, errOut)
	})
	jsonOut := runSchemaInvariantCommandWithDB(t, newSchemaInvariantDBWithTitle(""), func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
		return showCmd([]string{"--format", "json", "schema-1"}, staticDB(db), config{}, out, errOut)
	})

	entry := singleSchemaInvariantEntry(t, jsonOut)
	if got := entry["title"]; got != "" {
		t.Errorf("JSON title = %#v, want raw empty custom title", got)
	}
	if !strings.HasPrefix(markdown, "## schema-1\n\n") {
		t.Errorf("Markdown title = %q, want session ID fallback", markdown)
	}
}

func runSchemaInvariantCommand(t *testing.T, run func(*core.DB, *bytes.Buffer, *bytes.Buffer) (int, error)) string {
	return runSchemaInvariantCommandWithDB(t, newSchemaInvariantDB, run)
}

func runSchemaInvariantCommandWithDB(t *testing.T, newDB func(*testing.T) *core.DB, run func(*core.DB, *bytes.Buffer, *bytes.Buffer) (int, error)) string {
	t.Helper()
	db := newDB(t)
	var out, errOut bytes.Buffer
	code, err := run(db, &out, &errOut)
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	return out.String()
}

func newSchemaInvariantDB(t *testing.T) *core.DB {
	return newSchemaInvariantDBWithTitle("Title\twith\nline")(t)
}

func newSchemaInvariantDBWithTitle(title string) func(*testing.T) *core.DB {
	return func(t *testing.T) *core.DB {
		t.Helper()
		db, err := core.OpenDB(":memory:")
		if err != nil {
			t.Fatalf("OpenDB: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		meta := core.SessionMeta{
			Source: core.SourceClaudeCode, SessionID: "schema-1", CWD: "/Users/test/schema-project", RepoPath: "/Users/test/schema-project",
			StartedAt: "2026-03-28T15:00:00Z", EndedAt: "2026-03-28T16:00:00Z",
		}
		if err := db.UpsertSession(meta, "2026-03-28T16:00:00Z"); err != nil {
			t.Fatalf("UpsertSession: %v", err)
		}
		if err := db.UpdateSessionTitle(meta.Source, meta.SessionID, title, "2026-03-28T16:00:00Z"); err != nil {
			t.Fatalf("UpdateSessionTitle: %v", err)
		}
		insertOutlineMessage(t, db, meta.SessionID, "schema-u1", "user", "first\tline\nmore", "2026-03-28T15:00:00Z", false)
		insertOutlineMessage(t, db, meta.SessionID, "schema-a1", "assistant", "reply\nbody", "2026-03-28T15:01:00Z", false)
		return db
	}
}

func singleSchemaInvariantEntry(t *testing.T, output string) map[string]any {
	t.Helper()
	entries := decodeJSONArray(t, []byte(output))
	if len(entries) != 1 {
		t.Fatalf("JSON entries = %d, want 1: %s", len(entries), output)
	}
	return entries[0]
}

func assertSchemaInvariantKeys(t *testing.T, entry map[string]any, want []string) {
	t.Helper()
	if len(entry) != len(want) {
		t.Fatalf("JSON fields = %d, want %d: %v", len(entry), len(want), entry)
	}
	for _, field := range want {
		if _, ok := entry[field]; !ok {
			t.Errorf("JSON field %q missing from %v", field, entry)
		}
	}
}

func assertSchemaInvariantValues(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("values = %d, want %d: %q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d = %q, want %q", i, got[i], want[i])
		}
	}
}
