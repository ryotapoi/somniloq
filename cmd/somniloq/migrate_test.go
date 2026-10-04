package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
