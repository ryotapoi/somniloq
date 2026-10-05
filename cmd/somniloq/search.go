package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const searchUsageLine = "somniloq search [--config NAME_OR_PATH] [--session <REF>] [--input PATH...] [--source SOURCE...] [--project TEXT] [--since <time>] [--until <time>] [--time-mode active|started|last|overlap] [--imported-since <RFC3339>] [--day-boundary <HH:MM>] [--limit <n>] [--offset <n>] [--format tsv|json] [-e PATTERN...] [-F] [--all] [PATTERN]"
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
  Sort: lastAt descending, unknown last, group key ascending. Default unlimited; only explicit --limit caps results.
  JSON: {items,total,count,limit,offset,hasMore,nextOffset}.
  TSV: # page metadata, then ref,input,source,project,title,startedAt,lastAt,importedAt,members,matchedMembers,memberCount.
  Strings use reversible escapes, null is \N, arrays are compact JSON.
  Explicit limit 0 gives an empty page; offsets beyond the end retain total.
  Invalid input exits 2 with empty stdout; zero matches succeeds.

Session detail (--session REF):
  Requires patterns; searches self and confirmed descendants, excluding ancestors/siblings/root-only members.
  Uses the same candidate filters and matcher. Default unlimited; --limit/--offset page occurrences.
  Returns ref,messageNumber,occurrenceNumber,role,timestamp,startByte,endByte,patternIndexes,matchText,lineText.

Time filters:
  --since/--until accept dates or RFC3339 with timezone; relative/local datetimes are rejected.
  Dates use config dayBoundary or --day-boundary; until dates include the specified day.
  Lower bounds are inclusive; upper bounds exclusive. Explicit --time-mode requires a period.
  active (default) matches candidate bodies within the period, requiring at least one body.
  started/last select whole-group first/last times; overlap selects groups spanning the period.
  Other modes match candidate full bodies, including unknown timestamps.
  --imported-since accepts RFC3339 with timezone and selects candidate owners before matching.
  Detail accepts active only and applies the period to actual messages.
  Display dates and ordering always use all group members' original body timestamps.

Examples:
  somniloq search --config default --format json
  somniloq search --config default -e 'auth' -e 'bug' --all --project somniloq
  somniloq search --config default --limit 20 --offset 20 'migration'
  somniloq search --config default --session <REF> -F 'auth bug'

Selected day's original messages (POSIX sh; requires jq):
  Selects all matching groups; explicit --limit caps the selection.
  members includes the full group, including root-only members; matchedMembers contains candidates.
  Full REFs contain no whitespace/glob characters. sort order becomes show's conversation order.
  An empty selection skips show; the same period filters original messages.
sh <<'SH'
set -- $(somniloq search --config default --since 2026-10-01 --until 2026-10-01 --format json | jq -r '.items[].members[]' | sort -u)
if [ "$#" -gt 0 ]; then
  somniloq show --config default "$@" --since 2026-10-01 --until 2026-10-01 --format json
fi
SH`

type searchFlags struct {
	since, until, dayBoundary, project, format, session, timeMode, importedSince *string
	limit, offset                                                                *int
	patterns, inputs, sources                                                    *[]string
	fixed, all                                                                   *bool
}

func newSearchFlagSet() (*flag.FlagSet, searchFlags) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	f := searchFlags{session: fs.String("session", "", "search self and confirmed descendants"), since: fs.String("since", "", "inclusive activity lower bound (date or RFC3339)"), until: fs.String("until", "", "activity upper bound (date includes day; RFC3339 exclusive)"), dayBoundary: fs.String("day-boundary", "", "logical day boundary (HH:MM)"), project: fs.String("project", "", "case-sensitive project basename substring or exact alias"), limit: fs.Int("limit", 0, "maximum results (at least 0; default unlimited)"), offset: fs.Int("offset", 0, "ordered results to skip (at least 0)"), format: fs.String("format", "tsv", "output format (tsv, json)")}
	f.timeMode = fs.String("time-mode", "active", "list activity mode: active, started, last, overlap")
	f.importedSince = fs.String("imported-since", "", "candidate importedAt inclusive RFC3339 lower bound")
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
	if *f.importedSince != "" {
		t, err := time.Parse(time.RFC3339Nano, *f.importedSince)
		if err != nil {
			return c, fmt.Errorf("invalid --imported-since: require RFC3339 with timezone")
		}
		c.ImportedSince = t.UTC().Format(time.RFC3339Nano)
	}
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
	Limit      *int               `json:"limit"`
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
	filter, err := buildSearchFilter(fs, f, boundary, false)
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
		items, e = snapshot.SearchGroups(candidates, filter, matcher, *f.all, *f.timeMode)
		return e
	})
	if err != nil {
		return 1, err
	}
	var limit *int
	if flagWasProvided(fs, "limit") {
		limit = f.limit
	}
	page := searchGroupJSON{Items: []core.SearchGroup{}, Total: len(items), Limit: limit, Offset: *f.offset}
	start := min(page.Offset, len(items))
	end := len(items)
	if limit != nil {
		end = start + min(*limit, end-start)
	}
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
		Limit      *int `json:"limit"`
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

func buildSearchFilter(fs *flag.FlagSet, f searchFlags, boundary dayBoundary, detail bool) (core.SessionFilter, error) {
	filter := core.SessionFilter{}
	switch *f.timeMode {
	case "active", "started", "last", "overlap":
	default:
		return filter, fmt.Errorf("invalid --time-mode %q", *f.timeMode)
	}
	if detail && *f.timeMode != "active" {
		return filter, fmt.Errorf("--time-mode %s is list-only", *f.timeMode)
	}
	if flagWasProvided(fs, "time-mode") && *f.since == "" && *f.until == "" {
		return filter, fmt.Errorf("--time-mode requires --since or --until")
	}
	for _, name := range []string{"since", "until", "imported-since", "day-boundary"} {
		var value string
		switch name {
		case "since":
			value = *f.since
		case "until":
			value = *f.until
		case "imported-since":
			value = *f.importedSince
		case "day-boundary":
			value = *f.dayBoundary
		}
		if flagWasProvided(fs, name) && value == "" {
			return filter, fmt.Errorf("--%s requires a non-empty value", name)
		}
	}
	for _, bound := range []struct {
		value  string
		upper  bool
		target *string
	}{{*f.since, false, &filter.Since}, {*f.until, true, &filter.Until}} {
		if bound.value == "" {
			continue
		}
		t, err := parseShowTime(bound.value, bound.upper, boundary, time.Local)
		if err != nil {
			return filter, fmt.Errorf("invalid search time %q: require date or RFC3339 with timezone", bound.value)
		}
		*bound.target = t.UTC().Format(time.RFC3339Nano)
	}
	if filter.Since != "" && filter.Until != "" {
		lower, _ := time.Parse(time.RFC3339Nano, filter.Since)
		upper, _ := time.Parse(time.RFC3339Nano, filter.Until)
		if !lower.Before(upper) {
			return filter, fmt.Errorf("--since must be before --until")
		}
	}
	return filter, nil
}
