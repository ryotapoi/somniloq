package claudecode

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

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
