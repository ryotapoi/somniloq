package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

func TestGroupRepoResolutionLifetime(t *testing.T) {
	for _, migration := range []bool{false, true} {
		t.Run(fmt.Sprintf("migration=%t", migration), func(t *testing.T) {
			root := t.TempDir()
			var paths []string
			for _, id := range []string{"a", "b", "c"} {
				path := filepath.Join(root, id+".jsonl")
				cwd := "/cwd"
				if id == "c" {
					cwd = "/other"
				}
				data := fmt.Sprintf(`{"type":"session_meta","payload":{"id":%q,"cwd":%q}}`+"\n"+`{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"text","text":"hello"}]}}`+"\n", id, cwd)
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}
			calls := 0
			result := "/cwd" // A failed lookup's fallback is also reused within this pass.
			adapter := NewAdapter(func(cwd string) string {
				calls++
				if cwd == "/other" {
					return "/other-repository"
				}
				return result
			})
			build := adapter.BuildGroups
			if migration {
				build = adapter.BuildMigrationGroups
			}
			for _, want := range []string{"/cwd", "/new-repository"} {
				result = want
				groups, errs := build(root, paths, "")
				if len(errs) != 0 || len(groups) != 3 {
					t.Fatalf("groups=%+v errors=%v", groups, errs)
				}
				for _, g := range groups {
					expected := want
					if g.Session.CWD == "/other" {
						expected = "/other-repository"
					}
					if g.Err != nil || g.Session.RepoPath != expected {
						t.Fatalf("group=%+v want repo=%q", g, expected)
					}
				}
			}
			// Git work must scale with distinct cwd, but must be retried next pass.
			if calls != 4 {
				t.Fatalf("resolution calls=%d, want one per distinct cwd per pass", calls)
			}
		})
	}
}

func TestUnadoptedMetadataValidationWithoutRepoResolution(t *testing.T) {
	for _, suffix := range []string{
		`{"id":"parent","cwd":"/parent"}`,
		`{"id":"parent","cwd":"/parent","source":{"subagent":{"thread_spawn":{"parent_thread_id":123}}}}`,
		`{"id":"","cwd":"/parent"}`,
		`{"id":"parent","cwd":123}`,
	} {
		t.Run(suffix, func(t *testing.T) {
			calls := 0
			h := fileHandler{resolveRepoPath: func(cwd string) string { calls++; return cwd }, meta: &ingest.SessionMeta{SessionID: "child", ParentSessionID: "parent"}}
			rec := &RawRecord{Type: "session_meta", Payload: []byte(suffix)}
			err := h.applySessionMeta(rec)
			wantErr := suffix != `{"id":"parent","cwd":"/parent"}`
			if (err != nil) != wantErr {
				t.Fatalf("error=%v want error=%t", err, wantErr)
			}
			if calls != 0 || h.meta.SessionID != "child" {
				t.Fatalf("calls=%d owner=%q", calls, h.meta.SessionID)
			}
		})
	}
}
