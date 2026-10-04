package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const outlineUsageLine = "somniloq outline --config default [--source <source>] [--exclude-user-message-pattern <regex>] [--no-exclude-user-messages] [--format <fmt>] <REF>"

const outlineHelpDetails = `Columns (TSV, in order):
  turn: 1-based user turn number shared with show --turn and search results.
  time: local timestamp of the user message.
  body_size: UTF-8 byte size of all non-sidechain message bodies in that turn.
  first_line: first non-empty line of the user message, with tabs/newlines flattened for TSV.

JSON fields:
  turn, timestamp, bodySize, firstLine

Notes:
  A turn is a user message plus following non-user messages until the next user message.
  Sidechain messages are excluded. User-message exclusions do not change turn numbers, so numbering stays aligned with show --turn.
  --exclude-user-message-pattern may be repeated; patterns are ORed.
  --no-exclude-user-messages disables user-message exclusions for this invocation and cannot be combined with pattern flags.
  Recommended long-session flow: outline -> choose turn numbers -> show --turn N..M <REF>.
  --source accepts claude_code|claude-code|codex|cursor_agent|cursor-agent to restrict the full REF source.

Examples:
  somniloq outline --config default <REF>
  somniloq outline --config default --source cursor_agent <REF>
  somniloq outline --config default --exclude-user-message-pattern '^/clear' <REF>
  somniloq outline --config default --format json <REF>
  somniloq show --config default --turn 12..18 <REF>`

// outlineCmd runs the outline subcommand without calling os.Exit, so it can
// be tested directly.
func outlineCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, flags := newOutlineFlagSet()
	setUsage(fs, "List a session's user messages as turn number, time, body size, and first line", outlineUsageLine, outlineHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if err := validateFormat(*flags.format, "tsv", "json"); err != nil {
		return 1, err
	}
	matcher, err := newUserMessageMatcher(cfg, *flags.excludePatterns, *flags.noExclusions)
	if err != nil {
		return 1, err
	}

	outlineUsage := "usage: " + outlineUsageLine

	if fs.NArg() > 1 {
		writeUsageError(errOut, "too many arguments")
		fmt.Fprintln(errOut, outlineUsage)
		return 1, nil
	}
	sessionID := fs.Arg(0)
	if sessionID == "" {
		fmt.Fprintln(errOut, outlineUsage)
		return 1, nil
	}
	sourceSet := flagWasProvided(fs, "source")
	var source *core.Source
	if sourceSet {
		parsed, err := parseSessionSource(*flags.source)
		if err != nil {
			return 1, err
		}
		source = &parsed
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	session, code, err := resolveSessionREF(db, sessionID, source)
	if code != 0 {
		return code, err
	}

	messages, err := db.GetMessages(session.InputID, session.Source, session.SessionID)
	if err != nil {
		return 1, err
	}

	turns := assignTurns(messages)
	users := userTurnMessages(turns)
	bodySizes := turnBodySizes(turns)
	visibleUsers := make([]turnMessage, 0, len(users))
	for _, tm := range users {
		if !matcher.excludes(tm.Msg.Content) {
			visibleUsers = append(visibleUsers, tm)
		}
	}
	if *flags.format == "json" {
		entries := make([]outlineEntryJSON, 0, len(visibleUsers))
		for _, tm := range visibleUsers {
			entries = append(entries, outlineEntryJSON{
				Turn:      tm.Turn,
				Timestamp: tm.Msg.Timestamp,
				BodySize:  bodySizes[tm.Turn],
				FirstLine: firstLine(tm.Msg.Content),
			})
		}
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
		return 0, nil
	}

	for _, tm := range visibleUsers {
		if _, err := fmt.Fprintf(out, "%d\t%s\t%d\t%s\n",
			tm.Turn, sanitizeTSV(formatLocalTime(tm.Msg.Timestamp, time.Local)), bodySizes[tm.Turn], sanitizeTSV(firstLine(tm.Msg.Content))); err != nil {
			return 1, err
		}
	}
	return 0, nil
}

type outlineFlags struct {
	format, source  *string
	noExclusions    *bool
	excludePatterns *stringListFlag
}

func newOutlineFlagSet() (*flag.FlagSet, outlineFlags) {
	fs := flag.NewFlagSet("outline", flag.ContinueOnError)
	var excludePatterns stringListFlag
	flags := outlineFlags{
		format:          fs.String("format", "tsv", "output format (tsv, json)"),
		source:          fs.String("source", "", "restrict REF source (claude_code, claude-code, codex, cursor_agent, cursor-agent)"),
		noExclusions:    fs.Bool("no-exclude-user-messages", false, "disable user-message exclusions for this invocation"),
		excludePatterns: &excludePatterns,
	}
	fs.Var(&excludePatterns, "exclude-user-message-pattern", "exclude matching user messages (repeatable; OR)")
	return fs, flags
}
