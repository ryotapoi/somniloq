package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMigrationIndexRetainsOnlyOwnerMetadata(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owner.jsonl")
	data := []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"owner\"}}\n" +
		"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"input_text\",\"text\":\"hello\"}]}}\n")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	groups, errs := NewAdapter(func(s string) string { return s }).BuildMigrationIndex(root, []string{path}, "")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Err != nil {
		t.Fatalf("groups=%+v errs=%v", groups, errs)
	}
	g := groups[0]
	if g.Session.SessionID != "owner" || len(g.Messages) != 0 || len(g.Reports) != 1 || len(g.Reports[0].Data) != 0 {
		t.Fatalf("index retained body or lost owner: %+v", g)
	}
	if len(g.States) != 0 || len(g.Reports[0].Failures) != 0 {
		t.Fatalf("index retained body evidence: %+v", g)
	}
}

func TestMigrationAdapterReusesRepositoryAcrossBodyPasses(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "a.jsonl"), filepath.Join(root, "b.jsonl")}
	for i, path := range paths {
		data := []byte(`{"type":"session_meta","payload":{"id":"owner-` + string(rune('a'+i)) + `","cwd":"/same"}}` + "\n")
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	adapter := NewMigrationAdapter(func(string) string { calls++; return "/repo" })
	if _, errs := adapter.BuildMigrationIndex(root, paths, ""); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, path := range paths {
		if _, errs := adapter.BuildMigrationGroups(root, []string{path}, ""); len(errs) != 0 {
			t.Fatal(errs)
		}
	}
	if calls != 1 {
		t.Fatalf("repository resolutions=%d, want 1", calls)
	}
}

func TestMigrationGroupsRetainMetadataOnlyRollouts(t *testing.T) {
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
	if len(errs) != 0 || len(groups) != 1 || groups[0].Session.SessionID != "" || groups[0].Err == nil {
		t.Fatalf("groups=%+v errs=%v", groups, errs)
	}
}

func TestMigrationStrictDiagnosticRetainsPhysicalLineNumber(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owner.jsonl")
	data := "{\"type\":\"session_meta\",\"payload\":{\"id\":\"owner\"}}\n\n" +
		"{\"type\":\"session_meta\",\"payload\":{\"id\":\"other\"}}\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewMigrationAdapter(func(s string) string { return s })
	for _, build := range []func(string, []string, string) ([]Group, []error){adapter.BuildMigrationGroups} {
		groups, errs := build(root, []string{path}, "")
		if len(errs) != 0 || len(groups) != 1 || groups[0].Err == nil || !strings.Contains(groups[0].Err.Error(), path+":3:") {
			t.Fatalf("groups=%+v errors=%v", groups, errs)
		}
	}
}

func TestMigrationIndexUsesFirstValidMetadataAndDefersBodyValidation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "owner.jsonl")
	data := `{"type":"session_meta","payload":{"id":""}}` + "\n" + `{"type":"session_meta","payload":{"id":"child","source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}}}` + "\n" + `{"type":"session_meta","payload":{"id":"parent"}}` + "\ninvalid\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	a := NewMigrationAdapter(func(s string) string { return s })
	groups, errs := a.BuildMigrationIndex(root, []string{path}, "")
	if len(errs) != 0 || len(groups) != 1 || groups[0].Session.SessionID != "child" || groups[0].Session.ParentSessionID != "parent" || groups[0].Err != nil {
		t.Fatalf("%+v %v", groups, errs)
	}
	body, errs := a.BuildMigrationGroups(root, []string{path}, "")
	if len(errs) != 0 || len(body) != 1 || body[0].Err == nil {
		t.Fatalf("body failed to validate: %+v %v", body, errs)
	}
}
