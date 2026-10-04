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

func TestMainDispatchRequiresExplicitConfig(t *testing.T) {
	home := t.TempDir()
	for _, command := range []string{"import", "sessions", "search", "show", "outline", "projects"} {
		code, stdout, stderr := runSomniloqMain(t, home, command)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "Run somniloq config init, then use --config default.") {
			t.Fatalf("%s: %d %q %q", command, code, stdout, stderr)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".somniloq")); !os.IsNotExist(err) {
		t.Fatalf("missing config created state: %v", err)
	}
}

func TestMainDispatchHelpAndVersionWithoutConfig(t *testing.T) {
	for _, command := range []string{"import", "sessions", "show", "outline", "search", "projects"} {
		code, _, stderr := runSomniloqMain(t, t.TempDir(), command, "--help")
		if code != 0 || !strings.Contains(stderr, "Usage:") {
			t.Fatalf("%s help: %d %q", command, code, stderr)
		}
	}
	code, stdout, _ := runSomniloqMain(t, t.TempDir(), "--version")
	if code != 0 || !strings.Contains(stdout, "somniloq version") {
		t.Fatalf("version: %d %q", code, stdout)
	}
}

func TestMainDispatchConfigInitAndPlacement(t *testing.T) {
	home := t.TempDir()
	code, stdout, stderr := runSomniloqMain(t, home, "config", "init")
	if code != 0 {
		t.Fatalf("init: %d %q %q", code, stdout, stderr)
	}
	configPath := strings.TrimSpace(stdout)
	if _, err := os.Stat(filepath.Join(home, ".somniloq", "default.db")); !os.IsNotExist(err) {
		t.Fatalf("init created DB: %v", err)
	}
	for _, args := range [][]string{{"import", "--config", "default"}, {"--config", configPath, "import"}} {
		code, stdout, stderr = runSomniloqMain(t, home, args...)
		if code != 0 || !strings.Contains(stdout, "Imported 0 files") {
			t.Fatalf("%v: %d %q %q", args, code, stdout, stderr)
		}
	}
	code, _, stderr = runSomniloqMain(t, home, "sessions", "--config", "missing")
	if code != 2 || !strings.Contains(stderr, "Run somniloq config init, then use --config default.") {
		t.Fatalf("missing config: %d %q", code, stderr)
	}
	code, _, _ = runSomniloqMain(t, home, "--db", "x", "sessions")
	if code != 2 {
		t.Fatalf("--db accepted: %d", code)
	}
}

func TestMainDispatchReadMissingDBDoesNotCreate(t *testing.T) {
	home := t.TempDir()
	code, _, stderr := runSomniloqMain(t, home, "config", "init")
	if code != 0 {
		t.Fatal(stderr)
	}
	code, _, stderr = runSomniloqMain(t, home, "sessions", "--config", "default")
	if code != 2 {
		t.Fatalf("read missing: %d %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".somniloq", "default.db")); !os.IsNotExist(err) {
		t.Fatalf("read created DB: %v", err)
	}
}

func TestExtractCommandConfigPreservesFlagValues(t *testing.T) {
	value := ""
	args, err := extractCommandConfig("sessions", []string{"--project", "--config", "--config", "chosen"}, &value)
	if err != nil || value != "chosen" || len(args) != 2 || args[1] != "--config" {
		t.Fatalf("%v %q %v", args, value, err)
	}
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
