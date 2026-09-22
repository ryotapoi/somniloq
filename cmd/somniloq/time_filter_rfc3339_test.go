package main

import (
	"bytes"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const (
	rfc3339UTC    = "2026-03-28T10:00:37.123Z"
	rfc3339Offset = "2026-03-28T19:00:37.123+09:00"
	laterUTC      = "2026-03-28T10:00:37.1235Z"
	laterOffset   = "2026-03-28T19:00:37.1235+09:00"
)

func newRFC3339FilterDB(t *testing.T, timestamp string) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	if err := db.UpsertSession(core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "rfc3339-session",
		RepoPath:  "/Users/test/rfc3339",
		StartedAt: timestamp,
	}, timestamp); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.InsertMessage(core.NormalizedMessage{
		Source:    core.SourceClaudeCode,
		UUID:      "rfc3339-message",
		SessionID: "rfc3339-session",
		Role:      "user",
		Content:   "rfc3339 needle",
		Timestamp: timestamp,
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
		run  func([]string, string, *bytes.Buffer, *bytes.Buffer) (int, error)
	}{
		{
			name: "sessions",
			run: func(args []string, timestamp string, out, errOut *bytes.Buffer) (int, error) {
				return sessionsCmd(args, staticDB(newRFC3339FilterDB(t, timestamp)), config{}, out, errOut)
			},
		},
		{
			name: "projects",
			run: func(args []string, timestamp string, out, errOut *bytes.Buffer) (int, error) {
				return projectsCmd(args, staticDB(newRFC3339FilterDB(t, timestamp)), config{}, out, errOut)
			},
		},
		{
			name: "show",
			run: func(args []string, timestamp string, out, errOut *bytes.Buffer) (int, error) {
				return showCmd(args, staticDB(newRFC3339FilterDB(t, timestamp)), config{}, out, errOut)
			},
		},
		{
			name: "search",
			run: func(args []string, timestamp string, out, errOut *bytes.Buffer) (int, error) {
				return searchCmd(args, staticDB(newRFC3339FilterDB(t, timestamp)), config{}, out, errOut)
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
				code, err := tt.run(args, rfc3339UTC, &out, &errOut)
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
				code, err := tt.run(args, rfc3339UTC, &out, &errOut)
				if err != nil || code != 0 {
					t.Fatalf("--until %q = %d, %v (stderr: %q)", until, code, err, errOut.String())
				}
				if out.Len() != 0 {
					t.Fatalf("--until %q included equal boundary: %q", until, out.String())
				}
			}

			for _, until := range []string{laterUTC, laterOffset} {
				args := []string{"--until", until}
				if tt.name == "search" {
					args = append(args, "needle")
				}
				var out, errOut bytes.Buffer
				code, err := tt.run(args, rfc3339UTC, &out, &errOut)
				if err != nil || code != 0 {
					t.Fatalf("--until %q = %d, %v (stderr: %q)", until, code, err, errOut.String())
				}
				if out.Len() == 0 {
					t.Fatalf("--until %q excluded earlier fractional timestamp", until)
				}
			}

			for _, since := range []string{laterUTC, laterOffset} {
				args := []string{"--since", since}
				if tt.name == "search" {
					args = append(args, "needle")
				}
				var out, errOut bytes.Buffer
				code, err := tt.run(args, rfc3339UTC, &out, &errOut)
				if err != nil || code != 0 {
					t.Fatalf("--since %q = %d, %v (stderr: %q)", since, code, err, errOut.String())
				}
				if out.Len() != 0 {
					t.Fatalf("--since %q included earlier fractional timestamp: %q", since, out.String())
				}
			}

			for _, equal := range []struct {
				stored   string
				boundary string
			}{
				{stored: "2026-03-28T10:00:37Z", boundary: "2026-03-28T10:00:37.000Z"},
				{stored: "2026-03-28T10:00:37.000Z", boundary: "2026-03-28T10:00:37Z"},
				{stored: "2026-03-28T10:00:37.123Z", boundary: "2026-03-28T10:00:37.1230Z"},
				{stored: "2026-03-28T10:00:37.1230Z", boundary: "2026-03-28T10:00:37.123Z"},
			} {
				for _, flag := range []string{"--since", "--until"} {
					args := []string{flag, equal.boundary}
					if tt.name == "search" {
						args = append(args, "needle")
					}
					var out, errOut bytes.Buffer
					code, err := tt.run(args, equal.stored, &out, &errOut)
					if err != nil || code != 0 {
						t.Fatalf("%s %q with stored %q = %d, %v (stderr: %q)", flag, equal.boundary, equal.stored, code, err, errOut.String())
					}
					if flag == "--since" && out.Len() == 0 {
						t.Fatalf("--since %q excluded equal stored timestamp %q", equal.boundary, equal.stored)
					}
					if flag == "--until" && out.Len() != 0 {
						t.Fatalf("--until %q included equal stored timestamp %q: %q", equal.boundary, equal.stored, out.String())
					}
				}
			}
		})
	}
}

func TestSessionsCmd_ImportedSinceAcceptsEquivalentRFC3339Instants(t *testing.T) {
	oldLocal := time.Local
	time.Local = time.FixedZone("JST", 9*60*60)
	defer func() { time.Local = oldLocal }()

	for _, importedSince := range []string{"2026-03-28T10:00:36Z", "2026-03-28T19:00:36+09:00"} {
		t.Run(importedSince, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code, err := sessionsCmd([]string{"--imported-since", importedSince}, staticDB(newRFC3339FilterDB(t, rfc3339UTC)), config{}, &out, &errOut)
			if err != nil || code != 0 {
				t.Fatalf("sessionsCmd = %d, %v (stderr: %q)", code, err, errOut.String())
			}
			if out.Len() == 0 {
				t.Fatalf("--imported-since %q produced no matching output", importedSince)
			}
		})
	}
}
