package core

import (
	"database/sql"
	"errors"
	"fmt"
)

// MessageRow deliberately has no IsSidechain field: every query that
// produces it excludes sidechain rows in SQL, so the value would always be
// false.
type MessageRow struct {
	UUID      string
	Role      string
	Content   string
	Timestamp string
}

// GetMessages returns the session's messages in chronological order.
// Sidechain rows are excluded: they are subagent transcripts, not part of the
// user-facing conversation.
//
// rowid breaks timestamp ties: Codex records without per-record timestamps
// all inherit the session_meta timestamp, and rowid preserves insertion
// (JSONL line) order because messages are INSERT OR IGNORE, never replaced.
// Turn numbering is derived from this order, so it must stay deterministic.
func (d *DB) GetMessages(source Source, sessionID string) ([]MessageRow, error) {
	rows, err := d.execer().Query(`
		SELECT uuid, role, content, timestamp
		FROM messages
		WHERE source = ? AND session_id = ?
		  AND is_sidechain = 0
		ORDER BY timestamp ASC, rowid ASC`,
		string(source), sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages: query: %w", err)
	}
	return scanMessages(rows, "get messages")
}

// Prefixes of user message content that mark synthetic entries inserted by
// Claude Code itself (a /clear command echo and the caveat block that
// accompanies commands like /clear and shell `!` invocations). They are
// skipped by GetSummaryMessages so that --summary surfaces real user input.
//
// When adding a new prefix, check it does not contain SQLite LIKE wildcards
// (`%` or `_`). If it does, the LIKE clauses in GetSummaryMessages need
// `ESCAPE '\'` and the pattern must escape those characters.
const (
	clearCommandPrefix       = "<command-name>/clear</command-name>"
	localCommandCaveatPrefix = "<local-command-caveat>"
)

// GetSummaryMessages returns the first `limit` user messages of the session
// in chronological order, intended for --summary output. Returns an error if
// limit <= 0.
//
// Always filters to role='user' and is_sidechain=0. By default, also skips
// entries whose content starts with clearCommandPrefix or
// localCommandCaveatPrefix; includeClear=true disables that prefix skip only.
func (d *DB) GetSummaryMessages(source Source, sessionID string, limit int, includeClear bool) ([]MessageRow, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be >= 1")
	}

	query := `
		SELECT uuid, role, content, timestamp
		FROM messages
		WHERE source = ? AND session_id = ?
		  AND role = 'user'
		  AND is_sidechain = 0`
	args := []any{string(source), sessionID}
	if !includeClear {
		query += `
		  AND content NOT LIKE ?
		  AND content NOT LIKE ?`
		args = append(args, clearCommandPrefix+"%", localCommandCaveatPrefix+"%")
	}
	query += `
		ORDER BY timestamp ASC, rowid ASC
		LIMIT ?`
	args = append(args, limit)

	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("get summary messages: query: %w", err)
	}
	return scanMessages(rows, "get summary messages")
}

func scanMessages(rows *sql.Rows, operation string) ([]MessageRow, error) {
	defer rows.Close()

	result := []MessageRow{}
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.UUID, &m.Role, &m.Content, &m.Timestamp); err != nil {
			return nil, fmt.Errorf("%s: scan row: %w", operation, err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: iterate rows: %w", operation, err)
	}
	return result, nil
}
