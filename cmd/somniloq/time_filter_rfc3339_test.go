package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const (
	rfc3339UTC    = "2026-03-28T10:00:37Z"
	rfc3339Offset = "2026-03-28T19:00:37+09:00"
)

func newRFC3339FilterDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "rfc3339-session",
		RepoPath:  "/Users/test/rfc3339",
		StartedAt: rfc3339UTC,
	}, rfc3339UTC); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.InsertMessage(core.NormalizedMessage{
		Source:    core.SourceClaudeCode,
		UUID:      "rfc3339-message",
		SessionID: "rfc3339-session",
		Role:      "user",
		Content:   "rfc3339 needle",
		Timestamp: rfc3339UTC,
	}); err != nil {
		t.Fatalf("InsertMessage: %v", err)
	}
	return db
}

func TestRFC3339TimeFiltersAcrossCommands(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	defer func() { time.Local = oldLocal }()

	tests := []struct {
		name string
		run  func([]string, *bytes.Buffer, *bytes.Buffer) (int, error)
	}{
		{
			name: "sessions",
			run: func(args []string, out, errOut *bytes.Buffer) (int, error) {
				return sessionsCmd(args, staticDB(newRFC3339FilterDB(t)), config{}, out, errOut)
			},
		},
		{
			name: "projects",
			run: func(args []string, out, errOut *bytes.Buffer) (int, error) {
				return projectsCmd(args, staticDB(newRFC3339FilterDB(t)), config{}, out, errOut)
			},
		},
		{
			name: "show",
			run: func(args []string, out, errOut *bytes.Buffer) (int, error) {
				return showCmd(args, staticDB(newRFC3339FilterDB(t)), config{}, out, errOut)
			},
		},
		{
			name: "search",
			run: func(args []string, out, errOut *bytes.Buffer) (int, error) {
				return searchCmd(args, staticDB(newRFC3339FilterDB(t)), config{}, out, errOut)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, since := range []string{rfc3339UTC, rfc3339Offset} {
				args := []string{"--since", since}
				if tt.name == "search" {
					args = append(args, "needle")
				}
				var out, errOut bytes.Buffer
				code, err := tt.run(args, &out, &errOut)
				if err != nil || code != 0 {
					t.Fatalf("--since %q = %d, %v (stderr: %q)", since, code, err, errOut.String())
				}
				if out.Len() == 0 {
					t.Fatalf("--since %q produced no matching output", since)
				}
			}

			for _, until := range []string{rfc3339UTC, rfc3339Offset} {
				args := []string{"--until", until}
				if tt.name == "search" {
					args = append(args, "needle")
				}
				var out, errOut bytes.Buffer
				code, err := tt.run(args, &out, &errOut)
				if err != nil || code != 0 {
					t.Fatalf("--until %q = %d, %v (stderr: %q)", until, code, err, errOut.String())
				}
				if out.Len() != 0 {
					t.Fatalf("--until %q included equal boundary: %q", until, out.String())
				}
			}
		})
	}
}

func TestSessionsCmd_ImportedSinceAcceptsEquivalentRFC3339Instants(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	defer func() { time.Local = oldLocal }()

	for _, importedSince := range []string{rfc3339UTC, rfc3339Offset} {
		t.Run(importedSince, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code, err := sessionsCmd([]string{"--imported-since", importedSince}, staticDB(newRFC3339FilterDB(t)), config{}, &out, &errOut)
			if err != nil || code != 0 {
				t.Fatalf("sessionsCmd = %d, %v (stderr: %q)", code, err, errOut.String())
			}
			if out.Len() == 0 {
				t.Fatalf("--imported-since %q produced no matching output", importedSince)
			}
		})
	}
}
