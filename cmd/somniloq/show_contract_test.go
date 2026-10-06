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

func TestShowOriginalFixtureContract(t *testing.T) {
	base, err := filepath.Abs("../../internal/ingest/testdata/v0.14.0")
	if err != nil {
		t.Fatal(err)
	}
	roots := map[core.Source]string{core.SourceCodex: filepath.Join(base, "codex/input-a"), core.SourceClaudeCode: filepath.Join(base, "claude-code/input-a"), core.SourceCursorAgent: filepath.Join(base, "cursor-agent/input-a")}
	other := filepath.Join(base, "codex/input-b")
	open := func() (*core.DB, error) {
		db, err := core.OpenDB(":memory:")
		if err != nil {
			return nil, err
		}
		inputs := []core.Input{{Source: core.SourceCodex, Root: other}}
		for src, root := range roots {
			inputs = append(inputs, core.Input{Source: src, Root: root})
		}
		result, err := core.Import(db, core.ImportOptions{Inputs: inputs})
		if err != nil || len(result.Errors) > 0 {
			db.Close()
			t.Fatalf("import: %v %+v", err, result)
		}
		return db, nil
	}
	ref := func(src core.Source, identity string) string {
		return core.IdentityREF(core.InputKey(src, roots[src]), src, identity)
	}
	var oracle struct {
		Cases []struct {
			Path       string
			Identity   []string
			Utterances []struct {
				Number     int
				Role, Text string
				Timestamp  *string
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(base, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	for _, c := range oracle.Cases {
		if len(c.Utterances) == 0 || strings.Contains(c.Path, "child.jsonl") && strings.HasPrefix(c.Path, "codex/") {
			continue
		}
		src := core.SourceCodex
		if strings.HasPrefix(c.Path, "claude-code/") {
			src = core.SourceClaudeCode
		} else if strings.HasPrefix(c.Path, "cursor-agent/") {
			src = core.SourceCursorAgent
		}
		id, _ := json.Marshal(c.Identity)
		selected := ref(src, string(id))
		if strings.HasPrefix(c.Path, "codex/input-b/") {
			selected = core.IdentityREF(core.InputKey(src, other), src, string(id))
		}
		t.Run(c.Path, func(t *testing.T) {
			var out, diag bytes.Buffer
			code, err := showCmd([]string{"--format", "json", selected}, open, config{}, &out, &diag)
			if code != 0 || err != nil {
				t.Fatalf("%d %v", code, err)
			}
			var got showJSON
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Items) != len(c.Utterances) {
				t.Fatalf("items: %+v", got.Items)
			}
			for i, want := range c.Utterances {
				item := got.Items[i]
				if item.REF != selected || item.MessageNumber != want.Number || item.Text != want.Text || item.Role != want.Role || !reflect.DeepEqual(item.Timestamp, want.Timestamp) || item.Provenance != "source_record" {
					t.Fatalf("item: %+v want %+v", item, want)
				}
				if strings.Contains(c.Path, "agent-unresolved") && (item.ParentREF != nil || item.RootREF == nil || *item.RootREF != ref(src, `["cc-root"]`)) {
					t.Fatalf("unresolved: %+v", item)
				}
			}
			if src == core.SourceCursorAgent && !reflect.DeepEqual(got.Items[0].Blocks, []string{"First block", "Second block"}) {
				t.Fatalf("blocks: %+v", got.Items)
			}
		})
	}
	// Query fixture matches must lead back to the same owner text and number.
	var queryOracle struct {
		Cases []struct {
			ExpectedMatches []struct {
				REF           string `json:"ref"`
				MessageNumber int    `json:"messageNumber"`
				Role          string
				Timestamp     *string
				LineText      string `json:"lineText"`
			} `json:"expectedMatches"`
		}
	}
	queryData, err := os.ReadFile(filepath.Join(base, "../v0.14.0-query/expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(queryData, &queryOracle); err != nil {
		t.Fatal(err)
	}
	for _, expected := range queryOracle.Cases[0].ExpectedMatches {
		parts := strings.Split(expected.REF, ":")
		selected := ref(core.SourceCodex, `["root"]`)
		if parts[len(parts)-1] == "WyJjaGlsZCJd" {
			selected = ref(core.SourceCodex, `["child"]`)
		}
		var out, diag bytes.Buffer
		code, err := showCmd([]string{"--format", "json", selected}, open, config{}, &out, &diag)
		if code != 0 || err != nil {
			t.Fatalf("%d %v", code, err)
		}
		var got showJSON
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		item := got.Items[expected.MessageNumber-1]
		if item.Text != expected.LineText || item.MessageNumber != expected.MessageNumber || item.Role != expected.Role || !reflect.DeepEqual(item.Timestamp, expected.Timestamp) {
			t.Fatalf("query reference: %+v want %+v", item, expected)
		}
	}

	// Multiple selectors retain the first expansion order, including overlapping descendants.
	for _, ids := range [][]string{{`["root"]`, `["child"]`}, {`["child"]`, `["root"]`}} {
		args := []string{"--format", "json", "--descendants"}
		for _, id := range ids {
			args = append(args, ref(core.SourceCodex, id))
		}
		args = append(args, core.IdentityREF(core.InputKey(core.SourceCodex, other), core.SourceCodex, `["root"]`))
		var out, diag bytes.Buffer
		code, err := showCmd(args, open, config{}, &out, &diag)
		if code != 0 || err != nil {
			t.Fatalf("selectors: %d %v", code, err)
		}
		var got showJSON
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		order := []string{}
		seen := map[string]bool{}
		for _, item := range got.Items {
			if len(order) == 0 || order[len(order)-1] != item.REF {
				if seen[item.REF] {
					t.Fatalf("duplicate conversation: %+v", got)
				}
				seen[item.REF] = true
				order = append(order, item.REF)
			}
		}
		expected := []string{ref(core.SourceCodex, `["root"]`), ref(core.SourceCodex, `["child"]`), ref(core.SourceCodex, `["grandchild"]`), ref(core.SourceCodex, `["old-child"]`), ref(core.SourceCodex, `["ordinal-child"]`)}
		if ids[0] == `["child"]` {
			expected = []string{expected[1], expected[2], expected[0], expected[3], expected[4]}
		}
		expected = append(expected, core.IdentityREF(core.InputKey(core.SourceCodex, other), core.SourceCodex, `["root"]`))
		if !reflect.DeepEqual(order, expected) {
			t.Fatalf("order: %v want %v", order, expected)
		}
	}

	// Claude root-only membership and overlapping expansion remain distinct,
	// even when Codex and another input are selected in the same operation.
	for _, descendants := range []bool{false, true} {
		selected := []string{ref(core.SourceClaudeCode, `["cc-root","unresolved"]`), ref(core.SourceClaudeCode, `["cc-root","child"]`), ref(core.SourceCodex, `["child"]`), ref(core.SourceClaudeCode, `["cc-root","child"]`), ref(core.SourceClaudeCode, `["cc-root"]`)}
		args := []string{"--format", "json"}
		if descendants {
			args = append(args, "--descendants")
		}
		var out, diag bytes.Buffer
		code, err := showCmd(append(args, selected...), open, config{}, &out, &diag)
		if code != 0 || err != nil {
			t.Fatalf("mixed: %d %v", code, err)
		}
		var got showJSON
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		texts := []string{}
		for _, item := range got.Items {
			texts = append(texts, item.Text)
		}
		want := []string{"Unresolved prompt", "Child prompt", "Child result", "Child answer", "Second rollout answer", "Child follow-up", "Root question"}
		if descendants {
			want = []string{"Unresolved prompt", "Child prompt", "Child result", "Grandchild prompt", "Grandchild result", "Child answer", "Second rollout answer", "Child follow-up", "Grandchild answer", "Root question"}
		}
		if !reflect.DeepEqual(texts, want) {
			t.Fatalf("mixed descendants=%t: %v", descendants, texts)
		}
	}

	for _, tc := range []struct {
		src   core.Source
		id    string
		desc  bool
		texts []string
	}{
		{core.SourceCodex, `["root"]`, false, []string{"Inherited question", "Inherited answer"}},
		{core.SourceCodex, `["child"]`, false, []string{"Child answer", "Second rollout answer", "Child follow-up"}},
		{core.SourceCodex, `["child"]`, true, []string{"Child answer", "Second rollout answer", "Child follow-up", "Grandchild answer"}},
		{core.SourceCodex, `["root"]`, true, []string{"Inherited question", "Inherited answer", "Child answer", "Second rollout answer", "Child follow-up", "Grandchild answer", "Old assistant", "Old user", "Ordinal alone"}},
		{core.SourceClaudeCode, `["cc-root"]`, true, []string{"Root question", "Child prompt", "Child result", "Grandchild prompt", "Grandchild result"}},
	} {
		var out, diag bytes.Buffer
		args := []string{"--format", "json"}
		if tc.desc {
			args = append(args, "--descendants")
		}
		code, err := showCmd(append(args, ref(tc.src, tc.id)), open, config{}, &out, &diag)
		if code != 0 || err != nil {
			t.Fatalf("%d %v", code, err)
		}
		var got showJSON
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		texts := []string{}
		lastREF := ""
		number := 0
		for _, item := range got.Items {
			texts = append(texts, item.Text)
			if item.REF != lastREF {
				number = 0
				lastREF = item.REF
			}
			number++
			if item.MessageNumber != number {
				t.Fatalf("number: %+v", item)
			}
		}
		if !reflect.DeepEqual(texts, tc.texts) {
			t.Fatalf("%s descendants=%v: %v", tc.id, tc.desc, texts)
		}
	}
}

func TestShowJSONRawAndLegacySaved(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	if _, err = tx.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,imported_at) VALUES(?,'codex','old','now')`, digest); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,timestamp,number) VALUES(1,?,'old','codex','old','assistant',?,NULL,7)`, digest, "  <tag>\n raw\t "); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var out, diag bytes.Buffer
	code, err := showCmd([]string{"--format", "json", core.LegacyREF(digest, core.SourceCodex, "old")}, staticDB(db), config{}, &out, &diag)
	if code != 0 || err != nil {
		t.Fatalf("%d %v", code, err)
	}
	items := decodeShowItems(t, out.Bytes())
	item := items[0].(map[string]any)
	if item["messageNumber"] != float64(7) || item["text"] != "  <tag>\n raw\t " || item["blocks"] != nil || item["timestamp"] != nil || item["parentRef"] != nil || item["rootRef"] != nil || item["provenance"] != "legacy_saved" {
		t.Fatalf("legacy: %v", item)
	}
	for _, blocks := range [][]string{nil, {}, {" raw\n", "<tag>"}} {
		item := newShowMessageJSON(core.SessionRow{REF: "ref"}, core.MessageRow{Number: 9, Content: " raw\n\n<tag>", Timestamp: "bad-time", Blocks: blocks, Provenance: "source_record"})
		if item.Timestamp == nil || *item.Timestamp != "bad-time" || item.MessageNumber != 9 || !reflect.DeepEqual(item.Blocks, blocks) || item.Text != " raw\n\n<tag>" {
			t.Fatalf("raw: %+v", item)
		}
	}
}

func TestShowDescendantsValidation(t *testing.T) {
	for _, args := range [][]string{{"--descendants"}, {"--descendants", "--since", "24h"}, {"--descendants", "--until", "24h", fixtureREF(core.SourceCodex, "root")}} {
		var out, diag bytes.Buffer
		code, err := showCmd(args, func() (*core.DB, error) { t.Fatal("DB opened"); return nil, nil }, config{}, &out, &diag)
		if code != 2 || err == nil || out.Len() != 0 {
			t.Fatalf("%v: %d %v %s", args, code, err, out.String())
		}
	}
}

func TestShowMissingParentRawOrder(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	input := testInputID(t, db, core.SourceCodex)
	if err := db.UpsertSession(input, core.SessionMeta{Source: core.SourceCodex, SessionID: "orphan", ParentIdentity: `["missing"]`}, "now"); err != nil {
		t.Fatal(err)
	}
	times := []string{"2026-10-03T00:00:00Z", "bad-time", "", "2026-10-01T00:00:00Z"}
	for i, stamp := range times {
		if err := db.InsertMessage(input, core.NormalizedMessage{Source: core.SourceCodex, SessionID: "orphan", UUID: stamp + "id", Role: "assistant", Content: "  <tag>\nraw\t ", Timestamp: stamp, Number: i + 1, Blocks: []string{}}); err != nil {
			t.Fatal(err)
		}
	}
	var out, diag bytes.Buffer
	code, err := showCmd([]string{"--descendants", "--format", "json", fixtureREF(core.SourceCodex, "orphan")}, staticDB(db), config{}, &out, &diag)
	if code != 0 || err != nil {
		t.Fatalf("%d %v", code, err)
	}
	items := decodeShowItems(t, out.Bytes())
	if len(items) != 4 {
		t.Fatalf("items: %v", items)
	}
	for i, raw := range items {
		item := raw.(map[string]any)
		wantTimestamp := any(times[i])
		if times[i] == "" {
			wantTimestamp = nil
		}
		if item["messageNumber"] != float64(i+1) || item["timestamp"] != wantTimestamp || item["parentRef"] != nil || item["rootRef"] != nil || item["text"] != "  <tag>\nraw\t " || len(item["blocks"].([]any)) != 0 {
			t.Fatalf("item: %v", item)
		}
	}
}

func TestShowDescendantsInvalidBoolean(t *testing.T) {
	var out, diag bytes.Buffer
	code, err := showCmd([]string{"--descendants=invalid"}, func() (*core.DB, error) { t.Fatal("DB opened"); return nil, nil }, config{}, &out, &diag)
	if code != 2 || err != nil || out.Len() != 0 || !strings.Contains(diag.String(), "descendants") {
		t.Fatalf("%d %v %s", code, err, diag.String())
	}
}
