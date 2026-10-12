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
func TestFindAllLineText(t *testing.T) {
	for _, c := range []struct {
		name, text string
		patterns   []string
		want       []MatchSpan
	}{
		{"multiline", "one\ntwo\nthree", []string{"ne\\ntw"}, []MatchSpan{{1, 6, []int{1}, "ne\ntw", "one\ntwo"}}},
		{"multiline ending at newline", "one\ntwo", []string{"ne\\n"}, []MatchSpan{{1, 4, []int{1}, "ne\n", "one"}}},
		{"newline", "one\ntwo", []string{"\\n"}, []MatchSpan{{3, 4, []int{1}, "\n", "one"}}},
		{"zero width at newline", "one\ntwo", []string{"(?m)$"}, []MatchSpan{{3, 3, []int{1}, "", "one"}, {7, 7, []int{1}, "", "two"}}},
		{"zero width after newline", "one\ntwo", []string{"(?m)^"}, []MatchSpan{{0, 0, []int{1}, "", "one"}, {4, 4, []int{1}, "", "two"}}},
		{"zero width at text end", "one\ntwo", []string{"$"}, []MatchSpan{{7, 7, []int{1}, "", "two"}}},
		{"empty final line", "one\n", []string{"$"}, []MatchSpan{{4, 4, []int{1}, "", ""}}},
		{"overlapping spans", "one\ntwo", []string{"one\\ntwo", "two"}, []MatchSpan{{0, 7, []int{1}, "one\ntwo", "one\ntwo"}, {4, 7, []int{2}, "two", "two"}}},
		{"utf8 byte offsets", "猫a\n犬b", []string{"a", "犬"}, []MatchSpan{{3, 4, []int{1}, "a", "猫a"}, {5, 8, []int{2}, "犬", "犬b"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			matcher, err := CompilePatterns(c.patterns, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := matcher.FindAll(c.text); !reflect.DeepEqual(got, c.want) {
				t.Errorf("%+v want %+v", got, c.want)
			}
		})
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
