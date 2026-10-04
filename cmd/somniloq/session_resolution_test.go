package main

import (
	"bytes"
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
	for _, command := range []string{"show", "outline"} {
		db := newCrossSourceSessionTestDB(t)
		var out, errOut bytes.Buffer
		var code int
		var err error
		if command == "show" {
			code, err = showCmd([]string{"same-id"}, staticDB(db), config{}, &out, &errOut)
		} else {
			code, err = outlineCmd([]string{"same-id"}, staticDB(db), config{}, &out, &errOut)
		}
		if code != 2 || err == nil || out.Len() != 0 {
			t.Fatalf("%s bare ID: %d %v %q", command, code, err, out.String())
		}
	}
}

func TestResolveSessionREFSelectsExactInputAndSource(t *testing.T) {
	db := newCrossSourceSessionTestDB(t)
	for _, source := range []core.Source{core.SourceClaudeCode, core.SourceCodex} {
		ref := testREF(t, db, source, "same-id")
		session, code, err := resolveSessionREF(db, ref, &source)
		if code != 0 || err != nil || session.Source != source {
			t.Fatalf("resolve: %d %v %+v", code, err, session)
		}
	}
	_, code, err := resolveSessionREF(db, fixtureREF(core.SourceCodex, "absent"), nil)
	if code != 2 || err == nil {
		t.Fatalf("missing REF: %d %v", code, err)
	}
}
