package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

const searchUsageLine = "somniloq search --config default [--session <REF>] [--input PATH...] [--source SOURCE...] [--project TEXT] [--since <time>] [--until <time>] [--day-boundary <HH:MM>] [--limit <n>] [--offset <n>] [--format tsv|json] [-e PATTERN...] [-F] [--all] [PATTERN]"
const searchHelpDetails = `Search lists saved work groups, without body snippets. Omit patterns to list all candidates.
  Go regexp is case sensitive; positional PATTERN comes first, then repeated -e.
  -F treats every pattern literally; --all requires all patterns across candidate members.
  --input and --source are repeatable OR filters; different filter kinds intersect.
  Sources: claude-code, codex, cursor-agent (all is not accepted).
  Input paths use the config directory and canonical path rules.
  --project matches case-sensitive substrings of repo_path's final name; exact aliases expand.
  Candidates are selected before body matching. Members lists the full group;
  matchedMembers lists candidate members, including members without a pattern hit.
  Root project/title are not filled from children. Dates use all members' known own-body times.
  Sort: lastAt descending, unknown last, group key ascending. Default limit: 20.
  JSON: {items,total,count,limit,offset,hasMore,nextOffset}.
  TSV: # page metadata, then ref,input,source,project,title,startedAt,lastAt,importedAt,members,matchedMembers,memberCount.
  Strings use reversible escapes, null is \N, arrays are compact JSON.
  Explicit limit 0 gives an empty page; offsets beyond the end retain total.
  Invalid input exits 2 with empty stdout; zero matches succeeds.

Session detail (--session REF):
  Requires patterns; searches self and confirmed descendants, excluding ancestors/siblings/root-only members.
  Uses the same candidate filters and matcher. Default unlimited; --limit/--offset page occurrences.
  Returns ref,messageNumber,occurrenceNumber,role,timestamp,startByte,endByte,patternIndexes,matchText,lineText.

Temporary time behavior (list and detail):
  --since/--until filter candidate message timestamps, not group dates.
  Relative values, local dates/datetimes and RFC3339 are accepted; dates use --day-boundary.
  Unknown/invalid timestamps do not match a time filter. A pattern-free time-filtered list needs a matching body.
  Display dates and list ordering always use the whole group's original body timestamps.

Examples:
  somniloq search --config default --format json
  somniloq search --config default -e 'auth' -e 'bug' --all --project somniloq
  somniloq search --config default --limit 20 --offset 20 'migration'
  somniloq search --config default --session <REF> -F 'auth bug'`

type searchFlags struct {
	since, until, dayBoundary, project, format, session *string
	limit, offset                                       *int
	patterns, inputs, sources                           *[]string
	fixed, all                                          *bool
}

func newSearchFlagSet() (*flag.FlagSet, searchFlags) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	f := searchFlags{session: fs.String("session", "", "search self and confirmed descendants"), since: fs.String("since", "", "filter candidate message time"), until: fs.String("until", "", "exclusive candidate message time bound"), dayBoundary: fs.String("day-boundary", "", "logical day boundary (HH:MM)"), project: fs.String("project", "", "case-sensitive project basename substring or exact alias"), limit: fs.Int("limit", 0, "maximum results (at least 0; list default 20, detail unlimited)"), offset: fs.Int("offset", 0, "ordered results to skip (at least 0)"), format: fs.String("format", "tsv", "output format (tsv, json)")}
	f.fixed = fs.Bool("F", false, "treat all patterns as fixed strings")
	f.all = fs.Bool("all", false, "require every pattern across candidate bodies")
	f.patterns = new([]string)
	f.inputs = new([]string)
	f.sources = new([]string)
	fs.Func("e", "append a regexp pattern", func(v string) error { *f.patterns = append(*f.patterns, v); return nil })
	fs.Func("input", "candidate input root (repeatable)", func(v string) error { *f.inputs = append(*f.inputs, v); return nil })
	fs.Func("source", "candidate source: claude-code, codex, cursor-agent (repeatable)", func(v string) error { *f.sources = append(*f.sources, v); return nil })
	return fs, f
}
func searchCandidateFilter(f searchFlags, cfg config) (core.SearchCandidates, error) {
	c := core.SearchCandidates{Projects: cfg.expandProject(*f.project)}
	for _, path := range *f.inputs {
		if path == "" {
			return c, fmt.Errorf("--input requires a non-empty path")
		}
		p, err := core.CanonicalPath(path, filepath.Dir(cfg.Path))
		if err != nil {
			return c, err
		}
		c.Inputs = append(c.Inputs, p)
	}
	for _, source := range *f.sources {
		switch source {
		case "claude-code":
			c.Sources = append(c.Sources, core.SourceClaudeCode)
		case "codex":
			c.Sources = append(c.Sources, core.SourceCodex)
		case "cursor-agent":
			c.Sources = append(c.Sources, core.SourceCursorAgent)
		default:
			return c, fmt.Errorf("invalid --source %q (want claude-code, codex, or cursor-agent)", source)
		}
	}
	return c, nil
}

