package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func copyCodexFixture(t *testing.T, root, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "v0.14.0", "codex", "input-a", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}
func runCodexImport(t *testing.T, db *DB, root string, full bool) *ImportResult {
	t.Helper()
	r, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}, Full: full})
	if err != nil || len(r.Errors) > 0 {
		t.Fatalf("import=%+v err=%v", r, err)
	}
	return r
}
func TestCodexCanonicalGroupRebuildAndConflictRollback(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	for _, name := range []string{"child-extra.jsonl", "child.jsonl"} {
		copyCodexFixture(t, root, name)
	}
	runCodexImport(t, db, root, false)
	id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	read := func() []MessageRow {
		t.Helper()
		resolved := requireResolution(t, db, relationREF(root, SourceCodex, "child"))
		rows, err := db.GetIdentityMessages(resolved.Self.InputID, resolved.Self.Source, resolved.Self.Identity)
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	before := read()
	if len(before) != 3 || before[0].PayloadID != "c1" || before[1].PayloadID != "c3" || before[2].PayloadID != "c2" {
		t.Fatalf("merged=%+v", before)
	}
	var importedAt string
	if err := db.db.QueryRow("SELECT imported_at FROM sessions WHERE session_id='child'").Scan(&importedAt); err != nil {
		t.Fatal(err)
	}
	if r := runCodexImport(t, db, root, false); r.FilesSkipped != 2 {
		t.Fatalf("unchanged=%+v", r)
	}
	var afterTime string
	db.db.QueryRow("SELECT imported_at FROM sessions WHERE session_id='child'").Scan(&afterTime)
	if afterTime != importedAt {
		t.Fatal("unchanged import changed imported_at")
	}
	appendData, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "v0.14.0", "resume", "forward-rollout-append.txt"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "child-extra.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(appendData)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	runCodexImport(t, db, root, false)
	after := read()
	if len(after) != 4 || after[2].PayloadID != "c4" || after[3].PayloadID != "c2" {
		t.Fatalf("forward append=%+v", after)
	}
	for i, m := range after {
		if m.Number != i+1 {
			t.Fatalf("number %+v", m)
		}
	}
	runCodexImport(t, db, root, true)
	if got := read(); !reflect.DeepEqual(got, after) {
		t.Fatalf("full=%+v want=%+v", got, after)
	}
	oldState, err := db.GetImportState(id, path)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "Child answer", "Other answer", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		r, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}, Full: full})
		if err != nil || r.FilesFailed != 2 || len(r.Errors) != 1 {
			t.Fatalf("conflict=%+v %v", r, err)
		}
		if got := read(); !reflect.DeepEqual(got, after) {
			t.Fatalf("conflict changed body %+v", got)
		}
		state, _ := db.GetImportState(id, path)
		if !reflect.DeepEqual(state, oldState) {
			t.Fatal("conflict advanced state")
		}
	}
}

func TestCodexChildFirstParentLaterInputIsolationAndUnresolved(t *testing.T) {
	db := testDB(t)
	a, b := testTempDir(t), testTempDir(t)
	data, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "v0.14.0", "codex", "input-b", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(b, "root.jsonl"), data, 0600)
	runCodexImport(t, db, b, false)
	for _, name := range []string{"child.jsonl", "grandchild.jsonl", "missing-ordinal.jsonl", "no-boundary.jsonl"} {
		copyCodexFixture(t, a, name)
	}
	r := runCodexImport(t, db, a, false)
	if len(r.UnparsedDiagnostics) != 1 {
		t.Fatalf("unresolved diagnostics=%+v", r)
	}
	id, _ := db.EnsureInput(Input{Source: SourceCodex, Root: a})
	child, err := db.GetSession(id, SourceCodex, "child")
	if err != nil || child.ParentSessionID != "root" {
		t.Fatalf("child=%+v %v", child, err)
	}
	parent, err := db.GetSession(id, SourceCodex, "root")
	if err != nil || parent != nil {
		t.Fatalf("cross input parent %+v %v", parent, err)
	}
	messages, err := db.GetMessages(id, SourceCodex, "child")
	if err != nil || len(messages) != 2 || messages[0].Content != "Child answer" {
		t.Fatalf("child body=%+v %v", messages, err)
	}
	var unresolved int
	db.db.QueryRow("SELECT COUNT(*) FROM messages WHERE input_id=? AND session_id='unknown-owner' AND membership='unresolved' AND number=0", id).Scan(&unresolved)
	if unresolved != 1 {
		t.Fatal("unresolved original not preserved")
	}
	unknown, _ := db.GetSession(id, SourceCodex, "unknown-owner")
	if unknown.StartedAt != "" || unknown.MessageCount != 0 {
		t.Fatalf("unresolved activity %+v", unknown)
	}
	copyCodexFixture(t, a, "root.jsonl")
	runCodexImport(t, db, a, false)
	later, _ := db.GetSession(id, SourceCodex, "child")
	got, _ := db.GetMessages(id, SourceCodex, "child")
	if later.REF != child.REF || !reflect.DeepEqual(got, messages) {
		t.Fatal("parent arrival changed child")
	}
	parent, _ = db.GetSession(id, SourceCodex, "root")
	if parent == nil {
		t.Fatal("same input parent missing")
	}
	old, _ := db.GetMessages(id, SourceCodex, "old-child")
	for _, m := range old {
		if m.Timestamp != "" {
			t.Fatal("unknown timestamp filled")
		}
	}
}

