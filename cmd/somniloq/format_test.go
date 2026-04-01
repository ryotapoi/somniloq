package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

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
		ProjectDir:   "-Users-test-proj",
		StartedAt:    "2026-03-28T10:00:00Z",
		CustomTitle:  "Fix login bug",
		MessageCount: 2,
	}
	messages := []core.MessageRow{
		{UUID: "m1", Role: "user", Content: "fix the login", Timestamp: "2026-03-28T10:00:00Z"},
		{UUID: "m2", Role: "assistant", Content: "done", Timestamp: "2026-03-28T10:01:00Z"},
	}

	formatSession(&buf, session, messages, time.UTC)
	got := buf.String()

	// Check h2 uses custom_title
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
		SessionID:  "abc-123",
		ProjectDir: "-Users-test",
		StartedAt:  "2026-03-28T10:00:00Z",
	}

	formatSession(&buf, session, nil, time.UTC)
	got := buf.String()

	// Should fallback to session_id
	if !strings.Contains(got, "## abc-123\n") {
		t.Errorf("expected h2 with session_id fallback, got:\n%s", got)
	}
}

func TestFormatSession_SkipsSidechain(t *testing.T) {
	var buf bytes.Buffer

	session := core.SessionRow{
		SessionID:  "s1",
		ProjectDir: "-test",
		StartedAt:  "2026-03-28T10:00:00Z",
	}
	messages := []core.MessageRow{
		{UUID: "m1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z", IsSidechain: false},
		{UUID: "m2", Role: "assistant", Content: "sidechain thought", Timestamp: "2026-03-28T10:00:30Z", IsSidechain: true},
		{UUID: "m3", Role: "assistant", Content: "visible reply", Timestamp: "2026-03-28T10:01:00Z", IsSidechain: false},
	}

	formatSession(&buf, session, messages, time.UTC)
	got := buf.String()

	if strings.Contains(got, "sidechain thought") {
		t.Errorf("sidechain message should be excluded, got:\n%s", got)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("expected non-sidechain user message, got:\n%s", got)
	}
	if !strings.Contains(got, "visible reply") {
		t.Errorf("expected non-sidechain assistant message, got:\n%s", got)
	}
}

func stubGetMessages(data map[string][]core.MessageRow) func(string) ([]core.MessageRow, error) {
	return func(sessionID string) ([]core.MessageRow, error) {
		return data[sessionID], nil
	}
}

func TestFormatSessions_Multiple(t *testing.T) {
	var buf bytes.Buffer

	sessions := []core.SessionRow{
		{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T14:00:00Z", CustomTitle: "Session One"},
		{SessionID: "s2", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z", CustomTitle: "Session Two"},
	}
	msgs := map[string][]core.MessageRow{
		"s1": {{UUID: "m1", Role: "user", Content: "hello", Timestamp: "2026-03-28T14:00:00Z"}},
		"s2": {{UUID: "m2", Role: "user", Content: "world", Timestamp: "2026-03-28T10:00:00Z"}},
	}

	if err := formatSessions(&buf, sessions, stubGetMessages(msgs), time.UTC); err != nil {
		t.Fatalf("formatSessions failed: %v", err)
	}
	got := buf.String()

	if !strings.Contains(got, "## Session One\n") {
		t.Errorf("expected Session One heading, got:\n%s", got)
	}
	if !strings.Contains(got, "## Session Two\n") {
		t.Errorf("expected Session Two heading, got:\n%s", got)
	}

	// Check separator between sessions (but not after last)
	parts := strings.Split(got, "\n---\n")
	if len(parts) != 2 {
		t.Errorf("expected exactly one --- separator between 2 sessions, got %d parts:\n%s", len(parts), got)
	}
	if strings.HasSuffix(strings.TrimSpace(got), "---") {
		t.Errorf("should not end with ---, got:\n%s", got)
	}
}

func TestFormatSessions_Single(t *testing.T) {
	var buf bytes.Buffer

	sessions := []core.SessionRow{
		{SessionID: "s1", ProjectDir: "-test", StartedAt: "2026-03-28T10:00:00Z", CustomTitle: "Only One"},
	}
	msgs := map[string][]core.MessageRow{
		"s1": {{UUID: "m1", Role: "user", Content: "hello", Timestamp: "2026-03-28T10:00:00Z"}},
	}

	if err := formatSessions(&buf, sessions, stubGetMessages(msgs), time.UTC); err != nil {
		t.Fatalf("formatSessions failed: %v", err)
	}
	got := buf.String()

	if strings.Contains(got, "---") {
		t.Errorf("single session should not have separator, got:\n%s", got)
	}
}

func TestFormatSession_TitleWithNewline(t *testing.T) {
	var buf bytes.Buffer

	session := core.SessionRow{
		SessionID:   "abc-123",
		ProjectDir:  "-Users-test",
		StartedAt:   "2026-03-28T10:00:00Z",
		CustomTitle: "line1\nline2",
	}

	formatSession(&buf, session, nil, time.UTC)
	got := buf.String()

	// Newline in title should be replaced with space
	if !strings.Contains(got, "## line1 line2\n") {
		t.Errorf("expected newline sanitized in title, got:\n%s", got)
	}
}
