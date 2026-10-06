package core

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodexCyclicParentsRetainBodies(t *testing.T) {
	for _, initial := range []string{"normal", "full", "partial"} {
		t.Run(initial, func(t *testing.T) {
			db := testDB(t)
			root := testTempDir(t)
			write := func(id, parent string) {
				t.Helper()
				data := fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"source":{"subagent":{"thread_spawn":{"parent_thread_id":%q}}}}}`+"\n"+`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":%q}]}}`+"\n", id, parent, id+" body")
				must(t, os.WriteFile(filepath.Join(root, id+".jsonl"), []byte(data), 0600))
			}
			write("A", "B")
			if initial == "partial" {
				// A and its cursor are the successfully saved prefix of the old import.
				runCodexImport(t, db, root, false)
			}
			write("B", "A")
			write("leaf", "A")
			runCodexImport(t, db, root, initial == "full")
			inputID, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
			must(t, err)
			a, b, leaf := relationREF(root, SourceCodex, "A"), relationREF(root, SourceCodex, "B"), relationREF(root, SourceCodex, "leaf")
			read := func() map[string][]MessageRow {
				t.Helper()
				bodies := map[string][]MessageRow{}
				for id, parent := range map[string]string{"A": "B", "B": "A", "leaf": "A"} {
					rows, err := db.GetMessages(inputID, SourceCodex, id)
					must(t, err)
					if len(rows) != 1 || rows[0].Content != id+" body" || rows[0].Number != 1 {
						t.Fatalf("%s body=%+v", id, rows)
					}
					saved, err := db.GetSession(inputID, SourceCodex, id)
					must(t, err)
					if saved == nil || saved.ParentSessionID != parent || saved.ParentIdentity != rootIdentity(parent) {
						t.Fatalf("%s raw parent=%+v", id, saved)
					}
					state, err := db.GetImportState(inputID, filepath.Join(root, id+".jsonl"))
					must(t, err)
					if state == nil {
						t.Fatalf("missing cursor: %s", id)
					}
					bodies[id] = rows
				}
				ar, br, lr := requireResolution(t, db, a), requireResolution(t, db, b), requireResolution(t, db, leaf)
				wantDiagnostics := []string{"unconfirmed cyclic parent: " + a, "unconfirmed cyclic parent: " + b}
				if !reflect.DeepEqual(ar.Diagnostics, wantDiagnostics) || ar.Parent != nil || br.Parent != nil || ar.GroupKey == br.GroupKey || !reflect.DeepEqual(refs(ar.Descendants), []string{a, leaf}) || !reflect.DeepEqual(refs(br.Descendants), []string{b}) || lr.Parent == nil || lr.Parent.REF != a || lr.GroupKey != ar.GroupKey {
					t.Fatalf("relations A=%+v B=%+v leaf=%+v", ar, br, lr)
				}
				return bodies
			}
			before := read()
			if r := runCodexImport(t, db, root, false); r.FilesSkipped != 3 {
				t.Fatalf("unchanged=%+v", r)
			}
			if got := read(); !reflect.DeepEqual(got, before) {
				t.Fatal("unchanged import changed bodies")
			}
			for range 2 {
				runCodexImport(t, db, root, true)
				if got := read(); !reflect.DeepEqual(got, before) {
					t.Fatalf("full=%+v normal=%+v", got, before)
				}
			}
		})
	}
}
