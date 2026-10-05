package main

import (
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
