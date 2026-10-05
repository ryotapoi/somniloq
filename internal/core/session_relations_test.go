package core

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func relationREF(root string, source Source, parts ...string) string {
	return IdentityREF(InputKey(source, root), source, ingest.Identity(parts...))
}
func requireResolution(t *testing.T, db *DB, ref string) *SessionResolution {
	t.Helper()
	r, err := db.ResolveSession(ref)
	if err != nil || r == nil {
		t.Fatalf("resolve %s: %+v %v", ref, r, err)
	}
	return r
}
func refs(rows []SessionRow) []string {
	result := []string{}
	for _, s := range rows {
		result = append(result, s.REF)
	}
	return result
}
func TestResolutionCodexParentArrivalAndScope(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	other := testTempDir(t)
	for _, name := range []string{"child.jsonl", "grandchild.jsonl"} {
		copyCodexFixture(t, root, name)
	}
	runCodexImport(t, db, root, false)
	child := relationREF(root, SourceCodex, "child")
	grand := relationREF(root, SourceCodex, "grandchild")
	parent := relationREF(root, SourceCodex, "root")
	r := requireResolution(t, db, child)
	if r.Root != nil || r.Parent != nil || r.GroupKey != parent || r.RepresentativeREF != child || !reflect.DeepEqual(refs(r.Descendants), []string{child, grand}) || len(r.Members) != 2 {
		t.Fatalf("missing parent: %+v", r)
	}
	before, err := db.GetIdentityMessages(r.Self.InputID, r.Self.Source, r.Self.Identity)
	must(t, err)
	copyCodexFixture(t, other, "root.jsonl")
	runCodexImport(t, db, other, false)
	if requireResolution(t, db, child).Root != nil {
		t.Fatal("other input filled parent")
	}
	copyCodexFixture(t, root, "root.jsonl")
	runCodexImport(t, db, root, false)
	r = requireResolution(t, db, child)
	if r.Root == nil || r.Root.REF != parent || r.Parent == nil || r.RepresentativeREF != parent || len(r.Members) != 3 {
		t.Fatalf("arrival: %+v", r)
	}
	for _, full := range []bool{false, true} {
		runCodexImport(t, db, root, full)
		r = requireResolution(t, db, child)
		after, err := db.GetIdentityMessages(r.Self.InputID, r.Self.Source, r.Self.Identity)
		must(t, err)
		if !reflect.DeepEqual(after, before) {
			t.Fatalf("arrival changed original: %+v", after)
		}
	}
	matcher, err := CompilePatterns([]string{"Inherited question"}, false)
	must(t, err)
	hits, err := db.SearchOccurrences(child, SessionFilter{}, matcher, false, SearchCandidates{})
	must(t, err)
	if len(hits) != 0 {
		t.Fatalf("ancestor/context hit: %+v", hits)
	}
	matcher, err = CompilePatterns([]string{"answer"}, false)
	must(t, err)
	hits, err = db.SearchOccurrences(child, SessionFilter{}, matcher, false, SearchCandidates{})
	must(t, err)
	if len(hits) < 2 || hits[0].REF != child || hits[1].REF != grand {
		t.Fatalf("scope order: %+v", hits)
	}
	// The stored explicit cycle is unconfirmed, not repaired in storage.
	_, err = db.db.Exec(`UPDATE sessions SET parent_identity='["grandchild"]' WHERE input_id=? AND identity='["root"]'`, r.Self.InputID)
	must(t, err)
	r = requireResolution(t, db, child)
	if len(r.Diagnostics) != 3 || r.Parent != nil || len(r.Descendants) != 1 || len(r.Members) != 1 {
		t.Fatalf("cycle: %+v", r)
	}
}
func TestResolutionClaudeGroupingVersusDescendants(t *testing.T) {
	root, err := filepath.Abs("../ingest/testdata/v0.14.0/claude-code/input-a")
	must(t, err)
	db := testDB(t)
	importClaudeTest(t, db, root)
	child := claudeREF(root, "cc-root", "child")
	grand := claudeREF(root, "cc-root", "grandchild")
	r := requireResolution(t, db, child)
	if len(r.Members) != 5 || !reflect.DeepEqual(refs(r.Descendants), []string{child, grand}) || r.Root == nil {
		t.Fatalf("claude child: %+v", r)
	}
	unresolved := requireResolution(t, db, claudeREF(root, "cc-root", "unresolved"))
	if unresolved.Parent != nil || len(unresolved.Descendants) != 1 || unresolved.GroupKey != r.GroupKey {
		t.Fatalf("root-only: %+v", unresolved)
	}
	parent := requireResolution(t, db, claudeREF(root, "cc-root"))
	if !reflect.DeepEqual(refs(parent.Descendants), []string{parent.Self.REF, child, grand}) {
		t.Fatalf("DFS: %+v", parent.Descendants)
	}
}
func TestResolutionCursorIndependent(t *testing.T) {
	root, err := filepath.Abs("../ingest/testdata/v0.14.0/cursor-agent/input-a")
	must(t, err)
	db := testDB(t)
	_, err = Import(db, ImportOptions{Inputs: []Input{{Source: SourceCursorAgent, Root: root}}})
	must(t, err)
	r := requireResolution(t, db, relationREF(root, SourceCursorAgent, "cursor-root"))
	if r.Parent != nil || r.Root != nil || len(r.Members) != 1 || len(r.Descendants) != 1 {
		t.Fatalf("cursor: %+v", r)
	}
}

