package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationGroupsRetainAllPhysicalEvidenceAndMetadataOnlyRollouts(t *testing.T) {
	root := t.TempDir()
	meta := `{"type":"session_meta","payload":{"id":"owner"}}` + "\n"
	body := `{"type":"response_item","payload":{"id":"same","type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	paths := []string{filepath.Join(root, "c.jsonl"), filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")}
	for i, data := range []string{meta, meta + "\n" + body, meta + body} {
		if err := os.WriteFile(paths[i], []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	groups, errs := NewAdapter(func(s string) string { return s }).BuildMigrationGroups(root, paths, "")
	if len(errs) != 0 || len(groups) != 1 {
		t.Fatalf("groups=%+v errors=%v", groups, errs)
	}
	g := groups[0]
	if g.Err != nil || g.Files != 3 || len(g.Messages) != 1 || len(g.States) != 3 {
		t.Fatalf("group=%+v", g)
	}
	if len(g.Reports[0].Lines) != 3 || len(g.Reports[1].Lines) != 2 || len(g.Reports[2].Lines) != 1 {
		t.Fatalf("reports=%+v", g.Reports)
	}
	if g.Reports[0].Lines[2].UUID != messageUUID(paths[1], 3) || g.Reports[1].Lines[1].UUID != messageUUID(paths[2], 2) {
		t.Fatal("physical payload duplicates lost their identities")
	}
	for _, state := range g.States {
		if state.LastOffset != state.FileSize {
			t.Fatalf("not EOF: %+v", state)
		}
	}
}

func TestMigrationGroupsRejectUnsafeOwnershipWithoutChangingImportContract(t *testing.T) {
	meta := `{"type":"session_meta","payload":{"id":"owner"}}` + "\n"
	body := `{"type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}}` + "\n"
	cases := map[string]string{
		"unfinished":        meta + body + `{"type":`,
		"invalid":           meta + "invalid\n" + body,
		"before metadata":   body + meta + body,
		"owner conflict":    meta + `{"type":"session_meta","payload":{"id":"other"}}` + "\n" + body,
		"parent conflict":   meta + `{"type":"session_meta","payload":{"id":"owner","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}}}` + "\n" + body,
		"boundary conflict": meta + `{"type":"session_meta","payload":{"id":"owner","subagent_history_start_ordinal":3}}` + "\n" + body,
		"missing ordinal":   `{"type":"session_meta","payload":{"id":"owner","subagent_history_start_ordinal":3}}` + "\n" + body,
		"payload conflict":  meta + strings.Replace(body, `"type":"message"`, `"id":"same","type":"message"`, 1) + strings.Replace(strings.Replace(body, `"type":"message"`, `"id":"same","type":"message"`, 1), "hello", "different", 1),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "rollout.jsonl")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			adapter := NewAdapter(func(s string) string { return s })
			groups, errs := adapter.BuildMigrationGroups(root, []string{path}, "")
			if len(errs) != 0 || len(groups) != 1 || groups[0].Err == nil {
				t.Fatalf("groups=%+v errors=%v", groups, errs)
			}
			normal, errs := adapter.BuildGroups(root, []string{path}, "")
			if len(errs) != 0 || len(normal) != 1 || (name != "payload conflict" && normal[0].Err != nil) {
				t.Fatalf("ordinary import changed: %+v %v", normal, errs)
			}
		})
	}
}

func TestMigrationGroupsMergeMetadataOnlyFailuresWithOwner(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")}
	meta := `{"type":"session_meta","payload":{"id":"owner"}}` + "\n"
	for i, data := range []string{meta + "bad\n", meta + `{"type":"response_item","payload":{"type":"message","role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"} {
		if err := os.WriteFile(paths[i], []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	groups, errs := NewAdapter(func(s string) string { return s }).BuildMigrationGroups(root, paths, "")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Session.SessionID != "owner" || groups[0].Files != 2 || groups[0].Err == nil {
		t.Fatalf("groups=%+v errs=%v", groups, errs)
	}
}

func TestMigrationGroupsSeparateInheritedMetadataAndDetectCrossRolloutBoundaryConflict(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")}
	child := `{"type":"session_meta","payload":{"id":"child","subagent_history_start_ordinal":8,"source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}}}` + "\n"
	embedded := `{"type":"session_meta","payload":{"id":"parent","subagent_history_start_ordinal":2}}` + "\n"
	body := `{"type":"response_item","ordinal":8,"payload":{"type":"message","role":"assistant","content":[{"type":"text","text":"hello"}]}}` + "\n"
	if err := os.WriteFile(paths[0], []byte(child+embedded+body), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewAdapter(func(s string) string { return s })
	groups, errs := adapter.BuildMigrationGroups(root, paths[:1], "")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Err != nil || groups[0].Messages[0].Membership != "body" {
		t.Fatalf("groups=%+v errs=%v", groups, errs)
	}
	if err := os.WriteFile(paths[1], []byte(strings.Replace(child, `ordinal":8`, `ordinal":9`, 1)), 0600); err != nil {
		t.Fatal(err)
	}
	groups, errs = adapter.BuildMigrationGroups(root, paths, "")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Err == nil {
		t.Fatalf("groups=%+v errs=%v", groups, errs)
	}
}

func TestMigrationGroupsKeepUnidentifiedFailedRollout(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "unknown.jsonl")
	if err := os.WriteFile(path, []byte("\ninvalid"), 0600); err != nil {
		t.Fatal(err)
	}
	groups, errs := NewAdapter(func(s string) string { return s }).BuildMigrationGroups(root, []string{path}, "")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Session.SessionID != "" || groups[0].Err == nil || len(groups[0].Reports[0].Lines) != 2 {
		t.Fatalf("groups=%+v errs=%v", groups, errs)
	}
}
