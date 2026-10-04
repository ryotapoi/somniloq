package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenDB_FilePermissions(t *testing.T) {
	if os.Getenv("SOMNILOQ_DB_PERMISSIONS_CHILD") != "1" {
		// Isolate the process-wide umask from other tests.
		cmd := exec.Command("sh", "-c", `umask 022; exec "$@"`, "sh", os.Args[0], "-test.run=^TestOpenDB_FilePermissions$", "-test.v")
		cmd.Env = append(os.Environ(), "SOMNILOQ_DB_PERMISSIONS_CHILD=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("permission subprocess: %v\n%s", err, output)
		} else {
			t.Logf("%s", output)
		}
		return
	}

	parent := t.TempDir()
	must(t, os.Chmod(parent, 0o755))
	path := filepath.Join(parent, "somniloq.db")
	db, err := OpenDB(path)
	must(t, err)
	assertDBFileMode(t, path, 0o600)
	must(t, db.UpsertSession(testInput(t, db, SourceClaudeCode), SessionMeta{Source: SourceClaudeCode, SessionID: "session"}, "before"))
	must(t, db.InsertMessage(testInput(t, db, SourceClaudeCode), NormalizedMessage{Source: SourceClaudeCode, UUID: "private", SessionID: "session", Role: "user", Content: "private conversation"}))
	must(t, db.Close())

	db, err = OpenDB(path)
	must(t, err)
	messages, err := db.GetMessages(testInput(t, db, SourceClaudeCode), SourceClaudeCode, "session")
	must(t, err)
	if len(messages) != 1 || messages[0].Content != "private conversation" {
		t.Fatalf("new DB content after reopen = %+v", messages)
	}
	must(t, db.Close())
	assertDBFileMode(t, path, 0o600)

	// Represent an existing database created with the old public mode.
	must(t, os.Chmod(path, 0o644))
	db, err = OpenDB(path)
	must(t, err)
	t.Cleanup(func() { db.Close() })
	messages, err = db.GetMessages(testInput(t, db, SourceClaudeCode), SourceClaudeCode, "session")
	must(t, err)
	if len(messages) != 1 || messages[0].Content != "private conversation" {
		t.Fatalf("existing DB content after reopen = %+v", messages)
	}
	must(t, db.InsertMessage(testInput(t, db, SourceClaudeCode), NormalizedMessage{Source: SourceClaudeCode, UUID: "second", SessionID: "session", Role: "assistant", Content: "still writable"}))
	messages, err = db.GetMessages(testInput(t, db, SourceClaudeCode), SourceClaudeCode, "session")
	must(t, err)
	if len(messages) != 2 {
		t.Fatalf("existing DB message count after write = %d, want 2", len(messages))
	}
	assertDBFileMode(t, path, 0o644)
	assertDBFileMode(t, parent, 0o755)
}

func assertDBFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	must(t, err)
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
	t.Logf("%s mode = %04o", path, info.Mode().Perm())
}

func TestOpenDB_ReturnsErrorWhenNewFileCannotBeCreated(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write to a read-only directory")
	}
	parent := t.TempDir()
	must(t, os.Chmod(parent, 0o500))
	t.Cleanup(func() { os.Chmod(parent, 0o700) })
	path := filepath.Join(parent, "somniloq.db")
	db, err := OpenDB(path)
	if db != nil {
		db.Close()
		t.Fatal("OpenDB returned a DB when file creation failed")
	}
	if !os.IsPermission(err) {
		t.Fatalf("OpenDB error = %v, want permission error", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("DB file exists after failed creation: %v", err)
	}
}
