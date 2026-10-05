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

func TestMainDispatchMissingDefaultConfig(t *testing.T) {
	home := t.TempDir()
	for _, command := range []string{"import", "migrate", "search", "show", "projects"} {
		code, stdout, stderr := runSomniloqMain(t, home, command)
		explicitCode, explicitOut, explicitErr := runSomniloqMain(t, home, command, "--config", "default")
		if code != 2 || stdout != "" || !strings.Contains(stderr, "missing config") || !strings.Contains(stderr, configSetupHint) || code != explicitCode || stdout != explicitOut || stderr != explicitErr {
			t.Fatalf("%s: omitted=%d %q %q explicit=%d %q %q", command, code, stdout, stderr, explicitCode, explicitOut, explicitErr)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".somniloq")); !os.IsNotExist(err) {
		t.Fatalf("missing config created state: %v", err)
	}
}

func TestMainDispatchHelpAndVersionWithoutConfig(t *testing.T) {
	for _, command := range []string{"import", "migrate", "show", "search", "projects"} {
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
	for _, args := range [][]string{{"import"}, {"import", "--config", "default"}, {"--config", configPath, "import"}} {
		code, stdout, stderr = runSomniloqMain(t, home, args...)
		if code != 0 || !strings.Contains(stdout, "Imported 0 files") {
			t.Fatalf("%v: %d %q %q", args, code, stdout, stderr)
		}
	}
	code, _, stderr = runSomniloqMain(t, home, "search", "--config", "missing")
	if code != 2 || !strings.Contains(stderr, "Run somniloq config init, then use --config default.") {
		t.Fatalf("missing config: %d %q", code, stderr)
	}
	code, _, _ = runSomniloqMain(t, home, "--db", "x", "search")
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
	code, _, stderr = runSomniloqMain(t, home, "search", "--config", "default")
	if code != 2 {
		t.Fatalf("read missing: %d %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(home, ".somniloq", "default.db")); !os.IsNotExist(err) {
		t.Fatalf("read created DB: %v", err)
	}
}

func TestExtractCommandConfigPreservesFlagValues(t *testing.T) {
	value := ""
	args, err := extractCommandConfig("search", []string{"--project", "--config", "--config", "chosen"}, &value)
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

func TestRemovedOutlineAndShowHelpAfterREF(t *testing.T) {
	code, stdout, stderr := runSomniloqMain(t, t.TempDir(), "outline", "--help")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "unknown command: outline") {
		t.Fatalf("outline: %d %q %q", code, stdout, stderr)
	}
	code, stdout, stderr = runSomniloqMain(t, t.TempDir(), "show", "bare", "--help")
	if code != 0 || stdout != "" || !strings.Contains(stderr, "--messages") {
		t.Fatalf("show help: %d %q %q", code, stdout, stderr)
	}
}

func TestMainDispatchRejectsSessions(t *testing.T) {
	for _, args := range [][]string{{"sessions"}, {"sessions", "--help"}, {"sessions", "--config", "missing"}} {
		code, stdout, stderr := runSomniloqMain(t, t.TempDir(), args...)
		if code != 1 || stdout != "" || !strings.Contains(stderr, "unknown command: sessions") {
			t.Fatalf("%v: %d %q %q", args, code, stdout, stderr)
		}
	}
	code, _, stderr := runSomniloqMain(t, t.TempDir(), "--help")
	if code != 0 || strings.Contains(stderr, "  sessions ") || !strings.Contains(stderr, "  search ") {
		t.Fatalf("top-level help: %d %q", code, stderr)
	}
}

func TestMainDispatchDefaultEquivalence(t *testing.T) {
	home := t.TempDir()
	code, _, stderr := runSomniloqMain(t, home, "config", "init")
	if code != 0 {
		t.Fatal(stderr)
	}
	code, _, stderr = runSomniloqMain(t, home, "import")
	if code != 0 {
		t.Fatal(stderr)
	}
	for _, args := range [][]string{{"import"}, {"projects"}, {"search", "--format", "json"}, {"show", "bare"}, {"migrate"}} {
		code, stdout, stderr := runSomniloqMain(t, home, args...)
		explicit := append([]string{"--config", "default"}, args...)
		explicitCode, explicitOut, explicitErr := runSomniloqMain(t, home, explicit...)
		if code != explicitCode || stdout != explicitOut || stderr != explicitErr || strings.Contains(stderr, "missing config") {
			t.Fatalf("%v: omitted=%d %q %q explicit=%d %q %q", args, code, stdout, stderr, explicitCode, explicitOut, explicitErr)
		}
		if args[0] == "migrate" && !strings.Contains(stderr, "missing --from") {
			t.Fatalf("migrate did not reach argument validation: %q", stderr)
		}
	}
	for _, args := range [][]string{{"search", "--config="}, {"--config=", "search"}, {"search", "--config"}} {
		code, stdout, _ := runSomniloqMain(t, home, args...)
		if code != 2 || stdout != "" {
			t.Fatalf("%v: code=%d stdout=%q", args, code, stdout)
		}
	}
}
