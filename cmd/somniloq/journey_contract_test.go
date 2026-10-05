package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestConfiguredFixtureJourneys(t *testing.T) {
	// Date-only CLI bounds use Local; keep the fixture day independent of the host.
	previous := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previous })
	base, err := filepath.Abs("../../internal/ingest/testdata/v0.14.0")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	roots := []string{"codex/input-a", "codex/input-b", "claude-code/input-a", "cursor-agent/input-a"}
	sources := []core.Source{core.SourceCodex, core.SourceCodex, core.SourceClaudeCode, core.SourceCursorAgent}
	held := map[string][]byte{}
	err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".jsonl" || entry.Name() == "missing-ordinal.jsonl" {
			return nil
		}
		relative, e := filepath.Rel(base, path)
		if e != nil {
			return e
		}
		data, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if relative == "codex/input-a/root.jsonl" || relative == "claude-code/input-a/project/cc-root.jsonl" || relative == "claude-code/input-a/project/cc-root/subagents/agent-child.jsonl" {
			held[relative] = data
			return nil
		}
		target := filepath.Join(dir, relative)
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		return os.WriteFile(target, data, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	var cfg strings.Builder
	fmt.Fprintf(&cfg, "db = %q\n", filepath.Join(dir, "history.db"))
	for i, root := range roots {
		fmt.Fprintf(&cfg, "[[inputs]]\nsource = %q\nroot = %q\n", []string{"codex", "codex", "claude-code", "cursor-agent"}[i], root)
	}
	cfgPath := filepath.Join(dir, "config.toml")
	if err = os.WriteFile(cfgPath, []byte(cfg.String()), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) map[string]any {
		t.Helper()
		var out, diag bytes.Buffer
		argv := append(append([]string{}, args...), "--config", cfgPath)
		code, e := runCommand(argv, strings.NewReader(""), &out, &diag, false)
		if code != 0 || e != nil {
			t.Fatalf("%v: exit=%d error=%v stdout=%s stderr=%s", argv, code, e, out.String(), diag.String())
		}
		if args[0] == "import" {
			return nil
		}
		var page map[string]any
		if e = json.Unmarshal(out.Bytes(), &page); e != nil {
			t.Fatal(e, out.String())
		}
		journeyPage(t, page)
		return page
	}
	ref := func(input int, identity string) string {
		root, e := filepath.EvalSymlinks(filepath.Join(dir, roots[input]))
		if e != nil {
			t.Fatal(e)
		}
		return core.IdentityREF(core.InputKey(sources[input], root), sources[input], identity)
	}
	run("import")
	child, grand := ref(0, `["child"]`), ref(2, `["cc-root","grandchild"]`)
	before := map[string][]any{}
	for _, selected := range []string{child, grand} {
		page := run("show", "--format", "json", selected)
		before[selected] = page["items"].([]any)
		for _, raw := range before[selected] {
			item := raw.(map[string]any)
			journeyFields(t, item, "ref messageNumber role timestamp text blocks parentRef rootRef provenance")
			if item["parentRef"] != nil || item["rootRef"] != nil {
				t.Fatalf("missing parent fabricated: %v", item)
			}
		}
	}
	if len(before[child]) != 3 || before[child][0].(map[string]any)["text"] != "Child answer" || len(before[grand]) != 2 {
		t.Fatal(before)
	}
	missing := run("search", "--format", "json", "Inherited question")
	if missing["total"] != float64(0) {
		t.Fatalf("inherited context became body: %v", missing)
	}
	for relative, data := range held {
		target := filepath.Join(dir, relative)
		if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// A day-spanning conversation makes the date journey detect renumbering and
	// accidental whole-conversation retrieval, which the same-day oracle cannot.
	span := strings.ReplaceAll(claudeTestRecord("span-before", "Before day"), "same-id", "day-span")
	span = strings.ReplaceAll(span, "2026-03-28T14:00:00Z", "2026-09-30T23:00:00Z")
	for i, stamp := range []string{"2026-10-01T12:00:00Z", "2026-10-02T00:00:00Z"} {
		record := claudeTestRecord(fmt.Sprintf("span-%d", i), []string{"During day", "After day"}[i])
		record = strings.ReplaceAll(record, "same-id", "day-span")
		span += strings.ReplaceAll(record, "2026-03-28T14:00:00Z", stamp)
	}
	if err = os.WriteFile(filepath.Join(dir, roots[2], "project/day-span.jsonl"), []byte(span), 0600); err != nil {
		t.Fatal(err)
	}
	run("import")
	for selected, old := range before {
		now := run("show", "--format", "json", selected)["items"].([]any)
		if len(now) != len(old) {
			t.Fatal(now, old)
		}
		for i := range old {
			a, b := old[i].(map[string]any), now[i].(map[string]any)
			for _, key := range []string{"ref", "messageNumber", "text", "role", "timestamp", "blocks"} {
				if !reflect.DeepEqual(a[key], b[key]) {
					t.Fatalf("parent arrival changed %s: %v -> %v", key, a, b)
				}
			}
			wantParent, wantRoot := ref(0, `["root"]`), ref(0, `["root"]`)
			if selected == grand {
				wantParent, wantRoot = ref(2, `["cc-root","child"]`), ref(2, `["cc-root"]`)
			}
			if b["parentRef"] != wantParent || b["rootRef"] != wantRoot {
				t.Fatal(b)
			}
		}
	}
	// Build an independent expected owner/body set from the existing source oracle.
	var oracle struct {
		Cases []struct {
			Path       string
			Identity   []string
			Utterances []struct {
				Number     int
				Role, Text string
				Timestamp  *string
			}
		}
	}
	data, err := os.ReadFile(filepath.Join(base, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	expected := map[string][]map[string]any{}
	for _, c := range oracle.Cases {
		input := -1
		for i, root := range roots {
			if strings.HasPrefix(c.Path, root+"/") {
				input = i
			}
		}
		if input < 0 || len(c.Utterances) == 0 {
			continue
		}
		identity, _ := json.Marshal(c.Identity)
		selected := ref(input, string(identity))
		for _, utterance := range c.Utterances {
			var stamp any
			if utterance.Timestamp != nil {
				stamp = *utterance.Timestamp
			}
			expected[selected] = append(expected[selected], map[string]any{"ref": selected, "messageNumber": float64(utterance.Number), "role": utterance.Role, "text": utterance.Text, "timestamp": stamp})
		}
	}
	// expected.json's merged_order specifies the three-record child union.
	expected[child] = []map[string]any{}
	for i, text := range []string{"Child answer", "Second rollout answer", "Child follow-up"} {
		expected[child] = append(expected[child], map[string]any{"ref": child, "messageNumber": float64(i + 1), "role": []string{"assistant", "assistant", "user"}[i], "text": text, "timestamp": "2026-10-01T00:01:00Z"})
	}
	spanREF := ref(2, `["day-span"]`)
	for i, text := range []string{"Before day", "During day", "After day"} {
		expected[spanREF] = append(expected[spanREF], map[string]any{"ref": spanREF, "messageNumber": float64(i + 1), "role": "user", "text": text, "timestamp": []string{"2026-09-30T23:00:00Z", "2026-10-01T12:00:00Z", "2026-10-02T00:00:00Z"}[i]})
	}
	listed := run("search", "--format", "json", "--limit", "100")
	seen := map[string]bool{}
	for _, raw := range listed["items"].([]any) {
		group := raw.(map[string]any)
		journeyFields(t, group, "ref input source project title startedAt lastAt importedAt members matchedMembers memberCount")
		if group["source"] == "cursor_agent" && (group["startedAt"] != nil || group["lastAt"] != nil || group["project"] != nil) {
			t.Fatalf("unknown Cursor metadata was filled: %v", group)
		}
		members := group["members"].([]any)
		if group["memberCount"] != float64(len(members)) {
			t.Fatal(group)
		}
		for _, member := range members {
			selected := member.(string)
			if seen[selected] {
				t.Fatalf("duplicate member %s", selected)
			}
			seen[selected] = true
			shown := run("show", "--format", "json", selected)["items"].([]any)
			if len(shown) != len(expected[selected]) {
				t.Fatalf("%s: %v want %v", selected, shown, expected[selected])
			}
			for i, item := range shown {
				body := item.(map[string]any)
				journeyFields(t, body, "ref messageNumber role timestamp text blocks parentRef rootRef provenance")
				if _, ok := body["blocks"].([]any); !ok || body["provenance"] != "source_record" {
					t.Fatal(body)
				}
				for key, value := range expected[selected][i] {
					if !reflect.DeepEqual(body[key], value) {
						t.Fatalf("%s %s: %v want %v", selected, key, body, expected[selected][i])
					}
				}
			}
		}
	}
	if len(seen) != len(expected) {
		t.Fatalf("members=%v expected=%v", seen, expected)
	}
	// Word -> returned group REF -> all occurrences -> original owner message.
	words := []string{"Inherited question", "Child answer", "Grandchild answer"}
	args := []string{"search", "--format", "json", "--all"}
	for _, word := range words {
		args = append(args, "-e", word)
	}
	wordList := run(args...)
	if wordList["total"] != float64(1) {
		t.Fatal(wordList)
	}
	group := wordList["items"].([]any)[0].(map[string]any)
	if group["ref"] != ref(0, `["root"]`) || group["memberCount"] != float64(5) {
		t.Fatal(group)
	}
	args = append(args, "--session", group["ref"].(string))
	detail := run(args...)
	if detail["total"] != float64(3) {
		t.Fatal(detail)
	}
	matchOwners := map[string]string{ref(0, `["root"]`): words[0], child: words[1], ref(0, `["grandchild"]`): words[2]}
	for _, raw := range detail["items"].([]any) {
		match := raw.(map[string]any)
		journeyFields(t, match, "ref messageNumber occurrenceNumber role timestamp startByte endByte patternIndexes matchText lineText")
		selected := match["ref"].(string)
		if match["messageNumber"] != float64(1) || match["occurrenceNumber"] != float64(1) || match["lineText"] != matchOwners[selected] || match["matchText"] != matchOwners[selected] {
			t.Fatal(match)
		}
		shown := run("show", "--format", "json", selected)["items"].([]any)[0].(map[string]any)
		start, end := int(match["startByte"].(float64)), int(match["endByte"].(float64))
		if shown["text"].(string)[start:end] != match["matchText"] || shown["role"] != match["role"] || shown["timestamp"] != match["timestamp"] {
			t.Fatal(match, shown)
		}
	}
	local := run("search", "--session", child, "--format", "json", "--all", "-e", words[0], "-e", words[1])
	if local["total"] != float64(0) {
		t.Fatal(local)
	}
	// Date list -> deduplicated members -> a single show invocation.
	daily := run("search", "--format", "json", "--since", "2026-10-01", "--until", "2026-10-01", "--limit", "100")
	selected := map[string]bool{}
	for _, raw := range daily["items"].([]any) {
		group := raw.(map[string]any)
		if group["source"] == "cursor_agent" {
			t.Fatal(group)
		}
		for _, member := range group["members"].([]any) {
			selected[member.(string)] = true
		}
	}
	refs := []string{}
	for member := range selected {
		refs = append(refs, member)
	}
	sort.Strings(refs)
	if len(refs) < 3 {
		t.Fatal(daily)
	}
	args = append([]string{"show", "--format", "json", "--since", "2026-10-01", "--until", "2026-10-01"}, refs...)
	shown := run(args...)
	wantDay := map[string]map[string]any{}
	for selected, messages := range expected {
		for _, message := range messages {
			stamp, ok := message["timestamp"].(string)
			if ok && strings.HasPrefix(stamp, "2026-10-01") {
				wantDay[fmt.Sprintf("%s/%v", selected, message["messageNumber"])] = message
			}
		}
	}
	if shown["total"] != float64(len(wantDay)) {
		t.Fatal(shown, wantDay)
	}
	for _, raw := range shown["items"].([]any) {
		body := raw.(map[string]any)
		key := fmt.Sprintf("%s/%v", body["ref"], body["messageNumber"])
		want, ok := wantDay[key]
		if !ok {
			t.Fatal(body)
		}
		for key, value := range want {
			if body[key] != value {
				t.Fatal(body, want)
			}
		}
		delete(wantDay, key)
	}
	if len(wantDay) != 0 {
		t.Fatal(wantDay)
	}
}

func journeyFields(t *testing.T, object map[string]any, fields string) {
	t.Helper()
	for _, field := range strings.Fields(fields) {
		value, ok := object[field]
		if !ok {
			t.Fatalf("missing %s: %v", field, object)
		}
		switch field {
		case "messageNumber", "occurrenceNumber", "startByte", "endByte", "memberCount", "total", "count", "offset":
			n, ok := value.(float64)
			if !ok || n < 0 || n != float64(int(n)) {
				t.Fatalf("integer %s: %v", field, object)
			}
		case "limit", "nextOffset":
			if value != nil {
				n, ok := value.(float64)
				if !ok || n < 0 || n != float64(int(n)) {
					t.Fatalf("nullable integer %s: %v", field, object)
				}
			}
		case "hasMore":
			if _, ok := value.(bool); !ok {
				t.Fatal(object)
			}
		case "members", "matchedMembers", "blocks", "patternIndexes", "items":
			array, ok := value.([]any)
			if !ok {
				t.Fatalf("array %s: %v", field, object)
			}
			for _, item := range array {
				if field == "items" {
					continue
				}
				if field == "patternIndexes" {
					n, ok := item.(float64)
					if !ok || n < 1 || n != float64(int(n)) {
						t.Fatal(object)
					}
				} else if _, ok := item.(string); !ok {
					t.Fatal(object)
				}
			}
		case "input", "project", "title", "startedAt", "lastAt", "importedAt", "timestamp", "parentRef", "rootRef":
			if value != nil {
				if _, ok := value.(string); !ok {
					t.Fatal(object)
				}
			}
		default:
			if _, ok := value.(string); !ok {
				t.Fatalf("string %s: %v", field, object)
			}
		}
	}
}

func journeyPage(t *testing.T, page map[string]any) {
	t.Helper()
	journeyFields(t, page, "items total count limit offset hasMore nextOffset")
	items, ok := page["items"].([]any)
	if !ok {
		t.Fatal(page)
	}
	for _, key := range []string{"total", "count", "offset"} {
		n, ok := page[key].(float64)
		if !ok || n < 0 || n != float64(int(n)) {
			t.Fatalf("numeric %s: %v", key, page)
		}
	}
	if page["count"] != float64(len(items)) || page["offset"] != float64(0) || page["hasMore"] != false || page["nextOffset"] != nil || page["total"] != page["count"] {
		t.Fatal(page)
	}
	if page["limit"] != nil {
		if n, ok := page["limit"].(float64); !ok || n < page["count"].(float64) {
			t.Fatal(page)
		}
	}
}
