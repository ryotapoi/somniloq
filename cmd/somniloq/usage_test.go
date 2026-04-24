package main

import (
	"bytes"
	"flag"
	"strings"
	"testing"
)

func TestBackfillUsage(t *testing.T) {
	var buf bytes.Buffer
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(&buf)

	setUsage(fs, "Resolve repo_path for legacy sessions", "somniloq backfill")
	fs.Usage()

	out := buf.String()

	if !strings.Contains(out, "Resolve repo_path for legacy sessions") {
		t.Errorf("expected description in output, got:\n%s", out)
	}
	if !strings.Contains(out, "somniloq backfill") {
		t.Errorf("expected usage line in output, got:\n%s", out)
	}
}

func TestSetUsage(t *testing.T) {
	var buf bytes.Buffer
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&buf)
	fs.String("since", "", "filter by start time")

	setUsage(fs, "List sessions", "somniloq sessions [flags]")
	fs.Usage()

	out := buf.String()

	if !strings.Contains(out, "List sessions") {
		t.Errorf("expected description in output, got:\n%s", out)
	}
	if !strings.Contains(out, "somniloq sessions [flags]") {
		t.Errorf("expected usage line in output, got:\n%s", out)
	}
	if !strings.Contains(out, "Flags:") {
		t.Errorf("expected Flags section in output, got:\n%s", out)
	}
	if !strings.Contains(out, "-since") {
		t.Errorf("expected flag defaults in output, got:\n%s", out)
	}
}
