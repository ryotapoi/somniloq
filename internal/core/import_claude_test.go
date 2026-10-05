package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func importClaudeTest(t *testing.T, db *DB, roots ...string) *ImportResult {
	t.Helper()
	opts := ImportOptions{}
	for _, root := range roots {
		opts.Inputs = append(opts.Inputs, Input{Source: SourceClaudeCode, Root: root})
	}
	r, err := Import(db, opts)
	must(t, err)
	if len(r.Errors) > 0 {
		t.Fatalf("import: %+v", r)
	}
	return r
}

func claudeREF(root string, parts ...string) string {
	return IdentityREF(InputKey(SourceClaudeCode, root), SourceClaudeCode, ingest.Identity(parts...))
}

func TestClaudeFixtureOriginalsAndRelations(t *testing.T) {
	root, err := filepath.Abs("../ingest/testdata/v0.14.0/claude-code/input-a")
	must(t, err)
	db := testDB(t)
	r := importClaudeTest(t, db, root)
	if r.FilesImported != 5 {
		t.Fatalf("result=%+v", r)
	}
	var expected struct {
		Cases []struct {
			Path       string
			Identity   []string
			Parent     []string
			Utterances []struct {
				Line, Number          int
				Role, Text, Timestamp string
			}
		}
	}
	data, err := os.ReadFile("../ingest/testdata/v0.14.0/expected.json")
	must(t, err)
	must(t, json.Unmarshal(data, &expected))
	for _, c := range expected.Cases {
		if len(c.Path) < len("claude-code/") || c.Path[:len("claude-code/")] != "claude-code/" {
			continue
		}
		session, err := db.LookupSessionREF(claudeREF(root, c.Identity...))
		must(t, err)
		if session == nil {
			t.Fatalf("missing %v", c.Identity)
		}
		if len(c.Parent) > 0 && session.ParentREF != claudeREF(root, c.Parent...) {
			t.Fatalf("parent %v=%q", c.Identity, session.ParentREF)
		}
		if len(c.Parent) == 0 && session.ParentREF != "" {
			t.Fatalf("unexpected parent %v=%q", c.Identity, session.ParentREF)
		}
		if len(c.Identity) == 2 && session.RootREF != claudeREF(root, "cc-root") {
			t.Fatalf("root %v=%q", c.Identity, session.RootREF)
		}
		messages, err := db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
		must(t, err)
		if len(messages) != len(c.Utterances) {
			t.Fatalf("%v messages=%+v", c.Identity, messages)
		}
		for i, want := range c.Utterances {
			got := messages[i]
			if got.Content != want.Text || got.Number != want.Number || got.OriginLine != want.Line || got.Role != want.Role || got.Timestamp != want.Timestamp {
				t.Fatalf("%v message=%+v want=%+v", c.Identity, got, want)
			}
		}
	}
	var before string
	must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE identity='["cc-root","child"]'`).Scan(&before))
	previous := timeNow
	timeNow = func() string { return "2099-01-01T00:00:00Z" }
	defer func() { timeNow = previous }()
	r = importClaudeTest(t, db, root)
	var after string
	must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE identity='["cc-root","child"]'`).Scan(&after))
	if r.FilesSkipped != 5 || before != after {
		t.Fatalf("unchanged=%+v before/after=%s/%s", r, before, after)
	}
}

func writeClaude(t *testing.T, root, relative, data string) {
	t.Helper()
	path := filepath.Join(root, relative)
	must(t, os.MkdirAll(filepath.Dir(path), 0755))
	must(t, os.WriteFile(path, []byte(data), 0644))
}
func claudeBody(agent, text string) string {
	return `{"type":"user","uuid":"same","sessionId":"r","agentId":"` + agent + `","isSidechain":true,"timestamp":"invalid","message":{"role":"user","content":[{"type":"text","text":" ` + text + ` "},{"type":"text","text":"\nend"}]}}` + "\n"
}
func claudeCall(id string) string {
	return `{"type":"assistant","uuid":"call-` + id + `","sessionId":"r","timestamp":"2099-01-01T00:00:00Z","message":{"role":"assistant","content":[{"type":"tool_use","name":"Agent","id":"` + id + `"}]}}` + "\n"
}
func claudeResult(id, agent string) string {
	return `{"type":"user","uuid":"result-` + id + `","sessionId":"r","toolUseResult":{"agentId":"` + agent + `"},"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"` + id + `"}]}}` + "\n"
}

