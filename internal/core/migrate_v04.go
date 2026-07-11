package core

import (
	"database/sql"
	"fmt"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

// MigrateToV04IfNeeded performs v0.3 → v0.4 schema migration if the DB is
// still on v0.3. Idempotent: returns (0, 0, 0, nil) on a v0.4 DB.
//
// Callers must run this preflight before CountOrphanSessions and Backfill
// because the v0.4 SQL they emit requires the source column.
func MigrateToV04IfNeeded(db *DB) (sessions, messages, importStates int, err error) {
	needs, err := needsV04Migration(db)
	if err != nil {
		return 0, 0, 0, err
	}
	if !needs {
		return 0, 0, 0, nil
	}
	return migrateToV04(db)
}

// needsV04Migration reports whether the sessions table is still on v0.3 (no
// source column). Precondition: OpenDB has run, so all three tables exist;
// migration always rebuilds the three tables together inside one transaction
// (so a partial state never persists).
func needsV04Migration(db *DB) (bool, error) {
	present, err := tableColumnPresent(db.execer(), "sessions", "source")
	if err != nil {
		return false, fmt.Errorf("inspect sessions for v0.4 migration: %w", err)
	}
	return !present, nil
}

// migrateToV04 rebuilds sessions / messages / import_state in v0.4 form
// inside a single transaction. Existing rows are stamped source='claude_code'.
//
// PRAGMA foreign_keys is forced OFF for the duration so that DROP TABLE
// sessions does not fail on the messages FK reference, and is restored to its
// previous value via defer (in case OpenDB later starts enabling FK
// enforcement). The PRAGMA must be set outside the transaction because SQLite
// does not honor PRAGMA changes inside a tx.
//
// Named return is required so the defer can override err with a restore
// failure when the body itself succeeded.
func migrateToV04(db *DB) (sessionsN, messagesN, importStatesN int, err error) {
	return migrateToV04WithRestore(db, restoreForeignKeys)
}

func restoreForeignKeys(db execer, prevFK int) error {
	restoreSQL := "PRAGMA foreign_keys = OFF"
	if prevFK == 1 {
		restoreSQL = "PRAGMA foreign_keys = ON"
	}
	_, err := db.Exec(restoreSQL)
	return err
}

func migrateToV04WithRestore(db *DB, restore func(execer, int) error) (sessionsN, messagesN, importStatesN int, err error) {
	return migrateToV04WithRestoreAfterDropSessions(db, restore, nil)
}

// afterDropSessionsHook is used by tests to inspect and fail after the most
// destructive migration statement while its transaction is still open.
// Production migration passes nil.
type afterDropSessionsHook func(tx *sql.Tx) error

func migrateToV04WithRestoreAfterDropSessions(db *DB, restore func(execer, int) error, afterDropSessions afterDropSessionsHook) (sessionsN, messagesN, importStatesN int, err error) {
	var prevFK int
	if err = db.execer().QueryRow("PRAGMA foreign_keys").Scan(&prevFK); err != nil {
		return 0, 0, 0, fmt.Errorf("read pragma foreign_keys: %w", err)
	}
	if _, err = db.execer().Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return 0, 0, 0, fmt.Errorf("disable foreign_keys: %w", err)
	}
	defer func() {
		if restoreErr := restore(db.execer(), prevFK); restoreErr != nil && err == nil {
			err = fmt.Errorf("restore foreign_keys: %w", restoreErr)
		}
	}()

	tx, err := db.Begin()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("begin v0.4 migration: %w", err)
	}
	defer tx.Rollback()

	// sessions: rebuild with composite PK.
	if _, err = tx.Exec(`CREATE TABLE sessions_new (
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
	)`); err != nil {
		return 0, 0, 0, fmt.Errorf("create sessions_new: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO sessions_new (source, session_id, cwd, repo_path, git_branch, custom_title, agent_name, version, started_at, ended_at, imported_at)
		SELECT ?, session_id, cwd, repo_path, git_branch, custom_title, agent_name, version, started_at, ended_at, imported_at
		FROM sessions`, string(ingest.SourceClaudeCode)); err != nil {
		return 0, 0, 0, fmt.Errorf("copy sessions: %w", err)
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM sessions_new`).Scan(&sessionsN); err != nil {
		return 0, 0, 0, fmt.Errorf("count sessions_new: %w", err)
	}

	// messages: rebuild. FK references "sessions" (after rename) with composite key.
	if _, err = tx.Exec(`CREATE TABLE messages_new (
		uuid TEXT PRIMARY KEY,
		source TEXT NOT NULL CHECK(source <> ''),
		session_id TEXT NOT NULL,
		parent_uuid TEXT,
		role TEXT NOT NULL,
		content TEXT NOT NULL,
		timestamp TEXT NOT NULL,
		is_sidechain BOOLEAN DEFAULT FALSE,
		FOREIGN KEY (source, session_id) REFERENCES sessions(source, session_id)
	)`); err != nil {
		return 0, 0, 0, fmt.Errorf("create messages_new: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO messages_new (uuid, source, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		SELECT uuid, ?, session_id, parent_uuid, role, content, timestamp, is_sidechain
		FROM messages`, string(ingest.SourceClaudeCode)); err != nil {
		return 0, 0, 0, fmt.Errorf("copy messages: %w", err)
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM messages_new`).Scan(&messagesN); err != nil {
		return 0, 0, 0, fmt.Errorf("count messages_new: %w", err)
	}

	// import_state: rebuild. PK stays jsonl_path single column. Recreated
	// (instead of ALTER ADD COLUMN ... DEFAULT) so the DEFAULT clause does not
	// leak into the migrated DB schema and diverge from a fresh v0.4 DB.
	if _, err = tx.Exec(`CREATE TABLE import_state_new (
		jsonl_path TEXT PRIMARY KEY,
		source TEXT NOT NULL CHECK(source <> ''),
		file_size INTEGER,
		last_offset INTEGER,
		imported_at TEXT NOT NULL
	)`); err != nil {
		return 0, 0, 0, fmt.Errorf("create import_state_new: %w", err)
	}
	if _, err = tx.Exec(`INSERT INTO import_state_new (jsonl_path, source, file_size, last_offset, imported_at)
		SELECT jsonl_path, ?, file_size, last_offset, imported_at
		FROM import_state`, string(ingest.SourceClaudeCode)); err != nil {
		return 0, 0, 0, fmt.Errorf("copy import_state: %w", err)
	}
	if err = tx.QueryRow(`SELECT COUNT(*) FROM import_state_new`).Scan(&importStatesN); err != nil {
		return 0, 0, 0, fmt.Errorf("count import_state_new: %w", err)
	}

	// Drop old tables (FK referrer first), then rename.
	if _, err = tx.Exec(`DROP TABLE messages`); err != nil {
		return 0, 0, 0, fmt.Errorf("drop messages: %w", err)
	}
	if _, err = tx.Exec(`DROP TABLE sessions`); err != nil {
		return 0, 0, 0, fmt.Errorf("drop sessions: %w", err)
	}
	if afterDropSessions != nil {
		if err = afterDropSessions(tx); err != nil {
			return 0, 0, 0, fmt.Errorf("drop sessions: %w", err)
		}
	}
	if _, err = tx.Exec(`DROP TABLE import_state`); err != nil {
		return 0, 0, 0, fmt.Errorf("drop import_state: %w", err)
	}
	if _, err = tx.Exec(`ALTER TABLE sessions_new RENAME TO sessions`); err != nil {
		return 0, 0, 0, fmt.Errorf("rename sessions_new: %w", err)
	}
	if _, err = tx.Exec(`ALTER TABLE messages_new RENAME TO messages`); err != nil {
		return 0, 0, 0, fmt.Errorf("rename messages_new: %w", err)
	}
	if _, err = tx.Exec(`ALTER TABLE import_state_new RENAME TO import_state`); err != nil {
		return 0, 0, 0, fmt.Errorf("rename import_state_new: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, 0, 0, fmt.Errorf("commit v0.4 migration: %w", err)
	}
	return sessionsN, messagesN, importStatesN, nil
}
