package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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
		{"unterminated y confirms", "y", true},
		{"y with spaces confirms", " y \n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := strings.NewReader(tt.input)
			var out bytes.Buffer

			got, err := confirmFullImport(in, &out)
			if err != nil {
				t.Fatalf("confirmFullImport: %v", err)
			}

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

			got, err := confirmBackfillDelete(in, &out, 3)
			if err != nil {
				t.Fatalf("confirmBackfillDelete: %v", err)
			}

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
		_, err := confirmBackfillDelete(strings.NewReader("n\n"), &out, 42)
		if err != nil {
			t.Fatalf("confirmBackfillDelete: %v", err)
		}
		if !strings.Contains(out.String(), fmt.Sprintf("%d", 42)) {
			t.Errorf("expected count 42 in prompt, got %q", out.String())
		}
	})
}

type readError struct {
	data  string
	err   error
	reads int
}

func (r *readError) Read(p []byte) (int, error) {
	r.reads++
	if r.data == "" {
		return 0, r.err
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func TestConfirmYesNo_IOErrors(t *testing.T) {
	readErr := errors.New("read failed")
	tests := []struct {
		name string
		in   io.Reader
		out  io.Writer
		want error
	}{
		{"read", &readError{err: readErr}, &bytes.Buffer{}, readErr},
		{"data and read error", &readError{data: "y\\n", err: readErr}, &bytes.Buffer{}, readErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirmed, err := confirmYesNo(tt.in, tt.out, "Continue? [y/N] ")
			if confirmed {
				t.Error("confirmation succeeded after I/O error")
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("prompt write does not read", func(t *testing.T) {
		in := &readError{err: readErr}
		confirmed, err := confirmYesNo(in, failWriter{}, "Continue? [y/N] ")
		if confirmed {
			t.Error("confirmation succeeded after prompt write error")
		}
		if !errors.Is(err, errFailWriter) {
			t.Errorf("error = %v, want %v", err, errFailWriter)
		}
		if in.reads != 0 {
			t.Errorf("reader was called %d times after prompt write error, want 0", in.reads)
		}
	})

	t.Run("scanner token too long", func(t *testing.T) {
		confirmed, err := confirmYesNo(strings.NewReader(strings.Repeat("y", 128*1024)), &bytes.Buffer{}, "Continue? [y/N] ")
		if confirmed {
			t.Error("confirmation succeeded after scanner error")
		}
		if err == nil {
			t.Error("error = nil, want scanner error")
		}
	})
}
