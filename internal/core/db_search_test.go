package core

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestSearchMessages_ClosedDatabaseErrorIncludesOperationAndCause(t *testing.T) {
	db := testDB(t)
	must(t, db.Close())

	_, err := db.SearchMessages(SessionFilter{}, "query", SearchPagination{})
	if err == nil {
		t.Fatal("expected query error from closed database")
	}
	if !strings.Contains(err.Error(), "search messages") {
		t.Errorf("error %q does not identify operation", err)
	}
	if errors.Unwrap(err) == nil {
		t.Errorf("error %q does not retain its cause", err)
	}
}

func newSearchTestDB(t *testing.T) *DB {
	t.Helper()
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m1", SessionID: "s1", Role: "user", Content: "fix the auth bug", Timestamp: "2026-03-28T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m2", SessionID: "s1", Role: "assistant", Content: "the AUTH module looks fine", Timestamp: "2026-03-28T10:01:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "m3", SessionID: "s1", Role: "user", Content: "auth in a sidechain", Timestamp: "2026-03-28T10:02:00Z", IsSidechain: true}))

	must(t, db.UpsertSession(SessionMeta{Source: SourceCodex, SessionID: "s2", RepoPath: "/Users/test/somniloq", StartedAt: "2026-03-29T10:00:00Z"}, "2026-03-29T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m4", SessionID: "s2", Role: "user", Content: "auth on another day", Timestamp: "2026-03-29T10:00:00Z"}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: "m5", SessionID: "s2", Role: "user", Content: "nothing relevant", Timestamp: "2026-03-29T10:01:00Z"}))
	return db
}

func TestSearchMessages_MatchesNewestFirstExcludingSidechain(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (sidechain excluded): %+v", len(rows), rows)
	}
	// Newest first; "AUTH" matches because LIKE is ASCII case-insensitive.
	wantContents := []string{"auth on another day", "the AUTH module looks fine", "fix the auth bug"}
	for i, want := range wantContents {
		if rows[i].Content != want {
			t.Errorf("rows[%d].Content = %q, want %q", i, rows[i].Content, want)
		}
	}
	if rows[0].Source != SourceCodex || rows[0].UUID != "m4" || rows[0].SessionID != "s2" || rows[0].RepoPath != "/Users/test/somniloq" || rows[0].Timestamp != "2026-03-29T10:00:00Z" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
}

