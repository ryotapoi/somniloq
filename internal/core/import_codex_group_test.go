package core

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
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
	metadataEdited := strings.Replace(string(data), "/fixtures/project", "/fixtures/projecX", 1)
	if len(metadataEdited) != len(data) {
		t.Fatal("metadata edit must keep size")
	}
	must(t, os.WriteFile(path, []byte(metadataEdited), 0600))
	runCodexImport(t, db, root, false)
	id, _ := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	meta, err := db.GetSession(id, SourceCodex, "child")
	if err != nil || meta.CWD != "/fixtures/projecX" {
		t.Fatalf("same-size metadata edit: %+v %v", meta, err)
	}
	data = []byte(metadataEdited)
	edited := strings.Replace(string(data), "Child follow-up", "Other follow-up", 1)
	if len(edited) != len(data) {
		t.Fatal("test edit must keep size")
	}
	os.WriteFile(path, []byte(edited), 0600)
	runCodexImport(t, db, root, false)
	copyCodexFixture(t, root, "child-extra.jsonl")
	runCodexImport(t, db, root, false)
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

func TestCodexImportKeepsSavedOwnersWhenReadIsIncomplete(t *testing.T) {
	line := func(id, body string) string {
		return `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"/fixture"}}` + "\n" +
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + body + `"}]}}` + "\n"
	}
	for _, phase := range []string{"preflight", "later_normal", "later_full"} {
		t.Run(phase, func(t *testing.T) {
			db := testDB(t)
			root := testTempDir(t)
			a, b := filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")
			must(t, os.WriteFile(a, []byte(line("a", "old-a")), 0600))
			must(t, os.WriteFile(b, []byte(line("b", "old-b")), 0600))
			runCodexImport(t, db, root, false)
			id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
			must(t, err)
			oldState, err := db.GetImportState(id, b)
			must(t, err)
			must(t, os.WriteFile(a, []byte(line("a", "new-a")), 0600))
			if phase == "preflight" {
				must(t, os.Remove(b))
				must(t, os.Symlink(filepath.Join(root, "missing"), b))
			}
			resolver := func(string) string {
				if phase != "preflight" {
					must(t, os.Remove(b))
				}
				return ""
			}
			r, err := importCodexGroups(db, id, root, codex.NewAdapter(resolver), "2026-01-01T00:00:00Z", phase == "later_full")
			must(t, err)
			if len(r.Errors) == 0 || r.FilesFailed == 0 {
				t.Fatalf("result=%+v", r)
			}
			got, err := db.GetMessages(id, SourceCodex, "b")
			must(t, err)
			if len(got) != 1 || got[0].Content != "old-b" {
				t.Fatalf("saved b=%+v", got)
			}
			state, err := db.GetImportState(id, b)
			must(t, err)
			if !reflect.DeepEqual(state, oldState) {
				t.Fatalf("b cursor changed: %+v", state)
			}
			if phase != "later_normal" {
				got, err := db.GetMessages(id, SourceCodex, "a")
				must(t, err)
				if len(got) != 1 || got[0].Content != "old-a" {
					t.Fatalf("saved a=%+v", got)
				}
			}
		})
	}
}

