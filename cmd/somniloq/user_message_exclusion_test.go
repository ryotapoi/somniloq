package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestUserMessageMatcher_TrimmedFullTextAndOR(t *testing.T) {
	matcher, err := newUserMessageMatcher(config{}, []string{`^first\nsecond$`, `contains`}, false)
	if err != nil {
		t.Fatalf("newUserMessageMatcher: %v", err)
	}
	for _, content := range []string{" \n first\nsecond \n", "prefix contains suffix"} {
		if !matcher.excludes(content) {
			t.Errorf("matcher.excludes(%q) = false, want true", content)
		}
	}
	if matcher.excludes("first only") {
		t.Error("matcher excluded text that matched neither pattern")
	}
}

func TestNewUserMessageMatcherConfigOverrideDisableAndEmptyPattern(t *testing.T) {
	cfg := config{ExcludeUserMessagePatterns: []string{`.*`}}
	for _, emptyConfig := range []config{{}, {ExcludeUserMessagePatterns: []string{}}} {
		matcher, err := newUserMessageMatcher(emptyConfig, nil, false)
		if err != nil || matcher.excludes("anything") {
			t.Errorf("empty config matcher = %+v, %v; want no exclusions", matcher, err)
		}
	}

	configured, err := newUserMessageMatcher(cfg, nil, false)
	if err != nil || !configured.excludes("anything") {
		t.Fatalf("configured matcher = %+v, %v; want all messages excluded", configured, err)
	}

	overridden, err := newUserMessageMatcher(cfg, []string{`^keep`, `^also keep`}, false)
	if err != nil {
		t.Fatalf("override matcher: %v", err)
	}
	if !overridden.excludes("keep this") || !overridden.excludes("also keep this") || overridden.excludes("drop config pattern") {
		t.Error("CLI patterns did not OR together and replace the config list")
	}

	disabled, err := newUserMessageMatcher(cfg, nil, true)
	if err != nil || disabled.excludes("anything") {
		t.Fatalf("disabled matcher = %+v, %v; want no exclusions", disabled, err)
	}

	emptyPattern, err := newUserMessageMatcher(config{}, []string{""}, false)
	if err != nil || !emptyPattern.excludes("anything") {
		t.Fatalf("empty-pattern matcher = %+v, %v; want empty regex to match all", emptyPattern, err)
	}

	if _, err := newUserMessageMatcher(config{}, []string{"["}, false); err == nil || !strings.Contains(err.Error(), "invalid excludeUserMessagePatterns pattern") {
		t.Errorf("invalid override error = %v, want invalid regex diagnostic", err)
	}
	if _, err := newUserMessageMatcher(cfg, []string{`^keep`}, true); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Errorf("conflicting options error = %v, want conflict diagnostic", err)
	}
}

func newUserMessageExclusionDB(t *testing.T) *core.DB {
	t.Helper()
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "exclude-1",
		RepoPath:  "/Users/test/proj",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T10:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	for _, message := range []struct {
		id, role, body, time string
	}{
		{"u1", "user", "  ignore this\nsecond line  ", "2026-03-28T10:00:00Z"},
		{"a1", "assistant", "reply to ignored turn", "2026-03-28T10:00:30Z"},
		{"u2", "user", "<command-name>/clear</command-name>", "2026-03-28T10:01:00Z"},
		{"a2", "assistant", "reply to clear", "2026-03-28T10:01:30Z"},
		{"u3", "user", "<local-command-caveat>marker</local-command-caveat>", "2026-03-28T10:02:00Z"},
		{"u4", "user", "normal work", "2026-03-28T10:03:00Z"},
		{"u5", "user", "final request", "2026-03-28T10:04:00Z"},
	} {
		insertOutlineMessage(t, db, "exclude-1", message.id, message.role, message.body, message.time, false)
	}
	return db
}