func TestClaudeArrivalAppendIsolationAndFull(t *testing.T) {
	root := testTempDir(t)
	other := testTempDir(t)
	db := testDB(t)
	child := claudeBody("child", "child")
	writeClaude(t, root, "p/r/subagents/agent-child.jsonl", child)
	importClaudeTest(t, db, root)
	ref := claudeREF(root, "r", "child")
	s, err := db.LookupSessionREF(ref)
	must(t, err)
	if s == nil || s.ParentREF != "" || s.RootREF != "" {
		t.Fatalf("child first=%+v", s)
	}
	writeClaude(t, root, "p/r.jsonl", claudeCall("a"))
	importClaudeTest(t, db, root)
	var parentBefore string
	must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE identity='["r"]'`).Scan(&parentBefore))
	priorClock := timeNow
	timeNow = func() string { return "2099-01-01T00:00:00Z" }
	defer func() { timeNow = priorClock }()
	writeClaude(t, root, "p/r.jsonl", claudeCall("a")+claudeResult("a", "child"))
	importClaudeTest(t, db, root)
	s, err = db.LookupSessionREF(ref)
	must(t, err)
	var parentAfter string
	must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE identity='["r"]'`).Scan(&parentAfter))
	if parentBefore != parentAfter {
		t.Fatalf("non-text append updated unchanged owner time: %s/%s", parentBefore, parentAfter)
	}
	parent, err := db.LookupSessionREF(claudeREF(root, "r"))
	must(t, err)
	if parent.StartedAt != "" || parent.EndedAt != "" {
		t.Fatalf("tool-only time became activity: %+v", parent)
	}
	if s.ParentREF != claudeREF(root, "r") || s.RootREF != claudeREF(root, "r") {
		t.Fatalf("resolved=%+v", s)
	}
	before, err := db.GetIdentityMessages(s.InputID, s.Source, s.Identity)
	must(t, err)
	if len(before) != 1 || before[0].Content != " child \n\n\nend" || before[0].Timestamp != "invalid" || !reflect.DeepEqual(before[0].Blocks, []string{" child ", "\nend"}) {
		t.Fatalf("originals=%+v", before)
	}
	writeClaude(t, root, "p/r/subagents/agent-sibling.jsonl", claudeBody("sibling", "sibling"))
	writeClaude(t, root, "p/other/subagents/agent-child.jsonl", claudeBody("child", "different root"))
	writeClaude(t, other, "p/r/subagents/agent-child.jsonl", claudeBody("child", "other input"))
	importClaudeTest(t, db, root, other)
	for _, tc := range []struct {
		root  string
		parts []string
		text  string
	}{{root, []string{"r", "sibling"}, " sibling \n\n\nend"}, {root, []string{"other", "child"}, " different root \n\n\nend"}, {other, []string{"r", "child"}, " other input \n\n\nend"}} {
		owner, err := db.LookupSessionREF(claudeREF(tc.root, tc.parts...))
		must(t, err)
		if owner == nil {
			t.Fatalf("missing=%v", tc)
		}
		m, err := db.GetIdentityMessages(owner.InputID, owner.Source, owner.Identity)
		must(t, err)
		if len(m) != 1 || m[0].Content != tc.text {
			t.Fatalf("isolation=%+v", m)
		}
	}
	// An unchanged parent file still connects a later-arriving child.
	writeClaude(t, root, "p/r.jsonl", claudeCall("a")+claudeResult("a", "child")+claudeCall("late")+claudeResult("late", "late"))
	importClaudeTest(t, db, root, other)
	writeClaude(t, root, "p/r/subagents/agent-late.jsonl", claudeBody("late", "late"))
	importClaudeTest(t, db, root, other)
	late, err := db.LookupSessionREF(claudeREF(root, "r", "late"))
	must(t, err)
	if late == nil || late.ParentREF != claudeREF(root, "r") {
		t.Fatalf("late child=%+v", late)
	}
	// A second physical parent supplies a competing complete chain.
	conflict := claudeBody("sibling", "sibling")
	call := claudeCall("b")
	result := claudeResult("b", "child")
	// Subagent records carry their own agentId, including tool-only records.
	var records []map[string]any
	for _, line := range []string{call, result} {
		var r map[string]any
		must(t, json.Unmarshal([]byte(line), &r))
		r["agentId"] = "sibling"
		records = append(records, r)
	}
	for _, r := range records {
		b, err := json.Marshal(r)
		must(t, err)
		conflict += string(b) + "\n"
	}
	writeClaude(t, root, "p/r/subagents/agent-sibling.jsonl", conflict)
	r := importClaudeTest(t, db, root, other)
	if len(r.UnparsedDiagnostics) == 0 {
		t.Fatal("missing conflict diagnostic")
	}
	s, err = db.LookupSessionREF(ref)
	must(t, err)
	if s.ParentREF != "" || s.RootREF != claudeREF(root, "r") {
		t.Fatalf("conflict=%+v", s)
	}
	opts := ImportOptions{Full: true, Inputs: []Input{{Source: SourceClaudeCode, Root: root}}}
	r, err = Import(db, opts)
	must(t, err)
	if len(r.Errors) > 0 {
		t.Fatalf("full=%+v", r)
	}
	s, err = db.LookupSessionREF(ref)
	must(t, err)
	after, err := db.GetIdentityMessages(s.InputID, s.Source, s.Identity)
	must(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("full originals=%+v vs %+v", before, after)
	}
	preserved, err := db.LookupSessionREF(claudeREF(other, "r", "child"))
	must(t, err)
	if preserved == nil {
		t.Fatal("full deleted other input")
	}
	// Failed replacement leaves text, relation and cursor in the old transaction.
	path := filepath.Join(root, "p/r/subagents/agent-child.jsonl")
	old, err := db.GetImportState(s.InputID, path)
	must(t, err)
	_, err = db.db.Exec(`CREATE TRIGGER fail_claude BEFORE INSERT ON messages BEGIN SELECT RAISE(ABORT,'injected failure'); END`)
	must(t, err)
	writeClaude(t, root, "p/r/subagents/agent-child.jsonl", child+`{"type":"assistant","uuid":"new","sessionId":"r","agentId":"child","message":{"role":"assistant","content":"new"}}`+"\n")
	r, err = Import(db, ImportOptions{Inputs: opts.Inputs})
	must(t, err)
	if len(r.Errors) == 0 {
		t.Fatal("expected failure")
	}
	state, err := db.GetImportState(s.InputID, path)
	must(t, err)
	after, err = db.GetIdentityMessages(s.InputID, s.Source, s.Identity)
	must(t, err)
	if !reflect.DeepEqual(old, state) || !reflect.DeepEqual(before, after) {
		t.Fatalf("partial write state=%+v text=%+v", state, after)
	}
	// A later successful metadata update must not hide an earlier title failure.
	_, err = db.db.Exec(`DROP TRIGGER fail_claude; CREATE TRIGGER fail_title BEFORE UPDATE OF custom_title ON sessions BEGIN SELECT RAISE(ABORT,'injected title failure'); END`)
	must(t, err)
	writeClaude(t, root, "p/r/subagents/agent-child.jsonl", child+`{"type":"custom-title","sessionId":"r","customTitle":"new title"}`+"\n"+`{"type":"agent-name","sessionId":"r","agentName":"new name"}`+"\n")
	r, err = Import(db, ImportOptions{Inputs: opts.Inputs})
	must(t, err)
	if len(r.Errors) == 0 {
		t.Fatal("expected title failure")
	}
	state, err = db.GetImportState(s.InputID, path)
	must(t, err)
	var name string
	must(t, db.db.QueryRow(`SELECT COALESCE(agent_name,'') FROM sessions WHERE input_id=? AND identity=?`, s.InputID, s.Identity).Scan(&name))
	if !reflect.DeepEqual(old, state) || name != "" {
		t.Fatalf("partial metadata write: state=%+v name=%q", state, name)
	}

}

