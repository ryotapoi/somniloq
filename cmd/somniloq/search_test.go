package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSearchCmd_OutputColumns(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"second"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	want := fmt.Sprintf(fixtureREF(core.SourceClaudeCode, "sess-1")+"\t2\t%s\t/Users/test/proj\tsecond question after blank lines\tclaude_code\n",
		formatLocalTime("2026-03-28T15:03:00Z", time.Local))
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestSearchCmd_AssistantHitUsesOwningTurn(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"answer one"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	want := fmt.Sprintf(fixtureREF(core.SourceClaudeCode, "sess-1")+"\t1\t%s\t/Users/test/proj\tanswer one\tclaude_code\n",
		formatLocalTime("2026-03-28T15:01:00Z", time.Local))
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestSearchCmd_DayBoundaryFiltersDateOnlySince(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	defer func() { time.Local = oldLocal }()

	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "s1",
		RepoPath:  "/Users/test/proj",
		StartedAt: "2026-03-28T18:00:00Z",
	}, "2026-03-29T00:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	messages := []struct {
		uuid      string
		timestamp string
		content   string
	}{
		{"before", "2026-03-28T18:59:00Z", "needle before boundary"},
		{"after", "2026-03-28T19:00:00Z", "needle after boundary"},
	}
	for _, msg := range messages {
		if err := db.InsertMessage(testInputID(t, db,
			core.SourceClaudeCode), core.NormalizedMessage{
			Source:    core.SourceClaudeCode,
			UUID:      msg.uuid,
			SessionID: "s1",
			Role:      "user",
			Content:   msg.content,
			Timestamp: msg.timestamp,
		}); err != nil {
			t.Fatalf("InsertMessage(%s): %v", msg.uuid, err)
		}
	}

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--since", "2026-03-29", "--day-boundary", "04:00", "needle"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	got := out.String()
	if strings.Contains(got, "before boundary") {
		t.Fatalf("date-only --since should exclude the pre-boundary message:\n%s", got)
	}
	if !strings.Contains(got, "after boundary") {
		t.Fatalf("date-only --since should include the boundary message:\n%s", got)
	}
}

func TestSearchCmd_MissingQueryPrintsUsage(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd(nil, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: "+searchUsageLine) {
		t.Errorf("stderr = %q, want usage line", errOut.String())
	}
}

func TestSearchCmd_PaginationTSVPreservesTurns(t *testing.T) {
	var tsvOut, errOut bytes.Buffer
	code, err := searchCmd([]string{"--limit", "2", "--offset", "1", "needle"}, staticDB(newSearchPaginationTestDB(t)), config{}, &tsvOut, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("TSV search = %d, %v (stderr: %q)", code, err, errOut.String())
	}
	if got := tsvOut.String(); !strings.Contains(got, fixtureREF(core.SourceClaudeCode, "page")+"\t2\t") || !strings.Contains(got, "needle second") || !strings.Contains(got, fixtureREF(core.SourceClaudeCode, "page")+"\t1\t") || !strings.Contains(got, "needle first") || strings.Contains(got, "needle third") {
		t.Errorf("TSV page = %q, want the second and first hits with original turns", got)
	}
}

func newSearchPaginationTestDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source: core.SourceClaudeCode, SessionID: "page", RepoPath: "/Users/test/proj",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	for i, message := range []core.NormalizedMessage{
		{UUID: "page-1", Role: "user", Content: "needle first", Timestamp: "2026-03-28T10:00:00Z"},
		{UUID: "page-2", Role: "user", Content: "needle second", Timestamp: "2026-03-28T10:01:00Z"},
		{UUID: "page-3", Role: "user", Content: "needle third", Timestamp: "2026-03-28T10:02:00Z"},
	} {
		message.Source = core.SourceClaudeCode
		message.SessionID = "page"
		if err := db.InsertMessage(testInputID(t, db, message.Source), message); err != nil {
			t.Fatalf("InsertMessage(%d): %v", i, err)
		}
	}
	return db
}

func TestSearchCmd_InvalidPaginationDoesNotOpenDBOrWriteStdout(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid pagination")
		return nil, nil
	}
	for _, args := range [][]string{
		{"--limit", "0", "needle"},
		{"--limit", "-1", "needle"},
		{"--offset", "-1", "needle"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, errOut bytes.Buffer
			code, err := searchCmd(args, openDB, config{}, &out, &errOut)
			if code != 1 || err == nil {
				t.Errorf("searchCmd(%v) = %d, %v, want validation error", args, code, err)
			}
			if out.Len() != 0 {
				t.Errorf("stdout = %q, want empty", out.String())
			}
		})
	}
}

