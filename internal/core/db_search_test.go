package core

import "testing"

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

	rows, err := db.SearchMessages(SessionFilter{}, "auth")
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
	if rows[0].Source != SourceCodex || rows[0].SessionID != "s2" || rows[0].Timestamp != "2026-03-29T10:00:00Z" {
		t.Errorf("rows[0] = %+v", rows[0])
	}
}

func TestSearchMessages_TimeFilterUsesMessageTimestamp(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{Since: "2026-03-29T00:00:00.000Z"}, "auth")
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 1 || rows[0].Content != "auth on another day" {
		t.Fatalf("rows = %+v, want only the 03-29 message", rows)
	}

	rows, err = db.SearchMessages(SessionFilter{Until: "2026-03-29T00:00:00.000Z"}, "auth")
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want the two 03-28 messages", rows)
	}
}

func TestSearchMessages_ProjectFilter(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{Project: "Brimday"}, "auth")
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

// Mirrors TestListSessions_SinceFilter_MillisecondTimestamp: stored
// timestamps carry milliseconds (e.g. .977Z) while the filter always uses
// .000Z, and the string comparison must still include same-second rows.
func TestSearchMessages_SinceFilter_MillisecondTimestamp(t *testing.T) {
	db := testDB(t)

	must(t, db.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "ms", StartedAt: "2026-03-28T14:10:45.977Z"}, "2026-03-28T15:00:00Z"))
	must(t, db.InsertMessage(NormalizedMessage{Source: SourceClaudeCode, UUID: "ms1", SessionID: "ms", Role: "user", Content: "millisecond auth", Timestamp: "2026-03-28T14:10:45.977Z"}))

	rows, err := db.SearchMessages(SessionFilter{Since: "2026-03-28T14:10:45.000Z"}, "auth")
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (same-second millisecond timestamp must match)", len(rows))
	}

	rows, err = db.SearchMessages(SessionFilter{Until: "2026-03-28T14:10:45.000Z"}, "auth")
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("rows = %d, want 0 (until is exclusive and earlier than the row)", len(rows))
	}
}

func TestSearchMessages_NoMatch(t *testing.T) {
	db := newSearchTestDB(t)

	rows, err := db.SearchMessages(SessionFilter{}, "no-such-text")
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

	rows, err := db.SearchMessages(SessionFilter{}, "tied")
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