func TestResolutionResumeOriginalNumbers(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	path := filepath.Join(root, "project", "agent-transcripts", "resume", "resume.jsonl")
	must(t, os.MkdirAll(filepath.Dir(path), 0700))
	before, err := os.ReadFile("../ingest/testdata/v0.14.0/resume/before.txt")
	must(t, err)
	extra, err := os.ReadFile("../ingest/testdata/v0.14.0/resume/append.txt")
	must(t, err)
	must(t, os.WriteFile(path, before, 0600))
	opts := ImportOptions{Inputs: []Input{{Source: SourceCursorAgent, Root: root}}}
	_, err = Import(db, opts)
	must(t, err)
	read := func() []MessageRow {
		t.Helper()
		r := requireResolution(t, db, relationREF(root, SourceCursorAgent, "resume"))
		rows, err := db.GetIdentityMessages(r.Self.InputID, r.Self.Source, r.Self.Identity)
		must(t, err)
		return rows
	}
	first := read()
	if len(first) != 1 || first[0].Number != 1 || first[0].Content != "Before" {
		t.Fatalf("before: %+v", first)
	}
	must(t, os.WriteFile(path, append(before, extra...), 0600))
	_, err = Import(db, opts)
	must(t, err)
	after := read()
	if len(after) != 2 || after[0].Number != 1 || after[1].Number != 2 || after[1].Content != "After" {
		t.Fatalf("resume: %+v", after)
	}
	for _, full := range []bool{false, true} {
		opts.Full = full
		_, err = Import(db, opts)
		must(t, err)
		if got := read(); !reflect.DeepEqual(got, after) {
			t.Fatalf("reimport full=%t: %+v", full, got)
		}
	}
}
func TestResolutionSiblingDFSOrder(t *testing.T) {
	key := InputKey(SourceClaudeCode, "/designed")
	row := func(id, parent string) SessionRow {
		return SessionRow{Source: SourceClaudeCode, Identity: id, RootIdentity: "root", ParentIdentity: parent, REF: IdentityREF(key, SourceClaudeCode, id)}
	}
	root := row("root", "")
	a := row("a", "root")
	b := row("b", "root")
	grand := row("grand", "a")
	r := resolveSessionRelations(root, []SessionRow{b, grand, root, a}, key)
	want := []string{root.REF, a.REF, grand.REF, b.REF}
	if !reflect.DeepEqual(refs(r.Descendants), want) {
		t.Fatalf("DFS %+v want %v", r.Descendants, want)
	}
}
