package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestSearchCmd_OutputColumns(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"second"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}

	want := fmt.Sprintf("sess-1\t%s\tsecond question after blank lines\n",
		formatLocalTime("2026-03-28T15:03:00Z", time.Local))
	if out.String() != want {
		t.Errorf("output = %q, want %q", out.String(), want)
	}
}

func TestSearchCmd_ExcludesSidechain(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"sidechain"}, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, errOut.String())
	}
	if out.String() != "" {
		t.Errorf("output = %q, want empty (sidechain only match)", out.String())
	}
}

func TestSearchCmd_MissingQueryPrintsUsage(t *testing.T) {
	db := newOutlineTestDB(t)

	var out, errOut bytes.Buffer
	code, err := searchCmd(nil, staticDB(db), &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "usage: "+searchUsageLine) {
		t.Errorf("stderr = %q, want usage line", errOut.String())
	}
}

func TestSearchCmd_TooManyArguments(t *testing.T) {
	openDB := func() (*core.DB, error) {
		t.Fatal("openDB must not be called for invalid arguments")
		return nil, nil
	}

	var out, errOut bytes.Buffer
	code, err := searchCmd([]string{"one", "two"}, openDB, &out, &errOut)
	if err != nil {
		t.Fatalf("searchCmd: %v", err)
	}
	if code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "too many arguments") {
		t.Errorf("stderr = %q, want too many arguments", errOut.String())
	}
}

func TestSearchSnippet(t *testing.T) {
	long := strings.Repeat("a", 60) + "NEEDLE" + strings.Repeat("b", 60)

	tests := []struct {
		name    string
		content string
		query   string
		want    string
	}{
		{"short content untouched", "hello world", "world", "hello world"},
		{"truncated both sides", long, "NEEDLE",
			"..." + strings.Repeat("a", 40) + "NEEDLE" + strings.Repeat("b", 40) + "..."},
		{"match at head", "NEEDLE" + strings.Repeat("b", 60), "NEEDLE",
			"NEEDLE" + strings.Repeat("b", 40) + "..."},
		{"match at tail", strings.Repeat("a", 60) + "NEEDLE", "NEEDLE",
			"..." + strings.Repeat("a", 40) + "NEEDLE"},
		{"ascii case-insensitive fallback", strings.Repeat("a", 60) + "needle" + strings.Repeat("b", 60), "NEEDLE",
			"..." + strings.Repeat("a", 40) + "needle" + strings.Repeat("b", 40) + "..."},
		{"multibyte runes counted not bytes", strings.Repeat("あ", 50) + "鍵" + strings.Repeat("い", 50), "鍵",
			"..." + strings.Repeat("あ", 40) + "鍵" + strings.Repeat("い", 40) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchSnippet(tt.content, tt.query); got != tt.want {
				t.Errorf("searchSnippet = %q, want %q", got, tt.want)
			}
		})
	}
}

// searchSnippet must never panic on adversarial content: ToLower can grow
// non-ASCII bytes (İ becomes a 3-byte sequence), pushing the fallback index
// to or past len(content), and DB content is not guaranteed to be valid
// UTF-8.
func TestSearchSnippet_NoPanicOnAdversarialContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		query   string
	}{
		// 4 × İ (2 bytes) lower to 4 × i̇ (3 bytes): the lowered match
		// offset for "auth" is 12 == len(content).
		{"tolower growth lands on len(content)", "İİİİauth", "AUTH"},
		{"tolower growth lands past len(content)", strings.Repeat("İ", 10) + "auth", "AUTH"},
		{"invalid utf-8 around match", "a\x80\x80\x80needle\x80b", "NEEDLE"},
		{"invalid utf-8 only", "\x80\x80\x80", "\x80"},
		{"query longer than content", "ab", "abc"},
		{"empty content", "", "query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = searchSnippet(tt.content, tt.query) // must not panic
		})
	}
}
