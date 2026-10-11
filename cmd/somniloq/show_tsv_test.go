package main

import (
	"encoding/json"
	"testing"
)

func TestShowTSVStringControls(t *testing.T) {
	for _, c := range []struct{ name, input, want string }{
		{"C0 boundaries", "\x00\x01\x1f", `\x00\x01\x1f`},
		{"terminal sequences", "\x1b[2J\x1b]0;title\x07\x08", `\x1b[2J\x1b]0;title\x07\x08`},
		{"DEL and C1 boundaries", "\x7f\u0080\u0085\u009b\u009f", `\x7f\x80\x85\x9b\x9f`},
		{"existing escapes", "\\\t\n\r", `\\\t\n\r`},
		{"literal escapes and null", `\x1b\N\t`, `\\x1b\\N\\t`},
		{"Unicode and noncontrols", " 日本語🙂\u00a0", " 日本語🙂\u00a0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := showTSVString(c.input); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestShowTSVJSONControls(t *testing.T) {
	input := []string{"\x00\x1b\x7f\u0080\u009f日本語", `\x1b\u0080`}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	got := showTSVJSON(data)
	want := `["\u0000\u001b\u007f\u0080\u009f日本語","\\x1b\\u0080"]`
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	var restored []string
	if err := json.Unmarshal([]byte(got), &restored); err != nil {
		t.Fatal(err)
	}
	for i := range input {
		if restored[i] != input[i] {
			t.Fatalf("restored %q, want %q", restored[i], input[i])
		}
	}
}
