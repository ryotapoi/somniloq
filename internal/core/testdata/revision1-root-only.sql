
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
 parent_session_id TEXT NOT NULL DEFAULT '',
 parent_identity TEXT NOT NULL DEFAULT '',
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
 blocks_json TEXT NOT NULL DEFAULT '[]',
 timestamp TEXT NOT NULL,
 is_sidechain BOOLEAN DEFAULT FALSE,
 number INTEGER NOT NULL DEFAULT 0,
 origin_path TEXT NOT NULL DEFAULT '',
 origin_line INTEGER NOT NULL DEFAULT 0,
 membership TEXT NOT NULL DEFAULT 'body' CHECK(membership IN ('body','context','unresolved')),
 payload_id TEXT NOT NULL DEFAULT '',
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
 content_hash TEXT NOT NULL DEFAULT '',
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
