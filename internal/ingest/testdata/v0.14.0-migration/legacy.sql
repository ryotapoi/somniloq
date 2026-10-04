-- Synthetic legacy-v013 schema and normalized messages; no real log content.
PRAGMA user_version=0;
CREATE TABLE sessions (
    source TEXT NOT NULL CHECK(source <> ''),
    session_id TEXT NOT NULL,
    cwd TEXT,
    repo_path TEXT,
    git_branch TEXT,
    custom_title TEXT,
    agent_name TEXT,
    version TEXT,
    started_at TEXT,
    ended_at TEXT,
    imported_at TEXT NOT NULL,
    PRIMARY KEY (source, session_id)
);

CREATE TABLE messages (
    uuid TEXT PRIMARY KEY,
    source TEXT NOT NULL CHECK(source <> ''),
    session_id TEXT NOT NULL,
    parent_uuid TEXT,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    is_sidechain BOOLEAN DEFAULT FALSE,
    FOREIGN KEY (source, session_id) REFERENCES sessions(source, session_id)
);

CREATE INDEX messages_session_idx ON messages(source, session_id);

CREATE TABLE import_state (
    jsonl_path TEXT PRIMARY KEY,
    source TEXT NOT NULL CHECK(source <> ''),
    file_size INTEGER,
    last_offset INTEGER,
    imported_at TEXT NOT NULL
);
INSERT INTO sessions(source,session_id,imported_at) VALUES ('codex','parent','2026-01-01T00:00:00Z');
INSERT INTO sessions(source,session_id,imported_at) VALUES ('codex','multi','2026-01-01T00:00:00Z');
INSERT INTO sessions(source,session_id,imported_at) VALUES ('codex','conflict','2026-01-01T00:00:00Z');
INSERT INTO sessions(source,session_id,imported_at) VALUES ('codex','missing','2026-01-01T00:00:00Z');
INSERT INTO sessions(source,session_id,imported_at) VALUES ('claude_code','other','2026-01-01T00:00:00Z');
INSERT INTO sessions(source,session_id,imported_at) VALUES ('cursor_agent','other','2026-01-01T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (1,'codex:9f86d598cf04a7c765e37f2b6388d9204f252981400b395d65d1f68974bb5761','codex','parent','assistant','legacy inherited','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (2,'codex:594944de5576a900f35d1fa066335d5b9e7f52afb53c83f621e0a148e46ce5fd','codex','parent','assistant','legacy child own','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (3,'codex:6cff90dc527dfbb4f2e0f2e91821c8239b5325d404c6ddcf9448c95f2159049b','codex','parent','user','unmatched parent retained','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (4,'codex:aef2466c2b168af769896c82312ea6949428237864be366230b29636a6d8df5f','codex','multi','user','legacy first','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (5,'codex:636fc24904bee52378b09e70e653d73cd7691fed099d8802ba3a8f67be5f1a1a','codex','multi','assistant','legacy second','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (6,'codex:efb59ba9ab065c86d57e73b6b371cbcf1207a50192e8be45aa75b98aa2df7c8d','codex','conflict','user','ambiguous input retained','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (7,'claude-u1','claude_code','other','user','other source retained','2025-12-31T00:00:00Z');
INSERT INTO messages(rowid,uuid,source,session_id,role,content,timestamp) VALUES (8,'cursor-u1','cursor_agent','other','assistant','unknown time retained','');
INSERT INTO import_state VALUES ('/fixture/input-a/01-child.jsonl','codex',100,100,'2026-01-01T00:00:00Z');
INSERT INTO import_state VALUES ('/fixture/input-a/02-multi.jsonl','codex',100,100,'2026-01-01T00:00:00Z');
INSERT INTO import_state VALUES ('/fixture/input-a/03-multi.jsonl','codex',100,100,'2026-01-01T00:00:00Z');
INSERT INTO import_state VALUES ('/fixture/old/missing.jsonl','codex',100,100,'2026-01-01T00:00:00Z');
INSERT INTO import_state VALUES ('/fixture/shared/04-conflict.jsonl','codex',100,100,'2026-01-01T00:00:00Z');
