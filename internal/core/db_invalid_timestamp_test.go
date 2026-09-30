package core

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestInvalidSavedTimestampsRemainReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "saved.db")
	db, err := OpenDB(path)
	must(t, err)
	for _, s := range []SessionMeta{
		{Source: SourceClaudeCode, SessionID: "invalid", RepoPath: "/invalid", StartedAt: "bad-start", EndedAt: "bad-end"},
		{Source: SourceClaudeCode, SessionID: "valid", RepoPath: "/valid", StartedAt: "2026-03-28T09:00:00.5Z", EndedAt: "2026-03-28T09:30:00Z"},
	} {
		must(t, db.UpsertSession(s, "2026-03-28T10:00:00Z"))
	}
	for _, m := range []NormalizedMessage{
		{Source: SourceClaudeCode, SessionID: "invalid", UUID: "bad-1", Role: "user", Content: "needle bad one", Timestamp: "bad-message"},
		{Source: SourceClaudeCode, SessionID: "invalid", UUID: "bad-2", Role: "user", Content: "needle bad two", Timestamp: "also-bad"},
		{Source: SourceClaudeCode, SessionID: "invalid", UUID: "early", Role: "user", Content: "needle early", Timestamp: "2026-03-28T10:00:00+02:00"},
		{Source: SourceClaudeCode, SessionID: "valid", UUID: "valid", Role: "user", Content: "needle valid", Timestamp: "2026-03-28T09:00:00.5Z"},
	} {
		must(t, db.InsertMessage(m))
	}
	must(t, db.Close())
	// Reopen persisted values without reimporting or repairing them.
	db, err = OpenDB(path)
	must(t, err)
	defer db.Close()
	sessions, err := db.ListSessions(SessionFilter{})
	must(t, err)
	if len(sessions) != 2 || sessions[0].SessionID != "valid" || sessions[1].StartedAt != "bad-start" || sessions[1].EndedAt != "bad-end" {
		t.Fatalf("sessions = %+v", sessions)
	}
	projects, err := db.ListProjects(SessionFilter{})
	must(t, err)
	if !reflect.DeepEqual(projects, []ProjectRow{{RepoPath: "/valid", SessionCount: 1}, {RepoPath: "/invalid", SessionCount: 1}}) {
		t.Fatalf("projects = %+v", projects)
	}
	messages, err := db.GetMessages(SourceClaudeCode, "invalid")
	must(t, err)
	want := []MessageRow{
		{UUID: "bad-1", Role: "user", Content: "needle bad one", Timestamp: "bad-message"},
		{UUID: "bad-2", Role: "user", Content: "needle bad two", Timestamp: "also-bad"},
		{UUID: "early", Role: "user", Content: "needle early", Timestamp: "2026-03-28T10:00:00+02:00"},
	}
	if !reflect.DeepEqual(messages, want) {
		t.Fatalf("messages = %+v, want %+v", messages, want)
	}
	summary, err := db.GetSummaryMessages(SourceClaudeCode, "invalid", 3, false)
	must(t, err)
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("summary = %+v, want %+v", summary, want)
	}
	hits, err := db.SearchMessages(SessionFilter{}, "needle", SearchPagination{})
	must(t, err)
	var ids []string
	for _, hit := range hits {
		ids = append(ids, hit.UUID)
	}
	if !reflect.DeepEqual(ids, []string{"valid", "early", "bad-2", "bad-1"}) || hits[3].Timestamp != "bad-message" || hits[3].Content != "needle bad one" {
		t.Fatalf("hits = %+v", hits)
	}
	for _, filter := range []SessionFilter{
		{Since: "2026-03-28T09:00:00.500000000Z"},
		{Until: "2026-03-28T09:00:00.500000001Z"},
		{Since: "2026-03-28T09:00:00.500000000Z", Until: "2026-03-28T09:00:00.500000001Z"},
	} {
		sessions, err = db.ListSessions(filter)
		must(t, err)
		if len(sessions) != 1 || sessions[0].SessionID != "valid" {
			t.Fatalf("filter %+v sessions = %+v", filter, sessions)
		}
		projects, err = db.ListProjects(filter)
		must(t, err)
		if !reflect.DeepEqual(projects, []ProjectRow{{RepoPath: "/valid", SessionCount: 1}}) {
			t.Fatalf("filter %+v projects = %+v", filter, projects)
		}
		hits, err = db.SearchMessages(filter, "needle", SearchPagination{})
		must(t, err)
		expected := []string{"valid"}
		if filter.Since == "" {
			expected = []string{"valid", "early"}
		}
		ids = nil
		for _, hit := range hits {
			ids = append(ids, hit.UUID)
		}
		if !reflect.DeepEqual(ids, expected) {
			t.Fatalf("filter %+v hits = %+v", filter, hits)
		}
	}
}

func TestSessionUpsertInvalidTimestampsDoNotDisplaceValidRange(t *testing.T) {
	for _, invalidFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "invalid-first", false: "valid-first"}[invalidFirst], func(t *testing.T) {
			db := testDB(t)
			invalid := SessionMeta{Source: SourceClaudeCode, SessionID: "mixed", StartedAt: "bad-start", EndedAt: "bad-end"}
			valid := SessionMeta{Source: SourceClaudeCode, SessionID: "mixed", StartedAt: "2026-03-28T09:00:00Z", EndedAt: "2026-03-28T09:30:00Z"}
			sequence := []SessionMeta{invalid, invalid, valid, invalid}
			if !invalidFirst {
				sequence = []SessionMeta{valid, invalid, valid}
			}
			for _, s := range sequence {
				must(t, db.UpsertSession(s, "2026-03-28T10:00:00Z"))
			}
			row, err := db.GetSession(SourceClaudeCode, "mixed")
			must(t, err)
			if row.StartedAt != valid.StartedAt || row.EndedAt != valid.EndedAt {
				t.Fatalf("range = %+v", row)
			}
		})
	}
}
