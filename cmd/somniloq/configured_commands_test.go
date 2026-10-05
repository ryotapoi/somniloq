package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredCommandsKeepSameSessionAndMessageIDsSeparate(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.toml")
	dbPath := filepath.Join(dir, "db", "history.db")
	roots := []string{filepath.Join(dir, "a"), filepath.Join(dir, "b")}
	for i, root := range roots {
		if err := os.MkdirAll(filepath.Join(root, "project"), 0700); err != nil {
			t.Fatal(err)
		}
		var records strings.Builder
		if i == 1 {
			records.WriteString(claudeTestRecord("earlier", "earlier question"))
		}
		records.WriteString(claudeTestRecord("shared", "needle input "+string(rune('a'+i))))
		if err := os.WriteFile(filepath.Join(root, "project", "same-id.jsonl"), []byte(records.String()), 0600); err != nil {
			t.Fatal(err)
		}
	}
	configText := fmt.Sprintf("db = %q\n[[inputs]]\nsource = \"claude-code\"\nroot = \"a\"\n[[inputs]]\nsource = \"claude-code\"\nroot = \"b\"\n", dbPath)
	if err := os.WriteFile(cfgPath, []byte(configText), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		var out, errOut bytes.Buffer
		args = append(args, "--config", cfgPath)
		code, err := runCommand(args, strings.NewReader(""), &out, &errOut, false)
		if code != 0 || err != nil {
			t.Fatalf("%v: code %d error %v stdout %q stderr %q", args, code, err, out.String(), errOut.String())
		}
		return out.String()
	}
	run("import")
	var listed searchGroupJSON
	if err := json.Unmarshal([]byte(run("search", "--format", "json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 2 || listed.Items[0].REF == listed.Items[1].REF {
		t.Fatalf("sessions: %+v", listed)
	}
	var page searchGroupJSON
	if err := json.Unmarshal([]byte(run("search", "--format", "json", "needle")), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Count != 2 {
		t.Fatalf("hits: %+v", page)
	}
	for i := range roots {
		roots[i], _ = filepath.EvalSymlinks(roots[i])
	}
	seen := map[string]bool{}
	for _, hit := range page.Items {
		if hit.Input == nil {
			t.Fatal(hit)
		}
		seen[*hit.Input] = true
		text := run("show", "--format", "json", hit.REF)
		var shown showJSON
		if err := json.Unmarshal([]byte(text), &shown); err != nil {
			t.Fatal(err)
		}
		if *hit.Input == roots[0] && (len(shown.Items) != 1 || !strings.Contains(shown.Items[0].Text, "input a")) {
			t.Fatal(text)
		}
		if *hit.Input == roots[1] && (len(shown.Items) != 2 || !strings.Contains(shown.Items[1].Text, "input b")) {
			t.Fatal(text)
		}
	}
	if !seen[roots[0]] || !seen[roots[1]] {
		t.Fatal(page)
	}
	// Selector roots use the configuration's real parent, even when it is not cwd.
	if err := os.Remove(filepath.Join(roots[0], "project", "same-id.jsonl")); err != nil {
		t.Fatal(err)
	}
	run("import", "--input", "a", "--full", "--yes")
	if err := json.Unmarshal([]byte(run("search", "--format", "json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || listed.Items[0].Input == nil || *listed.Items[0].Input != roots[1] {
		t.Fatalf("selected full touched other input: %+v", listed)
	}
	var remaining showJSON
	if err := json.Unmarshal([]byte(run("show", "--format", "json", listed.Items[0].REF)), &remaining); err != nil || len(remaining.Items) != 2 {
		t.Fatalf("remaining body: %+v, %v", remaining, err)
	}
	run("import", "--input", "a", "--source", "codex", "--full", "--yes")
	if err := json.Unmarshal([]byte(run("search", "--format", "json")), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("empty selection deleted input: %+v", listed)
	}
}

func claudeTestRecord(uuid, content string) string {
	return fmt.Sprintf("{\"type\":\"user\",\"uuid\":%q,\"sessionId\":\"same-id\",\"timestamp\":\"2026-03-28T14:00:00Z\",\"message\":{\"role\":\"user\",\"content\":%q}}\n", uuid, content)
}
