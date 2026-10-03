package core

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenDBSessionIndexPreservesExistingMessages(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "empty"
		if existing {
			name = "existing"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sessions.db")
			want := []MessageRow{}
			if existing {
				legacy, err := sql.Open("sqlite", path)
				must(t, err)
				// The prior messages schema had no session index. Explicit rowids
				// make table rebuilding or row replacement observable.
				_, err = legacy.Exec(`CREATE TABLE messages (
					uuid TEXT PRIMARY KEY, source TEXT NOT NULL CHECK(source <> ''),
					session_id TEXT NOT NULL, parent_uuid TEXT, role TEXT NOT NULL,
					content TEXT NOT NULL, timestamp TEXT NOT NULL,
					is_sidechain BOOLEAN DEFAULT FALSE,
					FOREIGN KEY (source, session_id) REFERENCES sessions(source, session_id)
				);
				INSERT INTO messages(rowid, uuid, source, session_id, role, content, timestamp)
				VALUES (7, 'first', 'codex', 's1', 'user', 'question', '2026-03-28T09:00:00.100+01:00'),
				       (19, 'second', 'codex', 's1', 'assistant', 'answer', '2026-03-28T08:00:00.1Z');`)
				must(t, err)
				must(t, legacy.Close())
				want = []MessageRow{
					{UUID: "first", Role: "user", Content: "question", Timestamp: "2026-03-28T09:00:00.100+01:00"},
					{UUID: "second", Role: "assistant", Content: "answer", Timestamp: "2026-03-28T08:00:00.1Z"},
				}
			}
			var firstRoot, firstSchemaVersion int
			for open := 0; open < 2; open++ {
				db, err := OpenDB(path)
				must(t, err)
				defer db.Close()
				var root, schemaVersion int
				must(t, db.db.QueryRow(`SELECT rootpage FROM sqlite_master WHERE type = 'index' AND name = 'messages_session_idx'`).Scan(&root))
				must(t, db.db.QueryRow(`PRAGMA schema_version`).Scan(&schemaVersion))
				if open == 0 {
					firstRoot = root
					firstSchemaVersion = schemaVersion
				} else if root != firstRoot || schemaVersion != firstSchemaVersion {
					t.Errorf("reopen rootpage/schema_version = %d/%d, want unchanged %d/%d", root, schemaVersion, firstRoot, firstSchemaVersion)
				}
				got, err := db.GetMessages(SourceCodex, "s1")
				must(t, err)
				if !reflect.DeepEqual(got, want) {
					t.Errorf("messages = %+v, want %+v", got, want)
				}
				if existing {
					// A duplicate insert must leave rowids and original bodies intact.
					must(t, db.InsertMessage(NormalizedMessage{Source: SourceCodex, SessionID: "s1", UUID: "first", Role: "user", Content: "replacement", Timestamp: "invalid"}))
					for i, uuid := range []string{"first", "second"} {
						var rowid int
						must(t, db.db.QueryRow(`SELECT rowid FROM messages WHERE uuid = ?`, uuid).Scan(&rowid))
						if wantID := []int{7, 19}[i]; rowid != wantID {
							t.Errorf("%s rowid = %d, want %d", uuid, rowid, wantID)
						}
					}
				}
				must(t, db.Close())
			}
		})
	}
}
