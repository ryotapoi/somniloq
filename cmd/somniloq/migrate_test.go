package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestMigrateArgumentAndIOExitCodes(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "logs")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "archive.toml")
	if err := os.WriteFile(cfg, []byte("db = '"+filepath.Join(dir, "archive.db")+"'\n[[inputs]]\nsource = 'codex'\nroot = '"+root+"'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		args    []string
		code    int
		message string
	}{
		{"missing from", nil, 2, "missing --from"},
		{"unexpected positional", []string{"--from", "snapshot.db", "extra"}, 2, "unexpected arguments"},
		{"no source selection", []string{"--source", "codex"}, 2, "flag provided but not defined"},
		{"no full", []string{"--full"}, 2, "flag provided but not defined"},
		{"missing snapshot", []string{"--from", filepath.Join(dir, "missing.db")}, 1, "no such file"},
		{"help", []string{"--help"}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"migrate", "--config", cfg}, tc.args...)
			var out, errOut bytes.Buffer
			code, err := runCommand(args, strings.NewReader(""), &out, &errOut, false)
			diagnostic := errOut.String()
			if err != nil {
				diagnostic += err.Error()
			}
			if code != tc.code || !strings.Contains(diagnostic, tc.message) {
				t.Fatalf("code=%d err=%v stderr=%s", code, err, errOut.String())
			}
			if out.Len() != 0 {
				t.Fatalf("stdout before copy=%s", out.String())
			}
			if _, err := os.Stat(filepath.Join(dir, "archive.db")); !os.IsNotExist(err) {
				t.Fatalf("destination created: %v", err)
			}
		})
	}
}

// Summary, severity, and exit form one CLI contract after the initial copy.
func TestMigrateSafeOutcomesAndMixedFailure(t *testing.T) {
	for _, kind := range []string{"skip", "warning", "mixed"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "logs")
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal(err)
			}
			fixture := "../../internal/ingest/testdata/v0.14.0-migration"
			name := "05-inherited-only.jsonl"
			if kind != "skip" {
				name = "01-child.jsonl"
			}
			data, err := os.ReadFile(filepath.Join(fixture, name))
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "mixed" {
				if err = os.WriteFile(filepath.Join(root, "broken.jsonl"), []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"broken\"}}\n{broken\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			script, err := os.ReadFile(filepath.Join(fixture, "legacy.sql"))
			if err != nil {
				t.Fatal(err)
			}
			if kind != "skip" {
				script = append(script, []byte("INSERT INTO sessions(source,session_id,imported_at) VALUES ('codex','child',''); INSERT INTO messages(uuid,source,session_id,role,content,timestamp) VALUES ('old-path-row','codex','child','assistant','child own','');")...)
			}
			from := filepath.Join(dir, "snapshot.db")
			db, err := sql.Open("sqlite", from)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(string(script)); err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			cfg := config{DB: filepath.Join(dir, "new.db"), Inputs: []core.Input{{Source: core.SourceCodex, Root: root}}}
			for run := 0; run < 2; run++ {
				var out, stderr bytes.Buffer
				code, err := migrateCmd([]string{"--from", from}, cfg, &out, &stderr)
				wantCode := 0
				if kind == "mixed" {
					wantCode = 1
				}
				if err != nil || code != wantCode {
					t.Fatalf("code=%d err=%v stderr=%s", code, err, stderr.String())
				}
				var summary core.MigrationResult
				dec := json.NewDecoder(&out)
				if err = dec.Decode(&summary); err != nil {
					t.Fatal(err)
				}
				if dec.More() || out.Len() != 0 {
					t.Fatal("multiple summaries")
				}
				if summary.CopyPerformed != (run == 0) {
					t.Fatalf("copy: %+v", summary)
				}
				if kind == "skip" {
					if summary.GroupsSkipped != 1 || summary.GroupsFailed != 0 || !strings.Contains(stderr.String(), "migrate: skip:") {
						t.Fatalf("skip: %+v %s", summary, stderr.String())
					}
				} else {
					if summary.GroupsReplaced != 1 || summary.LegacyRetentionWarnings != 1 || summary.LegacyReplacementFailures != 0 || !strings.Contains(stderr.String(), "migrate: warning:") {
						t.Fatalf("warning: %+v %s", summary, stderr.String())
					}
				}
				if kind == "mixed" {
					if summary.GroupsFailed != 1 || !strings.Contains(stderr.String(), "migrate: error:") {
						t.Fatalf("mixed: %+v %s", summary, stderr.String())
					}
				} else if strings.Contains(stderr.String(), "migrate: error:") {
					t.Fatalf("unexpected error: %s", stderr.String())
				}
			}
		})
	}
}
