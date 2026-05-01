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

func strptr(s string) *string { return &s }

func queryRepoPath(t *testing.T, db *DB, sessionID string) (sql.NullString, error) {
	t.Helper()
	var s sql.NullString
	err := db.db.QueryRow(
		`SELECT repo_path FROM sessions WHERE session_id = ?`, sessionID,
	).Scan(&s)
	return s, err
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

func TestBackfillRepoPaths_FillsWorktreeCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", strptr("/Users/test/proj/.claude/worktrees/feature"))

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 1 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (1, 0)", resolved, unresolved)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != "/Users/test/proj" {
		t.Errorf("repo_path = %+v, want {/Users/test/proj, valid}", got)
	}
}

func TestBackfillRepoPaths_SkipsNullCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", nil)

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 0 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (0, 0)", resolved, unresolved)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if got.Valid {
		t.Errorf("repo_path = %+v, want NULL", got)
	}
}

func TestBackfillRepoPaths_SkipsEmptyCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "s1", strptr(""))

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 0 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (0, 0)", resolved, unresolved)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if got.Valid {
		t.Errorf("repo_path = %+v, want NULL", got)
	}
}

// TestBackfillRepoPaths_FillsNonGitCWDVerbatim は仕様 4（git 失敗時は cwd を
// そのまま返す）により、git 配下外の cwd でも repo_path が cwd 自体で埋まることを
// 担保する。
func TestBackfillRepoPaths_FillsNonGitCWDVerbatim(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	insertLegacySession(t, db, "s1", &missing)

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 1 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (1, 0)", resolved, unresolved)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != missing {
		t.Errorf("repo_path = %+v, want {%q, valid}", got, missing)
	}
}

func TestBackfillRepoPaths_LeavesFilledSessions(t *testing.T) {
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

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 0 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (0, 0)", resolved, unresolved)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != "/Users/test/existing" {
		t.Errorf("repo_path = %+v, want {/Users/test/existing, valid}", got)
	}
}

func TestBackfillRepoPaths_Idempotent(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	insertLegacySession(t, db, "ok", strptr("/Users/test/proj/.claude/worktrees/feature"))
	missing := filepath.Join(t.TempDir(), "does-not-exist")
	insertLegacySession(t, db, "bad", &missing)

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("first BackfillRepoPaths: %v", err)
	}
	if resolved != 2 || unresolved != 0 {
		t.Errorf("first counts = (%d, %d), want (2, 0)", resolved, unresolved)
	}

	resolved, unresolved, err = BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("second BackfillRepoPaths: %v", err)
	}
	if resolved != 0 || unresolved != 0 {
		t.Errorf("second counts = (%d, %d), want (0, 0)", resolved, unresolved)
	}
}

func TestBackfillRepoPaths_MultipleSessionsSameCWD(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	cwd := "/Users/test/proj/.claude/worktrees/feature"
	insertLegacySession(t, db, "s1", &cwd)
	insertLegacySession(t, db, "s2", &cwd)

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 2 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (2, 0)", resolved, unresolved)
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

// TestBackfillRepoPaths_GitToplevel verifies the git path resolution is exercised
// (not just the worktree string match), by using a real temp git repo as cwd.
func TestBackfillRepoPaths_GitToplevel(t *testing.T) {
	unsetAllGitEnv(t)

	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	dir := t.TempDir()
	want := initGitRepo(t, dir)
	insertLegacySession(t, db, "s1", &dir)

	resolved, unresolved, err := BackfillRepoPaths(db)
	if err != nil {
		t.Fatalf("BackfillRepoPaths: %v", err)
	}
	if resolved != 1 || unresolved != 0 {
		t.Errorf("counts = (%d, %d), want (1, 0)", resolved, unresolved)
	}

	got, err := queryRepoPath(t, db, "s1")
	if err != nil {
		t.Fatalf("queryRepoPath: %v", err)
	}
	if !got.Valid || got.String != want {
		t.Errorf("repo_path = %+v, want %q", got, want)
	}
}
