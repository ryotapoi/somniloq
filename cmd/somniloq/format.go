package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

var titleSanitizer = strings.NewReplacer("\n", " ", "\r", " ")

func formatSession(w io.Writer, session core.SessionRow, messages []core.MessageRow) {
	title := session.CustomTitle
	if title == "" {
		title = session.SessionID
	}
	title = titleSanitizer.Replace(title)

	fmt.Fprintf(w, "## %s\n\n", title)
	fmt.Fprintf(w, "- **Session**: `%s`\n", session.SessionID)
	fmt.Fprintf(w, "- **Project**: `%s`\n", session.ProjectDir)
	fmt.Fprintf(w, "- **Started**: `%s`\n", session.StartedAt)

	for _, msg := range messages {
		if msg.IsSidechain {
			continue
		}
		heading := msg.Role
		if len(heading) > 0 {
			heading = strings.ToUpper(heading[:1]) + heading[1:]
		}
		fmt.Fprintf(w, "\n### %s\n\n%s\n", heading, msg.Content)
	}
}

func formatSessions(w io.Writer, sessions []core.SessionRow, getMessages func(sessionID string) ([]core.MessageRow, error)) error {
	for i, session := range sessions {
		if i > 0 {
			fmt.Fprint(w, "\n---\n\n")
		}
		msgs, err := getMessages(session.SessionID)
		if err != nil {
			return err
		}
		formatSession(w, session, msgs)
	}
	return nil
}
