package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}

func writeUsageError(w io.Writer, message string) {
	fmt.Fprintf(w, "error: %s\n", message)
}

func writePageTSVHeader[T any](out io.Writer, page page[T], header string) error {
	metadata := struct {
		Total      int  `json:"total"`
		Count      int  `json:"count"`
		Limit      *int `json:"limit"`
		Offset     int  `json:"offset"`
		HasMore    bool `json:"hasMore"`
		NextOffset *int `json:"nextOffset"`
	}{page.Total, page.Count, page.Limit, page.Offset, page.HasMore, page.NextOffset}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "# page\t%s\n%s\n", data, header)
	return err
}