func TestCodexImportSkipsUnchangedGroupBeforeRepositoryResolution(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	for _, id := range []string{"a", "b"} {
		data := `{"type":"session_meta","payload":{"id":"` + id + `","cwd":"/same"}}` + "\n" +
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"body"}]}}` + "\n"
		must(t, os.WriteFile(filepath.Join(root, id+".jsonl"), []byte(data), 0600))
	}
	id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	must(t, err)
	calls := 0
	adapter := codex.NewAdapter(func(string) string { calls++; return "/same" })
	for pass := 1; pass <= 2; pass++ {
		r, err := importCodexGroups(db, id, root, adapter, "2026-01-01T00:00:00Z", false)
		must(t, err)
		if len(r.Errors) != 0 || calls != 1 || (pass == 2 && r.FilesSkipped != 2) {
			t.Fatalf("pass=%d result=%+v resolver calls=%d", pass, r, calls)
		}
	}
}

func TestCodexImportDoesNotEarlySkipWhenSavedRolloutIsMissing(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	for _, owner := range []string{"a", "b"} {
		data := `{"type":"session_meta","payload":{"id":"` + owner + `","cwd":"/same"}}` + "\n" +
			`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"body"}]}}` + "\n"
		must(t, os.WriteFile(filepath.Join(root, owner+".jsonl"), []byte(data), 0600))
	}
	runCodexImport(t, db, root, false)
	id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	must(t, err)
	missing := filepath.Join(root, "b.jsonl")
	oldState, err := db.GetImportState(id, missing)
	must(t, err)
	must(t, os.Remove(missing))
	calls := 0
	result, err := importCodexGroups(db, id, root, codex.NewAdapter(func(string) string {
		calls++
		return ""
	}), "later", false)
	must(t, err)
	if len(result.Errors) != 0 || calls != 1 {
		t.Fatalf("result=%+v resolver calls=%d", result, calls)
	}
	state, err := db.GetImportState(id, missing)
	must(t, err)
	if !reflect.DeepEqual(state, oldState) {
		t.Fatalf("removed rollout state changed: %+v", state)
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

func TestCodexMetadataOnlyEditRebuildsOwner(t *testing.T) {
	priorClock := timeNow
	timeNow = func() string { return "2026-10-01T00:00:00Z" }
	defer func() { timeNow = priorClock }()
	db := testDB(t)
	root := testTempDir(t)
	meta := `{"type":"session_meta","payload":{"id":"owner","cwd":"/fixtures/project"}}` + "\n"
	write := func(name, body string) {
		t.Helper()
		data := meta
		if body != "" {
			data += `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"` + body + `"}]}}` + "\n"
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	read := func() []MessageRow {
		t.Helper()
		rows, err := db.GetMessages(id, SourceCodex, "owner")
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	write("a.jsonl", "")
	runCodexImport(t, db, root, false)
	if state, err := db.GetImportState(id, filepath.Join(root, "a.jsonl")); err != nil || state != nil {
		t.Fatalf("new prefix advanced cursor: %+v %v", state, err)
	}
	write("a.jsonl", "Old a body")
	write("b.jsonl", "Remaining b body")
	runCodexImport(t, db, root, false)
	if rows := read(); len(rows) != 2 || rows[0].Number != 1 || rows[1].Number != 2 {
		t.Fatalf("initial body=%+v", rows)
	}
	for _, name := range []string{"a.jsonl", "b.jsonl"} {
		write(name, "")
		runCodexImport(t, db, root, false)
		rows := read()
		if name == "a.jsonl" && (len(rows) != 1 || rows[0].Content != "Remaining b body" || rows[0].Number != 1) {
			t.Fatalf("metadata-only edit retained old body: %+v", rows)
		}
		if name == "b.jsonl" && len(rows) != 0 {
			t.Fatalf("all metadata-only retained body: %+v", rows)
		}
		if state, err := db.GetImportState(id, filepath.Join(root, name)); err != nil || state != nil {
			t.Fatalf("metadata-only retained cursor: %+v %v", state, err)
		}
		var before, after string
		if err := db.db.QueryRow("SELECT COALESCE(MAX(imported_at),'') FROM sessions WHERE input_id=?", id).Scan(&before); err != nil {
			t.Fatal(err)
		}
		timeNow = func() string { return "2099-01-01T00:00:00Z" }
		runCodexImport(t, db, root, false)
		if err := db.db.QueryRow("SELECT COALESCE(MAX(imported_at),'') FROM sessions WHERE input_id=?", id).Scan(&after); err != nil {
			t.Fatal(err)
		}
		if after != before || !reflect.DeepEqual(read(), rows) {
			t.Fatal("unchanged metadata-only import changed owner")
		}
		runCodexImport(t, db, root, true)
		if got := read(); !reflect.DeepEqual(got, rows) {
			t.Fatalf("full=%+v normal=%+v", got, rows)
		}
	}
}

func TestCodexMissingRolloutHistorySurvivesAppendUntilFull(t *testing.T) {
	for _, missing := range []string{"child-extra.jsonl", "child.jsonl"} {
		t.Run(missing, func(t *testing.T) {
			db := testDB(t)
			root, other := testTempDir(t), testTempDir(t)
			for _, name := range []string{"child-extra.jsonl", "child.jsonl"} {
				copyCodexFixture(t, root, name)
			}
			// A boundary without an ordinal remains unresolved, never owner body.
			path := filepath.Join(root, "child.jsonl")
			data, err := os.ReadFile(path)
			must(t, err)
			data = append(data, []byte(`{"type":"response_item","payload":{"type":"message","id":"unknown","role":"user","content":[{"type":"input_text","text":"Unresolved original"}]}}`+"\n")...)
			must(t, os.WriteFile(path, data, 0600))
			copyCodexFixture(t, other, "child.jsonl")
			runCodexImport(t, db, other, false)
			otherID, err := db.EnsureInput(Input{Source: SourceCodex, Root: other})
			must(t, err)
			otherBefore, err := db.GetMessages(otherID, SourceCodex, "child")
			must(t, err)
			runCodexImport(t, db, root, false)
			id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
			must(t, err)
			read := func() []MessageRow {
				t.Helper()
				rows, err := db.GetMessages(id, SourceCodex, "child")
				must(t, err)
				return rows
			}
			originals := func() []string {
				t.Helper()
				rows, err := db.db.Query(`SELECT json_array(uuid,identity,parent_uuid,role,content,blocks_json,timestamp,is_sidechain,number,origin_path,origin_line,membership,payload_id)
     FROM messages WHERE input_id=? ORDER BY origin_path,origin_line`, id)
				must(t, err)
				defer rows.Close()
				var result []string
				for rows.Next() {
					var value string
					must(t, rows.Scan(&value))
					result = append(result, value)
				}
				must(t, rows.Err())
				return result
			}
			before, beforeOriginals := read(), originals()
			must(t, os.Remove(filepath.Join(root, missing)))
			runCodexImport(t, db, root, false)
			if !reflect.DeepEqual(originals(), beforeOriginals) {
				t.Fatal("deletion changed saved records")
			}
			remaining := "child.jsonl"
			if missing == remaining {
				remaining = "child-extra.jsonl"
			}
			remainingPath := filepath.Join(root, remaining)
			oldState, err := db.GetImportState(id, remainingPath)
			must(t, err)
			appendData, err := os.ReadFile(filepath.Join("..", "ingest", "testdata", "v0.14.0", "resume", "forward-rollout-append.txt"))
			must(t, err)
			f, err := os.OpenFile(remainingPath, os.O_APPEND|os.O_WRONLY, 0600)
			must(t, err)
			_, err = f.Write(appendData)
			must(t, err)
			must(t, f.Close())
			_, err = db.db.Exec(`CREATE TRIGGER fail_missing BEFORE INSERT ON messages WHEN NEW.payload_id='c4' BEGIN SELECT RAISE(ABORT,'injected write failure'); END`)
			must(t, err)
			result, err := Import(db, ImportOptions{Inputs: []Input{{Source: SourceCodex, Root: root}}})
			must(t, err)
			if len(result.Errors) != 1 || result.FilesFailed != 1 {
				t.Fatalf("failure=%+v", result)
			}
			if !reflect.DeepEqual(originals(), beforeOriginals) {
				t.Fatal("write failure lost saved records")
			}
			state, err := db.GetImportState(id, remainingPath)
			must(t, err)
			if !reflect.DeepEqual(state, oldState) {
				t.Fatal("write failure advanced cursor")
			}
			_, err = db.db.Exec("DROP TRIGGER fail_missing")
			must(t, err)
			runCodexImport(t, db, root, false)
			after := read()
			want := []string{"c1", "c3", "c2", "c4"}
			if missing == "child.jsonl" {
				want = []string{"c1", "c3", "c4", "c2"}
			}
			if len(after) != len(want) {
				t.Fatalf("merged=%+v", after)
			}
			for i, row := range after {
				if row.PayloadID != want[i] || row.Number != i+1 {
					t.Fatalf("physical order=%+v want=%v", after, want)
				}
				for _, old := range before {
					if old.PayloadID == row.PayloadID {
						row.Number = old.Number
						if !reflect.DeepEqual(row, old) {
							t.Fatalf("saved payload changed: %+v want=%+v", row, old)
						}
					}
				}
			}
			var context, unresolved int
			must(t, db.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE input_id=? AND membership='context' AND number=0`, id).Scan(&context))
			must(t, db.db.QueryRow(`SELECT COUNT(*) FROM messages WHERE input_id=? AND membership='unresolved' AND number=0`, id).Scan(&unresolved))
			if context != 2 || unresolved != 1 {
				t.Fatalf("context=%d unresolved=%d", context, unresolved)
			}
			session, err := db.GetSession(id, SourceCodex, "child")
			must(t, err)
			if session.ParentSessionID != "root" || session.StartedAt != "2026-10-01T00:01:00Z" {
				t.Fatalf("metadata=%+v", session)
			}
			mergedOriginals := originals()
			runCodexImport(t, db, root, false)
			if !reflect.DeepEqual(originals(), mergedOriginals) {
				t.Fatal("rerun changed merged records")
			}
			runCodexImport(t, db, root, true)
			fresh := testDB(t)
			runCodexImport(t, fresh, root, false)
			freshID, err := fresh.EnsureInput(Input{Source: SourceCodex, Root: root})
			must(t, err)
			current, err := fresh.GetMessages(freshID, SourceCodex, "child")
			must(t, err)
			if !reflect.DeepEqual(read(), current) || len(current) != 3 {
				t.Fatalf("full=%+v current=%+v", read(), current)
			}
			otherAfter, err := db.GetMessages(otherID, SourceCodex, "child")
			must(t, err)
			if !reflect.DeepEqual(otherAfter, otherBefore) {
				t.Fatal("full changed another input")
			}
		})
	}
}

