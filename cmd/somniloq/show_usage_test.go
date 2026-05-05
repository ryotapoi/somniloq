package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestShowUsageSessionIDFormFlagsFirst(t *testing.T) {
	var line string
	for _, l := range strings.Split(showUsageLine, "\n") {
		if strings.Contains(l, "<session-id>") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("no <session-id> line in showUsageLine: %q", showUsageLine)
	}

	idx := strings.Index(line, "<session-id>")
	head, tail := line[:idx], line[idx+len("<session-id>"):]

	for _, f := range []string{"--summary", "--include-clear", "--short"} {
		if !strings.Contains(head, f) {
			t.Errorf("%s must appear before <session-id>: line=%q", f, line)
		}
	}
	if strings.Contains(tail, "--") {
		t.Errorf("no flags should appear after <session-id>: tail=%q", tail)
	}
}

func TestWriteAmbiguousSessionErrorListsSources(t *testing.T) {
	var buf bytes.Buffer
	writeAmbiguousSessionError(&buf, "same-id", []core.SessionRow{
		{Source: core.SourceClaudeCode, SessionID: "same-id"},
		{Source: core.SourceCodex, SessionID: "same-id"},
	})

	out := buf.String()
	for _, want := range []string{
		`error: session id "same-id" is ambiguous; matched multiple sources:`,
		"claude_code\tsame-id",
		"codex\tsame-id",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}
