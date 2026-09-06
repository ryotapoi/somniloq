package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
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
		{"both empty", "", "", " ~"},
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

func TestFormatSession_WithTitle(t *testing.T) {
	var buf bytes.Buffer

	session := core.SessionRow{
		SessionID:    "abc-123",
		StartedAt:    "2026-03-28T10:00:00Z",
		CustomTitle:  "Fix login bug",
		MessageCount: 2,
	}
	messages := []core.MessageRow{
		{UUID: "m1", Role: "user", Content: "fix the login", Timestamp: "2026-03-28T10:00:00Z"},
		{UUID: "m2", Role: "assistant", Content: "done", Timestamp: "2026-03-28T10:01:00Z"},
	}
	displayName := "-Users-test-proj"

	if err := formatSession(&buf, session, displayName, messages, time.UTC); err != nil {
		t.Fatalf("formatSession failed: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "## Fix login bug\n") {
		t.Errorf("expected h2 with custom_title, got:\n%s", got)
	}
	if !strings.Contains(got, "- **Session**: `abc-123`") {
		t.Errorf("expected session ID in metadata, got:\n%s", got)
	}
	if !strings.Contains(got, "- **Project**: `-Users-test-proj`") {
		t.Errorf("expected project in metadata, got:\n%s", got)
	}
	if !strings.Contains(got, "- **Started**: `2026-03-28 10:00 ~`") {
		t.Errorf("expected started_at with time range in metadata, got:\n%s", got)
	}
	if !strings.Contains(got, "### User\n") {
		t.Errorf("expected User heading, got:\n%s", got)
	}
	if !strings.Contains(got, "fix the login") {
		t.Errorf("expected user content, got:\n%s", got)
	}
	if !strings.Contains(got, "### Assistant\n") {
		t.Errorf("expected Assistant heading, got:\n%s", got)
	}
	if !strings.Contains(got, "done") {
		t.Errorf("expected assistant content, got:\n%s", got)
	}
}

func TestFormatSession_EmptyTitle(t *testing.T) {
	var buf bytes.Buffer

	session := core.SessionRow{
		SessionID: "abc-123",
		StartedAt: "2026-03-28T10:00:00Z",
	}

	if err := formatSession(&buf, session, "-Users-test", nil, time.UTC); err != nil {
		t.Fatalf("formatSession failed: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "## abc-123\n") {
		t.Errorf("expected h2 with session_id fallback, got:\n%s", got)
	}
}

func TestFormatSession_TitleWithNewline(t *testing.T) {
	var buf bytes.Buffer

	session := core.SessionRow{
		SessionID:   "abc-123",
		StartedAt:   "2026-03-28T10:00:00Z",
		CustomTitle: "line1\nline2",
	}

	if err := formatSession(&buf, session, "-Users-test", nil, time.UTC); err != nil {
		t.Fatalf("formatSession failed: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "## line1 line2\n") {
		t.Errorf("expected newline sanitized in title, got:\n%s", got)
	}
}

func TestFormatSession_ReturnsWriteError(t *testing.T) {
	err := formatSession(failWriter{}, core.SessionRow{SessionID: "abc-123"}, "project", nil, time.UTC)
	if !errors.Is(err, errFailWriter) {
		t.Errorf("formatSession error = %v, want %v", err, errFailWriter)
	}
}
