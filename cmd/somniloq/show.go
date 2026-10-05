package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/core"
)

const showUsageLine = "somniloq show [--config NAME_OR_PATH] REF... [--descendants] [--role user|assistant] [--messages A:B] [--since VALUE] [--until VALUE] [--day-boundary HH:MM] [--limit N] [--offset N] [--tail N] [--one-line] [--format tsv|json]"
const showHelpDetails = `Output (TSV/JSON):
  Envelope: items, total, count, limit, offset, hasMore, nextOffset.
  Each item: ref, messageNumber, role, timestamp, text, blocks, parentRef, rootRef, provenance.
  TSV begins with # page metadata and a fixed header; null is \N and strings are reversibly escaped.
Notes:
  Full REF values are required. Selectors are expanded in input order and duplicates removed.
  --descendants includes confirmed descendants in parent-first order.
  Filter original messages by role, inclusive number range A:B (A: and :B allowed), and their own timestamp.
  Dates use the local day boundary; --until dates include that day. Datetimes require RFC3339 with timezone.
  Page after filtering, then --one-line keeps text before the first LF; blocks stay original.
  --tail selects the final N messages and cannot be combined with explicit --limit or --offset.
Examples:
  somniloq show --config default <REF> <REF> --since 2026-10-01 --until 2026-10-01 --format json
  somniloq show --config default <REF> --role user --one-line
  somniloq show --config default <REF> --messages 40:60 --limit 10

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

func showCmd(args []string, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	fs, f := newShowFlagSet()
	setUsage(fs, "Show original messages", showUsageLine, showHelpDetails)
	fs.SetOutput(errOut)
	args = showFlagArgs(fs, args)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 2, nil
	}
	if fs.NArg() == 0 {
		return 2, fmt.Errorf("at least one REF is required; usage: %s", showUsageLine)
	}
	if err := validateFormat(*f.format, "tsv", "json"); err != nil {
		return 2, err
	}
	if *f.role != "" && *f.role != "user" && *f.role != "assistant" {
		return 2, errors.New("--role must be user or assistant")
	}
	if flagWasProvided(fs, "role") && *f.role == "" {
		return 2, errors.New("--role must be user or assistant")
	}
	lo, hi := 0, 0
	if flagWasProvided(fs, "messages") {
		var err error
		lo, hi, err = parseMessageRange(*f.messages)
		if err != nil {
			return 2, err
		}
	}
	if *f.limit < 0 || *f.offset < 0 || *f.tail < 0 {
		return 2, errors.New("--limit, --offset and --tail must be nonnegative")
	}
	tailSet := flagWasProvided(fs, "tail")
	if tailSet && (flagWasProvided(fs, "limit") || flagWasProvided(fs, "offset")) {
		return 2, errors.New("--tail cannot be combined with --limit or --offset")
	}
	boundary, err := resolveDayBoundary(*f.dayBoundary, cfg)
	if err != nil {
		return 2, err
	}
	if flagWasProvided(fs, "day-boundary") && *f.dayBoundary == "" {
		return 2, errors.New("--day-boundary requires HH:MM")
	}
	var since, until *time.Time
	for _, bound := range []struct {
		name, value string
		upper       bool
		target      **time.Time
	}{{"since", *f.since, false, &since}, {"until", *f.until, true, &until}} {
		if flagWasProvided(fs, bound.name) {
			t, err := parseShowTime(bound.value, bound.upper, boundary, time.Local)
			if err != nil {
				return 2, err
			}
			*bound.target = &t
		}
	}
	if since != nil && until != nil && !since.Before(*until) {
		return 2, errors.New("--since must be before --until")
	}
	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()
	items := []showMessageJSON{}
	selectionCode := 1
	err = db.ReadSnapshot(func(snapshot *core.DB) error {
		sessions := []core.SessionRow{}
		seen := map[string]bool{}
		for _, ref := range fs.Args() {
			resolved, err := snapshot.ResolveSession(ref)
			if err != nil {
				var re *core.REFError
				if errors.As(err, &re) {
					selectionCode = 2
				}
				return err
			}
			if resolved == nil {
				selectionCode = 2
				return fmt.Errorf("session not found: %s", ref)
			}
			for _, diagnostic := range resolved.Diagnostics {
				fmt.Fprintln(errOut, diagnostic)
			}
			selected := []core.SessionRow{resolved.Self}
			if *f.descendants {
				selected = resolved.Descendants
			}
			for _, session := range selected {
				if !seen[session.REF] {
					seen[session.REF] = true
					sessions = append(sessions, session)
				}
			}
		}
		for _, session := range sessions {
			messages, err := snapshot.GetIdentityMessages(session.InputID, session.Source, session.Identity)
			if err != nil {
				return err
			}
			for _, m := range messages {
				if *f.role != "" && m.Role != *f.role || lo > 0 && m.Number < lo || hi > 0 && m.Number > hi {
					continue
				}
				if since != nil || until != nil {
					t, err := time.Parse(time.RFC3339Nano, m.Timestamp)
					if err != nil || since != nil && t.Before(*since) || until != nil && !t.Before(*until) {
						continue
					}
				}
				items = append(items, newShowMessageJSON(session, m))
			}
		}
		return nil
	})
	if err != nil {
		return selectionCode, err
	}
	page := showJSON{Items: []showMessageJSON{}, Total: len(items), Offset: *f.offset}
	if flagWasProvided(fs, "limit") {
		page.Limit = f.limit
	}
	if tailSet {
		page.Limit = f.tail
		page.Offset = max(len(items)-*f.tail, 0)
	}
	start := min(page.Offset, len(items))
	end := len(items)
	if page.Limit != nil {
		end = start + min(*page.Limit, end-start)
	}
	page.Items = append(page.Items, items[start:end]...)
	page.Count = len(page.Items)
	page.HasMore = page.Offset < page.Total && page.Count < page.Total-page.Offset
	if page.HasMore && page.Count > 0 {
		next := page.Offset + page.Count
		page.NextOffset = &next
	}
	if *f.oneLine {
		for i := range page.Items {
			page.Items[i].Text = showFirstLine(page.Items[i].Text)
		}
	}
	if *f.format == "json" {
		err = writeJSON(out, page)
	} else {
		err = writeShowTSV(out, page)
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}

// The standard flag parser stops at the first positional argument. Keep option
// values together when moving REF selectors behind the options.
func showFlagArgs(fs *flag.FlagSet, args []string) []string {
	options, refs := []string{}, []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			refs = append(refs, args[i+1:]...)
			break
		}
		name, hasValue, ok := splitFlagArg(args[i])
		if !ok {
			refs = append(refs, args[i])
			continue
		}
		options = append(options, args[i])
		if f := fs.Lookup(name); f != nil && !hasValue && flagConsumesValue(f) && i+1 < len(args) {
			i++
			options = append(options, args[i])
		}
	}
	return append(append(options, "--"), refs...)
}

type showFlags struct {
	since, until, dayBoundary, role, messages, format *string
	descendants, oneLine                              *bool
	limit, offset, tail                               *int
}

func newShowFlagSet() (*flag.FlagSet, showFlags) {
	fs := flag.NewFlagSet("show", flag.ContinueOnError)
	return fs, showFlags{
		since:       fs.String("since", "", "include messages at or after date or RFC3339 instant"),
		until:       fs.String("until", "", "include messages before instant or through local date"),
		dayBoundary: fs.String("day-boundary", "", "local date boundary HH:MM (overrides config)"),
		role:        fs.String("role", "", "filter user or assistant messages"),
		messages:    fs.String("messages", "", "inclusive original message number range A:B, A:, or :B"),
		format:      fs.String("format", "tsv", "output format (tsv, json)"),
		descendants: fs.Bool("descendants", false, "include confirmed descendants"),
		oneLine:     fs.Bool("one-line", false, "show text before first LF; retain original blocks"),
		limit:       fs.Int("limit", 0, "maximum number of filtered messages (default unlimited)"),
		offset:      fs.Int("offset", 0, "skip filtered messages"),
		tail:        fs.Int("tail", 0, "select final N filtered messages"),
	}
}
func parseMessageRange(value string) (int, int, error) {
	a, b, ok := strings.Cut(value, ":")
	invalid := fmt.Errorf("--messages requires positive inclusive A:B, A:, or :B, got %q", value)
	if !ok || a == "" && b == "" {
		return 0, 0, invalid
	}
	parse := func(s string) (int, error) {
		if s == "" {
			return 0, nil
		}
		for _, c := range s {
			if c < '0' || c > '9' {
				return 0, invalid
			}
		}
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			return 0, invalid
		}
		return n, nil
	}
	lo, err := parse(a)
	if err != nil {
		return 0, 0, err
	}
	hi, err := parse(b)
	if err != nil {
		return 0, 0, err
	}
	if hi > 0 && hi < lo {
		return 0, 0, invalid
	}
	return lo, hi, nil
}
func parseShowTime(value string, upper bool, boundary dayBoundary, loc *time.Location) (time.Time, error) {
	if date, err := time.Parse("2006-01-02", value); err == nil {
		offset := 0
		if upper {
			offset = 1
		}
		return boundary.onDate(date, offset, loc), nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid show datetime %q: require date or RFC3339 with timezone", value)
	}
	return t, nil
}
func showFirstLine(text string) string {
	line, _, found := strings.Cut(text, "\n")
	if found {
		line = strings.TrimSuffix(line, "\r")
	}
	return line
}