func TestCodexMissingRolloutKeepsParentAndBodyActivity(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	meta := `{"type":"session_meta","payload":{"id":"child","cwd":"/fixture","source":{"subagent":{"thread_spawn":{"parent_thread_id":"root"}}}}}` + "\n"
	body := func(id, timestamp string) string {
		return `{"type":"response_item","timestamp":"` + timestamp + `","payload":{"type":"message","id":"` + id + `","role":"user","content":[{"type":"input_text","text":"` + id + `"}]}}` + "\n"
	}
	a, b := filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")
	must(t, os.WriteFile(a, []byte(meta+body("old", "2026-10-01T01:00:00Z")), 0600))
	must(t, os.WriteFile(b, []byte(meta+body("current", "2026-10-01T02:00:00Z")), 0600))
	runCodexImport(t, db, root, false)
	must(t, os.Remove(a))
	meta = `{"type":"session_meta","payload":{"id":"child","source":"cli"}}` + "\n"
	must(t, os.WriteFile(b, []byte(meta+body("current", "2026-10-01T02:00:00Z")+body("new", "2026-10-01T03:00:00Z")), 0600))
	runCodexImport(t, db, root, false)
	id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	must(t, err)
	session, err := db.GetSession(id, SourceCodex, "child")
	must(t, err)
	if session.ParentSessionID != "root" || session.CWD != "/fixture" || session.StartedAt != "2026-10-01T01:00:00Z" || session.EndedAt != "2026-10-01T03:00:00Z" {
		t.Fatalf("retained metadata=%+v", session)
	}
}
