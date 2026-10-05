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
// raw (no TSV sanitizing), and show/search detail emit envelopes. Other commands emit JSON arrays.

type sessionJSON struct {
	REF          string `json:"ref"`
	Source       string `json:"source"`
	SessionID    string `json:"sessionId"`
	Project      string `json:"project"`
	Title        string `json:"title"`
	StartedAt    string `json:"startedAt"`
	EndedAt      string `json:"endedAt"`
	LogicalDay   string `json:"logicalDay"`
	MessageCount int    `json:"messageCount"`
	BodySize     int    `json:"bodySize"`
}

type projectJSON struct {
	Project      string `json:"project"`
	SessionCount int    `json:"sessionCount"`
}

type showMessageJSON struct {
	REF           string   `json:"ref"`
	MessageNumber int      `json:"messageNumber"`
	Role          string   `json:"role"`
	Timestamp     *string  `json:"timestamp"`
	Text          string   `json:"text"`
	Blocks        []string `json:"blocks"`
	ParentREF     *string  `json:"parentRef"`
	RootREF       *string  `json:"rootRef"`
	Provenance    string   `json:"provenance"`
}
type showJSON struct {
	Items      []showMessageJSON `json:"items"`
	Total      int               `json:"total"`
	Count      int               `json:"count"`
	Limit      *int              `json:"limit"`
	Offset     int               `json:"offset"`
	HasMore    bool              `json:"hasMore"`
	NextOffset *int              `json:"nextOffset"`
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
func newShowMessageJSON(session core.SessionRow, message core.MessageRow) showMessageJSON {
	return showMessageJSON{REF: session.REF, MessageNumber: message.Number, Role: message.Role,
		Timestamp: nullableString(message.Timestamp), Text: message.Content, Blocks: message.Blocks,
		ParentREF: nullableString(session.ParentREF), RootREF: nullableString(session.RootREF), Provenance: message.Provenance}
}

func newSessionJSON(r core.SessionRow, project, logicalDay string) sessionJSON {
	return sessionJSON{
		REF:          r.REF,
		Source:       string(r.Source),
		SessionID:    r.SessionID,
		Project:      project,
		Title:        r.CustomTitle,
		StartedAt:    r.StartedAt,
		EndedAt:      r.EndedAt,
		LogicalDay:   logicalDay,
		MessageCount: r.MessageCount,
		BodySize:     r.BodySize,
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
