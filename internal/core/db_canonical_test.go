package core

import (
	"reflect"
	"testing"
)

func TestCanonicalStorageParentAndBodyBoundary(t *testing.T) {
	db := testDB(t)
	inputA := testInput(t, db, SourceCodex)
	inputB, err := db.EnsureInput(Input{Source: SourceCodex, Root: "/other-codex-input"})
	must(t, err)
	child := SessionMeta{Source: SourceCodex, SessionID: "child", ParentSessionID: "root"}
	must(t, db.UpsertSession(inputA, child, "imported"))
	must(t, db.UpsertSession(inputB, SessionMeta{Source: SourceCodex, SessionID: "root"}, "imported"))
	readChild := func() SessionRow {
		t.Helper()
		row, err := db.GetSession(inputA, SourceCodex, "child")
		must(t, err)
		if row == nil {
			t.Fatal("missing child")
		}
		return *row
	}
	if got := readChild(); got.ParentSessionID != "root" || got.ParentREF != "" {
		t.Fatalf("cross-input parent resolved: %+v", got)
	}
	must(t, db.UpsertSession(inputA, SessionMeta{Source: SourceCodex, SessionID: "root"}, "imported"))
	root, err := db.GetSession(inputA, SourceCodex, "root")
	must(t, err)
	if got := readChild(); got.ParentREF != root.REF {
		t.Fatalf("same-input parent not resolved: %+v", got)
	}
	if err := db.UpsertSession(inputA, SessionMeta{Source: SourceCodex, SessionID: "root", ParentSessionID: "child"}, "imported"); err == nil {
		t.Fatal("cycle was accepted")
	}

	for _, msg := range []NormalizedMessage{
		{Source: SourceCodex, SessionID: "child", UUID: "context", Role: "user", Content: "inherited", Blocks: []string{"inherited"}, Membership: "context"},
		{Source: SourceCodex, SessionID: "child", UUID: "second", Role: "assistant", Content: " a \n\n b ", Blocks: []string{" a ", " b "}, Number: 2, Membership: "body", OriginPath: "later.jsonl", OriginLine: 7, PayloadID: "id-2", IsSidechain: true},
		{Source: SourceCodex, SessionID: "child", UUID: "first", Role: "user", Content: "first", Blocks: []string{"first"}, Number: 1, Membership: "body", OriginPath: "early.jsonl", OriginLine: 3},
		{Source: SourceCodex, SessionID: "child", UUID: "unresolved", Role: "assistant", Content: "unknown", Blocks: []string{"unknown"}, Membership: "unresolved"},
	} {
		must(t, db.InsertMessage(inputA, msg))
	}
	rows, err := db.GetMessages(inputA, SourceCodex, "child")
	must(t, err)
	if len(rows) != 2 || rows[0].UUID != "first" || rows[1].UUID != "second" || !reflect.DeepEqual(rows[1].Blocks, []string{" a ", " b "}) || rows[1].OriginPath != "later.jsonl" || rows[1].OriginLine != 7 || rows[1].PayloadID != "id-2" {
		t.Fatalf("body/read order or provenance: %+v", rows)
	}
	if got := readChild(); got.MessageCount != 2 || got.BodySize != len("first")+len(" a \n\n b ") {
		t.Fatalf("body summary: %+v", got)
	}
	matcher, err := CompilePatterns([]string{"inherited"}, false)
	must(t, err)
	search, err := db.SearchOccurrences(readChild().REF, SessionFilter{}, matcher, false, SearchCandidates{})
	must(t, err)
	if len(search) != 0 {
		t.Fatalf("context matched search: %+v", search)
	}
}
