package core

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// unsetAllGitEnv clears GIT_* variables because they can affect repository
// discovery. An empty GIT_DIR value is not equivalent to unsetting it.
func unsetAllGitEnv(t *testing.T) {
	t.Helper()
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq < 0 {
			continue
		}
		key := kv[:eq]
		if !strings.HasPrefix(key, "GIT_") {
			continue
		}
		orig, ok := os.LookupEnv(key)
		if !ok {
			continue
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("os.Unsetenv(%q): %v", key, err)
		}
		k := key
		v := orig
		t.Cleanup(func() {
			_ = os.Setenv(k, v)
		})
	}
}

func TestResolveRepoPath_Empty(t *testing.T) {
	if got := ResolveRepoPath(""); got != "" {
		t.Errorf("ResolveRepoPath(\"\") = %q, want empty", got)
	}
}

func TestResolveRepoPath_Worktree(t *testing.T) {
	// This pins worktree-marker precedence over Git discovery, including for a
	// cwd that does not exist.
	unsetAllGitEnv(t)

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{
			name: "worktree root",
			cwd:  "/Users/foo/repo/.claude/worktrees/feat-x",
			want: "/Users/foo/repo",
		},
		{
			name: "worktree subdirectory resolves to worktree host",
			cwd:  "/Users/foo/repo/.claude/worktrees/feat-x/sub/dir",
			want: "/Users/foo/repo",
		},
		{
			// Repeated markers resolve at the first occurrence.
			name: "multiple fragments cut at first occurrence",
			cwd:  "/foo/.claude/worktrees/x/.claude/worktrees/y",
			want: "/foo",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveRepoPath(tc.cwd); got != tc.want {
				t.Errorf("ResolveRepoPath(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}

	// A lookalike marker without the trailing slash must not select a worktree.
	t.Run("similar but not exact does not match", func(t *testing.T) {
		cwd := "/foo/bar/.claude/worktreesXYZ/baz"
		if got := ResolveRepoPath(cwd); got != cwd {
			t.Errorf("ResolveRepoPath(%q) = %q, want %q", cwd, got, cwd)
		}
	})
}

// Keep these tests sequential because GIT_* changes are process-wide.
func TestResolveRepoPath_GitToplevel(t *testing.T) {
	unsetAllGitEnv(t)

	dir := t.TempDir()
	// Resolve symlinks because Git reports the canonical temporary path on macOS.
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", dir, err)
	}

	// Allow git init under CI safe.directory restrictions without changing the
	// production Git invocation.
	cmd := exec.Command("git", "-c", "safe.directory=*", "-C", dir, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatalf("Mkdir(%q): %v", sub, err)
	}

	tests := []struct {
		name string
		cwd  string
	}{
		{"toplevel direct", dir},
		{"subdirectory resolves to root", sub},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveRepoPath(tc.cwd); got != want {
				t.Errorf("ResolveRepoPath(%q) = %q, want %q", tc.cwd, got, want)
			}
		})
	}
}

// TestResolveRepoPath_GitToplevel_PreservesTrailingSpace verifies that a
// trailing space in a valid repository path survives Git output handling.
func TestResolveRepoPath_GitToplevel_PreservesTrailingSpace(t *testing.T) {
	unsetAllGitEnv(t)

	parent := t.TempDir()
	repo := filepath.Join(parent, "trail ")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("Mkdir(%q): %v", repo, err)
	}
	want, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", repo, err)
	}

	cmd := exec.Command("git", "-c", "safe.directory=*", "-C", repo, "init", "-q")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	if got := ResolveRepoPath(repo); got != want {
		t.Errorf("ResolveRepoPath(%q) = %q, want %q (trailing space must survive)", repo, got, want)
	}
}

func TestResolveRepoPath_NotGitRepo(t *testing.T) {
	unsetAllGitEnv(t)

	nonGit := t.TempDir()
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	tests := []struct {
		name string
		cwd  string
		want string
	}{
		{"non-git directory", nonGit, nonGit},
		{"nonexistent path", missing, missing},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResolveRepoPath(tc.cwd); got != tc.want {
				t.Errorf("ResolveRepoPath(%q) = %q, want %q", tc.cwd, got, tc.want)
			}
		})
	}

	t.Run("nonexistent path with trailing space", func(t *testing.T) {
		cwd := filepath.Join(t.TempDir(), "missing dir ")
		if got := ResolveRepoPath(cwd); got != cwd {
			t.Errorf("ResolveRepoPath(%q) = %q, want %q", cwd, got, cwd)
		}
	})
}

// Keep Git discovery tests sequential because they clear process-wide GIT_*.
func linkedWorktree(t *testing.T) (string, string) {
	t.Helper()
	unsetAllGitEnv(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(parent, "main repo ")
	worktree := filepath.Join(parent, "linked tree ")
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", repo)
	run("-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "Initial")
	run("-C", repo, "worktree", "add", "-q", "-b", "linked", worktree)
	return repo, worktree
}

func TestResolveRepoPath_LinkedWorktree(t *testing.T) {
	repo, worktree := linkedWorktree(t)
	sub := filepath.Join(worktree, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, cwd := range []string{repo, worktree, sub} {
		if got := ResolveRepoPath(cwd); got != repo {
			t.Errorf("ResolveRepoPath(%q) = %q, want %q", cwd, got, repo)
		}
	}
}

func TestResolveRepoPath_Submodule(t *testing.T) {
	repo, _ := linkedWorktree(t)
	origin := filepath.Join(t.TempDir(), "origin")
	for _, args := range [][]string{
		{"init", "-q", origin},
		{"-C", origin, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", "Initial"},
		{"-C", repo, "-c", "protocol.file.allow=always", "submodule", "add", "-q", origin, "module"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	module := filepath.Join(repo, "module")
	if got := ResolveRepoPath(module); got != module {
		t.Errorf("ResolveRepoPath(%q) = %q, want submodule root", module, got)
	}
}
