package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestLoadConfig_MissingFileIsEmptyConfig(t *testing.T) {
	cfg, err := loadConfig(filepath.Join(t.TempDir(), "no-such-config.json"))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.ProjectAliases != nil {
		t.Errorf("ProjectAliases = %v, want nil", cfg.ProjectAliases)
	}
	if cfg.DayBoundary != "" {
		t.Errorf("DayBoundary = %q, want empty", cfg.DayBoundary)
	}
}

func TestLoadConfig_InvalidJSONIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want the config path in the message", err)
	}
}

func TestLoadConfig_InvalidCommandPatternIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"commandPatterns": ["["]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid commandPatterns, got nil")
	}
	if !strings.Contains(err.Error(), "invalid commandPatterns pattern") {
		t.Errorf("err = %v, want invalid commandPatterns pattern", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want the config path in the message", err)
	}
}

func TestLoadConfig_InvalidDayBoundaryIsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"dayBoundary": "24:00"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := loadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid dayBoundary, got nil")
	}
	if !strings.Contains(err.Error(), "invalid dayBoundary") {
		t.Errorf("err = %v, want invalid dayBoundary", err)
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("err = %v, want the config path in the message", err)
	}
}

func TestLoadConfig_ParsesProjectAliases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"projectAliases": {"somniloq": ["Brimday", "old-somniloq"]}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := map[string][]string{"somniloq": {"Brimday", "old-somniloq"}}
	if !reflect.DeepEqual(cfg.ProjectAliases, want) {
		t.Errorf("ProjectAliases = %v, want %v", cfg.ProjectAliases, want)
	}
}

func TestLoadConfig_ParsesDayBoundary(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	body := `{"dayBoundary": "04:00"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.DayBoundary != "04:00" {
		t.Errorf("DayBoundary = %q, want 04:00", cfg.DayBoundary)
	}
}

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

func TestExpandProject_EmptyConfigPassesThrough(t *testing.T) {
	got := config{}.expandProject("somniloq")
	if !reflect.DeepEqual(got, []string{"somniloq"}) {
		t.Errorf("expandProject = %v, want [somniloq]", got)
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
		if err := db.UpsertSession(meta, "2026-03-29T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", meta.SessionID, err)
		}
		if err := db.InsertMessage(core.NormalizedMessage{
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
	if !strings.Contains(got, "new-1") || !strings.Contains(got, "old-1") {
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
	code, err := showCmd([]string{"old-1"}, staticDB(db), cfg, &out, &errOut)
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

	got := out.String()
	if !strings.Contains(got, "somniloq\t2\n") {
		t.Errorf("projects output should aggregate alias rows under canonical name:\n%s", got)
	}
	if strings.Contains(got, "Brimday") || strings.Count(got, "somniloq\t") != 1 {
		t.Errorf("projects output should not duplicate or leak alias rows:\n%s", got)
	}
}

func TestProjectsCmd_FormatJSON_ProjectAliasDisplayAggregatesCanonical(t *testing.T) {
	db := newProjectAliasDisplayDB(t)
	cfg := config{ProjectAliases: map[string][]string{
		"somniloq": {"Brimday"},
	}}

	var out, errOut bytes.Buffer
	code, err := projectsCmd([]string{"--format", "json"}, staticDB(db), cfg, &out, &errOut)
	if err != nil {
		t.Fatalf("projectsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	got := decodeJSONArray(t, out.Bytes())
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2: %v", len(got), got)
	}
	if got[0]["project"] != "somniloq" || got[0]["sessionCount"] != float64(2) {
		t.Errorf("first entry = %v, want canonical aggregate", got[0])
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
		if err := db.UpsertSession(meta, "2026-03-29T15:00:00Z"); err != nil {
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
	if !strings.Contains(got, "new-1") || !strings.Contains(got, "old-1") {
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
		if err := db.UpsertSession(meta, "2026-03-28T15:00:00Z"); err != nil {
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
		if !strings.Contains(got, want) {
			t.Errorf("output missing session %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "other-1") {
		t.Errorf("output should not contain other-1:\n%s", got)
	}
}
