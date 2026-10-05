package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

type searchDetailJSON struct {
	Items      []core.SearchOccurrence `json:"items"`
	Total      int                     `json:"total"`
	Count      int                     `json:"count"`
	Limit      *int                    `json:"limit"`
	Offset     int                     `json:"offset"`
	HasMore    bool                    `json:"hasMore"`
	NextOffset *int                    `json:"nextOffset"`
}

func searchDetailCmd(fs *flag.FlagSet, f searchFlags, openDB func() (*core.DB, error), cfg config, out, errOut io.Writer) (int, error) {
	patterns := append([]string(nil), *f.patterns...)
	if fs.NArg() > 0 {
		patterns = append([]string{fs.Arg(0)}, patterns...)
	}
	matcher, err := core.CompilePatterns(patterns, *f.fixed)
	if err != nil {
		return 2, err
	}
	if *f.limit < 0 || *f.offset < 0 {
		return 2, fmt.Errorf("limit and offset must be at least 0")
	}
	if err := validateFormat(*f.format, "tsv", "json"); err != nil {
		return 2, err
	}
	boundary, err := resolveDayBoundary(*f.dayBoundary, cfg)
	if err != nil {
		return 2, err
	}
	filter, err := buildSearchFilter(fs, f, boundary, true)
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
	var items []core.SearchOccurrence
	code := 0
	err = db.ReadSnapshot(func(snapshot *core.DB) error {
		if _, c, e := resolveSessionREF(snapshot, *f.session, nil, errOut); c != 0 {
			code = c
			return e
		}
		var e error
		items, e = snapshot.SearchOccurrences(*f.session, filter, matcher, *f.all, candidates)
		return e
	})
	if code != 0 {
		return code, err
	}
	if err != nil {
		return 1, err
	}
	page := searchDetailJSON{Items: []core.SearchOccurrence{}, Total: len(items), Offset: *f.offset}
	if flagWasProvided(fs, "limit") {
		page.Limit = f.limit
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
	if *f.format == "json" {
		err = writeJSON(out, page)
	} else {
		err = writeSearchDetailTSV(out, page)
	}
	if err != nil {
		return 1, err
	}
	return 0, nil
}
func writeSearchDetailTSV(out io.Writer, page searchDetailJSON) error {
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
	if _, err = fmt.Fprintf(out, "# page\t%s\nref\tmessageNumber\toccurrenceNumber\trole\ttimestamp\tstartByte\tendByte\tpatternIndexes\tmatchText\tlineText\n", data); err != nil {
		return err
	}
	for _, m := range page.Items {
		indexes, err := json.Marshal(m.PatternIndexes)
		if err != nil {
			return err
		}
		fields := []string{showTSVString(m.REF), strconv.Itoa(m.MessageNumber), strconv.Itoa(m.OccurrenceNumber), showTSVString(m.Role), showTSVNullable(m.Timestamp), strconv.Itoa(m.StartByte), strconv.Itoa(m.EndByte), string(indexes), showTSVString(m.MatchText), showTSVString(m.LineText)}
		if _, err = fmt.Fprintln(out, strings.Join(fields, "\t")); err != nil {
			return err
		}
	}
	return nil
}
