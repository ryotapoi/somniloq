package main

import (
	"flag"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

const projectsHelpDetails = `Columns (TSV, in order):
  project: canonical alias name when configured, otherwise repo_path or basename with --short.
  session_count: number of sessions for that project.

JSON fields:
  project, sessionCount

Notes:
  Projects are grouped by repo_path in SQL, then alias-equivalent rows are merged for display.
  --since/--until filter session start time. Date-only filters use local 00:00; dayBoundary does not apply to projects.
  --short only affects projects that do not match projectAliases.

Examples:
  somniloq projects --since 30d --short
  somniloq projects --format json
  somniloq sessions --project somniloq --since 7d`

// projectsCmd runs the projects subcommand without calling os.Exit, so it can
// be tested directly.
func projectsCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, flags := newProjectsFlagSet()
	setUsage(fs, "List projects", "somniloq projects [flags]", projectsHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if err := validateFormat(*flags.format, "tsv", "json"); err != nil {
		return 1, err
	}

	if fs.NArg() != 0 {
		writeUsageError(errOut, "unexpected arguments")
		fmt.Fprintln(errOut, "usage: somniloq projects [--since <time>] [--until <time>] [--short] [--format <fmt>]")
		return 1, nil
	}

	filter, err := buildSessionFilter(*flags.since, *flags.until, "", config{}, dayBoundary{})
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
	displayRows := projectDisplayRows(rows, *flags.short, cfg)

	if *flags.format == "json" {
		entries := make([]projectJSON, len(displayRows))
		for i, r := range displayRows {
			entries[i] = projectJSON{Project: r.Project, SessionCount: r.SessionCount}
		}
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
		return 0, nil
	}

	for _, r := range displayRows {
		fmt.Fprintf(out, "%s\t%d\n", r.Project, r.SessionCount)
	}
	return 0, nil
}

type projectsFlags struct {
	since, until, format *string
	short                *bool
}

func newProjectsFlagSet() (*flag.FlagSet, projectsFlags) {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	return fs, projectsFlags{
		since:  fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time"),
		until:  fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time"),
		short:  fs.Bool("short", false, "shorten unaliased projects to repo basename"),
		format: fs.String("format", "tsv", "output format (tsv, json)"),
	}
}

type projectDisplayRow struct {
	Project      string
	SessionCount int
}

func projectDisplayRows(rows []core.ProjectRow, short bool, cfg config) []projectDisplayRow {
	result := []projectDisplayRow{}
	indexByKey := map[string]int{}
	for _, row := range rows {
		project := resolveDisplayName(row.RepoPath, short)
		key := "raw:" + row.RepoPath
		if canonical, ok := cfg.canonicalProjectName(row.RepoPath); ok {
			project = canonical
			key = "alias:" + canonical
		}
		if idx, ok := indexByKey[key]; ok {
			result[idx].SessionCount += row.SessionCount
			continue
		}
		indexByKey[key] = len(result)
		result = append(result, projectDisplayRow{Project: project, SessionCount: row.SessionCount})
	}
	return result
}