func TestCodexGroupStorageFailureRollsBackAllCursors(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	for _, name := range []string{"child-extra.jsonl", "child.jsonl"} {
		copyCodexFixture(t, root, name)
	}
	runCodexImport(t, db, root, false)
	id, _ := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	before, _ := db.GetMessages(id, SourceCodex, "child")
	states := map[string]*ImportState{}
	for _, name := range []string{"child-extra.jsonl", "child.jsonl"} {
		path := filepath.Join(root, name)
		states[path], _ = db.GetImportState(id, path)
	}
	_, err := db.db.Exec(`CREATE TRIGGER fail_group BEFORE INSERT ON messages WHEN NEW.payload_id='c4' BEGIN SELECT RAISE(ABORT,'injected group write failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "v0.14.0", "resume", "forward-rollout-append.txt"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "child-extra.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	f.Close()
	for _, full := range []bool{false, true} {
		r, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}, Full: full})
		if err != nil || len(r.Errors) != 1 || r.FilesFailed != 2 {
			t.Fatalf("failure=%+v %v", r, err)
		}
		after, _ := db.GetMessages(id, SourceCodex, "child")
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("body after failure %+v", after)
		}
		for path, old := range states {
			state, _ := db.GetImportState(id, path)
			if !reflect.DeepEqual(state, old) {
				t.Fatalf("cursor changed %s", path)
			}
		}
	}
}

func TestCodexSameSizeEditAndEarlierNewRolloutMatchFreshImport(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	copyCodexFixture(t, root, "child.jsonl")
	runCodexImport(t, db, root, false)
	path := filepath.Join(root, "child.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "Child follow-up", "Other follow-up", 1)
	if len(edited) != len(data) {
		t.Fatal("test edit must keep size")
	}
	os.WriteFile(path, []byte(edited), 0600)
	runCodexImport(t, db, root, false)
	copyCodexFixture(t, root, "child-extra.jsonl")
	runCodexImport(t, db, root, false)
	id, _ := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	got, _ := db.GetMessages(id, SourceCodex, "child")
	if len(got) != 3 || got[2].Content != "Other follow-up" {
		t.Fatalf("edit/new rollout=%+v", got)
	}
	fresh := testDB(t)
	runCodexImport(t, fresh, root, false)
	freshID, _ := fresh.EnsureInput(Input{Source: SourceCodex, Root: root})
	want, _ := fresh.GetMessages(freshID, SourceCodex, "child")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("incremental=%+v fresh=%+v", got, want)
	}
}

func TestCodexRootSourceStringVariantsNormalAndFull(t *testing.T) {
	for _, source := range []string{"cli", "vscode", "exec", "mcp"} {
		t.Run(source, func(t *testing.T) {
			db := testDB(t)
			root := testTempDir(t)
			data := `{"type":"session_meta","timestamp":"2026-10-01T00:00:00Z","payload":{"id":"root","source":"` + source + `","cwd":"/fixtures/project","cli_version":"root-version"}}` + "\n" +
				`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Root source variant"}]} }` + "\n"
			path := filepath.Join(root, "root.jsonl")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			var ref string
			for _, full := range []bool{false, true} {
				result := runCodexImport(t, db, root, full)
				if result.UnparsedLines != 0 || len(result.UnparsedDiagnostics) != 0 || result.FilesImported != 1 {
					t.Fatalf("full=%v result=%+v", full, result)
				}
				inputID, _ := db.EnsureInput(Input{Source: SourceCodex, Root: root})
				session, err := db.GetSession(inputID, SourceCodex, "root")
				if err != nil || session == nil {
					t.Fatalf("full=%v session=%+v err=%v", full, session, err)
				}
				if session.CWD != "/fixtures/project" || session.ParentREF != "" || session.ParentIdentity != "" {
					t.Fatalf("root metadata %+v", session)
				}
				if ref != "" && ref != session.REF {
					t.Fatal("full changed REF")
				}
				ref = session.REF
				resolved, err := db.LookupSessionREF(ref)
				if err != nil || resolved == nil || resolved.SessionID != "root" {
					t.Fatalf("REF=%+v err=%v", resolved, err)
				}
				messages, err := db.GetMessages(inputID, SourceCodex, "root")
				if err != nil || len(messages) != 1 || messages[0].Content != "Root source variant" || messages[0].Number != 1 || messages[0].Timestamp != "" {
					t.Fatalf("messages=%+v err=%v", messages, err)
				}
			}
		})
	}
}
