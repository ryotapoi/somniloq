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
		if i == 0 {
			return "/"
		}
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
	// Keep -C before the subcommand so cwd is consumed as its value, even when
	// it begins with a hyphen. Every query must use the filtered environment.
	gitOutput := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", cwd}, args...)...)
		cmd.Env = env
		// Git failures use the cwd/root fallback; keep expected misses silent.
		cmd.Stderr = io.Discard
		out, err := cmd.Output()
		return string(out), err
	}
	out, err := gitOutput("rev-parse", "--show-toplevel")
	if err != nil {
		return cwd
	}
	// Git adds one LF to each single path result. The path itself may end in
	// CR, LF, or spaces, so remove only that terminator.
	root := strings.TrimSuffix(out, "\n")
	// Query paths separately: embedded LF is a valid path byte, not a separator.
	gitDir, err := gitOutput("rev-parse", "--path-format=absolute", "--git-dir")
	if err != nil {
		return root
	}
	commonDir, err := gitOutput("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return root
	}
	// Only linked worktrees have a Git directory distinct from the common
	// directory. Ordinary repositories and submodules retain their top-level.
	if strings.TrimSuffix(gitDir, "\n") == strings.TrimSuffix(commonDir, "\n") {
		return root
	}
	// Git lists the main worktree first. Do not infer its root from the
	// common directory's location; that location need not be root/.git.
	out, err = gitOutput("worktree", "list", "--porcelain", "-z")
	if err != nil {
		return root
	}
	first, _, _ := strings.Cut(out, "\x00")
	if main, ok := strings.CutPrefix(first, "worktree "); ok {
		return main
	}
	return root
}
