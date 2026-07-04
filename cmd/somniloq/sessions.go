package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const sessionsHelpDetails = `Columns (TSV, in order):
  session_id: source-local session identifier.
  time_range: local started_at ~ ended_at; ended_at may be empty.
  logical_day: local YYYY-MM-DD after applying --day-boundary to ended_at, or started_at when ended_at is empty.
  project: canonical alias name when configured, otherwise repo_path or basename with --short.
  custom_title: raw session title, empty when unavailable.
  message_count: stored message rows, including sidechain rows.
  body_size: UTF-8 byte size of non-sidechain message bodies; use this to choose outline/show ranges.
  non_command_user_turn_count: user turns from outline numbering after excluding slash commands and config commandPatterns.
  first_non_command_user_line: first line of the first non-command user turn.

JSON fields:
  source, sessionId, project, title, startedAt, endedAt, logicalDay, messageCount, bodySize, nonCommandUserTurnCount, firstNonCommandUserLine

Notes:
  Date-only --since/--until values use --day-boundary or config dayBoundary. Relative times and datetimes do not.
  --project expands exact projectAliases matches, then filters repo_path by substring.

Examples:
  somniloq sessions --since 7d --short
  somniloq sessions --since 2026-03-28 --day-boundary 04:00 --format json
  somniloq sessions --project somniloq --since 30d`

// sessionsCmd runs the sessions subcommand without calling os.Exit, so it can
// be tested directly.
func sessionsCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	dayBoundaryFlag := fs.String("day-boundary", "", "logical day boundary for date filters and display (HH:MM, overrides config dayBoundary)")
	project := fs.String("project", "", "filter by repo path (substring match)")
	short := fs.Bool("short", false, "shorten unaliased projects to repo basename")
	format := fs.String("format", "tsv", "output format (tsv, json)")
	setUsage(fs, "List sessions", "somniloq sessions [flags]", sessionsHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if err := validateFormat(*format, "tsv", "json"); err != nil {
		return 1, err
	}
	matcher, err := newCommandMatcher(cfg)
	if err != nil {
		return 1, err
	}
	boundary, err := resolveDayBoundary(*dayBoundaryFlag, cfg)
	if err != nil {
		return 1, err
	}

	filter, err := buildSessionFilter(*since, *until, *project, cfg, boundary)
	if err != nil {
		return 1, err
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
	derived, err := deriveSessionUserTurnSummaries(db, rows, matcher)
	if err != nil {
		return 1, err
	}

	if *format == "json" {
		entries := make([]sessionJSON, len(rows))
		for i, r := range rows {
			entries[i] = newSessionJSON(r, resolveProjectDisplayName(r.RepoPath, *short, cfg), sessionLogicalDay(r, boundary, time.Local), derived[i])
		}
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
		return 0, nil
	}

	for i, r := range rows {
		title := sanitizeTSV(r.CustomTitle)
		proj := resolveProjectDisplayName(r.RepoPath, *short, cfg)
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%d\t%d\t%d\t%s\n",
			r.SessionID, formatTimeRange(r.StartedAt, r.EndedAt, time.Local), sessionLogicalDay(r, boundary, time.Local), proj, title, r.MessageCount, r.BodySize,
			derived[i].NonCommandUserTurnCount, sanitizeTSV(derived[i].FirstNonCommandUserLine))
	}
	return 0, nil
}

type sessionUserTurnSummary struct {
	NonCommandUserTurnCount int
	FirstNonCommandUserLine string
}

func deriveSessionUserTurnSummaries(db *core.DB, sessions []core.SessionRow, matcher commandMatcher) ([]sessionUserTurnSummary, error) {
	summaries := make([]sessionUserTurnSummary, len(sessions))
	for i, session := range sessions {
		messages, err := db.GetMessages(session.Source, session.SessionID)
		if err != nil {
			return nil, err
		}
		summaries[i] = summarizeNonCommandUserTurns(messages, matcher)
	}
	return summaries, nil
}

func summarizeNonCommandUserTurns(messages []core.MessageRow, matcher commandMatcher) sessionUserTurnSummary {
	var summary sessionUserTurnSummary
	for _, tm := range userTurnMessages(messages) {
		if matcher.isCommand(tm.Msg.Content) {
			continue
		}
		summary.NonCommandUserTurnCount++
		if summary.FirstNonCommandUserLine == "" {
			summary.FirstNonCommandUserLine = firstLine(tm.Msg.Content)
		}
	}
	return summary
}

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}
