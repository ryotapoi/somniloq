package main

import (
	"bytes"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMainDispatchSkipsBrokenConfigForConfigIndependentCommands(t *testing.T) {
	home := homeWithBrokenConfig(t)
	if err := os.MkdirAll(filepath.Join(home, ".claude", "projects"), 0o755); err != nil {
		t.Fatalf("create claude projects dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex", "sessions"), 0o755); err != nil {
		t.Fatalf("create codex sessions dir: %v", err)
	}

	tests := []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{
			name:     "import",
			args:     []string{"import", "--source", "claude-code"},
			wantCode: 0,
			want:     "Imported 0 files",
		},
		{
			name:     "backfill",
			args:     []string{"backfill", "--yes"},
			wantCode: 0,
			want:     "Backfilled:",
		},
		{
			name:     "outline",
			args:     []string{"outline"},
			wantCode: 1,
			want:     "usage: somniloq outline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runSomniloqMain(t, home, tt.args...)
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", code, tt.wantCode, stdout, stderr)
			}
			combined := stdout + stderr
			if !strings.Contains(combined, tt.want) {
				t.Fatalf("output missing %q\nstdout:\n%s\nstderr:\n%s", tt.want, stdout, stderr)
			}
			if strings.Contains(stderr, "parse config") {
				t.Fatalf("stderr unexpectedly contains config error:\n%s", stderr)
			}
		})
	}
}

func TestMainDispatchSubcommandHelpSkipsBrokenConfig(t *testing.T) {
	home := homeWithBrokenConfig(t)

	for _, subcommand := range []string{"import", "backfill", "sessions", "show", "outline", "search", "projects"} {
		t.Run(subcommand, func(t *testing.T) {
			code, stdout, stderr := runSomniloqMain(t, home, subcommand, "--help")
			if code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if !strings.Contains(stderr, "Usage:") {
				t.Fatalf("help output missing Usage\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
			if strings.Contains(stderr, "parse config") {
				t.Fatalf("stderr unexpectedly contains config error:\n%s", stderr)
			}
		})
	}
}

func TestMainDispatchSubcommandHelpAfterFlagsSkipsBrokenConfig(t *testing.T) {
	home := homeWithBrokenConfig(t)

	tests := []struct {
		name string
		args []string
	}{
		{
			name: "sessions",
			args: []string{"sessions", "--format", "json", "--help"},
		},
		{
			name: "show",
			args: []string{"show", "--short", "--help"},
		},
		{
			name: "search",
			args: []string{"search", "--project", "somniloq", "--help"},
		},
		{
			name: "projects",
			args: []string{"projects", "--format", "json", "--help"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runSomniloqMain(t, home, tt.args...)
			if code != 0 {
				t.Fatalf("exit code = %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
			if !strings.Contains(stderr, "Usage:") {
				t.Fatalf("help output missing Usage\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
			if strings.Contains(stderr, "parse config") {
				t.Fatalf("stderr unexpectedly contains config error:\n%s", stderr)
			}
		})
	}
}

func TestMainDispatchConfigDependentCommandLoadsBrokenConfig(t *testing.T) {
	home := homeWithBrokenConfig(t)

	code, stdout, stderr := runSomniloqMain(t, home, "sessions")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "parse config") {
		t.Fatalf("stderr missing config error\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

func TestMainDispatchHelpLikeFlagValueDoesNotSkipBrokenConfig(t *testing.T) {
	home := homeWithBrokenConfig(t)

	code, stdout, stderr := runSomniloqMain(t, home, "sessions", "--project", "-h")
	if code != 1 {
		t.Fatalf("exit code = %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if !strings.Contains(stderr, "parse config") {
		t.Fatalf("stderr missing config error\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

func homeWithBrokenConfig(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	configDir := filepath.Join(home, ".somniloq")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.json"), []byte("{"), 0o644); err != nil {
		t.Fatalf("write broken config: %v", err)
	}
	return home
}

func runSomniloqMain(t *testing.T, home string, args ...string) (int, string, string) {
	t.Helper()

	testArgs := append([]string{"-test.run=TestSomniloqMainHelper", "--"}, args...)
	cmd := exec.Command(os.Args[0], testArgs...)
	cmd.Env = append(os.Environ(), "SOMNILOQ_MAIN_HELPER=1", "HOME="+home)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return 0, stdout.String(), stderr.String()
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	}
	t.Fatalf("run helper: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	return 1, stdout.String(), stderr.String()
}

func TestSomniloqMainHelper(t *testing.T) {
	if os.Getenv("SOMNILOQ_MAIN_HELPER") != "1" {
		return
	}

	sep := -1
	for i, arg := range os.Args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == -1 {
		os.Exit(2)
	}

	os.Args = append([]string{"somniloq"}, os.Args[sep+1:]...)
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	main()
}
