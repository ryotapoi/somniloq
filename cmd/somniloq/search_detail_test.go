package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSearchDetailFixture(t *testing.T) {
	base, err := filepath.Abs("../../internal/ingest/testdata/v0.14.0")
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "codex/input-a")
	open := func() (*core.DB, error) {
		db, e := core.OpenDB(":memory:")
		if e != nil {
			return nil, e
		}
		r, e := core.Import(db, core.ImportOptions{Inputs: []core.Input{{Source: core.SourceCodex, Root: root}, {Source: core.SourceCodex, Root: filepath.Join(base, "codex/input-b")}, {Source: core.SourceCursorAgent, Root: filepath.Join(base, "cursor-agent/input-a")}}})
		if e != nil || len(r.Errors) > 0 {
			t.Fatalf("import %v %+v", e, r)
		}
		return db, nil
	}
	data, err := os.ReadFile(filepath.Join(base, "../v0.14.0-query/expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Cases []struct {
			Patterns        []string
			All             bool
			Session         string
			ExpectedTotal   int
			ExpectedMatches []core.SearchOccurrence
		}
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	rebase := func(ref string) string {
		parts := strings.Split(ref, ":")
		parts[1] = core.InputKey(core.SourceCodex, root)
		return strings.Join(parts, ":")
	}
	for _, c := range oracle.Cases[:2] {
		args := []string{"--session", rebase(c.Session), "--format", "json", "--all"}
		for _, p := range c.Patterns {
			args = append(args, "-e", p)
		}
		var out, diag bytes.Buffer
		code, e := searchCmd(args, open, config{}, &out, &diag)
		if code != 0 || e != nil {
			t.Fatalf("%d %v %s", code, e, diag.String())
		}
		var page searchDetailJSON
		if e = json.Unmarshal(out.Bytes(), &page); e != nil {
			t.Fatal(e)
		}
		if page.Total != c.ExpectedTotal || page.Count != c.ExpectedTotal || page.Limit != nil {
			t.Fatal(page)
		}
		for i := range c.ExpectedMatches {
			c.ExpectedMatches[i].REF = rebase(c.ExpectedMatches[i].REF)
		}
		if c.ExpectedTotal > 0 && !reflect.DeepEqual(page.Items, c.ExpectedMatches) {
			t.Fatalf("%+v want %+v", page.Items, c.ExpectedMatches)
		}
		for _, m := range page.Items {
			out.Reset()
			code, e = showCmd([]string{"--format", "json", m.REF}, open, config{}, &out, &diag)
			if code != 0 || e != nil {
				t.Fatal(e)
			}
			var show showJSON
			json.Unmarshal(out.Bytes(), &show)
			body := show.Items[m.MessageNumber-1]
			if body.Text[m.StartByte:m.EndByte] != m.MatchText || body.Role != m.Role || !reflect.DeepEqual(body.Timestamp, m.Timestamp) {
				t.Fatal(body, m)
			}
		}
	}
	ref := core.IdentityREF(core.InputKey(core.SourceCodex, root), core.SourceCodex, `["root"]`)
	for _, c := range []struct {
		flags []string
		count int
		more  bool
		next  *int
	}{{nil, 2, false, nil}, {[]string{"--limit", "0"}, 0, true, nil}, {[]string{"--offset", "1"}, 1, false, nil}, {[]string{"--limit", "1"}, 1, true, newInt(1)}, {[]string{"--offset", "10"}, 0, false, nil}} {
		args := append([]string{"--session", ref, "--format", "json"}, c.flags...)
		args = append(args, "-e", "Inherited question", "-e", "Child answer")
		var out, diag bytes.Buffer
		code, e := searchCmd(args, open, config{}, &out, &diag)
		if code != 0 || e != nil {
			t.Fatal(code, e)
		}
		var page searchDetailJSON
		json.Unmarshal(out.Bytes(), &page)
		if page.Total != 2 || page.Count != c.count || page.HasMore != c.more || !reflect.DeepEqual(page.NextOffset, c.next) {
			t.Fatalf("%+v", page)
		}
	}
	// List and detail share candidate selection, before the AND check.
	for _, c := range []struct {
		flags []string
		total int
	}{
		{[]string{"--source", "cursor-agent"}, 0},
		{[]string{"--input", filepath.Join(base, "codex/input-b")}, 0},
		{[]string{"--project", "fixtures"}, 0},
		{[]string{"--source", "codex", "--input", root, "--project", "project"}, 2},
	} {
		args := append([]string{"--session", ref, "--format", "json", "--all", "-e", "Inherited question", "-e", "Child answer"}, c.flags...)
		var out, diag bytes.Buffer
		code, e := searchCmd(args, open, config{}, &out, &diag)
		if code != 0 || e != nil {
			t.Fatal(code, e, diag.String())
		}
		var page searchDetailJSON
		if e = json.Unmarshal(out.Bytes(), &page); e != nil {
			t.Fatal(e)
		}
		if page.Total != c.total {
			t.Fatal(c, page)
		}
	}
	child := core.IdentityREF(core.InputKey(core.SourceCodex, root), core.SourceCodex, `["child"]`)
	for _, c := range []struct {
		args  []string
		total int
	}{
		{[]string{"--session", ref, "--since", "2026-10-02T00:00:00Z", "--all", "-e", "Inherited question", "-e", "Child answer"}, 0},
		{[]string{"--session", child, "-F", "-e", "answer", "--all", "--limit", "1", "--offset", "1", "Child"}, 5},
	} {
		args := append([]string{"--format", "json"}, c.args...)
		var out, diag bytes.Buffer
		code, e := searchCmd(args, open, config{}, &out, &diag)
		if code != 0 || e != nil {
			t.Fatal(code, e)
		}
		var page searchDetailJSON
		if e = json.Unmarshal(out.Bytes(), &page); e != nil {
			t.Fatal(e)
		}
		if page.Total != c.total {
			t.Fatalf("%v: %+v", c.args, page)
		}
		if c.total > 0 {
			m := page.Items[0]
			if m.MessageNumber != 1 || m.OccurrenceNumber != 2 || m.MatchText != "answer" || !reflect.DeepEqual(m.PatternIndexes, []int{2}) {
				t.Fatal(m)
			}
		}
	}
	db, e := open()
	if e != nil {
		t.Fatal(e)
	}
	sessions, e := db.ListSessions(core.SessionFilter{})
	if e != nil {
		t.Fatal(e)
	}
	db.Close()
	for _, session := range sessions {
		if session.Source != core.SourceCursorAgent {
			continue
		}
		var out, diag bytes.Buffer
		code, e := searchCmd([]string{"--session", session.REF, "--format", "json", "^"}, open, config{}, &out, &diag)
		if code != 0 || e != nil {
			t.Fatal(code, e)
		}
		var page searchDetailJSON
		json.Unmarshal(out.Bytes(), &page)
		if page.Count == 0 {
			t.Fatal("unknown timestamp body excluded")
		}
		for _, m := range page.Items {
			if m.Timestamp != nil {
				t.Fatal(m)
			}
		}
	}

	var out, diag bytes.Buffer
	code, e := searchCmd([]string{"--session", ref, "-e", "Inherited question", "-e", "Child answer"}, open, config{}, &out, &diag)
	if code != 0 || e != nil || !strings.HasPrefix(out.String(), "# page\t") || !strings.Contains(out.String(), "patternIndexes\tmatchText\tlineText\n") {
		t.Fatal(code, e, out.String())
	}
}
func newInt(v int) *int { return &v }
func TestSearchDetailValidationBeforeDB(t *testing.T) {
	for _, args := range [][]string{{"--session", "bad"}, {"--session", "bad", ""}, {"--session", "bad", "(?=foo)"}, {"--session", "bad", "--limit", "-1", "foo"}, {"--session", "bad", "--offset", "-1", "foo"}} {
		var out, diag bytes.Buffer
		code, e := searchCmd(args, func() (*core.DB, error) { t.Fatal("DB opened"); return nil, nil }, config{}, &out, &diag)
		if code != 2 || e == nil || out.Len() != 0 {
			t.Fatalf("%v: %d %v %q", args, code, e, out.String())
		}
	}
}
func TestSearchDetailTSVEscaping(t *testing.T) {
	page := searchDetailJSON{Items: []core.SearchOccurrence{{REF: "r", Role: "user", PatternIndexes: []int{1}, MatchText: "a\t\\\r\n", LineText: "a\t\\\r\n"}}, Total: 1, Count: 1}
	var out bytes.Buffer
	if e := writeSearchDetailTSV(&out, page); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(out.String(), "\\N\t0\t0\t[1]\ta\\t\\\\\\r\\n\ta\\t\\\\\\r\\n\n") {
		t.Fatal(out.String())
	}
}
