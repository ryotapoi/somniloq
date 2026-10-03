package core

import (
	"io"
	"os"
	"os/exec"
	"strings"
)

const worktreePathFragment = "/.claude/worktrees/"

// ResolveRepoPath returns the main repository root for a session cwd. It checks
// the Claude worktree marker first, then asks Git for the repository identity, and
// returns cwd unchanged when Git cannot resolve it. An empty cwd returns "".
//
// An empty cwd must not be passed to git -C: Git treats it as the caller's
// current directory, which could select an unrelated repository.
func ResolveRepoPath(cwd string) string {
	if cwd == "" {
		return ""
	}
	if i := strings.Index(cwd, worktreePathFragment); i >= 0 {
		return cwd[:i]
	}
	// Git environment overrides can select a repository unrelated to the log cwd.
	// Filter only the child environment, preserving PATH and other caller settings.
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "GIT_") {
			env = append(env, entry)
		}
	}
	// Keep -C before rev-parse so cwd is consumed as its value, even when it
	// begins with a hyphen.
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	// Git failures use the cwd fallback; discard stderr to keep expected misses silent.
	cmd.Env = env
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return cwd
	}
	// Git terminates this output with a newline. Trimming only CR/LF preserves
	// trailing spaces; TrimSpace would remove them.
	root := strings.TrimRight(string(out), "\r\n")
	// Only linked worktrees have a Git directory distinct from the common
	// directory. Ordinary repositories and submodules retain their top-level.
	cmd = exec.Command("git", "-C", cwd, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	cmd.Env = env
	cmd.Stderr = io.Discard
	out, err = cmd.Output()
	if err != nil {
		return root
	}
	dirs := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	if len(dirs) != 2 || dirs[0] == dirs[1] {
		return root
	}
	// Git lists the main worktree first. Do not infer its root from the
	// common directory's location; that location need not be root/.git.
	cmd = exec.Command("git", "-C", cwd, "worktree", "list", "--porcelain", "-z")
	cmd.Env = env
	cmd.Stderr = io.Discard
	out, err = cmd.Output()
	if err != nil {
		return root
	}
	first, _, _ := strings.Cut(string(out), "\x00")
	if main, ok := strings.CutPrefix(first, "worktree "); ok {
		return main
	}
	return root
}
