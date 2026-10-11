package core

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func TestCodexFullPreservesInputWhenIndexedOwnerDisappears(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		for _, replacement := range []struct {
			name string
			data string
		}{
			{"empty", ""},
			{"unidentified", `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"No owner"}]}}` + "\n"},
		} {
			t.Run(replacement.name+map[bool]string{false: "/single", true: "/multiple"}[multiple], func(t *testing.T) {
				db := testDB(t)
				root := testTempDir(t)
				names := []string{"child.jsonl", "root.jsonl"}
				if multiple {
					names = append(names, "child-extra.jsonl")
				}
				var paths []string
				for _, name := range names {
					copyCodexFixture(t, root, name)
					paths = append(paths, filepath.Join(root, name))
				}
				runCodexImport(t, db, root, false)
				id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
				must(t, err)
				oldMessages := map[string][]MessageRow{}
				oldSessions := map[string]*SessionRow{}
				for _, owner := range []string{"child", "root"} {
					oldMessages[owner], err = db.GetMessages(id, SourceCodex, owner)
					must(t, err)
					oldSessions[owner], err = db.GetSession(id, SourceCodex, owner)
					must(t, err)
				}
				oldStates := map[string]*ImportState{}
				for _, path := range paths {
					oldStates[path], err = db.GetImportState(id, path)
					must(t, err)
				}
				adapter := codex.NewAdapter(func(string) string { return "" })
				index, errs := adapter.BuildImportIndex(paths)
				if len(errs) != 0 {
					t.Fatal(errs)
				}
				must(t, os.WriteFile(filepath.Join(root, "child.jsonl"), []byte(replacement.data), 0600))
				result, err := replaceCodexInput(db, id, root, adapter, index, &ImportResult{FilesScanned: len(paths)}, "2099-01-01T00:00:00Z")
				must(t, err)
				if len(result.Errors) != 1 || result.FilesFailed != len(paths) || result.FilesImported != 0 {
					t.Fatalf("result=%+v", result)
				}
				for owner, before := range oldMessages {
					after, err := db.GetMessages(id, SourceCodex, owner)
					must(t, err)
					if !reflect.DeepEqual(after, before) {
						t.Fatalf("%s body changed: %+v", owner, after)
					}
					session, err := db.GetSession(id, SourceCodex, owner)
					must(t, err)
					if !reflect.DeepEqual(session, oldSessions[owner]) {
						t.Fatalf("%s session changed: %+v", owner, session)
					}
				}
				for path, before := range oldStates {
					after, err := db.GetImportState(id, path)
					must(t, err)
					if !reflect.DeepEqual(after, before) {
						t.Fatalf("%s cursor changed: %+v", path, after)
					}
				}
			})
		}
	}
}

func TestCodexFullReplacesIndexedOwnerWithValidShorterLog(t *testing.T) {
	db := testDB(t)
	root := testTempDir(t)
	copyCodexFixture(t, root, "child.jsonl")
	runCodexImport(t, db, root, false)
	id, err := db.EnsureInput(Input{Source: SourceCodex, Root: root})
	must(t, err)
	path := filepath.Join(root, "child.jsonl")
	adapter := codex.NewAdapter(func(string) string { return "" })
	index, errs := adapter.BuildImportIndex([]string{path})
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	data := []byte(`{"type":"session_meta","payload":{"id":"child","cwd":"/current"}}` + "\n" +
		`{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"Current body"}]}}` + "\n")
	oldState, err := db.GetImportState(id, path)
	must(t, err)
	if int64(len(data)) >= oldState.FileSize {
		t.Fatal("replacement must be shorter")
	}
	must(t, os.WriteFile(path, data, 0600))
	result, err := replaceCodexInput(db, id, root, adapter, index, &ImportResult{FilesScanned: 1}, "2099-01-01T00:00:00Z")
	must(t, err)
	if len(result.Errors) != 0 || result.FilesFailed != 0 || result.FilesImported != 1 {
		t.Fatalf("result=%+v", result)
	}
	messages, err := db.GetMessages(id, SourceCodex, "child")
	must(t, err)
	if len(messages) != 1 || messages[0].Content != "Current body" {
		t.Fatalf("current body=%+v", messages)
	}
	session, err := db.GetSession(id, SourceCodex, "child")
	must(t, err)
	if session == nil || session.CWD != "/current" {
		t.Fatalf("current session=%+v", session)
	}
	state, err := db.GetImportState(id, path)
	must(t, err)
	sum := sha256.Sum256(data)
	if state == nil || state.FileSize != int64(len(data)) || state.LastOffset != int64(len(data)) || state.ContentHash != hex.EncodeToString(sum[:]) || state.ImportedAt != "2099-01-01T00:00:00Z" {
		t.Fatalf("current cursor=%+v", state)
	}
}

func TestCodexImportIndexKeepsUnidentifiedOwnerHandling(t *testing.T) {
	index := codex.Group{Reports: []codex.FileReport{{Path: "unidentified.jsonl"}}}
	loaded := []codex.Group{{Reports: []codex.FileReport{{Path: "unidentified.jsonl"}}}}
	if !sameCodexImportIndex(index, loaded) {
		t.Fatal("unchanged unidentified owner should still match")
	}
	loaded[0].Session.SessionID = "new-owner"
	if sameCodexImportIndex(index, loaded) {
		t.Fatal("newly identified owner should remain an index mismatch")
	}
}
