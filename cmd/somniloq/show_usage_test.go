package main

import (
	"strings"
	"testing"
)

func TestShowUsageSessionIDFormFlagsFirst(t *testing.T) {
	var line string
	for _, l := range strings.Split(showUsageLine, "\n") {
		if strings.Contains(l, "<REF>") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("no <REF> line in showUsageLine: %q", showUsageLine)
	}

	idx := strings.Index(line, "<REF>")
	head, tail := line[:idx], line[idx+len("<REF>"):]

	for _, f := range []string{"--source", "--turn", "--tail", "--summary", "--exclude-user-message-pattern", "--no-exclude-user-messages", "--short", "--format"} {
		if !strings.Contains(head, f) {
			t.Errorf("%s must appear before <REF>: line=%q", f, line)
		}
	}
	if strings.Contains(tail, "--") {
		t.Errorf("no flags should appear after <REF>: tail=%q", tail)
	}
}
