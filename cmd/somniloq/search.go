package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ryotapoi/somniloq/internal/core"
)

const searchUsageLine = "somniloq search --config default [--session <REF>] [--since <time>] [--until <time>] [--day-boundary <HH:MM>] [--project <name>] [--limit <n>] [--offset <n>] [--format <fmt>] <query>"

const searchHelpDetails = `Columns (TSV, in order):
  ref: full slq1 reference containing the matching message.
  turn: outline/show turn number containing the hit.
  time: local timestamp of the matching message.
  project: canonical alias name when configured, otherwise repo_path.
  snippet: first match with about 40 runes of context on each side; tabs/newlines flattened for TSV.
  source: internal source identifier: claude_code, codex, or cursor_agent.

JSON fields:
  ref, source, sessionId, turn, timestamp, project, snippet

Notes:
  Search scans own message bodies using SQLite LIKE; Codex inheritance context and unresolved records are excluded.
  Claude Code and Codex body sidechain records are included.
  --session selects the named full REF and confirmed descendants, excluding ancestors, siblings, and root-only members.
  --since/--until accept RFC3339 instants (for example, 2026-03-28T15:00:00Z or 2026-03-29T00:00:00+09:00); dates and minute datetimes are local.
  LIKE is ASCII-case-insensitive; query text, including %, _, and \, is literal.
  --project expands exact projectAliases matches, then filters repo_path by literal substring (including %, _, and \).
  --since/--until filter message timestamps, not session start time.
  Unknown or invalid stored message timestamps do not match time filters.
  Date-only --since/--until values use --day-boundary or config dayBoundary.
  Date-only boundaries follow local calendar days across daylight saving time changes.
  --limit returns at most N results (N >= 1); --offset skips N ordered results (N >= 0).
  Continue a fixed search with --limit and increasing --offset. Database changes or
  different resolved relative-time filters can change later pages.
  Typical flow: search -> outline <REF> -> show --turn <turn-or-range> <REF>.

Examples:
  somniloq search --config default "auth bug"
  somniloq search --config default --since 7d --project somniloq "migration"
  somniloq search --config default --limit 50 --offset 50 "auth bug"
  somniloq search --config default --format json "auth bug"
  somniloq show --config default --turn 42 <REF>`

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

	if flagWasProvided(fs, "session") {
		if _, code, err := resolveSessionREF(db, *flags.session, nil, errOut); code != 0 {
			return code, err
		}
	}
	rows, err := db.SearchMessages(filter, query, core.SearchPagination{Limit: *flags.limit, Offset: *flags.offset, SessionREF: *flags.session})
	if err != nil {
		var refErr *core.REFError
		if errors.As(err, &refErr) {
			return 2, err
		}
		return 1, err
	}

	turnCache := map[searchSessionKey]map[string]int{}
	entries := make([]searchJSON, 0, len(rows))
	for _, r := range rows {
		turns, err := searchTurnsByUUID(db, turnCache, r.InputID, r.Source, r.Identity)
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
				REF:       r.REF,
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
			r.REF,
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
	since, until, dayBoundary, project, format, session *string
	limit, offset                                       *int
}

func newSearchFlagSet() (*flag.FlagSet, searchFlags) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	flags := searchFlags{
		session:     fs.String("session", "", "search the named full REF and confirmed descendants"),
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

type searchSessionKey struct {
	inputID   int64
	source    core.Source
	sessionID string
}

func searchTurnsByUUID(db *core.DB, cache map[searchSessionKey]map[string]int, inputID int64, source core.Source, sessionID string) (map[string]int, error) {
	key := searchSessionKey{inputID: inputID, source: source, sessionID: sessionID}
	if turns, ok := cache[key]; ok {
		return turns, nil
	}
	messages, err := db.GetIdentityTurnMessages(inputID, source, sessionID)
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
