package main

import "testing"

func TestParseImportSourceRejectsUnknownValue(t *testing.T) {
	_, err := parseImportSource("claude")
	if err == nil {
		t.Fatal("parseImportSource should reject unknown values")
	}
	want := `invalid --source "claude" (want all, claude-code, codex, or cursor-agent)`
	if err.Error() != want {
		t.Fatalf("error: got %q, want %q", err.Error(), want)
	}
}
