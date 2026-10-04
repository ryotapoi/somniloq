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
	var sessions []sessionJSON
	if err := json.Unmarshal([]byte(run("sessions", "--format", "json")), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 || sessions[0].REF == sessions[1].REF {
		t.Fatalf("sessions: %+v", sessions)
	}
	var hits []searchJSON
	if err := json.Unmarshal([]byte(run("search", "--format", "json", "needle")), &hits); err != nil {
		t.Fatal(err)
	}
	if len(hits) != 2 {
		t.Fatalf("hits: %+v", hits)
	}
	seen := map[int]bool{}
	for _, hit := range hits {
		seen[hit.Turn] = true
		text := run("show", "--format", "json", hit.REF)
		var shown showJSON
		if err := json.Unmarshal([]byte(text), &shown); err != nil {
			t.Fatal(err)
		}
		if len(shown.Items) == 0 || shown.Items[0].REF != hit.REF {
			t.Fatalf("show: %s", text)
		}
		if strings.Contains(hit.Snippet, "input a") && (len(shown.Items) != 1 || !strings.Contains(shown.Items[0].Text, "input a")) {
			t.Fatalf("mixed input a: %s", text)
		}
		if strings.Contains(hit.Snippet, "input b") && (len(shown.Items) != 2 || !strings.Contains(shown.Items[1].Text, "input b")) {
			t.Fatalf("mixed input b: %s", text)
		}
	}
	if !seen[1] || !seen[2] {
		t.Fatalf("input turn cache crossed boundary: %+v", hits)
	}
	// Selector roots use the configuration's real parent, even when it is not cwd.
	if err := os.Remove(filepath.Join(roots[0], "project", "same-id.jsonl")); err != nil {
		t.Fatal(err)
	}
	run("import", "--input", "a", "--full", "--yes")
	if err := json.Unmarshal([]byte(run("sessions", "--format", "json")), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].MessageCount != 2 {
		t.Fatalf("selected full touched other input: %+v", sessions)
	}
	run("import", "--input", "a", "--source", "codex", "--full", "--yes")
	if err := json.Unmarshal([]byte(run("sessions", "--format", "json")), &sessions); err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("empty selection deleted input: %+v", sessions)
	}
}

func claudeTestRecord(uuid, content string) string {
	return fmt.Sprintf("{\"type\":\"user\",\"uuid\":%q,\"sessionId\":\"same-id\",\"timestamp\":\"2026-03-28T14:00:00Z\",\"message\":{\"role\":\"user\",\"content\":%q}}\n", uuid, content)
}
