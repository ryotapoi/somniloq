package core

import (
	"database/sql"
	"os/exec"
	"path/filepath"
	"testing"
)

// insertLegacySession inserts a sessions row directly, bypassing upsertSession,
// so tests can construct pre-v0.3 state (repo_path NULL with cwd present, or
// cwd itself NULL/empty). A nil cwd argument stores SQL NULL; a non-nil value
// is stored verbatim (including the empty string).
func insertLegacySession(t *testing.T, db *DB, sessionID string, cwd *string) {
	t.Helper()
	var cwdArg any
	if cwd != nil {
		cwdArg = *cwd
	}
	if _, err := db.db.Exec(
		`INSERT INTO sessions (session_id, project_dir, cwd, repo_path, imported_at)
		 VALUES (?, '-test', ?, NULL, '2026-03-28T15:00:00Z')`,
		sessionID, cwdArg,
	); err != nil {
		t.Fatalf("insertLegacySession: %v", err)
	}
}

// insertLegacyMessage inserts a single messages row tied to sessionID, so the
// session is no longer an orphan. The uuid must be unique within the test DB.
func insertLegacyMessage(t *testing.T, db *DB, sessionID, uuid string) {
	t.Helper()
	if _, err := db.db.Exec(
		`INSERT INTO messages (uuid, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		 VALUES (?, ?, NULL, 'user', '{}', '2026-03-28T15:00:00Z', 0)`,
		uuid, sessionID,
	); err != nil {
		t.Fatalf("insertLegacyMessage: %v", err)
	}
}

func strptr(s string) *string { return &s }

func queryRepoPath(t *testing.T, db *DB, sessionID string) (sql.NullString, error) {
	t.Helper()
	var s sql.NullString
	err := db.db.QueryRow(
		`SELECT repo_path FROM sessions WHERE session_id = ?`, sessionID,
	).Scan(&s)
	return s, err
}

func sessionExists(t *testing.T, db *DB, sessionID string) bool {
	t.Helper()
	var n int
	if err := db.db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE session_id = ?`, sessionID,
	).Scan(&n); err != nil {
		t.Fatalf("sessionExists(%s): %v", sessionID, err)
	}
	return n > 0
}

// initGitRepo creates a git repository at dir and returns the EvalSymlinks'd
// path that git rev-parse --show-toplevel would report.
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-c", "safe.directory=*", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}
	return resolved
}

func TestBackfill_FillsWorktreeCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", strptr("/Users/test/proj/.claude/worktrees/feature"))
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 1 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:1 Unresolved:0}", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != "/Users/test/proj" {
		t.Errorf("repo_path = %+v, want {/Users/test/proj, valid}", got)
	}
}

func TestBackfill_SkipsNullCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", nil)
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want zero", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if got.Valid {
		t.Errorf("repo_path = %+v, want NULL", got)
	}
}

func TestBackfill_SkipsEmptyCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", strptr(""))
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want zero", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if got.Valid {
		t.Errorf("repo_path = %+v, want NULL", got)
	}
}

// TestBackfill_FillsNonGitCWDVerbatim は仕様 4（git 失敗時は cwd を
// そのまま返す）により、git 配下外の cwd でも repo_path が cwd 自体で埋まることを
// 担保する。
func TestBackfill_FillsNonGitCWDVerbatim(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	insertLegacySession(t, db, "s1", &missing)
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 1 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:1 Unresolved:0}", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != missing {
		t.Errorf("repo_path = %+v, want {%q, valid}", got, missing)
	}
}

func TestBackfill_LeavesFilledSessions(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := db.db.Exec(
		`INSERT INTO sessions (session_id, project_dir, cwd, repo_path, imported_at)
		 VALUES ('s1', '-test', '/Users/test/existing/.claude/worktrees/x', '/Users/test/existing', '2026-03-28T15:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want zero", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != "/Users/test/existing" {
		t.Errorf("repo_path = %+v, want {/Users/test/existing, valid}", got)
	}
}

