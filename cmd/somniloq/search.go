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

const searchUsageLine = "somniloq search [--since <time>] [--until <time>] [--day-boundary <HH:MM>] [--project <name>] [--limit <n>] [--offset <n>] [--format <fmt>] <query>"

const searchHelpDetails = `Columns (TSV, in order):
  session_id: source-local session identifier containing the matching message.
  turn: outline/show turn number containing the hit.
  time: local timestamp of the matching message.
  project: canonical alias name when configured, otherwise repo_path.
  snippet: first match with about 40 runes of context on each side; tabs/newlines flattened for TSV.
  source: internal source identifier: claude_code, codex, or cursor_agent.

JSON fields:
  source, sessionId, turn, timestamp, project, snippet

Notes:
  Search scans non-sidechain message bodies using SQLite LIKE.
  --since/--until accept RFC3339 instants (for example, 2026-03-28T15:00:00Z or 2026-03-29T00:00:00+09:00); dates and minute datetimes are local.
  LIKE is ASCII-case-insensitive; query text, including %, _, and \, is literal.
  --project expands exact projectAliases matches, then filters repo_path by literal substring (including %, _, and \).
  --since/--until filter message timestamps, not session start time.
  Date-only --since/--until values use --day-boundary or config dayBoundary.
  --limit returns at most N results (N >= 1); --offset skips N ordered results (N >= 0).
  Continue a fixed search with --limit and increasing --offset. Database changes or
  different resolved relative-time filters can change later pages.
  Typical flow: search -> outline <session-id> -> show --turn <turn-or-range> <session-id>.

Examples:
  somniloq search "auth bug"
  somniloq search --since 7d --project somniloq "migration"
  somniloq search --limit 50 --offset 50 "auth bug"
  somniloq search --format json "auth bug"
  somniloq show --turn 42 <session-id>`

// snippetContext is the number of runes kept on each side of the match.
const snippetContext = 40

// searchCmd runs the search subcommand without calling os.Exit, so it can be
// tested directly.
func searchCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, flags := newSearchFlagSet()
	setUsage(fs, "Search message content across sessions", searchUsageLine, searchHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	searchUsage := "usage: " + searchUsageLine

	if fs.NArg() > 1 {
		writeUsageError(errOut, "too many arguments")
		fmt.Fprintln(errOut, searchUsage)
		return 1, nil
	}
	query := fs.Arg(0)
	if query == "" {
		fmt.Fprintln(errOut, searchUsage)
		return 1, nil
	}
	if err := validateFormat(*flags.format, "tsv", "json"); err != nil {
		return 1, err
	}
	if *flags.limit < 0 || *flags.limit == 0 && flagWasProvided(fs, "limit") {
		return 1, fmt.Errorf("limit must be at least 1")
	}
	if *flags.offset < 0 {
		return 1, fmt.Errorf("offset must be at least 0")
	}

	boundary, err := resolveDayBoundary(*flags.dayBoundary, cfg)
	if err != nil {
		return 1, err
	}
	filter, err := buildSessionFilter(*flags.since, *flags.until, *flags.project, cfg, boundary)
	if err != nil {
		return 1, err
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	rows, err := db.SearchMessages(filter, query, core.SearchPagination{Limit: *flags.limit, Offset: *flags.offset})
	if err != nil {
		return 1, err
	}

	turnCache := map[searchSessionKey]map[string]int{}
	entries := make([]searchJSON, 0, len(rows))
	for _, r := range rows {
		turns, err := searchTurnsByUUID(db, turnCache, r.Source, r.SessionID)
		if err != nil {
			return 1, err
		}
		turn, ok := turns[r.UUID]
		if !ok {
			return 1, fmt.Errorf("turn not found for search hit %s/%s/%s", r.Source, r.SessionID, r.UUID)
		}
		project := resolveProjectDisplayName(r.RepoPath, false, cfg)
		snippet := searchSnippet(r.Content, query)
		if *flags.format == "json" {
			entries = append(entries, searchJSON{
				Source:    string(r.Source),
				SessionID: r.SessionID,
				Turn:      turn,
				Timestamp: r.Timestamp,
				Project:   project,
				Snippet:   snippet,
			})
			continue
		}
		if _, err := fmt.Fprintf(out, "%s\t%d\t%s\t%s\t%s\t%s\n",
			r.SessionID,
			turn,
			sanitizeTSV(formatLocalTime(r.Timestamp, time.Local)),
			sanitizeTSV(project),
			sanitizeTSV(snippet), r.Source); err != nil {
			return 1, err
		}
	}
	if *flags.format == "json" {
		if err := writeJSON(out, entries); err != nil {
			return 1, err
		}
	}
	return 0, nil
}

type searchFlags struct {
	since, until, dayBoundary, project, format *string
	limit, offset                              *int
}

func newSearchFlagSet() (*flag.FlagSet, searchFlags) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	flags := searchFlags{
		since:       fs.String("since", "", "filter by message time (relative, local date/datetime, or RFC3339 instant)"),
		until:       fs.String("until", "", "filter messages before a relative, local date/datetime, or RFC3339 instant"),
		dayBoundary: fs.String("day-boundary", "", "logical day boundary for date filters (HH:MM, overrides config dayBoundary)"),
		project:     fs.String("project", "", "filter by repo path (literal substring match)"),
		limit:       fs.Int("limit", 0, "maximum number of results (at least 1)"),
		offset:      fs.Int("offset", 0, "number of ordered results to skip (at least 0)"),
		format:      fs.String("format", "tsv", "output format (tsv, json)"),
	}
	return fs, flags
}

func flagWasProvided(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		found = found || f.Name == name
	})
	return found
}

type searchSessionKey struct {
	source    core.Source
	sessionID string
}

func searchTurnsByUUID(db *core.DB, cache map[searchSessionKey]map[string]int, source core.Source, sessionID string) (map[string]int, error) {
	key := searchSessionKey{source: source, sessionID: sessionID}
	if turns, ok := cache[key]; ok {
		return turns, nil
	}
	messages, err := db.GetMessages(source, sessionID)
	if err != nil {
		return nil, err
	}
	turns := map[string]int{}
	for _, tm := range assignTurns(messages) {
		turns[tm.Msg.UUID] = tm.Turn
	}
	cache[key] = turns
	return turns, nil
}

// searchSnippet extracts the text around the first match of query in content,
// keeping snippetContext runes on each side and marking truncation with
// "...". SQL already guaranteed a literal LIKE match; lookup follows LIKE's
// ASCII-only case rule and falls back to the content head only if the position
// cannot be pinned down.
func searchSnippet(content, query string) string {
	idx := indexASCIIFold(content, query)
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

// indexASCIIFold returns the first byte offset where needle occurs in haystack
// under SQLite LIKE's ASCII-only case-insensitive comparison. Bytes outside
// ASCII must match exactly, avoiding Unicode lowercasing and offset changes.
func indexASCIIFold(haystack, needle string) int {
	if needle == "" {
		return 0
	}
	for start := 0; start+len(needle) <= len(haystack); start++ {
		matched := true
		for i := 0; i < len(needle); i++ {
			if asciiLower(haystack[start+i]) != asciiLower(needle[i]) {
				matched = false
				break
			}
		}
		if matched {
			return start
		}
	}
	return -1
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
