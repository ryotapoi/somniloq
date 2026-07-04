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
	setUsage(fs, "Show session content in Markdown", showUsageLine)
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
	filter, err := buildSessionFilter(*since, *until, *project, cfg)
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

// resolveSessionByID looks up sessionID across sources and reduces the result
// to a single session. On failure it returns exit code 1, reporting an
// ambiguous match to errOut directly and a lookup failure via the returned
// error (matching how main prints command errors).
func resolveSessionByID(db *core.DB, sessionID string, errOut io.Writer) (core.SessionRow, int, error) {
	sessions, err := db.LookupSessionsByID(sessionID)
	if err != nil {
		return core.SessionRow{}, 1, err
	}
	if len(sessions) == 0 {
		return core.SessionRow{}, 1, fmt.Errorf("session not found: %s", sessionID)
	}
	if len(sessions) > 1 {
		writeAmbiguousSessionError(errOut, sessionID, sessions)
		return core.SessionRow{}, 1, nil
	}
	return sessions[0], 0, nil
}

func writeAmbiguousSessionError(w io.Writer, sessionID string, sessions []core.SessionRow) {
	fmt.Fprintf(w, "error: session id %q is ambiguous; matched multiple sources:\n", sessionID)
	for _, session := range sessions {
		fmt.Fprintf(w, "  %s\t%s\n", session.Source, session.SessionID)
	}
}
