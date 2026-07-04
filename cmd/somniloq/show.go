package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const showUsageLine = "somniloq show [--turn <N|N..M>] [--tail <N>] [--summary <N>] [--include-clear] [--short] [--format <fmt>] <session-id>\n" +
	"  somniloq show [--since <time>] [--until <time>] [--project <name>] [--turn <N|N..M>] [--tail <N>] [--summary <N>] [--include-clear] [--short] [--format <fmt>]"

const showHelpDetails = `Output (markdown):
  One or more sessions. Each session has a title, Session, Project, Started metadata, then message sections headed by role.
  Multiple sessions in time-range mode are separated by ---.

JSON fields:
  source, sessionId, project, title, startedAt, endedAt, messages
  messages fields: role, content, timestamp

Notes:
  Flags must come before <session-id>.
  Use either <session-id> or --since/--until. --project only applies in time-range mode.
  --summary N shows first N user messages per session, skipping /clear and local-command-caveat unless --include-clear is set.
  --turn N or --turn N..M shows inclusive turn ranges; --tail N shows the last N turns.
  --turn and --tail share outline numbering and cannot be combined with --summary.
  If a session_id exists in multiple sources, show prints an ambiguity error with source/session candidates.

Examples:
  somniloq show --summary 1 --since 24h --short
  somniloq show --turn 40..60 <session-id>
  somniloq show --format json --tail 3 <session-id>`

// showCmd runs the show subcommand without calling os.Exit, so it can be
// tested directly.
func showCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	project := fs.String("project", "", "filter by repo path (substring match)")
	short := fs.Bool("short", false, "shorten unaliased project to repo basename")
	summary := fs.Int("summary", 0, "show first N user messages skipping /clear and local-command-caveat (0 disables)")
	includeClear := fs.Bool("include-clear", false, "keep /clear and local-command-caveat messages in --summary output (requires --summary >= 1)")
	turnRange := fs.String("turn", "", "show only turn N or turns N..M (numbers match outline)")
	tail := fs.Int("tail", 0, "show only the last N turns (0 disables)")
	format := fs.String("format", "markdown", "output format (markdown, json)")
	setUsage(fs, "Show session content in Markdown", showUsageLine, showHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if *summary < 0 {
		return 1, errors.New("--summary must be >= 0")
	}
	if *includeClear && *summary == 0 {
		return 1, errors.New("--include-clear requires --summary >= 1")
	}
	if *tail < 0 {
		return 1, errors.New("--tail must be >= 0")
	}
	// Detect --turn via Visit so an explicit empty value (e.g. an unset shell
	// variable) is rejected by parseTurnRange instead of silently showing the
	// whole session.
	turnSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "turn" {
			turnSet = true
		}
	})
	if turnSet && *tail > 0 {
		return 1, errors.New("specify either --turn or --tail, not both")
	}
	turnFiltered := turnSet || *tail > 0
	if turnFiltered && *summary > 0 {
		return 1, errors.New("--turn/--tail cannot be combined with --summary")
	}
	var turnLo, turnHi int
	if turnSet {
		var err error
		turnLo, turnHi, err = parseTurnRange(*turnRange)
		if err != nil {
			return 1, err
		}
	}

	if err := validateFormat(*format, "markdown", "json"); err != nil {
		return 1, err
	}

	showUsage := "usage: " + showUsageLine

	if fs.NArg() > 1 {
		fmt.Fprintln(errOut, "error: too many arguments")
		fmt.Fprintln(errOut, showUsage)
		return 1, nil
	}

	sessionID := fs.Arg(0)

	if sessionID != "" && (*since != "" || *until != "") {
		return 1, errors.New("specify either session-id or --since/--until, not both")
	}
	if sessionID == "" && *since == "" && *until == "" {
		fmt.Fprintln(errOut, showUsage)
		return 1, nil
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	getMessages := func(src core.Source, id string) ([]core.MessageRow, error) {
		return db.GetMessages(src, id)
	}
	if *summary >= 1 {
		n, ic := *summary, *includeClear
		getMessages = func(src core.Source, id string) ([]core.MessageRow, error) {
			return db.GetSummaryMessages(src, id, n, ic)
		}
	}
	if turnFiltered {
		// Turn filtering must run on the full GetMessages output so the
		// numbers match outline (see assignTurns).
		tailN := *tail
		getMessages = func(src core.Source, id string) ([]core.MessageRow, error) {
			msgs, err := db.GetMessages(src, id)
			if err != nil {
				return nil, err
			}
			if tailN > 0 {
				return filterLastTurns(msgs, tailN), nil
			}
			return filterTurns(msgs, turnLo, turnHi), nil
		}
	}

	if sessionID != "" {
		session, code, err := resolveSessionByID(db, sessionID, errOut)
		if code != 0 {
			return code, err
		}
		proj := resolveProjectDisplayName(session.RepoPath, *short, cfg)
		messages, err := getMessages(session.Source, session.SessionID)
		if err != nil {
			return 1, err
		}
		if *format == "json" {
			// Always an array, so consumers parse single-session and
			// time-range output the same way.
			if err := writeJSON(out, []showSessionJSON{newShowSessionJSON(session, proj, messages)}); err != nil {
				return 1, err
			}
			return 0, nil
		}
		formatSession(out, session, proj, messages, time.Local)
		return 0, nil
	}

	// --since/--until mode
	filter, err := buildSessionFilter(*since, *until, *project, cfg, dayBoundary{})
	if err != nil {
		return 1, err
	}

	sessions, err := db.ListSessions(filter)
	if err != nil {
		return 1, err
	}
	if *format == "json" {
		entries := make([]showSessionJSON, len(sessions))
		for i, session := range sessions {
			messages, err := getMessages(session.Source, session.SessionID)
			if err != nil {
				return 1, err
			}
			entries[i] = newShowSessionJSON(session, resolveProjectDisplayName(session.RepoPath, *short, cfg), messages)
		}
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
		return 0, nil
	}
	if len(sessions) == 0 {
		return 0, nil
	}

	displayNames := make([]string, len(sessions))
	for i := range sessions {
		displayNames[i] = resolveProjectDisplayName(sessions[i].RepoPath, *short, cfg)
	}

	if err := formatSessions(out, sessions, displayNames, getMessages, time.Local); err != nil {
		return 1, err
	}
	return 0, nil
}
