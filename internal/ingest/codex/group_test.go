package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCodexFixtureOracle(t *testing.T) {
	root := filepath.Join("..", "testdata", "v0.14.0")
	data, err := os.ReadFile(filepath.Join(root, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Cases []struct {
			Path       string
			Identity   []string
			Parent     []string
			Utterances []struct {
				Line      int
				Number    int
				Role      string
				Text      string
				Timestamp *string
			}
			ContextLines    []int `json:"context_lines"`
			UnresolvedLines []int `json:"unresolved_lines"`
		}
	}
	if err := json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	for _, c := range oracle.Cases {
		if len(c.Path) < 6 || c.Path[:6] != "codex/" || c.Path == "codex/input-a/child-extra.jsonl" {
			continue
		}
		t.Run(c.Path, func(t *testing.T) {
			path := filepath.Join(root, c.Path)
			groups, errs := NewAdapter(func(cwd string) string { return cwd }).BuildGroups(filepath.Dir(path), []string{path}, "2026-10-02T00:00:00Z")
			if len(errs) != 0 || len(groups) != 1 || groups[0].Err != nil {
				t.Fatalf("groups=%+v errors=%v", groups, errs)
			}
			g := groups[0]
			if g.Session.SessionID != c.Identity[0] {
				t.Fatalf("owner=%q", g.Session.SessionID)
			}
			parent := ""
			if len(c.Parent) > 0 {
				parent = c.Parent[0]
			}
			if g.Session.ParentSessionID != parent {
				t.Fatalf("parent=%q want %q", g.Session.ParentSessionID, parent)
			}
			var contexts, unresolved []int
			var bodies int
			for _, m := range g.Messages {
				switch m.Membership {
				case "context":
					contexts = append(contexts, m.OriginLine)
				case "unresolved":
					unresolved = append(unresolved, m.OriginLine)
				case "body":
					if bodies >= len(c.Utterances) {
						t.Fatalf("unexpected body %+v", m)
					}
					want := c.Utterances[bodies]
					bodies++
					timestamp := ""
					if want.Timestamp != nil {
						timestamp = *want.Timestamp
					}
					if m.OriginLine != want.Line || m.Number != want.Number || m.Role != want.Role || m.Content != want.Text || m.Timestamp != timestamp {
						t.Fatalf("body=%+v want=%+v", m, want)
					}
				}
			}
			if bodies != len(c.Utterances) || !sameInts(contexts, c.ContextLines) || !sameInts(unresolved, c.UnresolvedLines) {
				t.Fatalf("bodies=%d context=%v unresolved=%v", bodies, contexts, unresolved)
			}
		})
	}
}
func sameInts(a, b []int) bool { return len(a) == len(b) && (len(a) == 0 || reflect.DeepEqual(a, b)) }

func TestCodexPreservesOwnerMetadataBlocksAndInvalidRawTimestamp(t *testing.T) {
	const data = `{"type":"session_meta","timestamp":"2026-10-01T00:00:00Z","payload":{"id":"child","cwd":"/child","cli_version":"child-version","git":{"branch":"child-branch"}}}
{"type":"session_meta","payload":{"id":"parent","cwd":"/parent","cli_version":"parent-version"}}
{"type":"response_item","timestamp":"bad-time","payload":{"type":"message","role":"assistant","content":[{"type":"output_text","text":" first "},{"type":"output_text","text":""},{"type":"text","text":"\nlast\n"}]}}
{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":" \n "}]}}
`
	path := filepath.Join(t.TempDir(), "rollout.jsonl")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	groups, errs := NewAdapter(func(cwd string) string { return cwd }).BuildGroups(filepath.Dir(path), []string{path}, "")
	if len(errs) > 0 || len(groups) != 1 {
		t.Fatalf("%+v %v", groups, errs)
	}
	g := groups[0]
	if g.Session.CWD != "/child" || g.Session.Version != "child-version" || g.Session.GitBranch != "child-branch" {
		t.Fatalf("owner metadata %+v", g.Session)
	}
	if len(g.Messages) != 1 || !reflect.DeepEqual(g.Messages[0].Blocks, []string{" first ", "", "\nlast\n"}) || g.Messages[0].Timestamp != "bad-time" || g.Messages[0].Number != 1 {
		t.Fatalf("messages %+v", g.Messages)
	}
}
