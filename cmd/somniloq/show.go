package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const showUsageLine = "somniloq show --config default [--source <source>] [--turn <N|N..M>] [--tail <N>] [--summary <N>] [--exclude-user-message-pattern <regex>] [--no-exclude-user-messages] [--short] [--format <fmt>] <REF>\n" +
	"  somniloq show --config default [--since <time>] [--until <time>] [--project <name>] [--turn <N|N..M>] [--tail <N>] [--summary <N>] [--exclude-user-message-pattern <regex>] [--no-exclude-user-messages] [--short] [--format <fmt>]"

const showHelpDetails = `Output (markdown):
  One or more sessions. Each session has a title, Session, Source, Project, Started metadata, then message sections headed by role.
  Multiple sessions in time-range mode are separated by ---.

JSON fields:
  ref, source, sessionId, project, title, startedAt, endedAt, messages
  messages fields: role, content, timestamp

Notes:
  Flags must come before <REF>.
  Codex messages follow canonical owner order; inheritance context and unresolved records are excluded.
  --since/--until accept RFC3339 instants (for example, 2026-03-28T15:00:00Z or 2026-03-29T00:00:00+09:00); dates and minute datetimes are local.
  Unknown or invalid stored start times do not match time filters.
  Use either a full <REF> or --since/--until. --project only applies in time-range mode.
  --project expands exact projectAliases matches, then filters repo_path by literal substring (including %, _, and \).
  --source accepts claude_code|claude-code|codex|cursor_agent|cursor-agent with <REF>; it cannot be used with --since/--until.
  --summary N shows the first N user messages per session after applying command-line exclusion patterns.
  --exclude-user-message-pattern may be repeated; patterns are ORed.
  --no-exclude-user-messages disables user-message exclusions for this invocation. Either exclusion flag requires --summary >= 1.
  --turn N or --turn N..M shows inclusive turn ranges; --tail N shows the last N turns.
  --turn and --tail share outline numbering and cannot be combined with --summary.
  Only full slq1 references from sessions/search are accepted; --source restricts the reference source.

Examples:
  somniloq show --config default --summary 1 --since 24h --short
  somniloq show --config default --summary 1 --exclude-user-message-pattern '^<command-name>/clear</command-name>' --since 24h
  somniloq show --config default --turn 40..60 <REF>
  somniloq show --config default --source codex <REF>
  somniloq show --config default --format json --tail 3 <REF>`

