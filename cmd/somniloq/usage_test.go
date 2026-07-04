package main

import (
	"bytes"
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestBackfillUsage(t *testing.T) {
	var buf bytes.Buffer
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	fs.SetOutput(&buf)
	fs.Bool("yes", false, "skip confirmation prompt")

	setUsage(fs, "Correct legacy session data (delete orphan sessions, resolve repo_path)", "somniloq backfill")
	fs.Usage()

	out := buf.String()

	if !strings.Contains(out, "Correct legacy session data") {
		t.Errorf("expected description in output, got:\n%s", out)
	}
	if !strings.Contains(out, "somniloq backfill") {
		t.Errorf("expected usage line in output, got:\n%s", out)
	}
	if !strings.Contains(out, "-yes") {
		t.Errorf("expected -yes flag in output, got:\n%s", out)
	}
}

func TestSetUsage(t *testing.T) {
	var buf bytes.Buffer
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(&buf)
	fs.String("since", "", "filter by start time")

	setUsage(fs, "List sessions", "somniloq sessions [flags]", "Examples:\n  somniloq sessions --since 7d")
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
	if !strings.Contains(out, "Examples:") {
		t.Errorf("expected details section in output, got:\n%s", out)
	}
}

func TestTopLevelUsageStaysShort(t *testing.T) {
	if strings.Contains(topLevelUsage, "Examples:") || strings.Contains(topLevelUsage, "Columns") {
		t.Fatalf("top-level usage must stay short, got:\n%s", topLevelUsage)
	}
	if lines := strings.Count(topLevelUsage, "\n"); lines > 25 {
		t.Fatalf("top-level usage grew too long: %d lines\n%s", lines, topLevelUsage)
	}
}

func TestSubcommandHelpIsSelfContained(t *testing.T) {
	openDB := func() (*core.DB, error) {
		return nil, errors.New("openDB must not be called for --help")
	}

	tests := []struct {
		name string
		run  func(*bytes.Buffer) (int, error)
		want []string
	}{
		{
			name: "import",
			run: func(errOut *bytes.Buffer) (int, error) {
				return importCmd([]string{"--help"}, openDB, "/claude", "/codex", strings.NewReader(""), &bytes.Buffer{}, errOut, false)
			},
			want: []string{"Examples:", "Output:", "Imported <imported> files", "somniloq import --source codex"},
		},
		{
			name: "backfill",
			run: func(errOut *bytes.Buffer) (int, error) {
				return backfillCmd([]string{"--help"}, openDB, strings.NewReader(""), &bytes.Buffer{}, errOut, false)
			},
			want: []string{"Examples:", "Output:", "Backfilled: deleted=<n> resolved=<n> unresolved=<n>", "somniloq backfill --yes"},
		},
		{
			name: "sessions",
			run: func(errOut *bytes.Buffer) (int, error) {
				return sessionsCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Columns (TSV, in order):", "logical_day", "non_command_user_turn_count", "firstNonCommandUserLine"},
		},
		{
			name: "show",
			run: func(errOut *bytes.Buffer) (int, error) {
				return showCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Output (markdown):", "messages fields: role, content, timestamp", "somniloq show --turn 40..60 <session-id>"},
		},
		{
			name: "outline",
			run: func(errOut *bytes.Buffer) (int, error) {
				return outlineCmd([]string{"--help"}, openDB, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Columns (TSV, in order):", "body_size", "Recommended long-session flow", "somniloq show --turn 12..18 <session-id>"},
		},
		{
			name: "search",
			run: func(errOut *bytes.Buffer) (int, error) {
				return searchCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Columns (TSV, in order):", "turn: outline/show turn number", "Typical flow: search -> outline", "somniloq search --since 7d"},
		},
		{
			name: "projects",
			run: func(errOut *bytes.Buffer) (int, error) {
				return projectsCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Columns (TSV, in order):", "session_count", "project, sessionCount", "somniloq projects --format json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			code, err := tt.run(&errOut)
			if err != nil {
				t.Fatalf("help returned error: %v", err)
			}
			if code != 0 {
				t.Fatalf("help exit code = %d, want 0", code)
			}
			out := errOut.String()
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("%s help missing %q:\n%s", tt.name, want, out)
				}
			}
		})
	}
}
