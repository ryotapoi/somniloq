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