func TestClaudeIncompleteSnapshotRetainsOwnerAndCursors(t *testing.T) {
	root := testTempDir(t)
	db := testDB(t)
	first := strings.Replace(claudeBody("", "first"), `"uuid":"same"`, `"uuid":"first"`, 1) + claudeCall("child-call") + claudeResult("child-call", "child")
	second := strings.Replace(claudeBody("", "second"), `"uuid":"same"`, `"uuid":"second"`, 1)
	writeClaude(t, root, "p1/r.jsonl", first)
	writeClaude(t, root, "p2/r.jsonl", second)
	writeClaude(t, root, "p1/r/subagents/agent-child.jsonl", claudeBody("child", "child"))
	importClaudeTest(t, db, root)
	inputID, err := db.EnsureInput(Input{Source: SourceClaudeCode, Root: root})
	must(t, err)
	sessions, err := db.ListSessions(SessionFilter{})
	must(t, err)
	messages, err := db.GetMessages(inputID, SourceClaudeCode, "r")
	must(t, err)
	if len(messages) != 2 {
		t.Fatalf("seed originals=%+v", messages)
	}
	child, err := db.LookupSessionREF(claudeREF(root, "r", "child"))
	must(t, err)
	if child == nil || child.ParentREF != claudeREF(root, "r") {
		t.Fatalf("seed relation=%+v", child)
	}
	paths := []string{"p1/r.jsonl", "p2/r.jsonl", "p1/r/subagents/agent-child.jsonl"}
	states := make([]*ImportState, len(paths))
	for i, path := range paths {
		states[i], err = db.GetImportState(inputID, filepath.Join(root, path))
		must(t, err)
	}
	// Keep the failed physical path discoverable while making ReadFile fail.
	failedPath := filepath.Join(root, "p2/r.jsonl")
	must(t, os.Remove(failedPath))
	must(t, os.Symlink(filepath.Join(root, "missing-target"), failedPath))
	appended := `{"type":"assistant","uuid":"new","sessionId":"r","message":{"role":"assistant","content":"new"}}` + "\n"
	writeClaude(t, root, "p1/r.jsonl", first+appended)
	for _, full := range []bool{false, true} {
		r, err := Import(db, ImportOptions{Full: full, Inputs: []Input{{Source: SourceClaudeCode, Root: root}}})
		must(t, err)
		if len(r.Errors) != 1 || r.FilesImported != 0 {
			t.Fatalf("full=%t result=%+v", full, r)
		}
		after, err := db.GetMessages(inputID, SourceClaudeCode, "r")
		must(t, err)
		if !reflect.DeepEqual(messages, after) {
			t.Fatalf("full=%t lost owner originals: %+v", full, after)
		}
		afterSessions, err := db.ListSessions(SessionFilter{})
		must(t, err)
		if !reflect.DeepEqual(sessions, afterSessions) {
			t.Fatalf("full=%t changed owner metadata or relations: %+v", full, afterSessions)
		}
		for i, path := range paths {
			state, err := db.GetImportState(inputID, filepath.Join(root, path))
			must(t, err)
			if !reflect.DeepEqual(states[i], state) {
				t.Fatalf("full=%t changed cursor %s: %+v", full, path, state)
			}
		}
	}
	must(t, os.Remove(failedPath))
	writeClaude(t, root, "p2/r.jsonl", second)
	importClaudeTest(t, db, root)
	after, err := db.GetMessages(inputID, SourceClaudeCode, "r")
	must(t, err)
	if len(after) != 3 || after[0].UUID != "first" || after[1].UUID != "new" || after[2].UUID != "second" {
		t.Fatalf("recovered originals=%+v", after)
	}
}

