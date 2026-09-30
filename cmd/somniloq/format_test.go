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

func TestFormatSession_WithTitle(t *testing.T) {
	var buf bytes.Buffer

	session := core.SessionRow{
		Source:       core.SourceClaudeCode,
		SessionID:    "abc-123",
		StartedAt:    "2026-03-28T10:00:00Z",
		CustomTitle:  "Title\twith\nline",
		EndedAt:      "2026-03-28T10:30:00Z",
		MessageCount: 2,
	}
	messages := []core.MessageRow{
		{UUID: "m1", Role: "user", Content: "fix\tthe login\nwith detail", Timestamp: "2026-03-28T10:00:00Z"},
		{UUID: "m2", Role: "assistant", Content: "done", Timestamp: "2026-03-28T10:01:00Z"},
	}
	displayName := "-Users-test-proj"

	if err := formatSession(&buf, session, displayName, messages, time.UTC); err != nil {
		t.Fatalf("formatSession failed: %v", err)
	}
	const want = "## Title\twith line\n\n" +
		"- **Session**: `abc-123`\n- **Source**: `claude_code`\n" +
		"- **Project**: `-Users-test-proj`\n- **Started**: `2026-03-28 10:00 ~ 2026-03-28 10:30`\n" +
		"\n### User\n\nfix\tthe login\nwith detail\n\n### Assistant\n\ndone\n"
	if got := buf.String(); got != want {
		t.Errorf("Markdown = %q, want %q", got, want)
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

func TestFormatSession_ReturnsWriteError(t *testing.T) {
	err := formatSession(failWriter{}, core.SessionRow{SessionID: "abc-123"}, "project", nil, time.UTC)
	if !errors.Is(err, errFailWriter) {
		t.Errorf("formatSession error = %v, want %v", err, errFailWriter)
	}
}
