package main

import (
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestResolveTimeFlag(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	jst := time.FixedZone("JST", 9*60*60)

	tests := []struct {
		name    string
		value   string
		isUntil bool
		loc     *time.Location
		want    string
	}{
		{"relative since", "24h", false, time.UTC, "2026-03-28T12:00:00.000Z"},
		{"date since", "2026-03-28", false, time.UTC, "2026-03-28T00:00:00.000Z"},
		{"date until adds day", "2026-03-28", true, time.UTC, "2026-03-29T00:00:00.000Z"},
		{"datetime until no add", "2026-03-28T15:00", true, time.UTC, "2026-03-28T15:00:00.000Z"},
		{"relative until no add", "2h", true, time.UTC, "2026-03-29T10:00:00.000Z"},
		{"date since JST", "2026-03-28", false, jst, "2026-03-27T15:00:00.000Z"},
		{"date until JST", "2026-03-28", true, jst, "2026-03-28T15:00:00.000Z"},
		{"datetime since JST", "2026-03-28T15:00", false, jst, "2026-03-28T06:00:00.000Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTimeFlag(tt.value, now, tt.isUntil, tt.loc, dayBoundary{})
			if err != nil {
				t.Fatalf("resolveTimeFlag(%q, _, %v) error: %v", tt.value, tt.isUntil, err)
			}
			if got != tt.want {
				t.Errorf("resolveTimeFlag(%q, _, %v) = %q, want %q", tt.value, tt.isUntil, got, tt.want)
			}
		})
	}
}

func TestResolveTimeFlag_DayBoundaryAppliesOnlyToDateOnly(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)
	jst := time.FixedZone("JST", 9*60*60)
	boundary := dayBoundary{offset: 4 * time.Hour}

	tests := []struct {
		name    string
		value   string
		isUntil bool
		want    string
	}{
		{"date since starts at boundary", "2026-03-28", false, "2026-03-27T19:00:00.000Z"},
		{"date until ends at next boundary", "2026-03-28", true, "2026-03-28T19:00:00.000Z"},
		{"datetime ignores boundary", "2026-03-28T15:00", false, "2026-03-28T06:00:00.000Z"},
		{"relative ignores boundary", "2h", false, "2026-03-29T10:00:00.000Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveTimeFlag(tt.value, now, tt.isUntil, jst, boundary)
			if err != nil {
				t.Fatalf("resolveTimeFlag(%q): %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("resolveTimeFlag(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestSessionLogicalDay_UsesEndedAtThenStartedAt(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	boundary := dayBoundary{offset: 4 * time.Hour}

	tests := []struct {
		name    string
		session core.SessionRow
		want    string
	}{
		{
			name: "ended before boundary belongs to previous day",
			session: core.SessionRow{
				StartedAt: "2026-03-28T10:00:00Z",
				EndedAt:   "2026-03-28T18:30:00Z", // 2026-03-29 03:30 JST
			},
			want: "2026-03-28",
		},
		{
			name: "ended after boundary belongs to local day",
			session: core.SessionRow{
				StartedAt: "2026-03-28T10:00:00Z",
				EndedAt:   "2026-03-28T19:00:00Z", // 2026-03-29 04:00 JST
			},
			want: "2026-03-29",
		},
		{
			name: "started fallback",
			session: core.SessionRow{
				StartedAt: "2026-03-28T18:30:00Z",
			},
			want: "2026-03-28",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionLogicalDay(tt.session, boundary, jst); got != tt.want {
				t.Errorf("sessionLogicalDay = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildSessionFilter_SinceAfterUntil(t *testing.T) {
	// Use dates far apart so TZ offset cannot invert the ordering.
	_, err := buildSessionFilter("2027-01-01", "2026-01-01", "", config{}, dayBoundary{})
	if err == nil {
		t.Error("expected error for since >= until, got nil")
	}
}

func TestResolveTimeFlag_Error(t *testing.T) {
	now := time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC)

	for _, value := range []string{"", "abc", "2026-13-01"} {
		t.Run(value, func(t *testing.T) {
			_, err := resolveTimeFlag(value, now, false, time.UTC, dayBoundary{})
			if err == nil {
				t.Errorf("resolveTimeFlag(%q) expected error, got nil", value)
			}
		})
	}
}
