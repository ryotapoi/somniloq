package core

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestImport_InputIsolationAndSelectedFull(t *testing.T) {
	db := testDB(t)
	roots := []string{testTempDir(t), testTempDir(t)}
	inputs := []Input{{Source: SourceClaudeCode, Root: roots[0], Name: "first"}, {Source: SourceClaudeCode, Root: roots[1], Name: "second"}}
	paths := make([]string, 2)
	write := func(i int, body string) {
		t.Helper()
		must(t, os.MkdirAll(filepath.Join(roots[i], "project"), 0700))
		paths[i] = filepath.Join(roots[i], "project", "same.jsonl")
		must(t, os.WriteFile(paths[i], []byte(fmt.Sprintf(`{"type":"user","uuid":"same-uuid","sessionId":"same-id","message":{"content":%q}}`+"\n", body)), 0600))
	}
	write(0, "needle first")
	write(1, "needle second")
	original := timeNow
	t.Cleanup(func() { timeNow = original })
	calls := 0
	timeNow = func() string { calls++; return "2026-10-01T00:00:00Z" }
	result, err := Import(db, ImportOptions{Inputs: inputs})
	must(t, err)
	if result.FilesImported != 2 || calls != 1 {
		t.Fatalf("result/clock = %+v/%d", result, calls)
	}
	rows, err := db.ListSessions(SessionFilter{})
	must(t, err)
	if len(rows) != 2 || rows[0].REF == rows[1].REF {
		t.Fatalf("rows=%+v", rows)
	}
	var importedAtCount int
	must(t, db.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE imported_at='2026-10-01T00:00:00Z'`).Scan(&importedAtCount))
	if importedAtCount != 2 {
		t.Fatalf("shared imported_at count = %d", importedAtCount)
	}
	refs := map[int64]string{}
	ids := make([]int64, 2)
	for i, input := range inputs {
		ids[i], err = db.EnsureInput(input)
		must(t, err)
		session, err := db.GetSession(ids[i], input.Source, "same-id")
		must(t, err)
		if session == nil || session.MessageCount != 1 || session.BodySize != len([]rune([]string{"needle first", "needle second"}[i])) {
			t.Fatalf("session=%+v", session)
		}
		refs[ids[i]] = session.REF
		resolved, err := db.LookupSessionREF(session.REF)
		must(t, err)
		if resolved == nil || resolved.InputID != ids[i] {
			t.Fatalf("resolved=%+v", resolved)
		}
		messages, err := db.GetMessages(ids[i], input.Source, "same-id")
		must(t, err)
		if len(messages) != 1 || messages[0].Content != []string{"needle first", "needle second"}[i] {
			t.Fatalf("messages=%+v", messages)
		}
		turns, err := db.GetTurnMessages(ids[i], input.Source, "same-id")
		must(t, err)
		if len(turns) != 1 {
			t.Fatalf("turns=%+v", turns)
		}
	}
	search, err := db.SearchMessages(SessionFilter{}, "needle", SearchPagination{})
	must(t, err)
	if len(search) != 2 || search[0].REF == search[1].REF {
		t.Fatalf("search=%+v", search)
	}
	stateBefore, err := db.GetImportState(ids[1], paths[1])
	must(t, err)
	write(0, "replacement")
	result, err = Import(db, ImportOptions{Inputs: inputs, InputPaths: []string{roots[0]}, Full: true})
	must(t, err)
	if result.FilesImported != 1 {
		t.Fatalf("result=%+v", result)
	}
	stateAfter, err := db.GetImportState(ids[1], paths[1])
	must(t, err)
	if stateBefore == nil || stateAfter == nil || *stateBefore != *stateAfter {
		t.Fatalf("other cursor changed: %+v -> %+v", stateBefore, stateAfter)
	}
	for i, id := range ids {
		session, err := db.LookupSessionREF(refs[id])
		must(t, err)
		if session == nil {
			t.Fatalf("REF lost: %s", refs[id])
		}
		messages, err := db.GetMessages(id, SourceClaudeCode, "same-id")
		must(t, err)
		if len(messages) != 1 || messages[0].Content != []string{"replacement", "needle second"}[i] {
			t.Fatalf("messages=%+v", messages)
		}
	}
	result, err = Import(db, ImportOptions{Inputs: inputs, InputPaths: []string{filepath.Join(roots[0], "unselected")}, Full: true})
	must(t, err)
	if result.FilesScanned != 0 {
		t.Fatalf("unselected result=%+v", result)
	}
	rows, err = db.ListSessions(SessionFilter{})
	must(t, err)
	if len(rows) != 2 {
		t.Fatalf("unselected full deleted sessions: %+v", rows)
	}
	// Renaming and repeating the same canonical root preserve identity and skip once.
	alias := filepath.Join(testTempDir(t), "alias")
	must(t, os.Symlink(roots[0], alias))
	result, err = Import(db, ImportOptions{Inputs: []Input{{Source: SourceClaudeCode, Root: roots[0], Name: "renamed"}, {Source: SourceClaudeCode, Root: alias}}})
	must(t, err)
	if result.FilesSkipped != 1 || result.FilesScanned != 1 {
		t.Fatalf("duplicate root result=%+v", result)
	}
}

func TestImportState_SamePathDifferentSources(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	for i, source := range []Source{SourceClaudeCode, SourceCodex} {
		id, err := db.EnsureInput(Input{Source: source, Root: root})
		must(t, err)
		must(t, db.UpsertImportState(id, ImportState{Source: source, JSONLPath: "same-path", FileSize: int64(i + 1), LastOffset: int64(i + 1)}))
	}
	for i, source := range []Source{SourceClaudeCode, SourceCodex} {
		id, err := db.EnsureInput(Input{Source: source, Root: root})
		must(t, err)
		state, err := db.GetImportState(id, "same-path")
		must(t, err)
		if state == nil || state.Source != source || state.LastOffset != int64(i+1) {
			t.Fatalf("state=%+v", state)
		}
	}
}

func TestRootREF_StableAcrossDatabasesAndInputNames(t *testing.T) {
	root := testTempDir(t)
	var ref string
	for _, name := range []string{"before", "renamed"} {
		db := testDB(t)
		inputID, err := db.EnsureInput(Input{Source: SourceCodex, Root: root, Name: name})
		must(t, err)
		must(t, db.UpsertSession(inputID, SessionMeta{Source: SourceCodex, SessionID: "same"}, "before"))
		session, err := db.GetSession(inputID, SourceCodex, "same")
		must(t, err)
		if ref != "" && session.REF != ref {
			t.Fatalf("REF changed: %q -> %q", ref, session.REF)
		}
		ref = session.REF
	}
	db := testDB(t)
	session, err := db.LookupSessionREF(ref)
	must(t, err)
	if session != nil {
		t.Fatalf("missing REF resolved to %+v", session)
	}
}