func TestUserMessageExclusionsShareOutlineAndSummaryFiltering(t *testing.T) {
	cfg := config{ExcludeUserMessagePatterns: []string{
		`^ignore this\nsecond line$`,
		`^<local-command-caveat>`,
		`^final request$`,
	}}

	var outlineOut, errOut bytes.Buffer
	code, err := outlineCmd([]string{"--format", "json", fixtureREF(core.SourceClaudeCode, "exclude-1")}, staticDB(newUserMessageExclusionDB(t)), cfg, &outlineOut, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("outlineCmd = (%d, %v), stderr %q", code, err, errOut.String())
	}
	var outline []outlineEntryJSON
	if err := json.Unmarshal(outlineOut.Bytes(), &outline); err != nil {
		t.Fatalf("decode outline: %v", err)
	}
	if len(outline) != 2 || outline[0].Turn != 2 || outline[0].FirstLine != "<command-name>/clear</command-name>" || outline[1].Turn != 4 || outline[1].FirstLine != "normal work" {
		t.Errorf("outline entries = %+v, want original turns 2 and 4", outline)
	}
	if got, want := outline[0].BodySize, len("<command-name>/clear</command-name>")+len("reply to clear"); got != want {
		t.Errorf("turn 2 bodySize = %d, want %d from the unfiltered turn", got, want)
	}

	var summaryOut bytes.Buffer
	code, err = showCmd([]string{"--summary", "2", "--format", "json", fixtureREF(core.SourceClaudeCode, "exclude-1")}, staticDB(newUserMessageExclusionDB(t)), cfg, &summaryOut, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("show summary = (%d, %v), stderr %q", code, err, errOut.String())
	}
	showEntries := decodeJSONArray(t, summaryOut.Bytes())
	if len(showEntries) != 1 {
		t.Fatalf("show entries = %d, want 1: %s", len(showEntries), summaryOut.String())
	}
	messages := showEntries[0]["messages"].([]any)
	if len(messages) != 2 || messages[0].(map[string]any)["content"] != "<command-name>/clear</command-name>" || messages[1].(map[string]any)["content"] != "normal work" {
		t.Errorf("summary messages = %v, want first two remaining user messages after filtering", messages)
	}

	var overriddenOut bytes.Buffer
	code, err = outlineCmd([]string{
		"--format", "json",
		"--exclude-user-message-pattern", `^ignore this\nsecond line$`,
		"--exclude-user-message-pattern", `^<local-command-caveat>`, fixtureREF(core.SourceClaudeCode, "exclude-1"),
	}, staticDB(newUserMessageExclusionDB(t)), config{ExcludeUserMessagePatterns: []string{`.*`}}, &overriddenOut, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("outline override = (%d, %v), stderr %q", code, err, errOut.String())
	}
	var overridden []outlineEntryJSON
	if err := json.Unmarshal(overriddenOut.Bytes(), &overridden); err != nil {
		t.Fatalf("decode overridden outline: %v", err)
	}
	if len(overridden) != 3 || overridden[0].Turn != 2 || overridden[1].Turn != 4 || overridden[2].Turn != 5 {
		t.Errorf("repeated CLI override entries = %+v, want turns 2, 4, 5", overridden)
	}
}

func TestShowSummaryMaxIntLimitWithOneUserMessage(t *testing.T) {
	db, err := core.OpenDB(":memory:")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.UpsertSession(testInputID(t, db,
		core.SourceClaudeCode), core.SessionMeta{
		Source:    core.SourceClaudeCode,
		SessionID: "one-message",
		StartedAt: "2026-03-28T10:00:00Z",
	}, "2026-03-28T10:00:00Z"); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	insertOutlineMessage(t, db, "one-message", "u1", "user", "available message", "2026-03-28T10:00:00Z", false)

	maxInt := int(^uint(0) >> 1)
	var out, errOut bytes.Buffer
	code, err := showCmd([]string{"--summary", strconv.Itoa(maxInt), "--format", "json", fixtureREF(core.SourceClaudeCode, "one-message")}, staticDB(db), config{}, &out, &errOut)
	if err != nil || code != 0 {
		t.Fatalf("showCmd with max-int summary = (%d, %v), stderr %q", code, err, errOut.String())
	}
	entries := decodeJSONArray(t, out.Bytes())
	if len(entries) != 1 {
		t.Fatalf("show entries = %d, want 1: %s", len(entries), out.String())
	}
	messages := entries[0]["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["content"] != "available message" {
		t.Errorf("summary messages = %v, want the one available user message", messages)
	}
}