type searchGroupJSON struct {
	Items      []core.SearchGroup `json:"items"`
	Total      int                `json:"total"`
	Count      int                `json:"count"`
	Limit      int                `json:"limit"`
	Offset     int                `json:"offset"`
	HasMore    bool               `json:"hasMore"`
	NextOffset *int               `json:"nextOffset"`
}

func searchCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, f := newSearchFlagSet()
	setUsage(fs, "Search saved work groups or all matches in a selected conversation", searchUsageLine, searchHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		if code != 0 {
			code = 2
		}
		return code, nil
	}
	if fs.NArg() > 1 {
		writeUsageError(errOut, "too many arguments")
		fmt.Fprintln(errOut, "usage: "+searchUsageLine)
		return 2, nil
	}
	if flagWasProvided(fs, "session") {
		return searchDetailCmd(fs, f, openDB, cfg, out, errOut)
	}
	patterns := append([]string(nil), *f.patterns...)
	if fs.NArg() > 0 {
		patterns = append([]string{fs.Arg(0)}, patterns...)
	}
	var matcher *core.PatternMatcher
	var err error
	if len(patterns) > 0 {
		matcher, err = core.CompilePatterns(patterns, *f.fixed)
		if err != nil {
			return 2, err
		}
	}
	if *f.limit < 0 || *f.offset < 0 {
		return 2, fmt.Errorf("limit and offset must be at least 0")
	}
	if err = validateFormat(*f.format, "tsv", "json"); err != nil {
		return 2, err
	}
	boundary, err := resolveDayBoundary(*f.dayBoundary, cfg)
	if err != nil {
		return 2, err
	}
	filter, err := buildSessionFilter(*f.since, *f.until, "", cfg, boundary)
	if err != nil {
		return 2, err
	}
	candidates, err := searchCandidateFilter(f, cfg)
	if err != nil {
		return 2, err
	}
	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()
	var items []core.SearchGroup
	err = db.ReadSnapshot(func(snapshot *core.DB) error {
		var e error
		items, e = snapshot.SearchGroups(candidates, filter, matcher, *f.all)
		return e
	})
	if err != nil {
		return 1, err
	}
	limit := 20
	if flagWasProvided(fs, "limit") {
		limit = *f.limit
	}
	page := searchGroupJSON{Items: []core.SearchGroup{}, Total: len(items), Limit: limit, Offset: *f.offset}
	start := min(page.Offset, len(items))
	end := start + min(limit, len(items)-start)
	page.Items = append(page.Items, items[start:end]...)
	page.Count = len(page.Items)
	page.HasMore = page.Offset < page.Total && page.Count < page.Total-page.Offset
	if page.HasMore && page.Count > 0 {
		next := page.Offset + page.Count
		page.NextOffset = &next
	}
	if *f.format == "json" {
		err = writeJSON(out, page)
	} else {
		err = writeSearchGroupTSV(out, page)
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}
func writeSearchGroupTSV(out io.Writer, page searchGroupJSON) error {
	metadata := struct {
		Total      int  `json:"total"`
		Count      int  `json:"count"`
		Limit      int  `json:"limit"`
		Offset     int  `json:"offset"`
		HasMore    bool `json:"hasMore"`
		NextOffset *int `json:"nextOffset"`
	}{page.Total, page.Count, page.Limit, page.Offset, page.HasMore, page.NextOffset}
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(out, "# page\t%s\nref\tinput\tsource\tproject\ttitle\tstartedAt\tlastAt\timportedAt\tmembers\tmatchedMembers\tmemberCount\n", data); err != nil {
		return err
	}
	for _, s := range page.Items {
		members, _ := json.Marshal(s.Members)
		matched, _ := json.Marshal(s.MatchedMembers)
		fields := []string{showTSVString(s.REF), showTSVNullable(s.Input), showTSVString(string(s.Source)), showTSVNullable(s.Project), showTSVNullable(s.Title), showTSVNullable(s.StartedAt), showTSVNullable(s.LastAt), showTSVNullable(s.ImportedAt), string(members), string(matched), strconv.Itoa(s.MemberCount)}
		if _, err = fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}
