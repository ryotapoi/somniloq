package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/ryotapoi/somniloq/internal/core"
)

const topLevelUsage = `Session log viewer for Claude Code, Codex, and Cursor Agent

Usage:
  somniloq [flags] <command>

Commands:
  import    Import Claude Code, Codex, and Cursor Agent session logs from JSONL files
  sessions  List sessions
  show      Show session content in Markdown
  outline   List a session's user messages as turn, time, body size, and first line
  search    Search message content across sessions with turn numbers
  projects  List projects
  config init Create a TOML configuration

Flags:
`

func main() {
	code, err := runCommand(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, isatty.IsTerminal(os.Stdin.Fd()))
	if err != nil {

		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	os.Exit(code)
}

func runCommand(args []string, in io.Reader, out, errOut io.Writer, isTTY bool) (code int, cmdErr error) {
	defer func() {
		var ioErr *ConfigIOError
		if errors.As(cmdErr, &ioErr) {
			return
		}
		var schemaError *core.SchemaError
		if errors.As(cmdErr, &schemaError) || errors.Is(cmdErr, os.ErrNotExist) {
			code = 2
		}
	}()
	fs := flag.NewFlagSet("somniloq", flag.ContinueOnError)
	cfgValue := fs.String("config", "", "TOML configuration name or path (required for DB commands)")
	version := fs.Bool("version", false, "print version and exit")
	fs.SetOutput(errOut)
	fs.Usage = func() { fmt.Fprint(errOut, topLevelUsage); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0, nil
		}
		return 2, nil
	}
	if *version {
		fmt.Fprintf(out, "somniloq version %s\n", getVersion())
		return 0, nil
	}
	args = fs.Args()
	if len(args) == 0 {
		fs.Usage()
		return 1, nil
	}
	command, commandArgs := args[0], args[1:]
	if command == "config" {
		if len(commandArgs) == 0 || commandArgs[0] != "init" {
			return 2, fmt.Errorf("usage: somniloq config init [NAME] [--output PATH] [--db PATH]")
		}
		return configInitCmd(commandArgs[1:], out, errOut)
	}
	if configCommandFlagSet(command) == nil {
		return 1, fmt.Errorf("unknown command: %s", command)
	}
	var err error
	commandArgs, err = extractCommandConfig(command, commandArgs, cfgValue)
	if err != nil {
		return 2, err
	}
	cfg := config{}
	if !isHelpRequest(command, commandArgs) {
		if *cfgValue == "" {
			return 2, fmt.Errorf("missing --config. Run somniloq config init, then use --config default.")
		}
		cfg, err = loadConfig(*cfgValue)
		if err != nil {
			var ioErr *ConfigIOError
			if errors.As(err, &ioErr) {
				return 1, err
			}
			return 2, err
		}
	}
	open := func() (*core.DB, error) { return core.OpenDBRead(cfg.DB) }
	switch command {
	case "import":
		return importConfiguredCmd(commandArgs, func() (*core.DB, error) { return openDB(cfg.DB) }, cfg, in, out, errOut, isTTY)
	case "sessions":
		return sessionsCmd(commandArgs, open, cfg, out, errOut)
	case "show":
		return showCmd(commandArgs, open, cfg, out, errOut)
	case "outline":
		return outlineCmd(commandArgs, open, cfg, out, errOut)
	case "search":
		return searchCmd(commandArgs, open, cfg, out, errOut)
	case "projects":
		return projectsCmd(commandArgs, open, cfg, out, errOut)
	}
	panic("unreachable")
}

// Extract the common option without consuming command-specific values.
func extractCommandConfig(command string, args []string, value *string) ([]string, error) {
	commandFlags := configCommandFlagSet(command)
	result := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			result = append(result, args[i:]...)
			break
		}
		name, hasValue, ok := splitFlagArg(args[i])
		if ok && name == "config" {
			if hasValue {
				_, *value, _ = strings.Cut(args[i], "=")
			} else {
				i++
				if i == len(args) {
					return nil, fmt.Errorf("--config requires a value")
				}
				*value = args[i]
			}
			continue
		}
		result = append(result, args[i])
		if ok && !hasValue {
			if f := commandFlags.Lookup(name); f != nil && flagConsumesValue(f) && i+1 < len(args) {
				i++
				result = append(result, args[i])
			}
		}
	}
	return result, nil
}

func isHelpRequest(command string, args []string) bool {
	fs := configCommandFlagSet(command)
	if fs == nil {
		return false
	}

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
		f := fs.Lookup(name)
		if f == nil {
			return false
		}
		if flagConsumesValue(f) && !hasValue {
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

func configCommandFlagSet(command string) *flag.FlagSet {
	switch command {
	case "import":
		fs := flag.NewFlagSet("import", flag.ContinueOnError)
		fs.Bool("full", false, "")
		fs.Bool("yes", false, "")
		fs.String("source", "all", "")
		fs.String("input", "", "")
		return fs
	case "sessions":
		fs, _ := newSessionsFlagSet()
		return fs
	case "show":
		fs, _ := newShowFlagSet()
		return fs
	case "search":
		fs, _ := newSearchFlagSet()
		return fs
	case "projects":
		fs, _ := newProjectsFlagSet()
		return fs
	case "outline":
		fs, _ := newOutlineFlagSet()
		return fs
	}
	return nil
}

func flagConsumesValue(f *flag.Flag) bool {
	boolFlag, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !boolFlag.IsBoolFlag()
}

func openDB(dbPath string) (*core.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	db, err := core.OpenDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return db, nil
}
