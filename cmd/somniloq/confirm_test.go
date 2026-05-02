package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestConfirmFullImport(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"y confirms", "y\n", true},
		{"Y confirms", "Y\n", true},
		{"yes rejects", "yes\n", false},
		{"empty rejects", "\n", false},
		{"n rejects", "n\n", false},
		{"EOF rejects", "", false},
		{"y with spaces confirms", " y \n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			var out bytes.Buffer

			got := confirmFullImport(in, &out)

			if got != tt.want {
				t.Errorf("input %q: got %v, want %v", tt.input, got, tt.want)
			}
			if !strings.Contains(out.String(), "[y/N]") {
				t.Errorf("expected prompt with [y/N], got %q", out.String())
			}
		})
	}
}

func TestConfirmBackfillDelete(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"y confirms", "y\n", true},
		{"Y confirms", "Y\n", true},
		{"yes rejects", "yes\n", false},
		{"empty rejects", "\n", false},
		{"n rejects", "n\n", false},
		{"EOF rejects", "", false},
		{"y with spaces confirms", " y \n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			var out bytes.Buffer

			got := confirmBackfillDelete(in, &out, 3)

			if got != tt.want {
				t.Errorf("input %q: got %v, want %v", tt.input, got, tt.want)
			}
			if !strings.Contains(out.String(), "[y/N]") {
				t.Errorf("expected prompt with [y/N], got %q", out.String())
			}
		})
	}

	t.Run("count appears in prompt", func(t *testing.T) {
		var out bytes.Buffer
		_ = confirmBackfillDelete(strings.NewReader("n\n"), &out, 42)
		if !strings.Contains(out.String(), fmt.Sprintf("%d", 42)) {
			t.Errorf("expected count 42 in prompt, got %q", out.String())
		}
	})
}
