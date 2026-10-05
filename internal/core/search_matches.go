package core

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// PatternMatcher preserves the caller's pattern order for occurrence indexes.
type PatternMatcher struct{ patterns []*regexp.Regexp }

func CompilePatterns(patterns []string, fixed bool) (*PatternMatcher, error) {
	if len(patterns) == 0 {
		return nil, fmt.Errorf("at least one pattern is required")
	}
	m := &PatternMatcher{}
	for i, p := range patterns {
		if p == "" {
			return nil, fmt.Errorf("empty pattern %d", i+1)
		}
		if fixed {
			p = regexp.QuoteMeta(p)
		}
		r, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern %d: %w", i+1, err)
		}
		m.patterns = append(m.patterns, r)
	}
	return m, nil
}

type MatchSpan struct {
	StartByte, EndByte  int
	PatternIndexes      []int
	MatchText, LineText string
}

func (m *PatternMatcher) FindAll(text string) []MatchSpan {
	bySpan := map[[2]int][]int{}
	for i, p := range m.patterns {
		for _, s := range p.FindAllStringIndex(text, -1) {
			k := [2]int{s[0], s[1]}
			bySpan[k] = append(bySpan[k], i+1)
		}
	}
	spans := make([]MatchSpan, 0, len(bySpan))
	for k, indexes := range bySpan {
		spans = append(spans, MatchSpan{k[0], k[1], indexes, text[k[0]:k[1]], matchLines(text, k[0], k[1])})
	}
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].StartByte != spans[j].StartByte {
			return spans[i].StartByte < spans[j].StartByte
		}
		return spans[i].EndByte < spans[j].EndByte
	})
	return spans
}

// LF belongs to the preceding line; an exclusive end after LF does not include
// the following line. A zero-width match after LF belongs to the following line.
func matchLines(text string, start, end int) string {
	first := strings.LastIndex(text[:start], "\n") + 1
	last := start
	if end > start {
		last = end - 1
	}
	stop := strings.IndexByte(text[last:], '\n')
	if stop < 0 {
		return text[first:]
	}
	return text[first : last+stop]
}

type SearchOccurrence struct {
	REF              string  `json:"ref"`
	MessageNumber    int     `json:"messageNumber"`
	OccurrenceNumber int     `json:"occurrenceNumber"`
	Role             string  `json:"role"`
	Timestamp        *string `json:"timestamp"`
	StartByte        int     `json:"startByte"`
	EndByte          int     `json:"endByte"`
	PatternIndexes   []int   `json:"patternIndexes"`
	MatchText        string  `json:"matchText"`
	LineText         string  `json:"lineText"`
}

// SearchOccurrences reads one selected conversation and confirmed descendants.
// Callers use ReadSnapshot to keep resolution, filters and bodies consistent.
func (d *DB) SearchOccurrences(ref string, filter SessionFilter, matcher *PatternMatcher, all bool) ([]SearchOccurrence, error) {
	scope, err := d.ResolveSession(ref)
	if err != nil {
		return nil, err
	}
	if scope == nil {
		return nil, fmt.Errorf("session not found: %s", ref)
	}
	// Reuse the current SQL project and timestamp semantics before matching.
	rows, err := d.SearchMessages(filter, "", SearchPagination{SessionREF: ref})
	if err != nil {
		return nil, err
	}
	allowed := map[string]map[string]bool{}
	for _, r := range rows {
		if allowed[r.REF] == nil {
			allowed[r.REF] = map[string]bool{}
		}
		allowed[r.REF][r.UUID] = true
	}
	members := append([]SessionRow(nil), scope.Descendants...)
	sort.Slice(members, func(i, j int) bool { return members[i].REF < members[j].REF })
	result := []SearchOccurrence{}
	seen := make([]bool, len(matcher.patterns))
	for _, s := range members {
		messages, err := d.GetIdentityMessages(s.InputID, s.Source, s.Identity)
		if err != nil {
			return nil, err
		}
		for _, msg := range messages {
			if !allowed[s.REF][msg.UUID] {
				continue
			}
			var timestamp *string
			if msg.Timestamp != "" {
				v := msg.Timestamp
				timestamp = &v
			}
			for i, span := range matcher.FindAll(msg.Content) {
				for _, index := range span.PatternIndexes {
					seen[index-1] = true
				}
				result = append(result, SearchOccurrence{s.REF, msg.Number, i + 1, msg.Role, timestamp, span.StartByte, span.EndByte, span.PatternIndexes, span.MatchText, span.LineText})
			}
		}
	}
	if all {
		for _, hit := range seen {
			if !hit {
				return []SearchOccurrence{}, nil
			}
		}
	}
	return result, nil
}
