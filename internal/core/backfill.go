package core

import "fmt"

// orphanSessionPredicate defines a session with no associated messages.
// Keep count, deletion, and the backfill-target complement aligned to this
// predicate: the count controls the destructive CLI confirmation.
const orphanSessionPredicate = `NOT EXISTS (SELECT 1 FROM messages m WHERE m.source = sessions.source AND m.session_id = sessions.session_id)`

type backfillTarget struct {
	source    Source
	sessionID string
	cwd       string
}

// BackfillResult reports per-category counts from a single Backfill run.
type BackfillResult struct {
	Deleted    int
	Resolved   int
	Unresolved int
}

// selectBackfillTargets returns a fully materialized slice so the rows
// connection is released before Backfill opens a transaction
// (SetMaxOpenConns(1) would otherwise deadlock the Begin call).
//
// The EXISTS clause excludes orphan sessions (no messages); Backfill DELETEs
// them, so resolving repo_path on rows that are about to be deleted would
// inflate the Resolved counter and waste a write.
func selectBackfillTargets(db *DB) ([]backfillTarget, error) {
	rows, err := db.execer().Query(`SELECT source, session_id, cwd FROM sessions WHERE repo_path IS NULL AND cwd IS NOT NULL AND cwd != '' AND NOT (` + orphanSessionPredicate + `)`)
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
// Callers must run MigrateToV04IfNeeded first when the DB may still be on
// v0.3.
func CountOrphanSessions(db *DB) (int, error) {
	var count int
	err := db.execer().QueryRow(
		`SELECT COUNT(*) FROM sessions WHERE ` + orphanSessionPredicate,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count orphan sessions: %w", err)
	}
	return count, nil
}

// Backfill corrects legacy session data:
//  1. Deletes sessions that have no messages (v0.2.x meta-prefix INSERT residue).
//  2. Resolves repo_path for sessions where it is still NULL and cwd is populated.
//
// Idempotent: re-running is a no-op once the DB is clean.
// The unresolved branch guards against residual pathological inputs (e.g. cwd
// starts with the worktree fragment → ResolveRepoPath returns "").
//
// Precondition: the v0.4 schema (source column, composite PK) is in place.
// Callers must run MigrateToV04IfNeeded first when the DB may still be on
// v0.3.
func Backfill(db *DB) (BackfillResult, error) {
	var result BackfillResult

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

	res, err := tx.Exec(`DELETE FROM sessions WHERE ` + orphanSessionPredicate)
	if err != nil {
		return result, fmt.Errorf("delete orphan sessions: %w", err)
	}
	// modernc.org/sqlite returns nil here; the check is defensive against a
	// future driver swap.
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
