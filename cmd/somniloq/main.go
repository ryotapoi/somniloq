package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattn/go-isatty"
	"github.com/ryotapoi/somniloq/internal/core"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	defaultDB := filepath.Join(homeDir, ".somniloq", "somniloq.db")
	defaultProjectsDir := filepath.Join(homeDir, ".claude", "projects")
	defaultCodexSessionsDir := filepath.Join(homeDir, ".codex", "sessions")

	dbPath := flag.String("db", defaultDB, "path to SQLite database")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `Session log viewer for Claude Code and Codex

Usage:
  somniloq [flags] <command>

Commands:
  import    Import Claude Code and Codex session logs from JSONL files
  backfill  Correct legacy session data
  sessions  List sessions
  show      Show session content in Markdown
  outline   List a session's user messages as turn, time, and first line
  search    Search message content across sessions
  projects  List projects

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("somniloq version %s\n", getVersion())
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	open := func() (*core.DB, error) {
		return openDB(*dbPath)
	}

	var code int
	var cmdErr error
	switch args[0] {
	case "import":
		code, cmdErr = importCmd(args[1:], open, defaultProjectsDir, defaultCodexSessionsDir, os.Stdin, os.Stdout, os.Stderr, isTTY)
	case "backfill":
		code, cmdErr = backfillCmd(args[1:], open, os.Stdin, os.Stdout, os.Stderr, isTTY)
	case "sessions":
		code, cmdErr = sessionsCmd(args[1:], open, os.Stdout, os.Stderr)
	case "show":
		code, cmdErr = showCmd(args[1:], open, os.Stdout, os.Stderr)
	case "outline":
		code, cmdErr = outlineCmd(args[1:], open, os.Stdout, os.Stderr)
	case "search":
		code, cmdErr = searchCmd(args[1:], open, os.Stdout, os.Stderr)
	case "projects":
		code, cmdErr = projectsCmd(args[1:], open, os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
	if cmdErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", cmdErr)
	}
	os.Exit(code)
}

func openDB(dbPath string) (*core.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	db, err := core.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}
