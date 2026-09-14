package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestImport_CursorAgentFixtureAndIncrementalContracts(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	path := filepath.Join(root, "synthetic-project", "agent-transcripts", "session-sample", "session-sample.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "cursor-agent", "cursor-agent.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, fixture, 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Import(db, ImportOptions{CursorProjectsDir: root, Source: ImportSourceCursorAgent})
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if result.FilesImported != 1 || result.UnparsedLines != 3 {
		t.Fatalf("Import result = %+v, want one file and three unparsed lines", result)
	}
	for i, line := range []int{9, 10, 11} {
		if got := result.UnparsedDiagnostics[i].Error(); !strings.HasPrefix(got, path+":"+strconv.Itoa(line)+":") {
			t.Errorf("diagnostic[%d] = %q, want %s:%d prefix", i, got, path, line)
		}
	}
	var got []string
	rows, err := db.db.Query("SELECT role, content FROM messages WHERE source = 'cursor_agent' ORDER BY timestamp, rowid")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var role, content string
		if err := rows.Scan(&role, &content); err != nil {
			t.Fatal(err)
		}
		got = append(got, role+":"+content)
	}
	want := []string{
		"user:Plan a harmless sample.",
		"assistant:First answer paragraph.\n\nSecond answer paragraph.",
		"user:Keep <timestamp>, <user_query>, and <cwd> as text.",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("messages = %q, want %q", got, want)
	}

	again, err := Import(db, ImportOptions{CursorProjectsDir: root, Source: ImportSourceCursorAgent})
	if err != nil || again.FilesSkipped != 1 {
		t.Fatalf("unchanged re-import = %+v, %v; want one skipped file", again, err)
	}
	appendLine := `{"role":"assistant","message":{"content":[{"type":"text","text":"new reply"}]}}` + "\n"
	if err := os.WriteFile(path, append(fixture, []byte(appendLine)...), 0o644); err != nil {
		t.Fatal(err)
	}
	appended, err := Import(db, ImportOptions{CursorProjectsDir: root, Source: ImportSourceCursorAgent})
	if err != nil {
		t.Fatalf("incremental Import failed: %v", err)
	}
	if appended.UnparsedLines != 0 {
		t.Errorf("incremental unparsed lines = %d, want 0", appended.UnparsedLines)
	}
	var count int
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE source = 'cursor_agent'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("message count after append = %d, want 4", count)
	}
	if err := os.WriteFile(path, fixture[:bytes.IndexByte(fixture, '\n')+1], 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Import(db, ImportOptions{CursorProjectsDir: root, Source: ImportSourceCursorAgent}); err != nil {
		t.Fatalf("shrunken Import failed: %v", err)
	}
	if err := db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE source = 'cursor_agent'").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Errorf("message count after shrink = %d, want preserved 4", count)
	}
}

func TestImport_AllIncludesCursorAgent(t *testing.T) {
	db := testDB(t)
	root := t.TempDir()
	path := filepath.Join(root, "project", "agent-transcripts", "session", "session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"role":"user","message":{"content":[{"type":"text","text":"hello"}]}}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Import(db, ImportOptions{CursorProjectsDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesImported != 1 {
		t.Errorf("all import files = %d, want 1", result.FilesImported)
	}
}
