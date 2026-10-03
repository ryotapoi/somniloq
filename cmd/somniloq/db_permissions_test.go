package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenDB_DirectoryPermissions(t *testing.T) {
	if os.Getenv("SOMNILOQ_CLI_PERMISSIONS_CHILD") != "1" {
		cmd := exec.Command("sh", "-c", `umask 022; exec "$@"`, "sh", os.Args[0], "-test.run=^TestOpenDB_DirectoryPermissions$", "-test.v")
		cmd.Env = append(os.Environ(), "SOMNILOQ_CLI_PERMISSIONS_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("permission subprocess: %v\n%s", err, output)
		} else {
			t.Logf("%s", output)
		}
		return
	}

	parent := t.TempDir()
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(parent, "new")
	second := filepath.Join(first, "nested")
	path := filepath.Join(second, "somniloq.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{parent: 0o755, first: 0o700, second: 0o700, path: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Fatalf("%s mode = %04o, want %04o", path, got, want)
		}
		t.Logf("%s mode = %04o", path, info.Mode().Perm())
	}
}
