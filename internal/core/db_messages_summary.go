package core

import (
	"database/sql"
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
		ORDER BY rfc3339_utc_nanos(timestamp) ASC, rowid ASC`,
		string(source), sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages: query: %w", err)
	}
	return scanMessages(rows, "get messages")
}

// GetTurnMessages returns only UUID and role for turn numbering. Content and
// Timestamp remain empty; ordering and sidechain exclusion match GetMessages.
func (d *DB) GetTurnMessages(source Source, sessionID string) ([]MessageRow, error) {
	rows, err := d.execer().Query(`
		SELECT uuid, role
		FROM messages
		WHERE source = ? AND session_id = ?
		  AND is_sidechain = 0
		ORDER BY rfc3339_utc_nanos(timestamp) ASC, rowid ASC`,
		string(source), sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("get turn messages: query: %w", err)
	}
	defer rows.Close()

	result := []MessageRow{}
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.UUID, &m.Role); err != nil {
			return nil, fmt.Errorf("get turn messages: scan row: %w", err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("get turn messages: iterate rows: %w", err)
	}
	return result, nil
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
