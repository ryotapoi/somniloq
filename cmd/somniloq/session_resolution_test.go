package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
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

func TestCommandsRejectBareSessionIDs(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)
	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"same-id"}, staticDB(db), config{}, &out, &errOut)
	if code != 2 || err == nil || out.Len() != 0 {
		t.Fatalf("bare ID: %d %v %q", code, err, out.String())
	}

}

func TestResolveSessionREFSelectsExactInputAndSource(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)
	for _, source := range []core.Source{core.SourceClaudeCode, core.SourceCodex} {
		ref := testREF(t, db, source, "same-id")
		session, code, err := resolveSessionREF(db, ref, &source, nil)
		if code != 0 || err != nil || session.Source != source {
			t.Fatalf("resolve: %d %v %+v", code, err, session)
		}
	}
	_, code, err := resolveSessionREF(db, fixtureREF(core.SourceCodex, "absent"), nil, nil)
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
	var hits []searchJSON
	if err := json.Unmarshal(out.Bytes(), &hits); err != nil {
		t.Fatal(err)
	}
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
