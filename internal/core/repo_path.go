package core

import (
	"io"
	"os/exec"
	"strings"
)

const worktreePathFragment = "/.claude/worktrees/"

// ResolveRepoPath returns the repository root for a Claude Code cwd. It checks
// the worktree marker first, then asks Git for the top-level directory, and
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
	// Keep -C before rev-parse so cwd is consumed as its value, even when it
	// begins with a hyphen.
	cmd := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel")
	// Git failures use the cwd fallback; discard stderr to keep expected misses silent.
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		return cwd
	}
	// Git terminates this output with a newline. Trimming only CR/LF preserves
	// trailing spaces; TrimSpace would remove them.
	return strings.TrimRight(string(out), "\r\n")
}
