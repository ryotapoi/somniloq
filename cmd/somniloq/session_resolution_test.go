package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func newCrossSourceSessionTestDB(t *testing.T) *core.DB {
	t.Helper()

	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, source := range []core.Source{core.SourceClaudeCode, core.SourceCodex} {
		if err := db.UpsertSession(testInputID(t, db,
			source), core.SessionMeta{
			Source:    source,
			SessionID: "same-id",
			CWD:       "/Users/test/proj",
			RepoPath:  "/Users/test/proj",
			StartedAt: "2026-03-28T15:00:00Z",
		}, "2026-03-28T15:00:00Z"); err != nil {
			t.Fatalf("UpsertSession(%s): %v", source, err)
		}
	}

	return db
}

func TestResolveSessionREFAcceptsCrossSourceRefs(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)
	for _, source := range []core.Source{core.SourceClaudeCode, core.SourceCodex} {
		ref := testREF(t, db, source, "same-id")
		resolved, code, err := resolveSessionREF(db, ref, nil)
		if code != 0 || err != nil || resolved == nil || resolved.Self.REF != ref {
			t.Fatalf("resolve: %d %v", code, err)
		}
	}
	_, code, err := resolveSessionREF(db, fixtureREF(core.SourceCodex, "absent"), nil)
	if code != 2 || err == nil {
		t.Fatalf("missing REF: %d %v", code, err)
	}
}

func TestSearchSessionRejectsInvalidAndMissingREF(t *testing.T) {
	for _, ref := range []string{"", "same-id", fixtureREF(core.SourceCodex, "absent")} {
		db := newCrossSourceSessionTestDB(t)
		var out, errOut bytes.Buffer
		code, err := searchCmd([]string{"--session", ref, "query"}, staticDB(db), config{}, &out, &errOut)
		if code != 2 || err == nil || out.Len() != 0 {
			t.Fatalf("search %q: %d %v %q", ref, code, err, out.String())
		}
	}
}

func TestSearchSessionFixtureAndOwnShow(t *testing.T) {
	root, err := filepath.Abs("../../internal/ingest/testdata/v0.14.0/claude-code/input-a")
	if err != nil {
		t.Fatal(err)
	}
	open := func() (*core.DB, error) {
		db, err := core.OpenDB(":memory:")
		if err != nil {
			return nil, err
		}
		result, err := core.Import(db, core.ImportOptions{Inputs: []core.Input{{Source: core.SourceClaudeCode, Root: root}}})
		if err != nil || len(result.Errors) > 0 {
			db.Close()
			t.Fatalf("import: %+v %v", result, err)
		}
		return db, nil
	}
	child := core.IdentityREF(core.InputKey(core.SourceClaudeCode, root), core.SourceClaudeCode, `["cc-root","child"]`)
	grand := core.IdentityREF(core.InputKey(core.SourceClaudeCode, root), core.SourceClaudeCode, `["cc-root","grandchild"]`)
	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--session", child, "--format", "json", "result"}, open, config{}, &out, &errOut)
	if code != 0 || err != nil {
		t.Fatalf("search: %d %v", code, err)
	}
	var page searchDetailJSON
	if err := json.Unmarshal(out.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	hits := page.Items
	if len(hits) != 2 {
		t.Fatalf("hits: %+v", hits)
	}
	for _, hit := range hits {
		if hit.REF != child && hit.REF != grand {
			t.Fatalf("ancestor/sibling: %+v", hit)
		}
	}
	out.Reset()
	errOut.Reset()
	code, err = showCmd([]string{"--format", "json", child}, open, config{}, &out, &errOut)
	if code != 0 || err != nil || !strings.Contains(out.String(), "Child result") || strings.Contains(out.String(), "Grandchild result") || strings.Contains(out.String(), "Root prompt") {
		t.Fatalf("own show: %d %v %s", code, err, out.String())
	}
}

func TestResolveSessionREFReadFailure(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)
	ref := testREF(t, db, core.SourceCodex, "same-id")
	db.Close()
	var diagnostics bytes.Buffer
	resolved, code, err := resolveSessionREF(db, ref, &diagnostics)
	if resolved != nil || code != 1 || err == nil || diagnostics.Len() != 0 {
		t.Fatalf("read failure: %+v %d %v %q", resolved, code, err, diagnostics.String())
	}
}

func TestSearchSessionCyclicDiagnosticsOnce(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for id, parent := range map[string]string{"A": "B", "B": "A"} {
		data := fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"source":{"subagent":{"thread_spawn":{"parent_thread_id":%q}}}}}`+"\n"+`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"needle"}]}}`+"\n", id, parent)
		if err := os.WriteFile(filepath.Join(root, id+".jsonl"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	result, err := core.Import(db, core.ImportOptions{Inputs: []core.Input{{Source: core.SourceCodex, Root: root}}})
	if err != nil || len(result.Errors) != 0 {
		t.Fatalf("import: %+v %v", result, err)
	}
	a := core.IdentityREF(core.InputKey(core.SourceCodex, root), core.SourceCodex, `["A"]`)
	b := core.IdentityREF(core.InputKey(core.SourceCodex, root), core.SourceCodex, `["B"]`)
	diagnostics := []string{"unconfirmed cyclic parent: " + a, "unconfirmed cyclic parent: " + b}
	sort.Strings(diagnostics)
	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"--session", a, "--format", "json", "needle"}, staticDB(db), config{}, &out, &errOut)
	if code != 0 || err != nil || errOut.String() != strings.Join(diagnostics, "\n")+"\n" {
		t.Fatalf("search: %d %v diagnostics %q", code, err, errOut.String())
	}
	var page searchDetailJSON
	if err := json.Unmarshal(out.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].REF != a {
		t.Fatalf("cyclic scope: %+v", page)
	}
}
