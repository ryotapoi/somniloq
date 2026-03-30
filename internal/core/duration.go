package core

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTimeRef parses a time reference that is either a relative duration
// ("24h", "7d") or an absolute date/datetime ("2026-03-28", "2026-03-28T15:00").
// Absolute dates are interpreted in the given location. Returns the resolved
// time and whether only a date (no time component) was provided.
func ParseTimeRef(s string, now time.Time, loc *time.Location) (time.Time, bool, error) {
	if strings.Contains(s, "-") {
		if t, err := time.ParseInLocation("2006-01-02T15:04", s, loc); err == nil {
			return t, false, nil
		}
		if t, err := time.ParseInLocation("2006-01-02", s, loc); err == nil {
			return t, true, nil
		}
		return time.Time{}, false, fmt.Errorf("invalid date: %q (use YYYY-MM-DD or YYYY-MM-DDThh:mm)", s)
	}
	d, err := ParseDuration(s)
	if err != nil {
		return time.Time{}, false, err
	}
	return now.Add(-d), false, nil
}

// ParseDuration parses a duration string like "24h", "7d", "30m".
// Supported units: m (minutes), h (hours), d (days).
func ParseDuration(s string) (time.Duration, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration: %q", s)
	}

	unit := s[len(s)-1]
	numStr := s[:len(s)-1]

	n, err := strconv.Atoi(numStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %q", s)
	}
	if n < 0 {
		return 0, fmt.Errorf("invalid duration: %q (negative)", s)
	}

	switch unit {
	case 'm':
		return time.Duration(n) * time.Minute, nil
	case 'h':
		return time.Duration(n) * time.Hour, nil
	case 'd':
		return time.Duration(n) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("invalid duration unit %q in %q (use m, h, or d)", string(unit), s)
	}
}
