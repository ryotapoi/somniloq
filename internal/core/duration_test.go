package core

import (
	"testing"
	"time"
)

func TestParseTimeRef(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		input    string
		wantTime time.Time
		wantDate bool
		wantErr  bool
	}{
		{"relative 24h", "24h", time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC), false, false},
		{"relative 7d", "7d", time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC), false, false},
		{"absolute date", "2026-03-28", time.Date(2026, 3, 28, 0, 0, 0, 0, time.UTC), true, false},
		{"absolute datetime", "2026-03-28T15:00", time.Date(2026, 3, 28, 15, 0, 0, 0, time.UTC), false, false},
		{"empty", "", time.Time{}, false, true},
		{"invalid", "abc", time.Time{}, false, true},
		{"invalid date", "2026-13-01", time.Time{}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, dateOnly, err := ParseTimeRef(tt.input, now)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseTimeRef(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if !got.Equal(tt.wantTime) {
				t.Errorf("ParseTimeRef(%q) time = %v, want %v", tt.input, got, tt.wantTime)
			}
			if dateOnly != tt.wantDate {
				t.Errorf("ParseTimeRef(%q) dateOnly = %v, want %v", tt.input, dateOnly, tt.wantDate)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		input   string
		want    time.Duration
		wantErr bool
	}{
		{"24h", 24 * time.Hour, false},
		{"7d", 7 * 24 * time.Hour, false},
		{"30m", 30 * time.Minute, false},
		{"0h", 0, false},
		{"", 0, true},
		{"abc", 0, true},
		{"d", 0, true},
		{"-1h", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
