package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestResolveDayBoundary_FlagOverridesConfig(t *testing.T) {
	got, err := resolveDayBoundary("05:30", config{DayBoundary: "04:00"})
	if err != nil {
		t.Fatalf("resolveDayBoundary: %v", err)
	}
	if got.offset.String() != "5h30m0s" {
		t.Errorf("offset = %v, want 5h30m0s", got.offset)
	}
}

func TestExpandProject(t *testing.T) {
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday", "old-somniloq"},
	}}

	tests := []struct {
		name    string
		project string
		want    []string
	}{
		{"empty means no filter", "", nil},
		{"canonical name expands to the group", "somniloq", []string{"somniloq", "Brimday", "old-somniloq"}},
		{"old name expands to the same group", "Brimday", []string{"somniloq", "Brimday", "old-somniloq"}},
		{"unaliased name passes through", "other", []string{"other"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.expandProject(tt.project); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("expandProject(%q) = %v, want %v", tt.project, got, tt.want)
			}
		})
	}
}

func TestResolveProjectDisplayName_ProjectAlias(t *testing.T) {
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday", "/archive/old-somniloq"},
	}}

	tests := []struct {
		name     string
		repoPath string
		short    bool
		want     string
	}{
		{"old basename displays canonical", "/Users/test/Brimday", false, "somniloq"},
		{"canonical basename displays canonical", "/Users/test/somniloq", false, "somniloq"},
		{"full alias path displays canonical", "/archive/old-somniloq", false, "somniloq"},
		{"unaliased raw path keeps existing default", "/Users/test/other", false, "/Users/test/other"},
		{"unaliased short path keeps basename", "/Users/test/other", true, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveProjectDisplayName(tt.repoPath, tt.short, cfg); got != tt.want {
				t.Errorf("resolveProjectDisplayName(%q, %v) = %q, want %q", tt.repoPath, tt.short, got, tt.want)
			}
		})
	}
}

func newProjectAliasDisplayDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	sessions := []core.SessionMeta{
		{Source: core.SourceClaudeCode, SessionID: "new-1", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-29T10:00:00Z"},
		{Source: core.SourceClaudeCode, SessionID: "old-1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"},
		{Source: core.SourceClaudeCode, SessionID: "other-1", RepoPath: "/Users/test/other", StartedAt: "2026-03-27T10:00:00Z"},
	}
	for _, meta := range sessions {
		if err := db.UpsertSession(testInputID(t, db, meta.Source), meta, "2026-03-29T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", meta.SessionID, err)
		}
		if err := db.InsertMessage(testInputID(t, db,
			meta.Source), core.NormalizedMessage{
			Source:    meta.Source,
			UUID:      meta.SessionID + "-m1",
			SessionID: meta.SessionID,
			Role:      "user",
			Content:   "alias-hit from " + meta.SessionID,
			Timestamp: meta.StartedAt,
		}); err != nil {
			t.Fatalf("InsertMessage(%s): %v", meta.SessionID, err)
		}
	}
	return db
}

func TestSessionsCmd_ProjectAliasDisplayUsesCanonical(t *testing.T) {
	db := newProjectAliasDisplayDB(t)
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday"},
	}}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd(nil, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := out.String()
	if !strings.Contains(got, fixtureREF(core.SourceClaudeCode, "new-1")) || !strings.Contains(got, fixtureREF(core.SourceClaudeCode, "old-1")) {
		t.Fatalf("output missing alias sessions:\n%s", got)
	}
	if strings.Contains(got, "Brimday") || strings.Contains(got, "/Users/test/somniloq") {
		t.Errorf("alias project output should use only the canonical name:\n%s", got)
	}
	if count := strings.Count(got, "\tsomniloq\t"); count != 2 {
		t.Errorf("canonical project column count = %d, want 2:\n%s", count, got)
	}
}

func TestShowCmd_ProjectAliasDisplayUsesCanonical(t *testing.T) {
	db := newProjectAliasDisplayDB(t)
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday"},
	}}

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{fixtureREF(core.SourceClaudeCode, "old-1")}, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := out.String()
	if !strings.Contains(got, "- **Project**: `somniloq`") {
		t.Errorf("show header should use canonical project name:\n%s", got)
	}
	if strings.Contains(got, "Brimday") {
		t.Errorf("show output should not leak old project name:\n%s", got)
	}
}

