package main

import (
	"strings"
	"testing"
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
