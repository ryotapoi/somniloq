package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/ryotapoi/somniloq/internal/core"
)

const topLevelUsage = `Session log viewer for Claude Code and Codex

Usage:
  somniloq [flags] <command>

Commands:
  import    Import Claude Code and Codex session logs from JSONL files
  backfill  Correct legacy session data
  sessions  List sessions
  show      Show session content in Markdown
  outline   List a session's user messages as turn, time, body size, and first line
  search    Search message content across sessions with turn numbers
  projects  List projects

Flags:
`

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	defaultDB := filepath.Join(homeDir, ".somniloq", "somniloq.db")
	defaultConfig := filepath.Join(homeDir, ".somniloq", "config.json")
	defaultProjectsDir := filepath.Join(homeDir, ".claude", "projects")
	defaultCodexSessionsDir := filepath.Join(homeDir, ".codex", "sessions")

	dbPath := flag.String("db", defaultDB, "path to SQLite database")
	configPath := flag.String("config", defaultConfig, "path to config file (JSON)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, topLevelUsage)
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

	loadCommandConfig := func(command string, commandArgs []string) config {
		cfg, err := loadConfigForCommand(*configPath, command, commandArgs)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return cfg
	}

	var code int
	var cmdErr error
	switch args[0] {
	case "import":
		code, cmdErr = importCmd(args[1:], open, defaultProjectsDir, defaultCodexSessionsDir, os.Stdin, os.Stdout, os.Stderr, isTTY)
	case "backfill":
		code, cmdErr = backfillCmd(args[1:], open, os.Stdin, os.Stdout, os.Stderr, isTTY)
	case "sessions":
		cfg := loadCommandConfig(args[0], args[1:])
		code, cmdErr = sessionsCmd(args[1:], open, cfg, os.Stdout, os.Stderr)
	case "show":
		cfg := loadCommandConfig(args[0], args[1:])
		code, cmdErr = showCmd(args[1:], open, cfg, os.Stdout, os.Stderr)
	case "outline":
		code, cmdErr = outlineCmd(args[1:], open, os.Stdout, os.Stderr)
	case "search":
		cfg := loadCommandConfig(args[0], args[1:])
		code, cmdErr = searchCmd(args[1:], open, cfg, os.Stdout, os.Stderr)
	case "projects":
		cfg := loadCommandConfig(args[0], args[1:])
		code, cmdErr = projectsCmd(args[1:], open, cfg, os.Stdout, os.Stderr)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
	if cmdErr != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", cmdErr)
	}
	os.Exit(code)
}

func loadConfigForCommand(configPath, command string, commandArgs []string) (config, error) {
	if isHelpRequest(command, commandArgs) {
		return config{}, nil
	}
	return loadConfig(configPath)
}

func isHelpRequest(command string, args []string) bool {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			return false
		}
		name, hasValue, ok := splitFlagArg(arg)
		if !ok {
			return false
		}
		if name == "h" || name == "help" {
			return true
		}
		consumesValue, known := configCommandFlagConsumesValue(command, name)
		if !known {
			return false
		}
		if consumesValue && !hasValue {
			i++
		}
	}
	return false
}

func splitFlagArg(arg string) (name string, hasValue bool, ok bool) {
	if strings.HasPrefix(arg, "--") {
		name = strings.TrimPrefix(arg, "--")
	} else if strings.HasPrefix(arg, "-") {
		name = strings.TrimPrefix(arg, "-")
	} else {
		return "", false, false
	}
	if name == "" {
		return "", false, false
	}
	name, _, hasValue = strings.Cut(name, "=")
	return name, hasValue, true
}

func configCommandFlagConsumesValue(command, name string) (bool, bool) {
	switch command {
	case "sessions":
		switch name {
		case "since", "until", "day-boundary", "project", "format":
			return true, true
		case "short":
			return false, true
		}
	case "show":
		switch name {
		case "since", "until", "project", "summary", "turn", "tail", "format":
			return true, true
		case "short", "include-clear":
			return false, true
		}
	case "search":
		switch name {
		case "since", "until", "day-boundary", "project":
			return true, true
		}
	case "projects":
		switch name {
		case "since", "until", "format":
			return true, true
		case "short":
			return false, true
		}
	}
	return false, false
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