func TestSearchSnippet(t *testing.T) {
	long := strings.Repeat("a", 60) + "NEEDLE" + strings.Repeat("b", 60)

	tests := []struct {
		name    string
		content string
		query   string
		want    string
	}{
		{"short content untouched", "hello world", "world", "hello world"},
		{"truncated both sides", long, "NEEDLE",
			"..." + strings.Repeat("a", 40) + "NEEDLE" + strings.Repeat("b", 40) + "..."},
		{"match at head", "NEEDLE" + strings.Repeat("b", 60), "NEEDLE",
			"NEEDLE" + strings.Repeat("b", 40) + "..."},
		{"match at tail", strings.Repeat("a", 60) + "NEEDLE", "NEEDLE",
			"..." + strings.Repeat("a", 40) + "NEEDLE"},
		{"ascii case-insensitive fallback", strings.Repeat("a", 60) + "needle" + strings.Repeat("b", 60), "NEEDLE",
			"..." + strings.Repeat("a", 40) + "needle" + strings.Repeat("b", 40) + "..."},
		{"multibyte runes counted not bytes", strings.Repeat("あ", 50) + "鍵" + strings.Repeat("い", 50), "鍵",
			"..." + strings.Repeat("あ", 40) + "鍵" + strings.Repeat("い", 40) + "..."},
		{"first ASCII-folded match before exact match", strings.Repeat("あ", 60) + "RATE_100%" + strings.Repeat("b", 60) + "rate_100%", "rate_100%",
			"..." + strings.Repeat("あ", 40) + "RATE_100%" + strings.Repeat("b", 40) + "..."},
		{"non-ASCII case difference does not win", strings.Repeat("あ", 60) + "RÄTE" + strings.Repeat("b", 60) + "räte", "räte",
			"..." + strings.Repeat("b", 40) + "räte"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchSnippet(tt.content, tt.query); got != tt.want {
				t.Errorf("searchSnippet = %q, want %q", got, tt.want)
			}
		})
	}
}

// searchSnippet must never panic on adversarial content: ToLower can grow
// non-ASCII bytes (İ becomes a 3-byte sequence), pushing the fallback index
// to or past len(content), and DB content is not guaranteed to be valid
// UTF-8.
func TestSearchSnippet_NoPanicOnAdversarialContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		query   string
	}{
		// 4 × İ (2 bytes) lower to 4 × i̇ (3 bytes): the lowered match
		// offset for "auth" is 12 == len(content).
		{"tolower growth lands on len(content)", "İİİİauth", "AUTH"},
		{"tolower growth lands past len(content)", strings.Repeat("İ", 10) + "auth", "AUTH"},
		{"invalid utf-8 around match", "a\x80\x80\x80needle\x80b", "NEEDLE"},
		{"invalid utf-8 only", "\x80\x80\x80", "\x80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = searchSnippet(tt.content, tt.query) // must not panic
		})
	}
}

func TestSearchCmd_FilteredAssistantRetainsFullConversationTurn(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSession(testInputID(t, db, core.SourceClaudeCode), core.SessionMeta{Source: core.SourceClaudeCode, SessionID: "filtered"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatal(err)
	}
	for i, m := range []core.NormalizedMessage{
		{Role: "assistant", Content: "preface"},
		{Role: "user", Content: "first question", Timestamp: "2026-03-28T10:00:00Z"},
		{Role: "user", Content: "second question", Timestamp: "2026-03-28T10:01:00Z"},
		{Role: "user", Content: "sidechain question", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: true},
		{Role: "assistant", Content: "needle answer", Timestamp: "2026-03-28T10:01:00Z"},
	} {
		m.Source, m.SessionID, m.UUID = core.SourceClaudeCode, "filtered", fmt.Sprintf("filtered-%d", i)
		if err := db.InsertMessage(testInputID(t, db, m.Source), m); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--since", "2026-03-28T10:01:00Z", "--limit", "1", "needle"}, staticDB(db), config{}, &out, &errOut)
	if code != 0 || err != nil || errOut.Len() != 0 {
		t.Fatalf("search = %d, %v, stderr %q", code, err, errOut.String())
	}
	want := fmt.Sprintf(fixtureREF(core.SourceClaudeCode, "filtered")+"\t2\t%s\t\tneedle answer\tclaude_code\n", formatLocalTime("2026-03-28T10:01:00Z", time.Local))
	if out.String() != want {
		t.Fatalf("output = %q, want %q", out.String(), want)
	}
}
