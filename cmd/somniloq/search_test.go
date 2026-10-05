package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSearchListPages(t *testing.T) {
	open := func() (*core.DB, error) {
		db, err := core.OpenDB(":memory:")
		if err != nil {
			return nil, err
		}
		input := testInputID(t, db, core.SourceCodex)
		for i := 0; i < 25; i++ {
			id := fmt.Sprintf("s-%02d", i)
			if err = db.UpsertSession(input, core.SessionMeta{Source: core.SourceCodex, SessionID: id, RepoPath: "/parent/app", StartedAt: "1999-01-01T00:00:00Z"}, "2026-10-05T00:00:00Z"); err != nil {
				t.Fatal(err)
			}
			if err = db.UpdateSessionTitle(input, core.SourceCodex, id, "title\t\\\n", "2026-10-05T00:00:00Z"); err != nil {
				t.Fatal(err)
			}
			if err = db.InsertMessage(input, core.NormalizedMessage{Source: core.SourceCodex, SessionID: id, UUID: id, Role: "user", Content: "needle", Timestamp: fmt.Sprintf("2026-10-01T00:%02d:00Z", i)}); err != nil {
				t.Fatal(err)
			}
		}
		return db, nil
	}
	for _, c := range []struct {
		flags                       []string
		total, count, limit, offset int
		more                        bool
		next                        *int
	}{
		{nil, 25, 20, 20, 0, true, newInt(20)}, {[]string{"--limit", "2", "--offset", "20"}, 25, 2, 2, 20, true, newInt(22)}, {[]string{"--limit", "0"}, 25, 0, 0, 0, true, nil}, {[]string{"--offset", "25"}, 25, 0, 20, 25, false, nil}, {[]string{"--offset", "99"}, 25, 0, 20, 99, false, nil}, {[]string{"--project", "missing"}, 0, 0, 20, 0, false, nil},
		{[]string{"--time-mode", "last", "--since", "2026-10-01T00:20:00Z", "--limit", "2", "--offset", "1"}, 5, 2, 2, 1, true, newInt(3)},
	} {
		t.Run(fmt.Sprint(c.flags), func(t *testing.T) {
			var out, diag bytes.Buffer
			args := append([]string{"--format", "json"}, c.flags...)
			code, err := searchCmd(args, open, config{}, &out, &diag)
			if err != nil || code != 0 {
				t.Fatal(code, err, diag.String())
			}
			var page searchGroupJSON
			if err = json.Unmarshal(out.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if page.Total != c.total || page.Count != c.count || page.Limit != c.limit || page.Offset != c.offset || page.HasMore != c.more || !reflect.DeepEqual(page.NextOffset, c.next) || page.Items == nil {
				t.Fatalf("%+v", page)
			}
			if page.Count > 0 {
				item := page.Items[0]
				if item.MemberCount != 1 || len(item.Members) != 1 || len(item.MatchedMembers) != 1 || item.StartedAt == nil || item.LastAt == nil || *item.StartedAt == "1999-01-01T00:00:00Z" {
					t.Fatal(item)
				}
				var envelope map[string]json.RawMessage
				json.Unmarshal(out.Bytes(), &envelope)
				var items []map[string]any
				json.Unmarshal(envelope["items"], &items)
				if len(items[0]) != 11 {
					t.Fatal(items[0])
				}
				for _, obsolete := range []string{"snippet", "turn", "sessionId"} {
					if _, ok := items[0][obsolete]; ok {
						t.Fatal(obsolete)
					}
				}
			}
		})
	}
	var out, diag bytes.Buffer
	code, err := searchCmd([]string{"--limit", "1"}, open, config{}, &out, &diag)
	if err != nil || code != 0 || !strings.HasPrefix(out.String(), "# page\t{\"total\":25,\"count\":1,\"limit\":1,") || !strings.Contains(out.String(), "title\\t\\\\\\n") {
		t.Fatal(code, err, out.String())
	}
}
func TestSearchListValidationBeforeDB(t *testing.T) {
	for _, args := range [][]string{{""}, {"(?=foo)"}, {"--limit", "-1"}, {"--offset", "-1"}, {"--format", "xml"}, {"--source", "all"}, {"--source", "claude_code"}, {"--input", ""}, {"--day-boundary", "25:00"}, {"--since", "bad"}, {"--all", "-e", ""}, {"one", "two"}, {"--unknown"}, {"--time-mode", "bad"}, {"--time-mode", "active"}, {"--time-mode", ""}, {"--since", ""}, {"--until", ""}, {"--day-boundary", ""}, {"--imported-since", ""}, {"--since", "7d"}, {"--since", "2026-10-01T12:00"}, {"--imported-since", "2026-10-01"}, {"--imported-since", "bad"}, {"--since", "2026-10-02", "--until", "2026-10-01"}, {"--since", "2026-10-01T00:00:00Z", "--until", "2026-10-01T00:00:00Z"}, {"--session", "REF", "--time-mode", "last", "--since", "2026-10-01", "needle"}} {
		var out, diag bytes.Buffer
		code, _ := searchCmd(args, func() (*core.DB, error) { t.Fatal("DB opened", args); return nil, nil }, config{}, &out, &diag)
		if code != 2 || out.Len() != 0 {
			t.Fatal(args, code, out.String())
		}
	}
}
func TestSearchListTSVNullAndArrays(t *testing.T) {
	var out bytes.Buffer
	page := searchGroupJSON{Items: []core.SearchGroup{{REF: "r", Source: core.SourceCodex, Members: []string{"r"}, MatchedMembers: []string{"r"}, MemberCount: 1}}, Total: 1, Count: 1, Limit: 20}
	if err := writeSearchGroupTSV(&out, page); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "r\t\\N\tcodex\t\\N\t\\N\t\\N\t\\N\t\\N\t[\"r\"]\t[\"r\"]\t1\n") {
		t.Fatal(out.String())
	}
}

func TestSearchTimeParsingAndBoundaryOverride(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("fixture", 9*60*60)
	defer func() { time.Local = old }()
	for _, c := range []struct {
		args         []string
		since, until string
	}{
		{[]string{"--since", "2026-10-01", "--until", "2026-10-01"}, "2026-09-30T19:00:00Z", "2026-10-01T19:00:00Z"},
		{[]string{"--since", "2026-10-01", "--until", "2026-10-01", "--day-boundary", "06:00"}, "2026-09-30T21:00:00Z", "2026-10-01T21:00:00Z"},
		{[]string{"--since", "2026-10-01T09:00:00.000000001+09:00", "--until", "2026-10-01T00:00:00.000000002Z"}, "2026-10-01T00:00:00.000000001Z", "2026-10-01T00:00:00.000000002Z"},
	} {
		fs, f := newSearchFlagSet()
		if err := fs.Parse(c.args); err != nil {
			t.Fatal(err)
		}
		boundary, err := resolveDayBoundary(*f.dayBoundary, config{DayBoundary: "04:00"})
		if err != nil {
			t.Fatal(err)
		}
		filter, err := buildSearchFilter(fs, f, boundary, false)
		if err != nil || filter.Since != c.since || filter.Until != c.until {
			t.Fatal(filter, err)
		}
	}
}
