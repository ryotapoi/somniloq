package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

// insertOrphanSession registers a session with no messages (repo_path NULL,
// cwd populated). Backfill should DELETE this row.
func insertOrphanSession(t *testing.T, db *core.DB, sessionID, cwd string) {
	t.Helper()
	if err := db.UpsertSession(core.SessionMeta{
		SessionID:  sessionID,
		CWD:        cwd,
		StartedAt:  "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession(%s): %v", sessionID, err)
	}
}

// insertSessionWithMessage registers a session with one message attached.
func insertSessionWithMessage(t *testing.T, db *core.DB, sessionID, cwd, uuid string) {
	t.Helper()
	if err := db.UpsertSession(core.SessionMeta{
		SessionID:  sessionID,
		CWD:        cwd,
		StartedAt:  "2026-03-28T15:00:00Z",
	}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession(%s): %v", sessionID, err)
	}
	if err := db.InsertMessage(core.ParsedMessage{
		UUID:      uuid,
		SessionID: sessionID,
		Role:      "user",
		Content:   "{}",
		Timestamp: "2026-03-28T15:00:00Z",
	}); err != nil {
		t.Fatalf("InsertMessage(%s): %v", uuid, err)
	}
}

// staticDB returns an opener that hands back the same *core.DB every call.
// backfillCmd Closes the DB it opens. A second Close from the test's defer
// is harmless because tests do not query the DB after backfillCmd returns.
func staticDB(db *core.DB) func() (*core.DB, error) {
	return func() (*core.DB, error) { return db, nil }
}

func TestBackfillCmd_NonInteractiveOrphanRequiresYes(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, staticDB(db), strings.NewReader(""), &out, &errOut, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "backfill requires confirmation when deleting sessions") {
		t.Errorf("err = %v, want one mentioning 'backfill requires confirmation when deleting sessions'", err)
	}
	if strings.Contains(out.String(), "Backfilled") {
		t.Errorf("stdout must not contain 'Backfilled' on non-interactive failure, got %q", out.String())
	}
}

func TestBackfillCmd_NonInteractiveYesSucceeds(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"--yes"}, staticDB(db), strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if !strings.Contains(out.String(), "deleted=1") {
		t.Errorf("stdout = %q, want it to contain 'deleted=1'", out.String())
	}
}

func TestBackfillCmd_InteractiveYesSkipsPrompt(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"--yes"}, staticDB(db), strings.NewReader(""), &out, &errOut, true)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Errorf("--yes must skip the confirmation prompt, but stderr = %q", errOut.String())
	}
}

func TestBackfillCmd_InteractiveDeclineDoesNothing(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	insertOrphanSession(t, db, "orphan", "/Users/test/proj")

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, staticDB(db), strings.NewReader("n\n"), &out, &errOut, true)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(out.String(), "Backfilled") {
		t.Errorf("stdout must not contain 'Backfilled' after decline, got %q", out.String())
	}
}

func TestBackfillCmd_NoOrphanRunsResolveOnly(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()
	// Session has a message attached, so it is not an orphan. cwd is set,
	// repo_path stays NULL until backfill resolves it.
	insertSessionWithMessage(t, db, "kept", "/Users/test/proj/.claude/worktrees/feature", "m1")

	var out, errOut bytes.Buffer
	code, err := backfillCmd(nil, staticDB(db), strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if strings.Contains(errOut.String(), "[y/N]") {
		t.Errorf("must not prompt when there are no orphans, stderr = %q", errOut.String())
	}
	if !strings.Contains(out.String(), "resolved=1") {
		t.Errorf("stdout = %q, want it to contain 'resolved=1'", out.String())
	}
}

// TestBackfillCmd_HelpDoesNotOpenDB ensures that --help short-circuits before
// the openDB callback runs. Otherwise `somniloq backfill -h` would create the
// DB directory / migrate the schema just to print usage.
func TestBackfillCmd_HelpDoesNotOpenDB(t *testing.T) {
	called := false
	open := func() (*core.DB, error) {
		called = true
		return nil, errors.New("openDB must not be called for --help")
	}
	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"-h"}, open, strings.NewReader(""), &out, &errOut, false)
	if err != nil {
		t.Fatalf("backfillCmd: %v", err)
	}
	if code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if called {
		t.Errorf("openDB was invoked for --help; flag parsing must short-circuit first")
	}
	if !strings.Contains(errOut.String(), "Correct legacy session data") {
		t.Errorf("usage missing from stderr: %q", errOut.String())
	}
}

// TestBackfillCmd_UnexpectedArgsDoesNotOpenDB makes sure positional-arg
// validation also short-circuits before openDB.
func TestBackfillCmd_UnexpectedArgsDoesNotOpenDB(t *testing.T) {
	called := false
	open := func() (*core.DB, error) {
		called = true
		return nil, errors.New("openDB must not be called when args are invalid")
	}
	var out, errOut bytes.Buffer
	code, err := backfillCmd([]string{"extra"}, open, strings.NewReader(""), &out, &errOut, false)
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if err == nil || !strings.Contains(err.Error(), "unexpected arguments") {
		t.Errorf("err = %v, want 'unexpected arguments'", err)
	}
	if called {
		t.Errorf("openDB was invoked for invalid args; validation must short-circuit first")
	}
}
