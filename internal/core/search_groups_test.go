package core

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSearchGroupsCandidatesAndMetadata(t *testing.T) {
	db := testDB(t)
	inputs := []struct{ key, root, source string }{{"a", "/input/a", "codex"}, {"b", "/input/b", "codex"}, {"c", "/input/c", "claude_code"}, {"d", "/input/d", "cursor_agent"}}
	for i, in := range inputs {
		_, err := db.db.Exec(`INSERT INTO inputs(id,input_key,source,root) VALUES(?,?,?,?)`, i+1, strings.Repeat(in.key, 64), in.source, in.root)
		must(t, err)
	}
	for _, s := range []struct {
		input                                              int
		source, id, parent, root, project, title, imported string
	}{
		{1, "codex", `["root"]`, "", "", "/parent/App", "root title", "2026-10-05T01:00:00+09:00"},
		{1, "codex", `["child"]`, `["root"]`, "", "/parent/Other", "child title", "2026-10-04T17:00:00Z"},
		{1, "codex", `["orphan"]`, `["missing"]`, "", "/parent/App", "orphan title", "2026-10-04T10:00:00Z"},
		{1, "codex", `["empty"]`, "", "", "", "", "2026-10-04T10:00:00Z"},
		{2, "codex", `["root"]`, "", "", "/parent/App", "other input", "2026-10-04T10:00:00Z"},
		{3, "claude_code", `["root"]`, "", `["root"]`, "/parent/App", "claude root", "2026-10-04T10:00:00Z"},
		{3, "claude_code", `["root","unknown"]`, "", `["root"]`, "/parent/Other", "root-only", "2026-10-04T10:00:00Z"},
		{4, "cursor_agent", `["root"]`, "", "", "/parent/App", "cursor", "2026-10-04T10:00:00Z"},
	} {
		_, err := db.db.Exec(`INSERT INTO sessions(input_id,source,identity,session_id,parent_identity,root_identity,repo_path,custom_title,imported_at,started_at) VALUES(?,?,?,?,?,?,?,?,?,'1999-01-01T00:00:00Z')`, s.input, s.source, s.id, s.id, s.parent, s.root, s.project, s.title, s.imported)
		must(t, err)
	}
	for _, m := range []struct {
		input                      int
		source, id, content, stamp string
	}{
		{1, "codex", `["root"]`, "alpha", "2026-10-01T10:00:00+09:00"}, {1, "codex", `["child"]`, "beta", "2026-10-01T02:00:00Z"}, {1, "codex", `["orphan"]`, "alpha beta", "bad"},
		{2, "codex", `["root"]`, "alpha", "2026-10-01T11:00:00+09:00"}, {3, "claude_code", `["root"]`, "alpha", "2026-10-01T01:00:00Z"}, {3, "claude_code", `["root","unknown"]`, "beta", "2026-10-01T03:00:00Z"}, {4, "cursor_agent", `["root"]`, "alpha", ""},
	} {
		_, err := db.db.Exec(`INSERT INTO messages(input_id,source,identity,session_id,uuid,role,content,timestamp,number) VALUES(?,?,?,?,?,'user',?,?,1)`, m.input, m.source, m.id, m.id, m.id, m.content, m.stamp)
		must(t, err)
	}
	digest := strings.Repeat("a", 64)
	_, err := db.db.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,repo_path,imported_at) VALUES(?,'codex','root','/parent/App','2026-10-04T10:00:00Z')`, digest)
	must(t, err)
	_, err = db.db.Exec(`INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,timestamp,number) VALUES(1,?,'saved','codex','root','user','alpha beta','',1)`, digest)
	must(t, err)
	query := func(c SearchCandidates, filter SessionFilter, patterns []string, all bool) []SearchGroup {
		t.Helper()
		var matcher *PatternMatcher
		if len(patterns) > 0 {
			matcher, err = CompilePatterns(patterns, false)
			must(t, err)
		}
		var items []SearchGroup
		must(t, db.ReadSnapshot(func(s *DB) error { items, err = s.SearchGroups(c, filter, matcher, all); return err }))
		return items
	}
	items := query(SearchCandidates{}, SessionFilter{}, nil, false)
	if len(items) != 7 {
		t.Fatalf("all groups: %+v", items)
	}
	if items[0].Source != SourceClaudeCode || items[1].REF != IdentityREF(strings.Repeat("a", 64), SourceCodex, `["root"]`) {
		t.Fatal(items)
	}
	if items[2].REF != IdentityREF(strings.Repeat("b", 64), SourceCodex, `["root"]`) {
		t.Fatal("same instant tie must use group key", items)
	}
	root := items[1]
	if root.Project == nil || *root.Project != "/parent/App" || root.Title == nil || *root.Title != "root title" || root.StartedAt == nil || *root.StartedAt != "2026-10-01T10:00:00+09:00" || root.LastAt == nil || *root.LastAt != "2026-10-01T02:00:00Z" || root.ImportedAt == nil || *root.ImportedAt != "2026-10-04T17:00:00Z" || root.MemberCount != 2 {
		t.Fatal(root)
	}
	for _, c := range []struct {
		candidate SearchCandidates
		patterns  []string
		all       bool
		want      int
	}{
		{SearchCandidates{}, []string{"alpha", "beta"}, true, 4},
		{SearchCandidates{Projects: []string{"Other"}}, []string{"alpha", "beta"}, true, 0},
		{SearchCandidates{Projects: []string{"Other"}}, []string{"beta"}, false, 2},
		{SearchCandidates{Projects: []string{"parent"}}, nil, false, 0},
		{SearchCandidates{Projects: []string{"app"}}, nil, false, 0},
		{SearchCandidates{Inputs: []string{"/input/a", "/input/b"}, Sources: []Source{SourceCodex}, Projects: []string{"App"}}, nil, false, 3},
	} {
		result := query(c.candidate, SessionFilter{}, c.patterns, c.all)
		if len(result) != c.want {
			t.Fatalf("%+v = %+v", c, result)
		}
		if reflect.DeepEqual(c.patterns, []string{"beta"}) {
			for _, g := range result {
				if g.MemberCount != 2 || len(g.MatchedMembers) != 1 {
					t.Fatal(g)
				}
			}
		}
	}
	items = query(SearchCandidates{Inputs: []string{"/input/a"}}, SessionFilter{}, []string{"alpha", "beta"}, true)
	for _, g := range items {
		if g.MemberCount == 1 {
			if g.Project != nil || g.Title != nil || g.LastAt != nil || len(g.Members) != 1 {
				t.Fatal("missing parent", g)
			}
		}
	}
	items = query(SearchCandidates{}, SessionFilter{Since: "2026-10-01T00:00:00Z"}, nil, false)
	if len(items) != 3 {
		t.Fatal(items)
	}
}

func TestSearchGroupsSnapshotConsistentWithWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.db")
	db, err := OpenDB(path)
	must(t, err)
	defer db.Close()
	_, err = db.db.Exec(`PRAGMA journal_mode=WAL`)
	must(t, err)
	writer, err := OpenDB(path)
	must(t, err)
	defer writer.Close()
	must(t, db.ReadSnapshot(func(snapshot *DB) error {
		before, e := snapshot.SearchGroups(SearchCandidates{}, SessionFilter{}, nil, false)
		if e != nil {
			return e
		}
		_, e = writer.db.Exec(`INSERT INTO inputs(id,input_key,source,root) VALUES(1,'a','codex','/a'); INSERT INTO sessions(input_id,source,session_id,identity,imported_at) VALUES(1,'codex','new','["new"]','2026-10-05T00:00:00Z')`)
		if e != nil {
			return e
		}
		after, e := snapshot.SearchGroups(SearchCandidates{}, SessionFilter{}, nil, false)
		if e != nil {
			return e
		}
		if len(before) != 0 || len(after) != 0 {
			t.Fatalf("snapshot drift: %v %v", before, after)
		}
		return nil
	}))
	after, err := db.SearchGroups(SearchCandidates{}, SessionFilter{}, nil, false)
	must(t, err)
	if len(after) != 1 {
		t.Fatal(after)
	}
}
