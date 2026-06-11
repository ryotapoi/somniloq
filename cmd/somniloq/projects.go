package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

// projectsCmd runs the projects subcommand without calling os.Exit, so it can
// be tested directly.
func projectsCmd(args []string, openDB func() (*core.DB, error), out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	short := fs.Bool("short", false, "shorten project names to repo basename")
	format := fs.String("format", "tsv", "output format (tsv, json)")
	setUsage(fs, "List projects", "somniloq projects [flags]")
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if err := validateFormat(*format, "tsv", "json"); err != nil {
		return 1, err
	}

	if fs.NArg() != 0 {
		fmt.Fprintln(errOut, "error: unexpected arguments")
		fmt.Fprintln(errOut, "usage: somniloq projects [--since <time>] [--until <time>] [--short] [--format <fmt>]")
		return 1, nil
	}

	filter, err := buildSessionFilter(*since, *until, "", config{})
	if err != nil {
		return 1, err
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	rows, err := db.ListProjects(filter)
	if err != nil {
		return 1, err
	}

	if *format == "json" {
		entries := make([]projectJSON, len(rows))
		for i, r := range rows {
			entries[i] = projectJSON{Project: resolveDisplayName(r.RepoPath, *short), SessionCount: r.SessionCount}
		}
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
		return 0, nil
	}

	for _, r := range rows {
		name := resolveDisplayName(r.RepoPath, *short)
		fmt.Fprintf(out, "%s\t%d\n", name, r.SessionCount)
	}
	return 0, nil
}