func TestUserMessageExclusionScopeAndLegacyBehavior(t *testing.T) {
	cfg := config{ExcludeUserMessagePatterns: []string{`^ignore this\nsecond line$`, `^<local-command-caveat>`, `^final request$`}}

	for _, tt := range []struct {
		args       []string
		wantMarkup string
	}{
		{[]string{"--format", "json", fixtureREF(core.SourceClaudeCode, "exclude-1")}, "ignore this"},
		{[]string{"--format", "json", "--turn", "1", fixtureREF(core.SourceClaudeCode, "exclude-1")}, "ignore this"},
		{[]string{"--format", "json", "--tail", "1", fixtureREF(core.SourceClaudeCode, "exclude-1")}, "final request"},
	} {
		var out, errOut bytes.Buffer
		code, err := showCmd(tt.args, staticDB(newUserMessageExclusionDB(t)), cfg, &out, &errOut)
		if err != nil || code != 0 {
			t.Fatalf("showCmd(%v) = (%d, %v), stderr %q", tt.args, code, err, errOut.String())
		}
		entries := decodeJSONArray(t, out.Bytes())
		messages := entries[0]["messages"].([]any)
		if len(messages) == 0 || !strings.Contains(messages[0].(map[string]any)["content"].(string), tt.wantMarkup) {
			t.Errorf("showCmd(%v) messages = %v, want unfiltered content containing %q", tt.args, messages, tt.wantMarkup)
		}
	}

	var searchOut, searchErr bytes.Buffer
	code, err := searchCmd([]string{"--format", "json", "ignore this"}, staticDB(newUserMessageExclusionDB(t)), cfg, &searchOut, &searchErr)
	if err != nil || code != 0 {
		t.Fatalf("searchCmd = (%d, %v), stderr %q", code, err, searchErr.String())
	}
	hits := decodeJSONArray(t, searchOut.Bytes())
	if len(hits) != 1 || hits[0]["turn"] != float64(1) {
		t.Errorf("search hits = %v, want excluded-from-display message at original turn 1", hits)
	}

	var defaultSummary bytes.Buffer
	code, err = showCmd([]string{"--summary", "5", "--format", "json", fixtureREF(core.SourceClaudeCode, "exclude-1")}, staticDB(newUserMessageExclusionDB(t)), config{}, &defaultSummary, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("unconfigured summary = (%d, %v)", code, err)
	}
	defaultMessages := decodeJSONArray(t, defaultSummary.Bytes())[0]["messages"].([]any)
	if len(defaultMessages) != 5 || defaultMessages[1].(map[string]any)["content"] != "<command-name>/clear</command-name>" || defaultMessages[2].(map[string]any)["content"] != "<local-command-caveat>marker</local-command-caveat>" {
		t.Errorf("unconfigured summary messages = %v, want all five stored user messages including synthetic prefixes", defaultMessages)
	}

	var disabledSummary bytes.Buffer
	code, err = showCmd([]string{"--summary", "5", "--no-exclude-user-messages", "--format", "json", fixtureREF(core.SourceClaudeCode, "exclude-1")}, staticDB(newUserMessageExclusionDB(t)), config{ExcludeUserMessagePatterns: []string{`.*`}}, &disabledSummary, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("disabled summary = (%d, %v)", code, err)
	}
	disabledMessages := decodeJSONArray(t, disabledSummary.Bytes())[0]["messages"].([]any)
	if len(disabledMessages) != 5 {
		t.Errorf("disabled summary messages = %d, want all five", len(disabledMessages))
	}

	var allExcludedOut bytes.Buffer
	code, err = showCmd([]string{"--summary", "1", "--exclude-user-message-pattern", "", "--format", "json", fixtureREF(core.SourceClaudeCode, "exclude-1")}, staticDB(newUserMessageExclusionDB(t)), config{}, &allExcludedOut, &bytes.Buffer{})
	if err != nil || code != 0 {
		t.Fatalf("all-excluded summary = (%d, %v)", code, err)
	}
	allExcludedEntries := decodeJSONArray(t, allExcludedOut.Bytes())
	if len(allExcludedEntries) != 1 || len(allExcludedEntries[0]["messages"].([]any)) != 0 {
		t.Errorf("all-excluded summary output = %v, want one session with empty messages", allExcludedEntries)
	}

	var emptyRegexOut bytes.Buffer
	code, err = outlineCmd([]string{"--format", "json", "--exclude-user-message-pattern", "", fixtureREF(core.SourceClaudeCode, "exclude-1")}, staticDB(newUserMessageExclusionDB(t)), config{}, &emptyRegexOut, &bytes.Buffer{})
	if err != nil || code != 0 || emptyRegexOut.String() != "[]\n" {
		t.Errorf("empty regex outline = (%d, %v, %q), want successful empty output", code, err, emptyRegexOut.String())
	}
}

func TestUserMessageExclusionFlagsValidateBeforeOpeningDB(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid exclusion options")
		return nil, nil
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"invalid outline regex", []string{"--exclude-user-message-pattern", "[", "s"}, "invalid excludeUserMessagePatterns pattern"},
		{"conflicting outline options", []string{"--exclude-user-message-pattern", "x", "--no-exclude-user-messages", "s"}, "cannot be combined"},
		{"show pattern requires summary", []string{"--exclude-user-message-pattern", "x", "s"}, "require --summary >= 1"},
		{"show disable requires summary", []string{"--no-exclude-user-messages", "s"}, "require --summary >= 1"},
		{"show override and disable conflict", []string{"--summary", "1", "--exclude-user-message-pattern", "x", "--no-exclude-user-messages", "s"}, "cannot be combined"},
		{"legacy include clear removed", []string{"--summary", "1", "--include-clear", "s"}, "flag provided but not defined"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			var code int
			var err error
			if strings.HasPrefix(tt.name, "invalid outline") || strings.HasPrefix(tt.name, "conflicting outline") {
				code, err = outlineCmd(tt.args, openDB, config{}, &out, &errOut)
			} else {
				code, err = showCmd(tt.args, openDB, config{}, &out, &errOut)
			}
			detail := errOut.String()
			if err != nil {
				detail = err.Error() + "\n" + detail
			}
			if code != 1 || !strings.Contains(detail, tt.want) {
				t.Errorf("command result = (%d, %v), stderr %q; want error containing %q", code, err, errOut.String(), tt.want)
			}
		})
	}
}
