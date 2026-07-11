package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const outlineUsageLine = "somniloq outline [--format <fmt>] <session-id>"

const outlineHelpDetails = `Columns (TSV, in order):
  turn: 1-based user turn number shared with show --turn and search results.
  time: local timestamp of the user message.
  body_size: UTF-8 byte size of all non-sidechain message bodies in that turn.
  first_line: first non-empty line of the user message, with tabs/newlines flattened for TSV.

JSON fields:
  turn, timestamp, bodySize, firstLine

Notes:
  A turn is a user message plus following non-user messages until the next user message.
  Sidechain messages are excluded. Synthetic user messages such as /clear still count, so numbering stays aligned with show --turn.
  Recommended long-session flow: outline -> choose turn numbers -> show --turn N..M <session-id>.

Examples:
  somniloq outline <session-id>
  somniloq outline --format json <session-id>
  somniloq show --turn 12..18 <session-id>`

// outlineCmd runs the outline subcommand without calling os.Exit, so it can
// be tested directly.
func outlineCmd(args []string, openDB func() (*core.DB, error), out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("outline", flag.ContinueOnError)
	format := fs.String("format", "tsv", "output format (tsv, json)")
	setUsage(fs, "List a session's user messages as turn number, time, body size, and first line", outlineUsageLine, outlineHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	if err := validateFormat(*format, "tsv", "json"); err != nil {
		return 1, err
	}

	outlineUsage := "usage: " + outlineUsageLine

	if fs.NArg() > 1 {
		fmt.Fprintln(errOut, "error: too many arguments")
		fmt.Fprintln(errOut, outlineUsage)
		return 1, nil
	}
	sessionID := fs.Arg(0)
	if sessionID == "" {
		fmt.Fprintln(errOut, outlineUsage)
		return 1, nil
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	session, code, err := resolveSessionByID(db, sessionID, errOut)
	if code != 0 {
		return code, err
	}

	messages, err := db.GetMessages(session.Source, session.SessionID)
	if err != nil {
		return 1, err
	}

	if *format == "json" {
		entries := []outlineEntryJSON{}
		bodySizes := turnBodySizes(messages)
		for _, tm := range userTurnMessages(messages) {
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

	bodySizes := turnBodySizes(messages)
	for _, tm := range userTurnMessages(messages) {
		fmt.Fprintf(out, "%d\t%s\t%d\t%s\n",
			tm.Turn, sanitizeTSV(formatLocalTime(tm.Msg.Timestamp, time.Local)), bodySizes[tm.Turn], sanitizeTSV(firstLine(tm.Msg.Content)))
	}
	return 0, nil
}
