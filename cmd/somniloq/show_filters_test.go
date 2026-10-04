package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func newShowFilterDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	stamps := []string{"2026-10-01T03:59:59.999999999+09:00", "2026-10-01T04:00:00+09:00", "2026-10-02T03:59:59.999999999+09:00", "2026-10-02T04:00:00+09:00", "bad", ""}
	for _, id := range []string{"a", "b"} {
		input := testInputID(t, db, core.SourceCodex)
		if err := db.UpsertSession(input, core.SessionMeta{Source: core.SourceCodex, SessionID: id, StartedAt: "2000-01-01T00:00:00Z"}, "now"); err != nil {
			t.Fatal(err)
		}
		for i, stamp := range stamps {
			role := "user"
			if i%2 == 1 {
				role = "assistant"
			}
			text := fmt.Sprintf(" %s%d\r\nDetail\t\\N", id, i+1)
			if err := db.InsertMessage(input, core.NormalizedMessage{Source: core.SourceCodex, SessionID: id, UUID: fmt.Sprintf("%s%d", id, i), Role: role, Number: i + 1, Content: text, Timestamp: stamp, Blocks: []string{text}}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return db
}
func runShowPage(t *testing.T, args []string) showJSON {
	t.Helper()
	var out, diag bytes.Buffer
	code, err := showCmd(append(args, "--format", "json"), staticDB(newShowFilterDB(t)), config{}, &out, &diag)
	if code != 0 || err != nil {
		t.Fatalf("%v: %d %v %s", args, code, err, diag.String())
	}
	var p showJSON
	if err := json.Unmarshal(out.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestShowFilterPageAndOriginalBlocks(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("JST", 9*3600)
	defer func() { time.Local = old }()
	a, b := fixtureREF(core.SourceCodex, "a"), fixtureREF(core.SourceCodex, "b")
	p := runShowPage(t, []string{a, b, a, "--role", "assistant", "--messages", "2:5", "--since", "2026-10-01", "--until", "2026-10-01", "--day-boundary", "04:00", "--offset", "1", "--limit", "1", "--one-line"})
	if p.Total != 2 || p.Count != 1 || p.Items[0].REF != b || p.Items[0].MessageNumber != 2 || p.Items[0].Text != " b2" || !reflect.DeepEqual(p.Items[0].Blocks, []string{" b2\r\nDetail\t\\N"}) || p.HasMore || p.NextOffset != nil {
		t.Fatalf("page: %+v", p)
	}
	for _, tc := range []struct {
		flags                []string
		total, count, offset int
		limit                *int
		more                 bool
		next                 *int
	}{
		{nil, 12, 12, 0, nil, false, nil},
		{[]string{"--limit", "2", "--offset", "5"}, 12, 2, 5, intPointer(2), true, intPointer(7)},
		{[]string{"--limit", "0"}, 12, 0, 0, intPointer(0), true, nil},
		{[]string{"--limit", "20", "--offset", "20"}, 12, 0, 20, intPointer(20), false, nil},
		{[]string{"--tail", "2"}, 12, 2, 10, intPointer(2), false, nil},
		{[]string{"--tail", "0"}, 12, 0, 12, intPointer(0), false, nil},
		{[]string{"--tail", "99"}, 12, 12, 0, intPointer(99), false, nil},
	} {
		p := runShowPage(t, append([]string{a, b}, tc.flags...))
		if p.Total != tc.total || p.Count != tc.count || p.Offset != tc.offset || !reflect.DeepEqual(p.Limit, tc.limit) || p.HasMore != tc.more || !reflect.DeepEqual(p.NextOffset, tc.next) {
			t.Fatalf("%v: %+v", tc.flags, p)
		}
	}
	p = runShowPage(t, []string{a, "--since", "2026-10-01T19:00:00.000000001Z", "--until", "2026-10-01T19:00:00.000000002Z"})
	if p.Total != 0 {
		t.Fatalf("nano boundary: %+v", p)
	}
	p = runShowPage(t, []string{a, "--since", "2026-09-30T19:00:00Z", "--until", "2026-10-01T18:59:59.999999999Z"})
	if p.Total != 1 || p.Items[0].MessageNumber != 2 {
		t.Fatalf("instant boundaries: %+v", p)
	}
}
func intPointer(n int) *int { return &n }
func TestShowInputErrorsAndAtomicOutput(t *testing.T) {
	ref := fixtureREF(core.SourceCodex, "a")
	for _, flags := range [][]string{{"--messages", ":"}, {"--messages", "2:1"}, {"--messages", "0:"}, {"--messages", "1"}, {"--messages", "+1:2"}, {"--messages", "1:2:3"}, {"--messages", ""}, {"--role", "system"}, {"--role", ""}, {"--since", "24h"}, {"--since", "2026-10-01T04:00"}, {"--since", ""}, {"--day-boundary", ""}, {"--day-boundary", "24:00"}, {"--limit", "-1"}, {"--offset", "-1"}, {"--tail", "-1"}, {"--tail", "0", "--limit", "0"}, {"--tail", "0", "--offset", "0"}, {"--format", "markdown"}, {"--summary", "1"}, {"--turn", "1"}, {"--project", "p"}, {"--short"}, {"--source", "codex"}, {"--exclude-user-message-pattern", "x"}, {"--no-exclude-user-messages"}} {
		var out, diag bytes.Buffer
		code, _ := showCmd(append([]string{ref}, flags...), func() (*core.DB, error) { t.Fatal("DB opened for invalid input"); return nil, nil }, config{}, &out, &diag)
		if code != 2 || out.Len() != 0 {
			t.Fatalf("%v: %d %s", flags, code, out.String())
		}
	}
	for _, last := range []string{"bare", fixtureREF(core.SourceCodex, "absent")} {
		var out, diag bytes.Buffer
		code, err := showCmd([]string{ref, last}, staticDB(newShowFilterDB(t)), config{}, &out, &diag)
		if code != 2 || err == nil || out.Len() != 0 {
			t.Fatalf("partial output: %d %v %s", code, err, out.String())
		}
	}
}
func TestShowFirstLineAndRange(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"Answer\nDetail", "Answer"}, {"\nnext", ""}, {" x \r\nrest", " x "}, {" x \r", " x \r"}, {"", ""}} {
		if got := showFirstLine(tc.in); got != tc.want {
			t.Fatalf("%q: %q", tc.in, got)
		}
	}
	for _, tc := range []struct {
		in     string
		lo, hi int
	}{{"1:2", 1, 2}, {"3:", 3, 0}, {":4", 0, 4}} {
		lo, hi, err := parseMessageRange(tc.in)
		if err != nil || lo != tc.lo || hi != tc.hi {
			t.Fatalf("%q: %d %d %v", tc.in, lo, hi, err)
		}
	}
}
func TestShowTSVWireFormatAndWriteErrors(t *testing.T) {
	text := "\\N\t\n\r"
	stamp := "bad"
	ref := "ref"
	p := showJSON{Items: []showMessageJSON{{REF: ref, MessageNumber: 7, Role: "user", Timestamp: &stamp, Text: text, Blocks: []string{}, Provenance: "source_record"}, {REF: ref, MessageNumber: 9, Role: "assistant", Blocks: nil, Provenance: "legacy_saved"}}, Total: 2, Count: 2}
	var out bytes.Buffer
	if err := writeShowTSV(&out, p); err != nil {
		t.Fatal(err)
	}
	want := "# page\t{\"total\":2,\"count\":2,\"limit\":null,\"offset\":0,\"hasMore\":false,\"nextOffset\":null}\nref\tmessageNumber\trole\ttimestamp\ttext\tblocks\tparentRef\trootRef\tprovenance\nref\t7\tuser\tbad\t\\\\N\\t\\n\\r\t[]\t\\N\t\\N\tsource_record\nref\t9\tassistant\t\\N\t\t\\N\t\\N\t\\N\tlegacy_saved\n"
	if out.String() != want {
		t.Fatalf("TSV: %q", out.String())
	}
	p.Items[0].Blocks = []string{"a\tb\n\\N"}
	out.Reset()
	if err := writeShowTSV(&out, p); err != nil {
		t.Fatal(err)
	}
	if strings.Count(out.String(), "\n") != 4 || !strings.Contains(out.String(), `["a\tb\n\\N"]`) {
		t.Fatalf("blocks: %q", out.String())
	}
	for _, format := range []string{"tsv", "json"} {
		code, err := showCmd([]string{fixtureREF(core.SourceCodex, "a"), "--format", format}, staticDB(newShowFilterDB(t)), config{}, failWriter{}, &bytes.Buffer{})
		if code != 1 || !errors.Is(err, errFailWriter) {
			t.Fatalf("write: %d %v", code, err)
		}
	}
}

func TestShowConfiguredBoundaryAndNanoPrecision(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("JST", 9*3600)
	defer func() { time.Local = old }()
	for _, args := range [][]string{{"--since", "2026-10-01", "--until", "2026-10-01"}, {"--since", "2026-10-01T18:59:59.999999999Z", "--until", "2026-10-01T19:00:00Z"}} {
		var out, diag bytes.Buffer
		code, err := showCmd(append(args, fixtureREF(core.SourceCodex, "a"), "--format", "json"), staticDB(newShowFilterDB(t)), config{DayBoundary: "04:00"}, &out, &diag)
		if code != 0 || err != nil {
			t.Fatalf("%d %v", code, err)
		}
		var p showJSON
		if err := json.Unmarshal(out.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		if len(args[1]) == 10 {
			if p.Count != 2 || p.Items[0].MessageNumber != 2 || p.Items[1].MessageNumber != 3 {
				t.Fatalf("configured day: %+v", p)
			}
		} else if p.Count != 1 || p.Items[0].MessageNumber != 3 {
			t.Fatalf("nano: %+v", p)
		}
	}
}