func TestClaudeBlankAppendPreservesOwnerAndImportTime(t *testing.T) {
	root := testTempDir(t)
	db := testDB(t)
	const firstTime = "2026-10-01T00:00:00Z"
	const secondTime = "2026-10-03T00:00:00Z"
	priorClock := timeNow
	timeNow = func() string { return firstTime }
	defer func() { timeNow = priorClock }()
	const relative = "p/r.jsonl"
	body := `{"type":"user","uuid":"u","sessionId":"r","timestamp":"2026-09-30T10:00:00+09:00","message":{"role":"user","content":[{"type":"text","text":" alpha "},{"type":"text","text":"\nend"}]}}` + "\n"
	writeClaude(t, root, relative, body)
	importClaudeTest(t, db, root)
	session, err := db.LookupSessionREF(claudeREF(root, "r"))
	must(t, err)
	if session == nil {
		t.Fatal("missing imported owner")
	}
	before, err := db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
	must(t, err)
	if len(before) != 1 || before[0].Number != 1 || before[0].Content != " alpha \n\n\nend" || before[0].OriginPath != relative || before[0].OriginLine != 1 || before[0].Timestamp != "2026-09-30T10:00:00+09:00" || !reflect.DeepEqual(before[0].Blocks, []string{" alpha ", "\nend"}) {
		t.Fatalf("initial originals=%+v", before)
	}
	readImportedAt := func() string {
		t.Helper()
		var value string
		must(t, db.db.QueryRow(`SELECT imported_at FROM sessions WHERE input_id=? AND identity=?`, session.InputID, session.Identity).Scan(&value))
		return value
	}
	if got := readImportedAt(); got != firstTime {
		t.Fatalf("initial imported_at=%s", got)
	}
	search := func(boundary string) []SearchGroup {
		t.Helper()
		groups, err := db.SearchGroups(SearchCandidates{ImportedSince: boundary}, SessionFilter{}, nil, false, "active")
		must(t, err)
		return groups
	}
	included := search(firstTime)
	excluded := search("2026-10-02T00:00:00Z")
	if len(included) != 1 || len(excluded) != 0 {
		t.Fatalf("initial search=%+v/%+v", included, excluded)
	}
	oldState, err := db.GetImportState(session.InputID, filepath.Join(root, relative))
	must(t, err)
	if oldState == nil {
		t.Fatal("missing initial import state")
	}
	timeNow = func() string { return secondTime }
	writeClaude(t, root, relative, body+"\n\n")
	result := importClaudeTest(t, db, root)
	after, err := db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
	must(t, err)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("originals changed: before=%+v after=%+v", before, after)
	}
	if got := readImportedAt(); got != firstTime {
		t.Fatalf("blank append changed imported_at: got %s want %s", got, firstTime)
	}
	if !reflect.DeepEqual(included, search(firstTime)) || !reflect.DeepEqual(excluded, search("2026-10-02T00:00:00Z")) {
		t.Fatal("blank append changed imported-since results")
	}
	state, err := db.GetImportState(session.InputID, filepath.Join(root, relative))
	must(t, err)
	if result.FilesImported != 1 || state == nil || state.ContentHash == oldState.ContentHash || state.FileSize != int64(len(body)+2) || state.LastOffset != state.FileSize {
		t.Fatalf("blank append did not advance state: result=%+v state=%+v", result, state)
	}
	if result = importClaudeTest(t, db, root); result.FilesSkipped != 1 || result.FilesImported != 0 {
		t.Fatalf("processed input not skipped: %+v", result)
	}
	writeClaude(t, root, relative, strings.Replace(body, "alpha", "changed", 1))
	importClaudeTest(t, db, root)
	changed, err := db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
	must(t, err)
	if len(changed) != 1 || !strings.Contains(changed[0].Content, "changed") || readImportedAt() != secondTime {
		t.Fatalf("changed body not imported: %+v", changed)
	}
}
