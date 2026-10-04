package core

import (
	"database/sql"
	"fmt"
	"reflect"
)

const schema = `
CREATE TABLE inputs (
 id INTEGER PRIMARY KEY,
 input_key TEXT NOT NULL UNIQUE,
 source TEXT NOT NULL CHECK(source IN ('claude_code','codex','cursor_agent')),
 root TEXT NOT NULL,
 UNIQUE(source,root),
 UNIQUE(id,source)
);
CREATE TABLE sessions (
 input_id INTEGER NOT NULL,
 source TEXT NOT NULL CHECK(source <> ''),
 session_id TEXT NOT NULL,
 identity TEXT NOT NULL,
 cwd TEXT,
 repo_path TEXT,
 git_branch TEXT,
 custom_title TEXT,
 agent_name TEXT,
 version TEXT,
 started_at TEXT,
 ended_at TEXT,
 imported_at TEXT NOT NULL,
 PRIMARY KEY(input_id,source,session_id),
 UNIQUE(input_id,identity),
 FOREIGN KEY(input_id,source) REFERENCES inputs(id,source)
);
CREATE TABLE messages (
 input_id INTEGER NOT NULL,
 uuid TEXT NOT NULL,
 source TEXT NOT NULL CHECK(source <> ''),
 session_id TEXT NOT NULL,
 parent_uuid TEXT,
 role TEXT NOT NULL,
 content TEXT NOT NULL,
 timestamp TEXT NOT NULL,
 is_sidechain BOOLEAN DEFAULT FALSE,
 PRIMARY KEY(input_id,uuid),
 FOREIGN KEY(input_id,source,session_id) REFERENCES sessions(input_id,source,session_id)
);
CREATE INDEX messages_session_idx ON messages(input_id,source,session_id);
CREATE TABLE import_state (
 input_id INTEGER NOT NULL,
 jsonl_path TEXT NOT NULL,
 source TEXT NOT NULL CHECK(source <> ''),
 file_size INTEGER,
 last_offset INTEGER,
 imported_at TEXT NOT NULL,
 PRIMARY KEY(input_id,jsonl_path),
 FOREIGN KEY(input_id,source) REFERENCES inputs(id,source)
);
CREATE TABLE migration_origin (
 id INTEGER PRIMARY KEY CHECK(id=1),
 snapshot_sha256 TEXT NOT NULL,
 legacy_shape TEXT NOT NULL,
 copy_complete INTEGER NOT NULL CHECK(copy_complete=1),
 snapshot_path TEXT NOT NULL
);
PRAGMA user_version=1;
`

type SchemaError struct {
	Revision int
	Reason   string
}

func (e *SchemaError) Error() string {
	if e.Revision == 0 {
		return "unsupported database schema (revision 0); dedicated migration is not implemented yet; use a new database"
	}
	return fmt.Sprintf("unsupported database schema (revision %d): %s", e.Revision, e.Reason)
}

func schemaObjects(e execer) ([][3]string, error) {
	rows, err := e.Query(`SELECT type,name,COALESCE(sql,'') FROM sqlite_master WHERE substr(name,1,7) <> 'sqlite_' ORDER BY type,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	objects := [][3]string{}
	for rows.Next() {
		var item [3]string
		if err := rows.Scan(&item[0], &item[1], &item[2]); err != nil {
			return nil, err
		}
		objects = append(objects, item)
	}
	return objects, rows.Err()
}

// Compare the complete declared schema, including indexes and constraints.
func inspectSchema(e execer) (empty bool, err error) {
	var revision int
	if err = e.QueryRow(`PRAGMA user_version`).Scan(&revision); err != nil {
		return false, err
	}
	objects, err := schemaObjects(e)
	if err != nil {
		return false, err
	}
	if revision == 0 && len(objects) == 0 {
		return true, nil
	}
	if revision != 1 {
		return false, &SchemaError{Revision: revision, Reason: "unsupported revision"}
	}
	expected, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return false, err
	}
	defer expected.Close()
	expected.SetMaxOpenConns(1)
	if _, err = expected.Exec(schema); err != nil {
		return false, err
	}
	want, err := schemaObjects(expected)
	if err != nil {
		return false, err
	}
	if !reflect.DeepEqual(objects, want) {
		return false, &SchemaError{Revision: revision, Reason: "unknown shape"}
	}
	return false, nil
}
