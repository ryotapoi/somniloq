package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const outlineUsageLine = "somniloq outline <session-id>"

// outlineCmd runs the outline subcommand without calling os.Exit, so it can
// be tested directly.
func outlineCmd(args []string, openDB func() (*core.DB, error), out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("outline", flag.ContinueOnError)
	setUsage(fs, "List a session's user messages as turn number, time, and first line", outlineUsageLine)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
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

	for _, tm := range assignTurns(messages) {
		if tm.Msg.Role != "user" {
			continue
		}
		fmt.Fprintf(out, "%d\t%s\t%s\n",
			tm.Turn, sanitizeTSV(formatLocalTime(tm.Msg.Timestamp, time.Local)), sanitizeTSV(firstLine(tm.Msg.Content)))
	}
	return 0, nil
}

// firstLine returns the first line of the content after trimming surrounding
// whitespace, so leading blank lines do not produce an empty outline entry.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimRight(line, "\r")
}
