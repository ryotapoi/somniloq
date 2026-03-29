package main

import (
	"bytes"
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
