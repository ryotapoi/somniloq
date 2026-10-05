package core

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestPatternFixture(t *testing.T) {
	data, err := os.ReadFile("../ingest/testdata/v0.14.0-query/expected.json")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Cases []struct {
			Name, Text             string
			Patterns               []string
			ExpectedSpans          [][2]int
			ExpectedPatternIndexes [][]int
			Exit                   int
		}
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	for _, c := range oracle.Cases {
		if len(c.Patterns) == 0 || c.Text == "" && c.Exit == 0 {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			m, err := CompilePatterns(c.Patterns, false)
			if c.Exit == 2 {
				if err == nil {
					t.Fatal("accepted invalid pattern")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			spans := m.FindAll(c.Text)
			got := [][2]int{}
			indexes := [][]int{}
			for _, s := range spans {
				got = append(got, [2]int{s.StartByte, s.EndByte})
				indexes = append(indexes, s.PatternIndexes)
				if s.MatchText != c.Text[s.StartByte:s.EndByte] {
					t.Fatal(s)
				}
			}
			if !reflect.DeepEqual(got, c.ExpectedSpans) {
				t.Fatalf("%v want %v", got, c.ExpectedSpans)
			}
			if c.ExpectedPatternIndexes != nil && !reflect.DeepEqual(indexes, c.ExpectedPatternIndexes) {
				t.Fatalf("%v want %v", indexes, c.ExpectedPatternIndexes)
			}
		})
	}
}
func TestMatchLines(t *testing.T) {
	for _, c := range []struct {
		text       string
		start, end int
		want       string
	}{
		{"one\ntwo\nthree", 1, 6, "one\ntwo"}, {"one\ntwo", 3, 4, "one"}, {"one\ntwo", 4, 4, "two"}, {"one\n", 4, 4, ""}, {"one\ntwo", 7, 7, "two"}, {"one\ntwo", 3, 3, "one"},
	} {
		if got := matchLines(c.text, c.start, c.end); got != c.want {
			t.Errorf("%q [%d,%d): %q want %q", c.text, c.start, c.end, got, c.want)
		}
	}
}
func TestPatternModes(t *testing.T) {
	for _, c := range []struct {
		pattern, text string
		fixed         bool
		count         int
	}{{"foo", "Foo", false, 0}, {"(?i)foo", "Foo", false, 1}, {"a.b", "a.b axb", true, 1}, {"a.b", "a.b axb", false, 2}} {
		m, e := CompilePatterns([]string{c.pattern}, c.fixed)
		if e != nil {
			t.Fatal(e)
		}
		if n := len(m.FindAll(c.text)); n != c.count {
			t.Errorf("%+v got %d", c, n)
		}
	}
}
