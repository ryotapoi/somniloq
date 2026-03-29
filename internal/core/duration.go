package core

import (
	"fmt"
	"strconv"
	"time"
)

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