func TestProjectsCmd_ProjectAliasDisplayAggregatesCanonical(t *testing.T) {
	db := newProjectAliasDisplayDB(t)
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday"},
	}}

	var out, errOut bytes.Buffer
	code, err := projectsCmd(nil, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("projectsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	const want = "somniloq\t2\n/Users/test/other\t1\n"
	if got := out.String(); got != want {
		t.Errorf("projects TSV = %q, want alias aggregation and unaliased path %q", got, want)
	}
}

func TestProjectsCmd_ShortDoesNotAggregateUnaliasedBasenameCollisions(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	for _, meta := range []core.SessionMeta{
		{Source: core.SourceClaudeCode, SessionID: "app-a", RepoPath: "/Users/a/app", StartedAt: "2026-03-29T10:00:00Z"},
		{Source: core.SourceClaudeCode, SessionID: "app-b", RepoPath: "/Users/b/app", StartedAt: "2026-03-28T10:00:00Z"},
	} {
		if err := db.UpsertSession(testInputID(t, db, meta.Source), meta, "2026-03-29T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", meta.SessionID, err)
		}
	}

	var out, errOut bytes.Buffer
	code, err := projectsCmd([]string{"--short"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("projectsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	if got := strings.Count(out.String(), "app\t1\n"); got != 2 {
		t.Errorf("short output should preserve unaliased basename-collision rows, got count %d:\n%s", got, out.String())
	}
	if strings.Contains(out.String(), "app\t2\n") {
		t.Errorf("short output should not aggregate unaliased basename collisions:\n%s", out.String())
	}
}

func TestSearchCmd_ProjectAliasDisplayUsesCanonical(t *testing.T) {
	db := newProjectAliasDisplayDB(t)
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday"},
	}}

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--project", "Brimday", "alias-hit"}, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := out.String()
	if !strings.Contains(got, fixtureREF(core.SourceClaudeCode, "new-1")) || !strings.Contains(got, fixtureREF(core.SourceClaudeCode, "old-1")) {
		t.Fatalf("search output missing alias sessions:\n%s", got)
	}
	if strings.Contains(got, "Brimday") || strings.Contains(got, "/Users/test/somniloq") {
		t.Errorf("search output should use only the canonical project name:\n%s", got)
	}
	if count := strings.Count(got, "\tsomniloq\t"); count != 2 {
		t.Errorf("canonical project column count = %d, want 2:\n%s", count, got)
	}
}

// End-to-end: --project with an old name must list sessions stored under both
// the old and the new repo path.
func TestSessionsCmd_ProjectAliasExpansion(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	sessions := []core.SessionMeta{
		{Source: core.SourceClaudeCode, SessionID: "new-1", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-28T10:00:00Z"},
		{Source: core.SourceClaudeCode, SessionID: "old-1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-27T10:00:00Z"},
		{Source: core.SourceClaudeCode, SessionID: "other-1", RepoPath: "/Users/test/other", StartedAt: "2026-03-26T10:00:00Z"},
	}
	for _, meta := range sessions {
		if err := db.UpsertSession(testInputID(t, db, meta.Source), meta, "2026-03-28T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", meta.SessionID, err)
		}
	}

	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday"},
	}}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd([]string{"--project", "Brimday"}, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := out.String()
	for _, want := range []string{"new-1", "old-1"} {
		if !strings.Contains(got, fixtureREF(core.SourceClaudeCode, want)) {
			t.Errorf("output missing session %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, fixtureREF(core.SourceClaudeCode, "other-1")) {
		t.Errorf("output should not contain other-1:\n%s", got)
	}
}

func TestProjectAliasLiteralConditionsAcrossCommands(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("UTC-11", -11*60*60)
	defer func() { time.Local = oldLocal }()

	aliases := []string{"new_rate_100%", `old\repo`, "legacy_100%"}
	cfg := config{ProjectAliases: map[string][]string{aliases[0]: aliases[1:]}}
	for _, input := range aliases {
		t.Run(input, func(t *testing.T) {
			for _, command := range []struct {
				name string
				run  func(*core.DB, *bytes.Buffer, *bytes.Buffer) (int, error)
			}{
				{"sessions", func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
					return sessionsCmd([]string{"--project", input}, staticDB(db), cfg, out, errOut)
				}},
				{"show", func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
					return showCmd([]string{"--since", "2026-03-28T00:00:00Z", "--project", input}, staticDB(db), cfg, out, errOut)
				}},
				{"search", func(db *core.DB, out, errOut *bytes.Buffer) (int, error) {
					return searchCmd([]string{"--project", input, "literal alias hit"}, staticDB(db), cfg, out, errOut)
				}},
			} {
				t.Run(command.name, func(t *testing.T) {
					db := newLiteralAliasTestDB(t, aliases)
					var out, errOut bytes.Buffer
					code, err := command.run(db, &out, &errOut)
					if err != nil || code != 0 {
						t.Fatalf("%s = %d, %v (stderr: %q)", command.name, code, err, errOut.String())
					}
					for i := range aliases {
						expected := fmt.Sprintf("alias-%d", i)
						if command.name != "show" {
							expected = fixtureREF(core.SourceClaudeCode, expected)
						}
						if !strings.Contains(out.String(), expected) {
							t.Errorf("%s output missing alias-%d:\n%s", command.name, i, out.String())
						}
					}
					if strings.Contains(out.String(), "false-positive") || strings.Contains(out.String(), fixtureREF(core.SourceClaudeCode, "false-positive")) {
						t.Errorf("%s output included wildcard false positive:\n%s", command.name, out.String())
					}
				})
			}
		})
	}
}

func newLiteralAliasTestDB(t *testing.T, aliases []string) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	for i, name := range aliases {
		id := fmt.Sprintf("alias-%d", i)
		if err := db.UpsertSession(testInputID(t, db,
			core.SourceClaudeCode), core.SessionMeta{
			Source: core.SourceClaudeCode, SessionID: id,
			RepoPath: "/Users/test/" + name, StartedAt: "2026-03-28T10:00:00Z",
		}, "2026-03-28T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", id, err)
		}
		if err := db.InsertMessage(testInputID(t, db,
			core.SourceClaudeCode), core.NormalizedMessage{
			Source: core.SourceClaudeCode, UUID: "message-" + id, SessionID: id,
			Role: "user", Content: "literal alias hit", Timestamp: "2026-03-28T10:00:00Z",
		}); err != nil {
			t.Fatalf("InsertMessage(%s): %v", id, err)
		}
	}
	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source: core.SourceClaudeCode, SessionID: "false-positive",
		RepoPath: "/Users/test/newXrateX100anything", StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession(false-positive): %v", err)
	}
	return db
}
