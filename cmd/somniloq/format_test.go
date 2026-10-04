package main

import (
	"errors"
	"testing"
	"time"
)

var errFailWriter = errors.New("write failed")

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) {
	return 0, errFailWriter
}

func TestFormatLocalTime(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)

	tests := []struct {
		name  string
		input string
		loc   *time.Location
		want  string
	}{
		{"RFC3339Nano", "2026-03-28T10:00:00.123Z", jst, "2026-03-28 19:00"},
		{"RFC3339", "2026-03-28T10:00:00Z", jst, "2026-03-28 19:00"},
		{"UTC loc", "2026-03-28T10:00:00Z", time.UTC, "2026-03-28 10:00"},
		{"invalid", "invalid", jst, "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatLocalTime(tt.input, tt.loc)
			if got != tt.want {
				t.Errorf("formatLocalTime(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatTimeRange(t *testing.T) {
	loc := time.UTC

	tests := []struct {
		name    string
		started string
		ended   string
		want    string
	}{
		{"both valid", "2026-03-28T10:00:00Z", "2026-03-28T10:30:00Z", "2026-03-28 10:00 ~ 2026-03-28 10:30"},
		{"ended empty", "2026-03-28T10:00:00Z", "", "2026-03-28 10:00 ~"},
		{"ended invalid", "2026-03-28T10:00:00Z", "invalid", "2026-03-28 10:00 ~ invalid"},
		{"started empty", "", "2026-03-28T10:30:00Z", " ~ 2026-03-28 10:30"},
		{"both empty", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimeRange(tt.started, tt.ended, loc)
			if got != tt.want {
				t.Errorf("formatTimeRange(%q, %q) = %q, want %q", tt.started, tt.ended, got, tt.want)
			}
		})
	}
}
