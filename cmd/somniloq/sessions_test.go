package main

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSessionsCmd_OutputColumns(t *testing.T) {
	db := newOutlineTestDB(t)

	// The CLI must print exactly what ListSessions returns; the value
	// semantics (bytes, sidechain exclusion) are pinned by core tests.
	// Read before sessionsCmd, which closes the DB on exit.
	rows, err := db.ListSessions(core.SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 session, got %d", len(rows))
	}

	var out, errOut bytes.Buffer
	code, err := sessionsCmd(nil, staticDB(db), config{}, &out, &errOut)
	if err != nil {
		t.Fatalf("sessionsCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	line := strings.TrimSuffix(out.String(), "\n")
	fields := strings.Split(line, "\t")
	if len(fields) != 6 {
		t.Fatalf("fields = %d, want 6 (SessionID, TimeRange, Project, Title, MessageCount, BodySize): %q", len(fields), line)
	}
	if want := strconv.Itoa(rows[0].MessageCount); fields[4] != want {
		t.Errorf("MessageCount column = %s, want %s", fields[4], want)
	}
	if want := strconv.Itoa(rows[0].BodySize); fields[5] != want {
		t.Errorf("BodySize column = %s, want %s", fields[5], want)
	}
	if rows[0].BodySize == 0 {
		t.Error("fixture BodySize should be non-zero")
	}
}
