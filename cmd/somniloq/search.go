package main

import (
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ryotapoi/somniloq/internal/core"
)

const searchUsageLine = "somniloq search [--since <time>] [--until <time>] [--project <name>] <query>"

// snippetContext is the number of runes kept on each side of the match.
const snippetContext = 40

// searchCmd runs the search subcommand without calling os.Exit, so it can be
// tested directly.
func searchCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	since := fs.String("since", "", "filter by message time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter messages before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	project := fs.String("project", "", "filter by repo path (substring match)")
	setUsage(fs, "Search message content across sessions", searchUsageLine)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	searchUsage := "usage: " + searchUsageLine

	if fs.NArg() > 1 {
		fmt.Fprintln(errOut, "error: too many arguments")
		fmt.Fprintln(errOut, searchUsage)
		return 1, nil
	}
	query := fs.Arg(0)
	if query == "" {
		fmt.Fprintln(errOut, searchUsage)
		return 1, nil
	}

	filter, err := buildSessionFilter(*since, *until, *project, cfg)
	if err != nil {
		return 1, err
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	rows, err := db.SearchMessages(filter, query)
	if err != nil {
		return 1, err
	}

	for _, r := range rows {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n",
			r.SessionID,
			sanitizeTSV(formatLocalTime(r.Timestamp, time.Local)),
			sanitizeTSV(resolveProjectDisplayName(r.RepoPath, false, cfg)),
			sanitizeTSV(searchSnippet(r.Content, query)))
	}
	return 0, nil
}

// searchSnippet extracts the text around the first match of query in content,
// keeping snippetContext runes on each side and marking truncation with
// "...". SQL already guaranteed a LIKE match; the exact lookup falls back to
// an ASCII-insensitive one (LIKE's case rule), and to the content head if the
// position still cannot be pinned down.
func searchSnippet(content, query string) string {
	idx := strings.Index(content, query)
	if idx < 0 {
		idx = strings.Index(strings.ToLower(content), strings.ToLower(query))
	}
	// idx >= len(content) is reachable: ToLower can grow non-ASCII bytes
	// (e.g. İ), so an offset found in the lowered string can point past the
	// original content.
	if idx < 0 || idx >= len(content) {
		idx = 0
	}
	// ToLower can shift byte offsets for non-ASCII content, so re-anchor the
	// index to a rune boundary before slicing.
	for idx > 0 && !utf8.RuneStart(content[idx]) {
		idx--
	}

	end := idx + len(query)
	if end > len(content) {
		end = len(content)
	}
	for end > 0 && end < len(content) && !utf8.RuneStart(content[end]) {
		end--
	}

	start := idx
	for i := 0; i < snippetContext && start > 0; i++ {
		_, size := utf8.DecodeLastRuneInString(content[:start])
		start -= size
	}
	for i := 0; i < snippetContext && end < len(content); i++ {
		_, size := utf8.DecodeRuneInString(content[end:])
		end += size
	}

	// Trim surrounding whitespace so leading blank lines do not pad the
	// snippet once newlines are flattened for TSV.
	snippet := strings.TrimSpace(content[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(content) {
		snippet += "..."
	}
	return snippet
}
