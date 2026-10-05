package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/ryotapoi/somniloq/internal/core"
)

func configInitCmd(args []string, out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("config init", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() {
		fmt.Fprintln(errOut, `Create a TOML configuration only when both the config and database paths are absent.

Usage:
  somniloq config init [NAME] [--output PATH] [--db PATH]

NAME defaults to default. Existing files, directories, and symlinks are refused.
Only the configuration is created; the database is not opened.

Flags:`)
		fs.PrintDefaults()
	}
	output := fs.String("output", "", "configuration destination")
	db := fs.String("db", "", "database path")
	// flag accepts options before positional arguments; init also accepts NAME first.
	var flags, names []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			names = append(names, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			if arg == "--output" || arg == "-output" || arg == "--db" || arg == "-db" {
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
		} else {
			names = append(names, arg)
		}
	}
	if err := fs.Parse(flags); err != nil {
		if err == flag.ErrHelp {
			return 0, nil
		}
		return 2, nil
	}
	name := "default"
	if len(names) > 1 {
		fmt.Fprintln(errOut, "config init accepts at most one NAME")
		return 2, nil
	}
	if len(names) == 1 {
		name = names[0]
	}
	if !configNamePattern.MatchString(name) {
		fmt.Fprintln(errOut, "invalid config NAME (use letters, digits, underscore, or hyphen)")
		return 2, nil
	}
	destination := *output
	if destination == "" {
		var err error
		destination, err = configLocation(name)
		if err != nil {
			return 1, err
		}
	}
	// Resolve the parent, leaving the final component intact so exclusive creation
	// rejects every existing destination, including dangling symlinks.
	destination, err := absoluteConfigPath(destination)
	if err != nil {
		return 1, err
	}
	parent, err := core.CanonicalPath(filepath.Dir(destination), "")
	if err != nil {
		return 1, err
	}
	destination = filepath.Join(parent, filepath.Base(destination))
	dbPath := *db
	if dbPath == "" {
		dbPath = "~/.somniloq/" + name + ".db"
	}
	if dbPath == "" {
		fmt.Fprintln(errOut, "db must not be empty")
		return 2, nil
	}
	// Keep the final DB component intact so dangling symlinks are also refused.
	resolvedDB := dbPath
	if !filepath.IsAbs(resolvedDB) && resolvedDB != "~" && !strings.HasPrefix(resolvedDB, "~/") {
		resolvedDB = filepath.Join(parent, resolvedDB)
	}
	resolvedDB, err = absoluteConfigPath(resolvedDB)
	if err != nil {
		return 1, err
	}
	dbParent, err := core.CanonicalPath(filepath.Dir(resolvedDB), "")
	if err != nil {
		return 1, fmt.Errorf("resolve database directory: %w", err)
	}
	resolvedDB = filepath.Join(dbParent, filepath.Base(resolvedDB))
	if _, err := os.Lstat(resolvedDB); err == nil {
		fmt.Fprintf(errOut, "database destination already exists: %s\n", resolvedDB)
		return 2, nil
	} else if !os.IsNotExist(err) {
		return 1, fmt.Errorf("check database destination: %w", err)
	}
	boundary := "00:00"
	data, err := toml.Marshal(tomlConfig{
		DB: dbPath, DayBoundary: &boundary, ProjectAliases: map[string][]string{},
		Inputs: []tomlInput{
			{Name: "Claude", Source: "claude-code", Root: "~/.claude/projects"},
			{Name: "Codex", Source: "codex", Root: "~/.codex/sessions"},
			{Name: "Cursor", Source: "cursor-agent", Root: "~/.cursor/projects"},
		},
	})
	if err != nil {
		return 1, err
	}
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return 1, fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		fmt.Fprintf(errOut, "config destination already exists: %s\n", destination)
		return 2, nil
	}
	if err != nil {
		return 1, fmt.Errorf("create config: %w", err)
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	if writeErr != nil {
		return 1, fmt.Errorf("write config: %w", writeErr)
	}
	if closeErr != nil {
		return 1, fmt.Errorf("close config: %w", closeErr)
	}
	if _, err := fmt.Fprintln(out, destination); err != nil {
		return 1, err
	}
	return 0, nil
}
