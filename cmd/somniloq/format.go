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

// firstLine returns the first line of the content after trimming surrounding
// whitespace, so leading blank lines do not produce an empty outline entry.
func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimRight(line, "\r")
}

func formatSession(w io.Writer, session core.SessionRow, displayName string, messages []core.MessageRow, loc *time.Location) error {
	title := session.CustomTitle
	if title == "" {
		title = session.SessionID
	}
	title = titleSanitizer.Replace(title)

	if _, err := fmt.Fprintf(w, "## %s\n\n", title); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- **Session**: `%s`\n", session.SessionID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- **Project**: `%s`\n", displayName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- **Started**: `%s`\n", formatTimeRange(session.StartedAt, session.EndedAt, loc)); err != nil {
		return err
	}

	for _, msg := range messages {
		heading := msg.Role
		if len(heading) > 0 {
			heading = strings.ToUpper(heading[:1]) + heading[1:]
		}
		if _, err := fmt.Fprintf(w, "\n### %s\n\n%s\n", heading, msg.Content); err != nil {
			return err
		}
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
	writeUsageError(w, fmt.Sprintf("session id %q is ambiguous; matched multiple sources:", sessionID))
	for _, session := range sessions {
		fmt.Fprintf(w, "  %s\t%s\n", session.Source, session.SessionID)
	}
}

func writeUsageError(w io.Writer, message string) {
	fmt.Fprintf(w, "error: %s\n", message)
}
