package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const sessionsHelpDetails = `Columns (TSV, in order):
  ref: full slq1 reference for this input and session.
  time_range: local started_at ~ ended_at; ended_at may be empty; tabs/newlines flattened.
  logical_day: local YYYY-MM-DD using the calendar day's boundary and ended_at, or started_at when ended_at is empty.
  project: canonical alias name when configured, otherwise repo_path or basename with --short; tabs/newlines flattened.
  custom_title: session title with tabs/newlines flattened, empty when unavailable.
  message_count: stored message rows, including sidechain rows.
  body_size: UTF-8 byte size of non-sidechain message bodies; use this to choose outline/show ranges.
  source: internal source identifier: claude_code, codex, or cursor_agent.

JSON fields:
  ref, source, sessionId, project, title, startedAt, endedAt, logicalDay, messageCount, bodySize

Notes:
  --since/--until and --imported-since accept RFC3339 instants (for example, 2026-03-28T15:00:00Z or 2026-03-29T00:00:00+09:00); dates and minute datetimes are local.
  Date-only --since/--until values use --day-boundary or config dayBoundary. Relative times and datetimes do not.
  Date-only boundaries follow local calendar days across daylight saving time changes.
  --imported-since filters sessions imported at or after a time; date-only values start at local midnight and ignore --day-boundary.
  Unknown or invalid stored start times do not match time filters.
  Use the resulting REF with somniloq show --config default <REF> to read the session.
  --project expands exact projectAliases matches, then filters repo_path by literal substring (including %, _, and \).

Examples:
  somniloq sessions --config default --since 7d --short
  somniloq sessions --config default --since 2026-03-28 --day-boundary 04:00 --format json
  somniloq sessions --config default --project somniloq --since 30d`

// sessionsCmd runs the sessions subcommand without calling os.Exit, so it can
// be tested directly.
func sessionsCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	return sessionsCmdAt(time.Now().UTC(), args, openDB, cfg, out, errOut)
}

// sessionsCmdAt runs the sessions subcommand using the supplied current time.
func sessionsCmdAt(now time.Time, args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, flags := newSessionsFlagSet()
	setUsage(fs, "List sessions", "somniloq sessions --config default [flags]", sessionsHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}
	if fs.NArg() != 0 {
		writeUsageError(errOut, "unexpected arguments")
		fmt.Fprintln(errOut, "usage: somniloq sessions --config default [flags]")
		return 1, nil
	}
	if err := validateFormat(*flags.format, "tsv", "json"); err != nil {
		return 1, err
	}
	boundary, err := resolveDayBoundary(*flags.dayBoundary, cfg)
	if err != nil {
		return 1, err
	}

	filter, err := buildSessionFilterAt(now, *flags.since, *flags.until, *flags.project, cfg, boundary)
	if err != nil {
		return 1, err
	}
	if flagWasProvided(fs, "imported-since") {
		importedSince, err := resolveImportedSince(*flags.importedSince, now, time.Local)
		if err != nil {
			return 1, err
		}
		filter.ImportedSince = importedSince
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	rows, err := db.ListSessions(filter)
	if err != nil {
		return 1, err
	}
	if *flags.format == "json" {
		entries := make([]sessionJSON, len(rows))
		for i, r := range rows {
			entries[i] = newSessionJSON(r, resolveProjectDisplayName(r.RepoPath, *flags.short, cfg), sessionLogicalDay(r, boundary, time.Local))
		}
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
		return 0, nil
	}

	for _, r := range rows {
		title := sanitizeTSV(r.CustomTitle)
		proj := sanitizeTSV(resolveProjectDisplayName(r.RepoPath, *flags.short, cfg))
		if _, err := fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%s\n",
			r.REF, sanitizeTSV(formatTimeRange(r.StartedAt, r.EndedAt, time.Local)), sessionLogicalDay(r, boundary, time.Local), proj, title, r.MessageCount, r.BodySize,
			r.Source); err != nil {
			return 1, err
		}
	}
	return 0, nil
}

type sessionsFlags struct {
	since, until, importedSince, dayBoundary, project, format *string
	short                                                     *bool
}

func newSessionsFlagSet() (*flag.FlagSet, sessionsFlags) {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	flags := sessionsFlags{
		since:         fs.String("since", "", "filter by start time (relative, local date/datetime, or RFC3339 instant)"),
		until:         fs.String("until", "", "filter sessions started before a relative, local date/datetime, or RFC3339 instant"),
		importedSince: fs.String("imported-since", "", "filter by import time (relative, local date/datetime, or RFC3339 instant)"),
		dayBoundary:   fs.String("day-boundary", "", "logical day boundary for date filters and display (HH:MM, overrides config dayBoundary)"),
		project:       fs.String("project", "", "filter by repo path (literal substring match)"),
		short:         fs.Bool("short", false, "shorten unaliased projects to repo basename"),
		format:        fs.String("format", "tsv", "output format (tsv, json)"),
	}
	return fs, flags
}
