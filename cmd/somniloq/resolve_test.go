package main

import (
	"testing"
	"time"
)

func TestResolveTimeFlag(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		value   string
		isUntil bool
		want    string
	}{
		{"relative since", "24h", false, "2026-03-28T12:00:00.000Z"},
		{"date since", "2026-03-28", false, "2026-03-28T00:00:00.000Z"},
		{"date until adds 24h", "2026-03-28", true, "2026-03-29T00:00:00.000Z"},
		{"datetime until no add", "2026-03-28T15:00", true, "2026-03-28T15:00:00.000Z"},
		{"relative until no add", "2h", true, "2026-03-29T10:00:00.000Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTimeFlag(tt.value, now, tt.isUntil)
			if err != nil {
				t.Fatalf("resolveTimeFlag(%q, _, %v) error: %v", tt.value, tt.isUntil, err)
			}
			if got != tt.want {
				t.Errorf("resolveTimeFlag(%q, _, %v) = %q, want %q", tt.value, tt.isUntil, got, tt.want)
			}
		})
	}
}

func TestBuildSessionFilter_SinceAfterUntil(t *testing.T) {
	_, err := buildSessionFilter("2026-03-29", "2026-03-28", "")
	if err == nil {
		t.Error("expected error for since >= until, got nil")
	}
}

func TestResolveTimeFlag_Error(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	for _, value := range []string{"", "abc", "2026-13-01"} {
		t.Run(value, func(t *testing.T) {
			_, err := resolveTimeFlag(value, now, false)
			if err == nil {
				t.Errorf("resolveTimeFlag(%q) expected error, got nil", value)
			}
		})
	}
}
