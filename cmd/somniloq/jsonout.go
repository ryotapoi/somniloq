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
// raw (no TSV sanitizing), and show/search emit envelopes. Projects emits a JSON array.

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
type page[T any] struct {
	Items      []T  `json:"items"`
	Total      int  `json:"total"`
	Count      int  `json:"count"`
	Limit      *int `json:"limit"`
	Offset     int  `json:"offset"`
	HasMore    bool `json:"hasMore"`
	NextOffset *int `json:"nextOffset"`
}
type showJSON = page[showMessageJSON]

func paginate[T any](items []T, limit *int, offset int) page[T] {
	result := page[T]{Items: []T{}, Total: len(items), Limit: limit, Offset: offset}
	start := min(offset, len(items))
	end := len(items)
	if limit != nil {
		end = start + min(*limit, end-start)
	}
	result.Items = append(result.Items, items[start:end]...)
	result.Count = len(result.Items)
	result.HasMore = offset < result.Total && result.Count < result.Total-offset
	if result.HasMore && result.Count > 0 {
		next := offset + result.Count
		result.NextOffset = &next
	}
	return result
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

// writeJSON encodes v as indented JSON. HTML escaping is disabled so message
// content with <, >, & stays readable.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
