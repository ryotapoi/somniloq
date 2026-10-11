package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

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
				return importConfiguredCmd([]string{"--help"}, openDB, config{}, strings.NewReader(""), &bytes.Buffer{}, errOut, false)
			},
			want: []string{"Examples:", "Output:", "Imported <imported> files", "Parse/normalization diagnostics: up to five file:line: error entries are printed to stderr.", "somniloq import --config default --source cursor-agent", "somniloq import [--config NAME_OR_PATH] [--source all|claude-code|codex|cursor-agent] [flags]", "source to import: all, claude-code, codex, cursor-agent"},
		},
		{
			name: "show",
			run: func(errOut *bytes.Buffer) (int, error) {
				return showCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Output (TSV/JSON):", "messageNumber", "--messages", "--one-line"},
		},
		{
			name: "search",
			run: func(errOut *bytes.Buffer) (int, error) {
				return searchCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "matchedMembers", "# page metadata", "Default unlimited", "-limit", "-offset", "--session <REF>"},
		},
		{
			name: "projects",
			run: func(errOut *bytes.Buffer) (int, error) {
				return projectsCmd([]string{"--help"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: []string{"Examples:", "Columns (TSV, in order):", "session_count", "project, sessionCount", "somniloq projects --config default --format json"},
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
