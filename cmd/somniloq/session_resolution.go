package main

import (
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

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
