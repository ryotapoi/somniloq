package core

import "fmt"

type backfillTarget struct {
	source    Source
	sessionID string
	cwd       string
}

// BackfillResult reports per-category counts from a single Backfill run.
type BackfillResult struct {
	// MigratedSessions / MigratedMessages / MigratedImportStates report the
	// number of rows migrated from v0.3 to v0.4 schema. CLI callers should
	// invoke MigrateToV04IfNeeded directly to capture these counts because
	// Backfill is idempotent and re-runs the migration step (which produces 0
	// when CLI already ran it). These fields are meaningful only for
	// tests / other direct callers of Backfill.
	MigratedSessions     int
	MigratedMessages     int
	MigratedImportStates int

	Deleted    int
	Resolved   int
	Unresolved int
}

// MigrateToV04IfNeeded performs v0.3 → v0.4 schema migration if the DB is
// still on v0.3. Idempotent: returns (0, 0, 0, nil) on a v0.4 DB.
//
// CLI callers should run this preflight before CountOrphanSessions because
// the v0.4 SQL it emits requires the source column. Backfill also calls this
// internally to keep direct-callers safe.
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
	present, err := tableColumnPresent(db.db, "sessions", "source")
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
	var prevFK int
	if err = db.db.QueryRow("PRAGMA foreign_keys").Scan(&prevFK); err != nil {
		return 0, 0, 0, fmt.Errorf("read pragma foreign_keys: %w", err)
	}
	if _, err = db.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return 0, 0, 0, fmt.Errorf("disable foreign_keys: %w", err)
	}
	defer func() {
		restoreSQL := "PRAGMA foreign_keys = OFF"
		if prevFK == 1 {
			restoreSQL = "PRAGMA foreign_keys = ON"
		}
		if _, restoreErr := db.db.Exec(restoreSQL); restoreErr != nil && err == nil {
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
		SELECT 'claude_code', session_id, cwd, repo_path, git_branch, custom_title, agent_name, version, started_at, ended_at, imported_at
		FROM sessions`); err != nil {
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
		SELECT uuid, 'claude_code', session_id, parent_uuid, role, content, timestamp, is_sidechain
		FROM messages`); err != nil {
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
		SELECT jsonl_path, 'claude_code', file_size, last_offset, imported_at
		FROM import_state`); err != nil {
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

// selectBackfillTargets returns a fully materialized slice so the rows
// connection is released before Backfill opens a transaction
// (SetMaxOpenConns(1) would otherwise deadlock the Begin call).
//
// The EXISTS clause excludes orphan sessions (no messages); Backfill DELETEs
// them, so resolving repo_path on rows that are about to be deleted would
// inflate the Resolved counter and waste a write.
func selectBackfillTargets(db *DB) ([]backfillTarget, error) {
	rows, err := db.db.Query(`SELECT source, session_id, cwd FROM sessions WHERE repo_path IS NULL AND cwd IS NOT NULL AND cwd != '' AND EXISTS (SELECT 1 FROM messages m WHERE m.source = sessions.source AND m.session_id = sessions.session_id)`)
	if err != nil {
		return nil, fmt.Errorf("select sessions for backfill: %w", err)
	}
	defer rows.Close()
	var out []backfillTarget
	for rows.Next() {
		var t backfillTarget
		var src string
		if err := rows.Scan(&src, &t.sessionID, &t.cwd); err != nil {
			return nil, fmt.Errorf("scan session for backfill: %w", err)
		}
		t.source = Source(src)
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions for backfill: %w", err)
	}
	return out, nil
}

// CountOrphanSessions returns the number of sessions with no messages.
// Used by the CLI to decide whether to prompt before a destructive backfill.
//
// Precondition: the v0.4 schema (source column, composite PK) is in place.
// Callers should run MigrateToV04IfNeeded first when the DB may still be on
// v0.3.
func CountOrphanSessions(db *DB) (int, error) {
	var count int
	err := db.db.QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE NOT EXISTS (SELECT 1 FROM messages m WHERE m.source = sessions.source AND m.session_id = sessions.session_id)`,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count orphan sessions: %w", err)
	}
	return count, nil
}

// Backfill corrects legacy session data:
//  1. Migrates v0.3 → v0.4 schema if needed (idempotent).
//  2. Deletes sessions that have no messages (v0.2.x meta-prefix INSERT residue).
//  3. Resolves repo_path for sessions where it is still NULL and cwd is populated.
//
// Idempotent: re-running is a no-op once the DB is clean.
// The unresolved branch guards against residual pathological inputs (e.g. cwd
// starts with the worktree fragment → ResolveRepoPath returns "").
//
// MigratedXxx fields in BackfillResult are populated by the internal
// MigrateToV04IfNeeded call. CLI callers preflight migration separately and
// will see 0 here for the second invocation, which is expected.
func Backfill(db *DB) (BackfillResult, error) {
	var result BackfillResult

	ms, mm, mi, err := MigrateToV04IfNeeded(db)
	if err != nil {
		return result, err
	}
	result.MigratedSessions = ms
	result.MigratedMessages = mm
	result.MigratedImportStates = mi

	todo, err := selectBackfillTargets(db)
	if err != nil {
		return result, err
	}

	// Resolve outside the transaction. git rev-parse is an external process
	// and SetMaxOpenConns(1) means holding a tx would block DB access.
	cache := map[string]string{}
	for _, t := range todo {
		if _, ok := cache[t.cwd]; !ok {
			cache[t.cwd] = ResolveRepoPath(t.cwd)
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("begin backfill tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`DELETE FROM sessions WHERE NOT EXISTS (SELECT 1 FROM messages m WHERE m.source = sessions.source AND m.session_id = sessions.session_id)`)
	if err != nil {
		return result, fmt.Errorf("delete orphan sessions: %w", err)
	}
	// modernc.org/sqlite always returns nil here (see references/knowledge.md);
	// the check is defensive against a future driver swap.
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return result, fmt.Errorf("count deleted rows: %w", err)
	}
	result.Deleted = int(rowsAffected)

	for _, t := range todo {
		repo := cache[t.cwd]
		if repo == "" {
			result.Unresolved++
			continue
		}
		if _, err := tx.Exec(`UPDATE sessions SET repo_path = ? WHERE source = ? AND session_id = ?`, repo, string(t.source), t.sessionID); err != nil {
			return result, fmt.Errorf("update session %s/%s: %w", t.source, t.sessionID, err)
		}
		result.Resolved++
	}

	if err := tx.Commit(); err != nil {
		return result, fmt.Errorf("commit backfill: %w", err)
	}
	return result, nil
}
