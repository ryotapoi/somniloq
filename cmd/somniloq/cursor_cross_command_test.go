package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

const cursorFixtureSessionID = "session-sample"

func newCursorCrossCommandDB(t *testing.T, addAmbiguousSource bool) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	root := t.TempDir()
	path := filepath.Join(root, "project", "agent-transcripts", cursorFixtureSessionID, cursorFixtureSessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "..", "internal", "ingest", "testdata", "cursor-agent", "cursor-agent.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile fixture: %v", err)
	}
	if err := os.WriteFile(path, fixture, 0o644); err != nil {
		t.Fatalf("WriteFile fixture: %v", err)
	}
	if _, err := core.Import(db, core.ImportOptions{Inputs: []core.Input{{Source: core.SourceCursorAgent, Root: root}}, Source: core.ImportSourceCursorAgent}); err != nil {
		t.Fatalf("Import cursor fixture: %v", err)
	}

	if addAmbiguousSource {
		meta := core.SessionMeta{Source: core.SourceClaudeCode, SessionID: cursorFixtureSessionID, RepoPath: "/Users/test/existing", StartedAt: "2026-03-28T10:00:00Z"}
		if err := db.UpsertSession(testInputID(t, db, meta.Source), meta, "2026-03-28T10:00:00Z"); err != nil {
			t.Fatalf("UpsertSession existing source: %v", err)
		}
		for _, message := range []core.NormalizedMessage{
			{Source: meta.Source, UUID: "existing-source-first", SessionID: meta.SessionID, Role: "user", Content: "existing source first turn", Timestamp: meta.StartedAt},
			{Source: meta.Source, UUID: "existing-source-second", SessionID: meta.SessionID, Role: "user", Content: "harmless existing source second turn", Timestamp: "2026-03-28T10:01:00Z"},
		} {
			if err := db.InsertMessage(testInputID(t, db, message.Source), message); err != nil {
				t.Fatalf("InsertMessage %s: %v", message.UUID, err)
			}
		}
	}
	return db
}

func TestCursorFixture_CrossCommandReferenceContract(t *testing.T) {
	t.Run("search list preserves unknown metadata and excludes it by time", func(t *testing.T) {
		var out, errOut bytes.Buffer
		db := newCursorCrossCommandDB(t, false)
		ref := testREF(t, db, core.SourceCursorAgent, cursorFixtureSessionID)
		code, err := searchCmd([]string{"--format", "json"}, staticDB(db), config{}, &out, &errOut)
		var page searchGroupJSON
		if err != nil || code != 0 || json.Unmarshal(out.Bytes(), &page) != nil || page.Count != 1 || page.Items[0].REF != ref || page.Items[0].Project != nil || page.Items[0].StartedAt != nil || page.Items[0].LastAt != nil {
			t.Fatalf("search list = %d, %v, %s", code, err, out.String())
		}
		out.Reset()
		code, err = searchCmd([]string{"--format", "json", "--until", "2026-03-29"}, staticDB(newCursorCrossCommandDB(t, false)), config{}, &out, &errOut)
		if err != nil || code != 0 || json.Unmarshal(out.Bytes(), &page) != nil || page.Count != 0 {
			t.Fatalf("time-filtered search list = %d, %v, %s", code, err, out.String())
		}
	})

	t.Run("search keeps sources separate and includes unknown body times", func(t *testing.T) {
		var out, errOut bytes.Buffer
		db := newCursorCrossCommandDB(t, true)
		cursorREF := testREF(t, db, core.SourceCursorAgent, cursorFixtureSessionID)
		code, err := searchCmd([]string{"harmless"}, staticDB(db), config{}, &out, &errOut)
		if err != nil || code != 0 {
			t.Fatalf("searchCmd = %d, %v (stderr: %q)", code, err, errOut.String())
		}
		if !strings.Contains(out.String(), fixtureREF(core.SourceClaudeCode, cursorFixtureSessionID)) || !strings.Contains(out.String(), cursorREF) || !strings.Contains(out.String(), "\tcursor_agent\t") {
			t.Fatal(out.String())
		}

	})

	t.Run("show and projects expose the cursor session", func(t *testing.T) {
		var out, errOut bytes.Buffer
		db := newCursorCrossCommandDB(t, false)
		ref := testREF(t, db, core.SourceCursorAgent, cursorFixtureSessionID)
		code, err := showCmd([]string{ref}, staticDB(db), config{}, &out, &errOut)
		if err != nil || code != 0 || !strings.Contains(out.String(), "\t\\N\tPlan a harmless sample.") {
			t.Fatalf("show = %d, %v, %q", code, err, out.String())
		}

		out.Reset()
		errOut.Reset()
		code, err = projectsCmd(nil, staticDB(newCursorCrossCommandDB(t, false)), config{}, &out, &errOut)
		if err != nil || code != 0 || out.String() != "\t1\n" {
			t.Fatalf("projects = %d, %v, %q", code, err, out.String())
		}
	})
}
