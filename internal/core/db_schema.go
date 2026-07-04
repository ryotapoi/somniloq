package core

import (
	"database/sql"
	"fmt"
)

// ensureSessionsProjectDirColumnDropped removes the legacy project_dir column
// if it is still present. Required when upgrading from v0.2.x DBs.
// Precondition: the sessions table exists. SQLite 3.35+ required for
// DROP COLUMN.
func ensureSessionsProjectDirColumnDropped(db execer) error {
	present, err := tableColumnPresent(db, "sessions", "project_dir")
	if err != nil {
		return fmt.Errorf("inspect sessions table: %w", err)
	}
	if !present {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE sessions DROP COLUMN project_dir"); err != nil {
		// Race re-check: if the column is now absent, another instance dropped
		// it between our inspect and ALTER — treat as success. Any other state
		// (re-check failed, or column still present) returns the original
		// ALTER error.
		if present2, pErr := tableColumnPresent(db, "sessions", "project_dir"); pErr == nil && !present2 {
			return nil
		}
		return fmt.Errorf("migrate drop project_dir column: %w", err)
	}
	return nil
}

// ensureSessionsRepoPathColumn adds sessions.repo_path if it is missing.
// Precondition: the sessions table exists.
func ensureSessionsRepoPathColumn(db execer) error {
	present, err := tableColumnPresent(db, "sessions", "repo_path")
	if err != nil {
		return fmt.Errorf("inspect sessions table: %w", err)
	}
	if present {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE sessions ADD COLUMN repo_path TEXT"); err != nil {
		// Belt-and-suspenders: re-check state rather than match driver-specific
		// error strings. Covers the narrow cross-process race where another
		// instance added the column between our inspect and ALTER. Only treat
		// it as success when the column is actually present now — otherwise
		// surface the original ALTER error.
		if present2, pErr := tableColumnPresent(db, "sessions", "repo_path"); pErr == nil && present2 {
			return nil
		}
		return fmt.Errorf("migrate repo_path column: %w", err)
	}
	return nil
}

// tableColumnPresent reports whether the given column exists on the table.
// Precondition: the table exists. PRAGMA table_info returns no rows when the
// table is missing, so the caller must guarantee existence (e.g. by running
// the schema constant first).
//
// SECURITY: `table` is interpolated into the SQL because PRAGMA does not
// accept `?` placeholders. Pass only trusted internal constants
// ("sessions", "messages", "import_state"); never propagate user input here.
func tableColumnPresent(db execer, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
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

CREATE TABLE IF NOT EXISTS messages (
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

CREATE TABLE IF NOT EXISTS import_state (
    jsonl_path TEXT PRIMARY KEY,
    source TEXT NOT NULL CHECK(source <> ''),
    file_size INTEGER,
    last_offset INTEGER,
    imported_at TEXT NOT NULL
);
`
