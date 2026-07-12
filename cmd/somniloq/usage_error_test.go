package main

import (
	"bytes"
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestUsageErrorsKeepExactStderr(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid arguments")
		return nil, nil
	}

	tests := []struct {
		name string
		run  func(*bytes.Buffer) (int, error)
		want string
	}{
		{
			name: "show",
			run: func(errOut *bytes.Buffer) (int, error) {
				return showCmd([]string{"one", "two"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: "error: too many arguments\nusage: " + showUsageLine + "\n",
		},
		{
			name: "search",
			run: func(errOut *bytes.Buffer) (int, error) {
				return searchCmd([]string{"one", "two"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: "error: too many arguments\nusage: " + searchUsageLine + "\n",
		},
		{
			name: "outline",
			run: func(errOut *bytes.Buffer) (int, error) {
				return outlineCmd([]string{"one", "two"}, openDB, &bytes.Buffer{}, errOut)
			},
			want: "error: too many arguments\nusage: " + outlineUsageLine + "\n",
		},
		{
			name: "projects",
			run: func(errOut *bytes.Buffer) (int, error) {
				return projectsCmd([]string{"one"}, openDB, config{}, &bytes.Buffer{}, errOut)
			},
			want: "error: unexpected arguments\nusage: somniloq projects [--since <time>] [--until <time>] [--short] [--format <fmt>]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var errOut bytes.Buffer
			code, err := tt.run(&errOut)
			if err != nil {
				t.Fatalf("command returned error: %v", err)
			}
			if code != 1 {
				t.Fatalf("exit code = %d, want 1", code)
			}
			if got := errOut.String(); got != tt.want {
				t.Errorf("stderr = %q, want %q", got, tt.want)
			}
		})
	}
}
