package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBuildSnapshotsReusesRepositoryPerCWDWithinPass(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")}
	inputs := []struct {
		cwd, branch, role string
	}{
		{"/project/a", "main", "user"},
		{"/project/a", "feature", "assistant"},
		{"/project/b", "other", "user"},
		{"/not-a-repository", "", "user"},
	}
	for _, path := range paths {
		var data []byte
		for i, input := range inputs {
			record, err := json.Marshal(RawRecord{
				Type: input.role, UUID: string(rune('a' + i)), SessionID: "session",
				CWD: input.cwd, GitBranch: input.branch,
				Message: json.RawMessage(`{"role":"` + input.role + `","content":"text"}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, record...)
			data = append(data, '\n')
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	repositories := map[string]string{"/project/a": "/repo/a", "/project/b": "/repo/b", "/not-a-repository": ""}
	calls := map[string]int{}
	adapter := NewAdapter(func(cwd string) string {
		calls[cwd]++
		return repositories[cwd]
	})
	for pass := 1; pass <= 2; pass++ {
		files, readErrors, diagnostics := adapter.BuildSnapshots(root, paths, "now")
		if len(readErrors) != 0 || len(diagnostics) != 0 || len(files) != len(paths) {
			t.Fatalf("files=%d readErrors=%v diagnostics=%v", len(files), readErrors, diagnostics)
		}
		for _, file := range files {
			if len(file.Failures) != 0 || len(file.Records) != len(inputs) {
				t.Fatalf("records=%d failures=%v", len(file.Records), file.Failures)
			}
			for i, record := range file.Records {
				input := inputs[i]
				if record.Session.RepoPath != repositories[input.cwd] || record.Session.GitBranch != input.branch {
					t.Fatalf("pass=%d cwd=%q repo=%q branch=%q", pass, input.cwd, record.Session.RepoPath, record.Session.GitBranch)
				}
			}
		}
		for cwd := range repositories {
			if calls[cwd] != pass {
				t.Fatalf("pass=%d cwd=%q resolver calls=%d want=%d", pass, cwd, calls[cwd], pass)
			}
		}
		repositories["/project/a"] = "/repo/changed"
	}
}

func TestResolveParents(t *testing.T) {
	for _, tc := range []struct {
		name        string
		candidates  map[string]map[string]bool
		want        map[string]string
		diagnostics int
	}{
		{"one", map[string]map[string]bool{"child": {"root": true}}, map[string]string{"child": "root"}, 0},
		{"conflict", map[string]map[string]bool{"child": {"root": true, "sibling": true}}, map[string]string{}, 1},
		{"cycle", map[string]map[string]bool{"a": {"b": true}, "b": {"a": true}, "leaf": {"a": true}}, map[string]string{"leaf": "a"}, 2},
		{"self", map[string]map[string]bool{"a": {"a": true}}, map[string]string{}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, diagnostics := resolveParents(tc.candidates)
			if !reflect.DeepEqual(got, tc.want) || len(diagnostics) != tc.diagnostics {
				t.Fatalf("parents=%v diagnostics=%v", got, diagnostics)
			}
		})
	}
}

func TestPhysicalParentEvidenceBoundaries(t *testing.T) {
	call := `{"type":"assistant","uuid":"call","sessionId":"r","message":{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"Task"}]}}` + "\n"
	result := `{"type":"user","uuid":"result","sessionId":"r","toolUseResult":{"agentId":"child"},"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"a"}]}}` + "\n"
	child := `{"type":"user","uuid":"child","sessionId":"r","agentId":"child","message":{"role":"user","content":"prompt with agentId child"}}` + "\n"
	for _, tc := range []struct {
		name, root, sibling string
		parent              bool
	}{
		{"Task duplicate evidence", call + result + result, "", true},
		{"different physical file", call, result, false},
		{"different call", call + strings.ReplaceAll(result, `"tool_use_id":"a"`, `"tool_use_id":"b"`), "", false},
		{"different agent", call + strings.ReplaceAll(result, `"agentId":"child"`, `"agentId":"other"`), "", false},
		{"text ID only", call + `{"type":"user","uuid":"text","sessionId":"r","message":{"role":"user","content":"agentId child"}}` + "\n", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "p/r.jsonl")
			childPath := filepath.Join(root, "p/r/subagents/agent-child.jsonl")
			if err := os.MkdirAll(filepath.Dir(childPath), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.root), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(childPath, []byte(child), 0644); err != nil {
				t.Fatal(err)
			}
			paths := []string{path, childPath}
			if tc.sibling != "" {
				siblingPath := filepath.Join(root, "p/r/subagents/agent-sibling.jsonl")
				body := strings.ReplaceAll(tc.sibling, `"sessionId":"r"`, `"sessionId":"r","agentId":"sibling"`)
				if err := os.WriteFile(siblingPath, []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, siblingPath)
			}
			files, errors, _ := NewAdapter(func(cwd string) string { return cwd }).BuildSnapshots(root, paths, "now")
			if len(errors) > 0 {
				t.Fatal(errors)
			}
			found := false
			for _, f := range files {
				for _, r := range f.Records {
					if r.Session.Identity == `["r","child"]` {
						found = true
						want := ""
						if tc.parent {
							want = `["r"]`
						}
						if r.Session.ParentIdentity != want {
							t.Fatalf("parent=%q want=%q", r.Session.ParentIdentity, want)
						}
					}
				}
			}
			if !found {
				t.Fatal("missing child")
			}
		})
	}
}
