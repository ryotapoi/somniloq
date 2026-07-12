package main

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func newCrossSourceSessionTestDB(t *testing.T) *core.DB {
	t.Helper()

	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, source := range []core.Source{core.SourceClaudeCode, core.SourceCodex} {
		if err := db.UpsertSession(core.SessionMeta{
			Source:    source,
			SessionID: "same-id",
			CWD:       "/Users/test/proj",
			RepoPath:  "/Users/test/proj",
			StartedAt: "2026-03-28T15:00:00Z",
		}, "2026-03-28T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", source, err)
		}
	}

	return db
}

func TestShowCmd_AmbiguousCrossSourceSessionID(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)

	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"same-id"}, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("showCmd: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty", out.String())
	}
	const wantErr = "error: session id \"same-id\" is ambiguous; matched multiple sources:\n" +
		"  claude_code\tsame-id\n" +
		"  codex\tsame-id\n"
	if errOut.String() != wantErr {
		t.Errorf("stderr = %q, want %q", errOut.String(), wantErr)
	}
}

func TestResolveSessionByID_AmbiguousCrossSourceSessionID(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)

	var errOut bytes.Buffer
	_, code, err := resolveSessionByID(db, "same-id", &errOut)
	if err != nil {
		t.Fatalf("resolveSessionByID: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	const wantErr = "error: session id \"same-id\" is ambiguous; matched multiple sources:\n" +
		"  claude_code\tsame-id\n" +
		"  codex\tsame-id\n"
	if errOut.String() != wantErr {
		t.Errorf("stderr = %q, want %q", errOut.String(), wantErr)
	}
}

func TestResolveSessionByID_NotFound(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)

	var errOut bytes.Buffer
	_, code, err := resolveSessionByID(db, "no-such", &errOut)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || err.Error() != "session not found: no-such" {
		t.Errorf("err = %v, want session not found: no-such", err)
	}
	if errOut.Len() != 0 {
		t.Errorf("stderr = %q, want empty (main prints returned errors)", errOut.String())
	}
}

func TestMainOutline_NotFoundWritesErrorToStderr(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "somniloq.db")
	db, err := core.OpenDB(dbPath)
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	code, stdout, stderr := runSomniloqMain(t, t.TempDir(), "--db", dbPath, "outline", "no-such")
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if stderr != "error: session not found: no-such\n" {
		t.Errorf("stderr = %q, want %q", stderr, "error: session not found: no-such\\n")
	}
}
