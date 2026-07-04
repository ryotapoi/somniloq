package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func formatLocalTime(utcStr string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339Nano, utcStr)
	if err != nil {
		return utcStr
	}
	return t.In(loc).Format("2006-01-02 15:04")
}

func formatTimeRange(startedAt, endedAt string, loc *time.Location) string {
	s := formatLocalTime(startedAt, loc)
	if endedAt == "" {
		return s + " ~"
	}
	return s + " ~ " + formatLocalTime(endedAt, loc)
}

var titleSanitizer = strings.NewReplacer("\n", " ", "\r", " ")

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}

func formatSession(w io.Writer, session core.SessionRow, displayName string, messages []core.MessageRow, loc *time.Location) {
	title := session.CustomTitle
	if title == "" {
		title = session.SessionID
	}
	title = titleSanitizer.Replace(title)

	fmt.Fprintf(w, "## %s\n\n", title)
	fmt.Fprintf(w, "- **Session**: `%s`\n", session.SessionID)
	fmt.Fprintf(w, "- **Project**: `%s`\n", displayName)
	fmt.Fprintf(w, "- **Started**: `%s`\n", formatTimeRange(session.StartedAt, session.EndedAt, loc))

	for _, msg := range messages {
		heading := msg.Role
		if len(heading) > 0 {
			heading = strings.ToUpper(heading[:1]) + heading[1:]
		}
		fmt.Fprintf(w, "\n### %s\n\n%s\n", heading, msg.Content)
	}
}

// formatSessions requires len(displayNames) == len(sessions).
func formatSessions(w io.Writer, sessions []core.SessionRow, displayNames []string, getMessages func(source core.Source, sessionID string) ([]core.MessageRow, error), loc *time.Location) error {
	for i, session := range sessions {
		if i > 0 {
			fmt.Fprint(w, "\n---\n\n")
		}
		msgs, err := getMessages(session.Source, session.SessionID)
		if err != nil {
			return err
		}
		formatSession(w, session, displayNames[i], msgs, loc)
	}
	return nil
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
