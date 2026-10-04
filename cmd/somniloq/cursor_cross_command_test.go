package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	t.Run("sessions preserves unknown metadata without a filter and excludes it by time", func(t *testing.T) {
		var out, errOut bytes.Buffer
		db := newCursorCrossCommandDB(t, false)
		ref := testREF(t, db, core.SourceCursorAgent, cursorFixtureSessionID)
		code, err := sessionsCmd(nil, staticDB(db), config{}, &out, &errOut)
		if err != nil || code != 0 {
			t.Fatalf("sessionsCmd = %d, %v (stderr: %q)", code, err, errOut.String())
		}
		if !strings.HasSuffix(out.String(), "\tcursor_agent\n") || !strings.Contains(out.String(), ref+"\t\t\t\t") {
			t.Fatalf("sessions output = %q, want Cursor source and unknown timestamp/repository fields", out.String())
		}

		out.Reset()
		errOut.Reset()
		code, err = sessionsCmd([]string{"--until", "2026-03-29"}, staticDB(newCursorCrossCommandDB(t, false)), config{}, &out, &errOut)
		if err != nil || code != 0 || out.Len() != 0 {
			t.Fatalf("time-filtered sessions = %d, %v, %q; want successful empty output", code, err, out.String())
		}
	})

	t.Run("search keeps source-local turns and excludes unknown metadata by filters", func(t *testing.T) {
		var out, errOut bytes.Buffer
		db := newCursorCrossCommandDB(t, true)
		cursorREF := testREF(t, db, core.SourceCursorAgent, cursorFixtureSessionID)
		code, err := searchCmd([]string{"harmless"}, staticDB(db), config{}, &out, &errOut)
		if err != nil || code != 0 {
			t.Fatalf("searchCmd = %d, %v (stderr: %q)", code, err, errOut.String())
		}
		if !strings.Contains(out.String(), fixtureREF(core.SourceClaudeCode, cursorFixtureSessionID)+"\t2\t"+formatLocalTime("2026-03-28T10:01:00Z", time.Local)+"\t/Users/test/existing\tharmless existing source second turn\tclaude_code\n") ||
			!strings.Contains(out.String(), cursorREF+"\t1\t\t\tPlan a harmless sample.\tcursor_agent\n") {
			t.Fatalf("search output = %q, want source-local turns for both sources", out.String())
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
