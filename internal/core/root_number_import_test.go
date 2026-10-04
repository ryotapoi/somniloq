package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootBodyNumbersContinueAcrossAppend(t *testing.T) {
	for _, tc := range []struct {
		name, relativePath, sessionID, first, appendLine string
		source                                           Source
	}{
		{
			name: "Claude Code", source: SourceClaudeCode, relativePath: "project/root.jsonl", sessionID: "root",
			first: `{"type":"user","uuid":"u1","sessionId":"root","message":{"role":"user","content":"first"}}` + "\n" +
				`{"type":"assistant","uuid":"side","sessionId":"root","isSidechain":true,"message":{"role":"assistant","content":"sidechain"}}` + "\n" +
				`{"type":"user","uuid":"empty","sessionId":"root","message":{"role":"user","content":"  "}}` + "\n" +
				"{broken\n",
			appendLine: `{"type":"assistant","uuid":"a2","sessionId":"root","message":{"role":"assistant","content":"second"}}` + "\n",
		},
		{
			name: "Cursor Agent", source: SourceCursorAgent, relativePath: "project/agent-transcripts/root/root.jsonl", sessionID: "root",
			first: `{"role":"user","message":{"content":[{"type":"text","text":"first"}]}}` + "\n" +
				`{"role":"user","message":{"content":[{"type":"text","text":"  "}]}}` + "\n" +
				"{broken\n",
			appendLine: `{"role":"assistant","message":{"content":[{"type":"text","text":"second"}]}}` + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			root := testTempDir(t)
			path := filepath.Join(root, tc.relativePath)
			must(t, os.MkdirAll(filepath.Dir(path), 0o755))
			must(t, os.WriteFile(path, []byte(tc.first), 0o644))
			options := ImportOptions{Inputs: []Input{{Source: tc.source, Root: root}}}
			_, err := Import(db, options)
			must(t, err)
			must(t, os.WriteFile(path, []byte(tc.first+tc.appendLine), 0o644))
			_, err = Import(db, options)
			must(t, err)
			inputID, err := db.EnsureInput(Input{Source: tc.source, Root: root})
			must(t, err)
			rows, err := db.GetMessages(inputID, tc.source, tc.sessionID)
			must(t, err)
			want := []string{"first", "second"}
			if tc.source == SourceClaudeCode {
				want = []string{"first", "sidechain", "second"}
			}
			if len(rows) != len(want) {
				t.Fatalf("body numbers after append = %+v", rows)
			}
			for i, text := range want {
				if rows[i].Content != text || rows[i].Number != i+1 {
					t.Fatalf("body numbers after append = %+v", rows)
				}
			}
		})
	}
}
