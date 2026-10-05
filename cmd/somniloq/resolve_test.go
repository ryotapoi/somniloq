package main

import (
	"testing"
	"time"
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
		{"relative since", "24h", false, time.UTC, "2026-03-28T12:00:00Z"},
		{"date since", "2026-03-28", false, time.UTC, "2026-03-28T00:00:00Z"},
		{"date until adds day", "2026-03-28", true, time.UTC, "2026-03-29T00:00:00Z"},
		{"datetime until no add", "2026-03-28T15:00", true, time.UTC, "2026-03-28T15:00:00Z"},
		{"relative until no add", "2h", true, time.UTC, "2026-03-29T10:00:00Z"},
		{"date since JST", "2026-03-28", false, jst, "2026-03-27T15:00:00Z"},
		{"date until JST", "2026-03-28", true, jst, "2026-03-28T15:00:00Z"},
		{"datetime since JST", "2026-03-28T15:00", false, jst, "2026-03-28T06:00:00Z"},
		{"RFC3339 UTC ignores location", "2026-03-28T15:00:37Z", false, jst, "2026-03-28T15:00:37Z"},
		{"RFC3339 offset resolves same instant", "2026-03-29T00:00:37+09:00", false, time.UTC, "2026-03-28T15:00:37Z"},
		{"RFC3339 fractional seconds preserve precision", "2026-03-28T15:00:37.1235Z", false, time.UTC, "2026-03-28T15:00:37.1235Z"},
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
		{"date since starts at boundary", "2026-03-28", false, "2026-03-27T19:00:00Z"},
		{"date until ends at next boundary", "2026-03-28", true, "2026-03-28T19:00:00Z"},
		{"datetime ignores boundary", "2026-03-28T15:00", false, "2026-03-28T06:00:00Z"},
		{"relative ignores boundary", "2h", false, "2026-03-29T10:00:00Z"},
		{"RFC3339 ignores boundary", "2026-03-29T00:00:37+09:00", false, "2026-03-28T15:00:37Z"},
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

func TestBuildSessionFilter_SinceAfterUntil(t *testing.T) {
	// Use dates far apart so TZ offset cannot invert the ordering.
	_, err := buildSessionFilter("2027-01-01", "2026-01-01", "", config{}, dayBoundary{})
	if err == nil {
		t.Error("expected error for since >= until, got nil")
	}
}

func TestBuildSessionFilter_OrdersFractionalSecondsByInstant(t *testing.T) {
	_, err := buildSessionFilterAt(time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), "2026-03-28T10:00:37.1231Z", "2026-03-28T10:00:37.1235Z", "", config{}, dayBoundary{})
	if err != nil {
		t.Fatalf("ordered fractional-second range: %v", err)
	}

	for _, until := range []string{"2026-03-28T10:00:37.1230Z", "2026-03-28T10:00:37.1231Z", "2026-03-28T19:00:37.1231+09:00"} {
		_, err := buildSessionFilterAt(time.Date(2026, 3, 29, 12, 0, 0, 0, time.UTC), "2026-03-28T10:00:37.1231Z", until, "", config{}, dayBoundary{})
		if err == nil {
			t.Errorf("non-increasing range ending at %q was accepted", until)
		}
	}
}

func TestDayBoundaryDST(t *testing.T) {
	boundary := dayBoundary{offset: 4 * time.Hour}
	for _, tt := range []struct {
		name, day, previousDay, nextBoundary, boundaryInstant, location string
	}{
		{"spring", "2026-03-08", "2026-03-07", "2026-03-09T08:00:00Z", "2026-03-08T08:00:00Z", "America/New_York"},
		{"fall", "2026-11-01", "2026-10-31", "2026-11-02T09:00:00Z", "2026-11-01T09:00:00Z", "America/New_York"},
		{"midnight gap", "2026-09-06", "2026-09-05", "2026-09-07T07:00:00Z", "2026-09-06T07:00:00Z", "America/Santiago"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			loc, err := time.LoadLocation(tt.location)
			if err != nil {
				t.Fatal(err)
			}
			for _, filter := range []struct {
				day   string
				until bool
				want  string
			}{
				{tt.day, false, tt.boundaryInstant},
				{tt.previousDay, true, tt.boundaryInstant},
				{tt.day, true, tt.nextBoundary},
			} {
				got, err := resolveTimeFlag(filter.day, time.Time{}, filter.until, loc, boundary)
				if err != nil {
					t.Fatal(err)
				}
				if got != filter.want {
					t.Errorf("date %s until=%v: got %s, want %s", filter.day, filter.until, got, filter.want)
				}
			}
		})
	}
}

func TestDayBoundaryDSTTransitionTimeSharesInstant(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		day, previousDay, boundary string
		hour, minute               int
	}{
		{"2026-03-08", "2026-03-07", "02:30", 2, 30},
		{"2026-11-01", "2026-10-31", "01:30", 1, 30},
	} {
		t.Run(tt.day, func(t *testing.T) {
			boundary, err := parseDayBoundary(tt.boundary)
			if err != nil {
				t.Fatal(err)
			}
			date, err := time.Parse("2006-01-02", tt.day)
			if err != nil {
				t.Fatal(err)
			}
			// Go may choose either side of a DST transition; both paths must use that instant.
			instant := time.Date(date.Year(), date.Month(), date.Day(), tt.hour, tt.minute, 0, 0, loc)
			wantInstant := instant.UTC().Format(time.RFC3339Nano)
			for _, filter := range []struct {
				day   string
				until bool
			}{{tt.day, false}, {tt.previousDay, true}} {
				got, err := resolveTimeFlag(filter.day, time.Time{}, filter.until, loc, boundary)
				if err != nil {
					t.Fatal(err)
				}
				if got != wantInstant {
					t.Errorf("date %s until=%v: got %s, want shared instant %s", filter.day, filter.until, got, wantInstant)
				}
			}
		})
	}
}
