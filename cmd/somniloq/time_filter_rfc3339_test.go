package main

import (
	"bytes"
	"encoding/json"
	"strings"
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
	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "rfc3339-session",
		RepoPath:  "/Users/test/rfc3339",
		StartedAt: timestamp,
	}, timestamp); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.InsertMessage(testInputID(t, db,
		core.SourceClaudeCode), core.NormalizedMessage{
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
			name: "projects",
			run: func(args []string, timestamp string, out, errOut *bytes.Buffer) (int, error) {
				return projectsCmd(args, staticDB(newRFC3339FilterDB(t, timestamp)), config{}, out, errOut)
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
				if !timeFilterHasResults(t, tt.name, &out) {
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
				if timeFilterHasResults(t, tt.name, &out) {
					t.Fatalf("--until %q included equal boundary: %q", until, out.String())
				}
			}
			if tt.name != "projects" {
				return
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
				if !timeFilterHasResults(t, tt.name, &out) {
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
				if timeFilterHasResults(t, tt.name, &out) {
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

func timeFilterHasResults(t *testing.T, command string, out *bytes.Buffer) bool {
	t.Helper()
	if command != "search" {
		return out.Len() > 0
	}
	first := strings.SplitN(out.String(), "\n", 2)[0]
	var metadata struct {
		Total int `json:"total"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(first, "# page\t")), &metadata); err != nil {
		t.Fatal(err, out.String())
	}
	return metadata.Total > 0
}

func TestRFC3339OffsetValidation(t *testing.T) {
	for _, offset := range []string{"Z", "+00:00", "-05:00", "+23:59", "-23:59", "+24:00", "-24:00", "+09:60", "-09:60"} {
		for _, seconds := range []string{"37", "37.123"} {
			value := "2026-03-28T10:00:" + seconds + offset
			t.Run(value, func(t *testing.T) {
				wantError := strings.Contains(offset, "24:") || strings.HasSuffix(offset, ":60")
				_, err := parseShowTime(value, false, dayBoundary{}, time.UTC)
				if (err != nil) != wantError {
					t.Fatalf("parseShowTime(%q): %v, wantError=%v", value, err, wantError)
				}
				_, flags := newSearchFlagSet()
				*flags.importedSince = value
				_, err = searchCandidateFilter(flags, config{})
				if (err != nil) != wantError {
					t.Fatalf("imported-since %q: %v, wantError=%v", value, err, wantError)
				}
			})
		}
	}
}

func TestRFC3339ShowAndImportedSinceBoundaries(t *testing.T) {
	for _, value := range []string{rfc3339UTC, rfc3339Offset, "2026-03-28T05:00:37.123-05:00", "2026-03-29T09:59:37.123+23:59", "2026-03-27T10:01:37.123-23:59", laterUTC} {
		t.Run(value, func(t *testing.T) {
			for _, flag := range []string{"--since", "--until", "--imported-since"} {
				var out, diag bytes.Buffer
				db := newRFC3339FilterDB(t, rfc3339UTC)
				t.Cleanup(func() { db.Close() })
				args := []string{flag, value}
				var code int
				var err error
				wantResult := value != laterUTC
				if flag == "--imported-since" {
					code, err = searchCmd(args, staticDB(db), config{}, &out, &diag)
				} else {
					args = append(args, fixtureREF(core.SourceClaudeCode, "rfc3339-session"), "--format", "json")
					code, err = showCmd(args, staticDB(db), config{}, &out, &diag)
					if flag == "--until" {
						wantResult = !wantResult
					}
				}
				if code != 0 || err != nil {
					t.Fatalf("%s: %d %v %s", flag, code, err, diag.String())
				}
				hasResult := false
				if flag == "--imported-since" {
					hasResult = timeFilterHasResults(t, "search", &out)
				} else {
					var page showJSON
					if err := json.Unmarshal(out.Bytes(), &page); err != nil {
						t.Fatal(err)
					}
					hasResult = page.Total > 0
				}
				if hasResult != wantResult {
					t.Fatalf("%s: hasResult=%v want=%v: %s", flag, hasResult, wantResult, out.String())
				}
			}
		})
	}
}
