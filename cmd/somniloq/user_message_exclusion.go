package main

import (
	"fmt"
	"regexp"
	"strings"
)

type userMessageMatcher struct {
	patterns []*regexp.Regexp
}

func newUserMessageMatcher(cfg config, overrides []string, disabled bool) (userMessageMatcher, error) {
	if disabled && len(overrides) > 0 {
		return userMessageMatcher{}, fmt.Errorf("--exclude-user-message-pattern cannot be combined with --no-exclude-user-messages")
	}
	if disabled {
		return userMessageMatcher{}, nil
	}
	patterns := cfg.ExcludeUserMessagePatterns
	if len(overrides) > 0 {
		patterns = overrides
	}
	compiled, err := compileUserMessagePatterns(patterns)
	if err != nil {
		return userMessageMatcher{}, err
	}
	return userMessageMatcher{patterns: compiled}, nil
}

func compileUserMessagePatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid excludeUserMessagePatterns pattern %q: %w", pattern, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

func (m userMessageMatcher) excludes(content string) bool {
	text := strings.TrimSpace(content)
	for _, pattern := range m.patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

type stringListFlag []string

func (v *stringListFlag) String() string {
	return strings.Join(*v, ",")
}

func (v *stringListFlag) Set(value string) error {
	*v = append(*v, value)
	return nil
}
