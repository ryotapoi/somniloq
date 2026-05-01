package core

import "fmt"

type backfillTarget struct{ sessionID, cwd string }

// selectBackfillTargets returns a fully materialized slice so the rows
// connection is released before BackfillRepoPaths opens a transaction
// (SetMaxOpenConns(1) would otherwise deadlock the Begin call).
func selectBackfillTargets(db *DB) ([]backfillTarget, error) {
	rows, err := db.db.Query(`SELECT session_id, cwd FROM sessions WHERE repo_path IS NULL AND cwd IS NOT NULL AND cwd != ''`)
	if err != nil {
		return nil, fmt.Errorf("select sessions for backfill: %w", err)
	}
	defer rows.Close()
	var out []backfillTarget
	for rows.Next() {
		var t backfillTarget
		if err := rows.Scan(&t.sessionID, &t.cwd); err != nil {
			return nil, fmt.Errorf("scan session for backfill: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sessions for backfill: %w", err)
	}
	return out, nil
}

// BackfillRepoPaths resolves repo_path for sessions where it is still NULL and
// cwd is populated. Idempotent: re-running is a no-op because SELECT filters by
// repo_path IS NULL. The `unresolved` branch guards against residual pathological
// inputs (e.g. cwd starts with the worktree fragment → step 2 returns "").
func BackfillRepoPaths(db *DB) (resolved, unresolved int, err error) {
	todo, err := selectBackfillTargets(db)
	if err != nil {
		return 0, 0, err
	}
	if len(todo) == 0 {
		return 0, 0, nil
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
		return 0, 0, fmt.Errorf("begin backfill tx: %w", err)
	}
	defer tx.Rollback()
	for _, t := range todo {
		repo := cache[t.cwd]
		if repo == "" {
			unresolved++
			continue
		}
		if _, err := tx.Exec(`UPDATE sessions SET repo_path = ? WHERE session_id = ?`, repo, t.sessionID); err != nil {
			return 0, 0, fmt.Errorf("update session %s: %w", t.sessionID, err)
		}
		resolved++
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit backfill: %w", err)
	}
	return resolved, unresolved, nil
}
