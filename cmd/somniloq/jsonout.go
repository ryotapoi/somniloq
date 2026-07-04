package main

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

// validateFormat rejects unsupported --format values. Called before opening
// the DB so a bad value fails fast.
func validateFormat(format string, supported ...string) error {
	if slices.Contains(supported, format) {
		return nil
	}
	return fmt.Errorf("unknown format: %q (supported: %s)", format, strings.Join(supported, ", "))
}

// JSON output is the machine-readable counterpart of the TSV/Markdown views
// (ADR 0012). Timestamps stay in the stored RFC3339 UTC form, strings are
// raw (no TSV sanitizing), and every command emits a JSON array — [] when
// empty — so consumers get a stable schema.

type sessionJSON struct {
	Source                  string `json:"source"`
	SessionID               string `json:"sessionId"`
	Project                 string `json:"project"`
	Title                   string `json:"title"`
	StartedAt               string `json:"startedAt"`
	EndedAt                 string `json:"endedAt"`
	LogicalDay              string `json:"logicalDay"`
	MessageCount            int    `json:"messageCount"`
	BodySize                int    `json:"bodySize"`
	NonCommandUserTurnCount int    `json:"nonCommandUserTurnCount"`
	FirstNonCommandUserLine string `json:"firstNonCommandUserLine"`
}

type projectJSON struct {
	Project      string `json:"project"`
	SessionCount int    `json:"sessionCount"`
}

type outlineEntryJSON struct {
	Turn      int    `json:"turn"`
	Timestamp string `json:"timestamp"`
	FirstLine string `json:"firstLine"`
}

type messageJSON struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
}

type showSessionJSON struct {
	Source    string        `json:"source"`
	SessionID string        `json:"sessionId"`
	Project   string        `json:"project"`
	Title     string        `json:"title"`
	StartedAt string        `json:"startedAt"`
	EndedAt   string        `json:"endedAt"`
	Messages  []messageJSON `json:"messages"`
}

func newSessionJSON(r core.SessionRow, project, logicalDay string, userTurns sessionUserTurnSummary) sessionJSON {
	return sessionJSON{
		Source:                  string(r.Source),
		SessionID:               r.SessionID,
		Project:                 project,
		Title:                   r.CustomTitle,
		StartedAt:               r.StartedAt,
		EndedAt:                 r.EndedAt,
		LogicalDay:              logicalDay,
		MessageCount:            r.MessageCount,
		BodySize:                r.BodySize,
		NonCommandUserTurnCount: userTurns.NonCommandUserTurnCount,
		FirstNonCommandUserLine: userTurns.FirstNonCommandUserLine,
	}
}

func newShowSessionJSON(r core.SessionRow, project string, messages []core.MessageRow) showSessionJSON {
	msgs := make([]messageJSON, len(messages))
	for i, m := range messages {
		msgs[i] = messageJSON{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp}
	}
	return showSessionJSON{
		Source:    string(r.Source),
		SessionID: r.SessionID,
		Project:   project,
		Title:     r.CustomTitle,
		StartedAt: r.StartedAt,
		EndedAt:   r.EndedAt,
		Messages:  msgs,
	}
}

// writeJSON encodes v as indented JSON. HTML escaping is disabled so message
// content with <, >, & stays readable.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
