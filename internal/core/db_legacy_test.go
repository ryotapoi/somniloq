package core

import (
	"strings"
	"testing"
)

func TestLegacyHistoryReadAndCoexistence(t *testing.T) {
	db := testDB(t)
	digest := strings.Repeat("a", 64)
	for _, id := range []string{"shared", "metadata-only"} {
		_, err := db.db.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,imported_at) VALUES (?,'codex',?,'saved')`, digest, id)
		must(t, err)
	}
	_, err := db.db.Exec(`INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,timestamp,number) VALUES (9,?,'old','codex','shared','user','legacy saved',NULL,3)`, digest)
	must(t, err)
	input := testInput(t, db, SourceCodex)
	must(t, db.UpsertSession(input, SessionMeta{Source: SourceCodex, SessionID: "shared"}, "now"))
	must(t, db.InsertMessage(input, NormalizedMessage{Source: SourceCodex, SessionID: "shared", UUID: "normal", Role: "user", Content: "normal"}))
	ref := LegacyREF(digest, SourceCodex, "shared")
	resolved := requireResolution(t, db, ref)
	if resolved.Parent != nil || resolved.Root != nil || len(resolved.Members) != 1 || len(resolved.Descendants) != 1 || resolved.Self.REF != ref {
		t.Fatalf("legacy resolution: %+v", resolved)
	}
	key, src, id, err := parseREF(ref)
	must(t, err)
	if key != "legacy:"+digest || src != SourceCodex || id != "shared" {
		t.Fatalf("parsed %q %q %q", key, src, id)
	}
	session, err := db.LookupSessionREF(ref)
	must(t, err)
	if session == nil || session.InputID != LegacyInputID || session.REF != ref || session.MessageCount != 1 || session.ParentREF != "" {
		t.Fatalf("session %+v", session)
	}
	messages, err := db.GetIdentityMessages(session.InputID, session.Source, session.Identity)
	must(t, err)
	if len(messages) != 1 || messages[0].Number != 3 || messages[0].Blocks != nil || messages[0].Timestamp != "" || messages[0].LegacyRowID != 9 || messages[0].Provenance != "legacy_saved" {
		t.Fatalf("messages %+v", messages)
	}
	matches, err := db.LookupSessionsByID("shared")
	must(t, err)
	if len(matches) != 2 || matches[0].REF == matches[1].REF {
		t.Fatalf("matches %+v", matches)
	}
	sessions, err := db.ListSessions(SessionFilter{})
	must(t, err)
	if len(sessions) != 3 {
		t.Fatalf("sessions %+v", sessions)
	}
	meta, err := db.LookupSessionREF(LegacyREF(digest, SourceCodex, "metadata-only"))
	must(t, err)
	if meta == nil || meta.MessageCount != 0 {
		t.Fatalf("metadata %+v", meta)
	}
	results, err := db.SearchMessages(SessionFilter{}, "legacy", SearchPagination{SessionREF: ref})
	must(t, err)
	if len(results) != 1 || results[0].REF != ref {
		t.Fatalf("search %+v", results)
	}
	results, err = db.SearchMessages(SessionFilter{Since: "2020-01-01T00:00:00Z"}, "legacy", SearchPagination{})
	must(t, err)
	if len(results) != 0 {
		t.Fatalf("unknown timestamp matched %+v", results)
	}
	must(t, db.DeleteInputs([]int64{input}))
	session, err = db.LookupSessionREF(ref)
	must(t, err)
	if session == nil || session.MessageCount != 1 {
		t.Fatalf("legacy erased %+v", session)
	}
}

func TestLegacyREFRejectsNoncanonicalReferences(t *testing.T) {
	digest := strings.Repeat("a", 64)
	for _, ref := range []string{
		"slq1:legacy:" + digest + ":codex:",
		"slq1:legacy:" + strings.ToUpper(digest) + ":codex:cw",
		"slq1:legacy:" + digest + ":unknown:cw",
		"slq1:legacy:" + digest + ":codex:cw==",
	} {
		if _, _, _, err := parseREF(ref); err == nil {
			t.Fatalf("accepted %q", ref)
		}
	}
}