func TestSearchMessages_LiteralMetacharacters(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "literal", RepoPath: "/Users/test/literal", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	for _, message := range []NormalizedMessage{
		{Source: SourceClaudeCode, UUID: "percent", SessionID: "literal", Role: "user", Content: "rate_100% is exact", Timestamp: "2026-03-28T10:01:00Z"},
		{Source: SourceClaudeCode, UUID: "underscore", SessionID: "literal", Role: "user", Content: "rate_100 only", Timestamp: "2026-03-28T10:02:00Z"},
		{Source: SourceClaudeCode, UUID: "escape", SessionID: "literal", Role: "user", Content: `path\segment`, Timestamp: "2026-03-28T10:03:00Z"},
		{Source: SourceClaudeCode, UUID: "combined", SessionID: "literal", Role: "user", Content: `mix%_\`, Timestamp: "2026-03-28T10:04:00Z"},
		{Source: SourceClaudeCode, UUID: "false", SessionID: "literal", Role: "user", Content: "rateX100anything mixZZ", Timestamp: "2026-03-28T10:05:00Z"},
	} {
		must(t, db.InsertMessage(message))
	}

	for _, tt := range []struct {
		query string
		want  []string
	}{
		{"rate_100%", []string{"percent"}},
		{"rate_100", []string{"underscore", "percent"}},
		{`path\segment`, []string{"escape"}},
		{`mix%_\`, []string{"combined"}},
	} {
		t.Run(tt.query, func(t *testing.T) {
			rows, err := db.SearchMessages(SessionFilter{}, tt.query, SearchPagination{})
			if err != nil {
				t.Fatalf("SearchMessages: %v", err)
			}
			got := make([]string, len(rows))
			for i, row := range rows {
				got[i] = row.UUID
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("SearchMessages(%q) UUIDs = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestSearchMessages_TimeFilterUsesMessageTimestamp(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{Since: "2026-03-29T00:00:00.000Z"}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 1 || rows[0].Content != "auth on another day" {
		t.Fatalf("rows = %+v, want only the 03-29 message", rows)
	}

	rows, err = db.SearchMessages(SessionFilter{Until: "2026-03-29T00:00:00.000Z"}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the two 03-28 messages", rows)
	}
}

func TestSearchMessages_FiltersExcludeUnknownTimestampAndRepoPath(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceCursorAgent, SessionID: "unknown", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCursorAgent, UUID: "unknown-time", SessionID: "unknown", Role: "user", Content: "needle", Timestamp: ""}))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceCursorAgent, UUID: "known-time", SessionID: "unknown", Role: "user", Content: "needle", Timestamp: "2026-03-28T10:00:00Z"}))

	rows, err := db.SearchMessages(SessionFilter{}, "needle", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages without filter: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("unfiltered rows = %d, want 2", len(rows))
	}

	rows, err = db.SearchMessages(SessionFilter{Until: "2026-03-29T00:00:00.000Z"}, "needle", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages with time filter: %v", err)
	}
	if len(rows) != 1 || rows[0].UUID != "known-time" {
		t.Fatalf("time-filtered rows = %+v, want only known timestamp", rows)
	}

	rows, err = db.SearchMessages(SessionFilter{Projects: []string{"%"}}, "needle", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages with project filter: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("project-filtered rows = %+v, want no unknown repo path", rows)
	}
}

func TestSearchMessages_ProjectFilter(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{Projects: []string{"Brimday"}}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2: %+v", len(rows), rows)
	}
	for _, r := range rows {
		if r.SessionID != "s1" {
			t.Errorf("unexpected session %q in Brimday results", r.SessionID)
		}
	}
}

// Multiple patterns come from project-alias expansion: a row matches when
// ANY pattern matches (OR), not when all do.
func TestSearchMessages_MultipleProjectsMatchAny(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{Projects: []string{"Brimday", "somniloq"}}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (both projects, sidechain excluded): %+v", len(rows), rows)
	}
}

func TestSearchMessages_CombinedFiltersUseMessageTimestamp(t *testing.T) {
	db := newSearchTestDB(t)

	// This session began before the range, but its matching message is inside
	// it. ADR 0013 requires search to filter the message timestamp instead.
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "long-brim", RepoPath: "/Users/test/Brimday", StartedAt: "2026-03-27T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "late-auth", SessionID: "long-brim", Role: "user", Content: "late auth update", Timestamp: "2026-03-28T12:00:00Z"}))

	rows, err := db.SearchMessages(SessionFilter{
		Since:    "2026-03-28T11:00:00.000Z",
		Until:    "2026-03-28T13:00:00.000Z",
		Projects: []string{"Brimday"},
	}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 1 || rows[0].SessionID != "long-brim" {
		t.Fatalf("rows = %+v, want only the in-range Brimday message", rows)
	}
}

// Mirrors TestListSessions_SinceFilter_MillisecondTimestamp: stored
// timestamps carry milliseconds (e.g. .977Z) while the filter always uses
// .000Z, and the string comparison must still include same-second rows.
func TestSearchMessages_SinceFilter_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "ms", StartedAt: "2026-03-28T14:10:45.977Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "ms1", SessionID: "ms", Role: "user", Content: "millisecond auth", Timestamp: "2026-03-28T14:10:45.977Z"}))

	rows, err := db.SearchMessages(SessionFilter{Since: "2026-03-28T14:10:45.000Z"}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (same-second millisecond timestamp must match)", len(rows))
	}

	rows, err = db.SearchMessages(SessionFilter{Until: "2026-03-28T14:10:45.000Z"}, "auth", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 (until is exclusive and earlier than the row)", len(rows))
	}
}

func TestSearchMessages_TimeFiltersCompareVariableFractionalSecondsAsInstants(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "fractional", StartedAt: "2026-03-28T14:10:45.123Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "fractional-message", SessionID: "fractional", Role: "user", Content: "fractional auth", Timestamp: "2026-03-28T14:10:45.123Z"}))

	for _, filter := range []struct {
		name   string
		filter SessionFilter
		want   int
	}{
		{"until later fraction includes", SessionFilter{Until: "2026-03-28T14:10:45.1235Z"}, 1},
		{"since later fraction excludes", SessionFilter{Since: "2026-03-28T14:10:45.1235Z"}, 0},
		{"since equal includes", SessionFilter{Since: "2026-03-28T14:10:45.123Z"}, 1},
		{"until equal excludes", SessionFilter{Until: "2026-03-28T14:10:45.123Z"}, 0},
	} {
		t.Run(filter.name, func(t *testing.T) {
			rows, err := db.SearchMessages(filter.filter, "auth", SearchPagination{})
			if err != nil {
				t.Fatalf("SearchMessages: %v", err)
			}
			if len(rows) != filter.want {
				t.Fatalf("SearchMessages rows = %d, want %d", len(rows), filter.want)
			}
		})
	}
}

func TestSearchMessages_NoMatch(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{}, "no-such-text", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want empty", rows)
	}
}

func TestSearchMessages_TimestampTieBrokenByRowid(t *testing.T) {
	db := testDB(t)

	// Old-format Codex rollouts give every record the same timestamp.
	must(t, db.UpsertSession(SessionMeta{Source: SourceCodex, SessionID: "tie", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"))
	for _, uuid := range []string{"t1", "t2", "t3"} {
		must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, UUID: uuid, SessionID: "tie", Role: "user", Content: "tied " + uuid, Timestamp: "2026-03-28T10:00:00Z"}))
	}

	rows, err := db.SearchMessages(SessionFilter{}, "tied", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	want := []string{"tied t3", "tied t2", "tied t1"} // newest first = reverse insertion
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, w := range want {
		if rows[i].Content != w {
			t.Errorf("rows[%d].Content = %q, want %q", i, rows[i].Content, w)
		}
	}
}

func TestSearchMessages_OrderByInstantBeforePagination(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "instant-search"}, "2026-03-28T15:00:00Z"))
	for _, message := range []NormalizedMessage{
		{Source: SourceClaudeCode, UUID: "earliest", SessionID: "instant-search", Role: "user", Content: "instant needle", Timestamp: "2026-03-28T10:00:00+02:00"},
		{Source: SourceClaudeCode, UUID: "tie-first", SessionID: "instant-search", Role: "user", Content: "instant needle", Timestamp: "2026-03-28T08:00:00.1Z"},
		{Source: SourceClaudeCode, UUID: "tie-second", SessionID: "instant-search", Role: "user", Content: "instant needle", Timestamp: "2026-03-28T09:00:00.100+01:00"},
		{Source: SourceClaudeCode, UUID: "latest", SessionID: "instant-search", Role: "user", Content: "instant needle", Timestamp: "2026-03-28T08:00:00.2Z"},
	} {
		must(t, db.InsertMessage(message))
	}

	all, err := db.SearchMessages(SessionFilter{}, "instant needle", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	want := []string{"latest", "tie-second", "tie-first", "earliest"}
	if got := searchUUIDs(all); !slices.Equal(got, want) {
		t.Fatalf("SearchMessages order = %v, want %v", got, want)
	}
	first, err := db.SearchMessages(SessionFilter{}, "instant needle", SearchPagination{Limit: 2})
	if err != nil {
		t.Fatalf("SearchMessages first page: %v", err)
	}
	second, err := db.SearchMessages(SessionFilter{}, "instant needle", SearchPagination{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("SearchMessages second page: %v", err)
	}
	if got := append(searchUUIDs(first), searchUUIDs(second)...); !slices.Equal(got, want) {
		t.Fatalf("SearchMessages pages = %v, want %v", got, want)
	}
}

func TestSearchMessages_PaginationPreservesOrderedPages(t *testing.T) {
	db := testDB(t)
	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "page-a", RepoPath: "/Users/test/Brimday"}, "2026-03-28T15:00:00Z"))
	must(t, db.UpsertSession(SessionMeta{Source: SourceCodex, SessionID: "page-b", RepoPath: "/Users/test/other"}, "2026-03-28T15:00:00Z"))
	for _, message := range []NormalizedMessage{
		{Source: SourceClaudeCode, UUID: "old", SessionID: "page-a", Role: "user", Content: "page needle old", Timestamp: "2026-03-28T10:00:00Z"},
		{Source: SourceClaudeCode, UUID: "tie-a", SessionID: "page-a", Role: "user", Content: "page needle tie a", Timestamp: "2026-03-28T11:00:00Z"},
		{Source: SourceClaudeCode, UUID: "tie-b", SessionID: "page-a", Role: "user", Content: "page needle tie b", Timestamp: "2026-03-28T11:00:00Z"},
		{Source: SourceCodex, UUID: "new", SessionID: "page-b", Role: "user", Content: "page needle new", Timestamp: "2026-03-28T12:00:00Z"},
	} {
		must(t, db.InsertMessage(message))
	}

	all, err := db.SearchMessages(SessionFilter{}, "page needle", SearchPagination{})
	if err != nil {
		t.Fatalf("SearchMessages full: %v", err)
	}
	if got, want := searchUUIDs(all), []string{"new", "tie-b", "tie-a", "old"}; !slices.Equal(got, want) {
		t.Fatalf("full order = %v, want %v", got, want)
	}

	first, err := db.SearchMessages(SessionFilter{}, "page needle", SearchPagination{Limit: 2})
	if err != nil {
		t.Fatalf("SearchMessages first page: %v", err)
	}
	second, err := db.SearchMessages(SessionFilter{}, "page needle", SearchPagination{Limit: 2, Offset: 2})
	if err != nil {
		t.Fatalf("SearchMessages second page: %v", err)
	}
	if got := append(searchUUIDs(first), searchUUIDs(second)...); !slices.Equal(got, searchUUIDs(all)) {
		t.Errorf("concatenated pages = %v, want %v", got, searchUUIDs(all))
	}

	offsetOnly, err := db.SearchMessages(SessionFilter{}, "page needle", SearchPagination{Offset: 3})
	if err != nil {
		t.Fatalf("SearchMessages offset-only: %v", err)
	}
	if got, want := searchUUIDs(offsetOnly), []string{"old"}; !slices.Equal(got, want) {
		t.Errorf("offset-only page = %v, want %v", got, want)
	}
	final, err := db.SearchMessages(SessionFilter{}, "page needle", SearchPagination{Limit: 2, Offset: 3})
	if err != nil {
		t.Fatalf("SearchMessages final page: %v", err)
	}
	if got, want := searchUUIDs(final), []string{"old"}; !slices.Equal(got, want) {
		t.Errorf("final page = %v, want %v", got, want)
	}
	outside, err := db.SearchMessages(SessionFilter{}, "page needle", SearchPagination{Limit: 2, Offset: 4})
	if err != nil {
		t.Fatalf("SearchMessages outside page: %v", err)
	}
	if len(outside) != 0 {
		t.Errorf("outside page = %v, want empty", searchUUIDs(outside))
	}

	filtered, err := db.SearchMessages(SessionFilter{Projects: []string{"Brimday"}}, "page needle", SearchPagination{Limit: 1, Offset: 1})
	if err != nil {
		t.Fatalf("SearchMessages filtered page: %v", err)
	}
	if got, want := searchUUIDs(filtered), []string{"tie-a"}; !slices.Equal(got, want) {
		t.Errorf("filtered page = %v, want %v (filters must precede pagination)", got, want)
	}
}

func searchUUIDs(rows []SearchRow) []string {
	uuid := make([]string, len(rows))
	for i, row := range rows {
		uuid[i] = row.UUID
	}
	return uuid
}
