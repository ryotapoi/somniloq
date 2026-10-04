package main

import (
	"fmt"
	"io"
	"strings"
	"time"
)

func formatLocalTime(utcStr string, loc *time.Location) string {
	t, err := time.Parse(time.RFC3339Nano, utcStr)
	if err != nil {
		return utcStr
	}
	return t.In(loc).Format("2006-01-02 15:04")
}

func formatTimeRange(startedAt, endedAt string, loc *time.Location) string {
	if startedAt == "" && endedAt == "" {
		return ""
	}
	s := formatLocalTime(startedAt, loc)
	if endedAt == "" {
		return s + " ~"
	}
	return s + " ~ " + formatLocalTime(endedAt, loc)
}

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}

func writeUsageError(w io.Writer, message string) {
	fmt.Fprintf(w, "error: %s\n", message)
}