// showCmd runs the show subcommand without calling os.Exit, so it can be
// tested directly.
func showCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, flags := newShowFlagSet()
	setUsage(fs, "Show session content in Markdown", showUsageLine, showHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if *flags.summary < 0 {
		return 1, errors.New("--summary must be >= 0")
	}
	exclusionFlagsSet := flagWasProvided(fs, "exclude-user-message-pattern") || flagWasProvided(fs, "no-exclude-user-messages")
	if exclusionFlagsSet && *flags.summary < 1 {
		return 1, errors.New("user-message exclusion flags require --summary >= 1")
	}
	matcher, err := newUserMessageMatcher(cfg, *flags.excludePatterns, *flags.noExclusions)
	if err != nil {
		return 1, err
	}
	if *flags.tail < 0 {
		return 1, errors.New("--tail must be >= 0")
	}
	// Detect --turn via Visit so an explicit empty value (e.g. an unset shell
	// variable) is rejected by parseTurnRange instead of silently showing the
	// whole session.
	turnSet := flagWasProvided(fs, "turn")
	if turnSet && *flags.tail > 0 {
		return 1, errors.New("specify either --turn or --tail, not both")
	}
	turnFiltered := turnSet || *flags.tail > 0
	if turnFiltered && *flags.summary > 0 {
		return 1, errors.New("--turn/--tail cannot be combined with --summary")
	}
	var turnLo, turnHi int
	if turnSet {
		var err error
		turnLo, turnHi, err = parseTurnRange(*flags.turnRange)
		if err != nil {
			return 1, err
		}
	}

	if err := validateFormat(*flags.format, "markdown", "json"); err != nil {
		return 1, err
	}

	showUsage := "usage: " + showUsageLine

	if fs.NArg() > 1 {
		writeUsageError(errOut, "too many arguments")
		fmt.Fprintln(errOut, showUsage)
		return 1, nil
	}

	sessionID := fs.Arg(0)
	sourceSet := flagWasProvided(fs, "source")
	var source *core.Source
	if sourceSet {
		parsed, err := parseSessionSource(*flags.source)
		if err != nil {
			return 1, err
		}
		source = &parsed
	}

	if sessionID != "" && (*flags.since != "" || *flags.until != "") {
		return 1, errors.New("specify either REF or --since/--until, not both")
	}
	if source != nil && sessionID == "" {
		return 1, errors.New("--source requires REF and cannot be combined with --since/--until")
	}
	if sessionID == "" && *flags.since == "" && *flags.until == "" {
		fmt.Fprintln(errOut, showUsage)
		return 1, nil
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	var sessions []core.SessionRow
	if sessionID != "" {
		session, code, err := resolveSessionREF(db, sessionID, source, errOut)
		if code != 0 {
			return code, err
		}
		sessions = []core.SessionRow{session}
	} else {
		filter, err := buildSessionFilter(*flags.since, *flags.until, *flags.project, cfg, dayBoundary{})
		if err != nil {
			return 1, err
		}
		sessions, err = db.ListSessions(filter)
		if err != nil {
			return 1, err
		}
	}

	var entries []showSessionJSON
	if *flags.format == "json" {
		entries = make([]showSessionJSON, 0, len(sessions))
	}
	for i, session := range sessions {
		if *flags.format == "markdown" && i > 0 {
			if _, err := fmt.Fprint(out, "\n---\n\n"); err != nil {
				return 1, err
			}
		}

		var messages []core.MessageRow
		if *flags.summary >= 1 {
			messages, err = db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
			if err == nil {
				messages = filterSummaryMessages(messages, *flags.summary, matcher)
			}
		} else {
			messages, err = db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
			if err == nil && turnFiltered {
				// Turn filtering must run on the full GetMessages output so the
				// numbers match outline (see assignTurns).
				if *flags.tail > 0 {
					messages = filterLastTurns(messages, *flags.tail)
				} else {
					messages = filterTurns(messages, turnLo, turnHi)
				}
			}
		}
		if err != nil {
			return 1, err
		}

		project := resolveProjectDisplayName(session.RepoPath, *flags.short, cfg)
		if *flags.format == "json" {
			entries = append(entries, newShowSessionJSON(session, project, messages))
			continue
		}
		if err := formatSession(out, session, project, messages, time.Local); err != nil {
			return 1, err
		}
	}
	if *flags.format == "json" {
		// Always an array, so consumers parse single-session and time-range
		// output the same way.
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
	}
	return 0, nil
}

type showFlags struct {
	since, until, project, turnRange, format, source *string
	short, noExclusions                              *bool
	summary, tail                                    *int
	excludePatterns                                  *stringListFlag
}

func newShowFlagSet() (*flag.FlagSet, showFlags) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	var excludePatterns stringListFlag
	fs.Var(&excludePatterns, "exclude-user-message-pattern", "exclude matching user messages from --summary (repeatable; OR)")
	return fs, showFlags{
		since:           fs.String("since", "", "filter by start time (relative, local date/datetime, or RFC3339 instant)"),
		until:           fs.String("until", "", "filter sessions started before a relative, local date/datetime, or RFC3339 instant"),
		project:         fs.String("project", "", "filter by repo path (literal substring match)"),
		short:           fs.Bool("short", false, "shorten unaliased project to repo basename"),
		summary:         fs.Int("summary", 0, "show first N user messages after exclusions (0 disables)"),
		noExclusions:    fs.Bool("no-exclude-user-messages", false, "disable user-message exclusions for this --summary invocation"),
		turnRange:       fs.String("turn", "", "show only turn N or turns N..M (numbers match outline)"),
		tail:            fs.Int("tail", 0, "show only the last N turns (0 disables)"),
		format:          fs.String("format", "markdown", "output format (markdown, json)"),
		source:          fs.String("source", "", "restrict REF source (claude_code, claude-code, codex, cursor_agent, cursor-agent)"),
		excludePatterns: &excludePatterns,
	}
}

func filterSummaryMessages(messages []core.MessageRow, limit int, matcher userMessageMatcher) []core.MessageRow {
	capacity := min(limit, len(messages))
	filtered := make([]core.MessageRow, 0, capacity)
	for _, message := range messages {
		if message.Role != "user" || matcher.excludes(message.Content) {
			continue
		}
		filtered = append(filtered, message)
		if len(filtered) == limit {
			break
		}
	}
	return filtered
}
