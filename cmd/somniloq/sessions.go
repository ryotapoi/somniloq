package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

// sessionsCmd runs the sessions subcommand without calling os.Exit, so it can
// be tested directly.
func sessionsCmd(args []string, openDB func() (*core.DB, error), out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	project := fs.String("project", "", "filter by repo path (substring match)")
	short := fs.Bool("short", false, "shorten project to repo basename")
	setUsage(fs, "List sessions", "somniloq sessions [flags]")
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	filter, err := buildSessionFilter(*since, *until, *project)
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

	for _, r := range rows {
		title := sanitizeTSV(r.CustomTitle)
		proj := resolveDisplayName(r.RepoPath, *short)
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%d\n",
			r.SessionID, formatTimeRange(r.StartedAt, r.EndedAt, time.Local), proj, title, r.MessageCount)
	}
	return 0, nil
}

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}