func TestBackfill_Idempotent(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "ok", strptr("/Users/test/proj/.claude/worktrees/feature"))
	insertLegacyMessage(t, db, "ok", "m-ok")
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	insertLegacySession(t, db, "bad", &missing)
	insertLegacyMessage(t, db, "bad", "m-bad")
	insertLegacySession(t, db, "orphan", strptr("/Users/test/proj"))

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("first Backfill: %v", err)
	}
	if result.Resolved != 2 || result.Unresolved != 0 || result.Deleted != 1 {
		t.Errorf("first result = %+v, want {Deleted:1 Resolved:2 Unresolved:0}", result)
	}

	result, err = Backfill(db)
	if err != nil {
		t.Fatalf("second Backfill: %v", err)
	}
	if result.Resolved != 0 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("second result = %+v, want zero", result)
	}
}

func TestBackfill_MultipleSessionsSameCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	cwd := "/Users/test/proj/.claude/worktrees/feature"
	insertLegacySession(t, db, "s1", &cwd)
	insertLegacyMessage(t, db, "s1", "m1")
	insertLegacySession(t, db, "s2", &cwd)
	insertLegacyMessage(t, db, "s2", "m2")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 2 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:2 Unresolved:0}", result)
	}

	for _, id := range []string{"s1", "s2"} {
		got, err := queryRepoPath(t, db, id)
		if err != nil {
			t.Fatalf("queryRepoPath(%s): %v", id, err)
		}
		if !got.Valid || got.String != "/Users/test/proj" {
			t.Errorf("repo_path[%s] = %+v, want {/Users/test/proj, valid}", id, got)
		}
	}
}

// TestBackfill_GitToplevel verifies the git path resolution is exercised
// (not just the worktree string match), by using a real temp git repo as cwd.
func TestBackfill_GitToplevel(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	want := initGitRepo(t, dir)
	insertLegacySession(t, db, "s1", &dir)
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Resolved != 1 || result.Unresolved != 0 || result.Deleted != 0 {
		t.Errorf("result = %+v, want {Deleted:0 Resolved:1 Unresolved:0}", result)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != want {
		t.Errorf("repo_path = %+v, want %q", got, want)
	}
}

func TestBackfill_DeletesOrphanSessions(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "orphan", strptr("/Users/test/proj"))

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Deleted != 1 || result.Resolved != 0 || result.Unresolved != 0 {
		t.Errorf("result = %+v, want {Deleted:1 Resolved:0 Unresolved:0}", result)
	}
	if sessionExists(t, db, "orphan") {
		t.Errorf("orphan session not deleted")
	}
}

func TestBackfill_KeepsSessionsWithMessages(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	if _, err := db.db.Exec(
		`INSERT INTO sessions (session_id, project_dir, cwd, repo_path, imported_at)
		 VALUES ('s1', '-test', '/Users/test/proj', '/Users/test/proj', '2026-03-28T15:00:00Z')`,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}
	insertLegacyMessage(t, db, "s1", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Deleted != 0 {
		t.Errorf("result.Deleted = %d, want 0", result.Deleted)
	}
	if !sessionExists(t, db, "s1") {
		t.Errorf("session s1 must remain")
	}
}

// TestBackfill_DeletePlusResolveCombined verifies a single Backfill call can
// both delete orphans and resolve repo_path on the same DB without
// double-counting the deleted session in Resolved.
func TestBackfill_DeletePlusResolveCombined(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// orphan: messages 0 件かつ repo_path NULL — DELETE 対象
	insertLegacySession(t, db, "orphan", strptr("/Users/test/proj"))

	// kept: messages 1 件、repo_path NULL — UPDATE 対象（resolve）
	insertLegacySession(t, db, "kept", strptr("/Users/test/proj/.claude/worktrees/feature"))
	insertLegacyMessage(t, db, "kept", "m1")

	result, err := Backfill(db)
	if err != nil {
		t.Fatalf("Backfill: %v", err)
	}
	if result.Deleted != 1 || result.Resolved != 1 || result.Unresolved != 0 {
		t.Errorf("result = %+v, want {Deleted:1 Resolved:1 Unresolved:0}", result)
	}
	if sessionExists(t, db, "orphan") {
		t.Errorf("orphan session not deleted")
	}
	if !sessionExists(t, db, "kept") {
		t.Errorf("kept session must remain")
	}
}

func TestCountOrphanSessions(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "orphan-a", strptr("/x"))
	insertLegacySession(t, db, "orphan-b", strptr("/y"))
	insertLegacySession(t, db, "kept", strptr("/z"))
	insertLegacyMessage(t, db, "kept", "m1")

	count, err := CountOrphanSessions(db)
	if err != nil {
		t.Fatalf("CountOrphanSessions: %v", err)
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}
