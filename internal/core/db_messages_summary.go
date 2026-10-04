package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// MessageRow carries the original block and physical provenance of a body row.
type MessageRow struct {
	UUID       string
	Role       string
	Content    string
	Timestamp  string
	Blocks     []string
	Number     int
	OriginPath string
	OriginLine int
	PayloadID  string
}

// GetMessages returns body messages by canonical number, with rowid as the
// order for sources that have no numbered stream.
func (d *DB) GetMessages(inputID int64, source Source, sessionID string) ([]MessageRow, error) {
	return d.GetIdentityMessages(inputID, source, rootIdentity(sessionID))
}

func (d *DB) GetIdentityMessages(inputID int64, source Source, identity string) ([]MessageRow, error) {
	rows, err := d.execer().Query(`
		SELECT uuid, role, content, timestamp, blocks_json, number, origin_path, origin_line, payload_id
		FROM messages
		WHERE input_id = ? AND source = ? AND identity = ?
		  AND membership = 'body'
		  AND (source IN ('codex','claude_code') OR is_sidechain = 0)
		ORDER BY CASE WHEN number > 0 THEN 0 ELSE 1 END, number, rowid`,
		inputID, string(source), identity,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages: query: %w", err)
	}
	return scanMessages(rows, "get messages")
}

// GetTurnMessages returns only UUID and role in the same order as GetMessages.
func (d *DB) GetTurnMessages(inputID int64, source Source, sessionID string) ([]MessageRow, error) {
	return d.GetIdentityTurnMessages(inputID, source, rootIdentity(sessionID))
}

func (d *DB) GetIdentityTurnMessages(inputID int64, source Source, identity string) ([]MessageRow, error) {
	rows, err := d.execer().Query(`
		SELECT uuid, role
		FROM messages
		WHERE input_id = ? AND source = ? AND identity = ?
		  AND membership = 'body'
		  AND (source IN ('codex','claude_code') OR is_sidechain = 0)
		ORDER BY CASE WHEN number > 0 THEN 0 ELSE 1 END, number, rowid`,
		inputID, string(source), identity,
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
		var blocksJSON string
		if err := rows.Scan(&m.UUID, &m.Role, &m.Content, &m.Timestamp, &blocksJSON, &m.Number, &m.OriginPath, &m.OriginLine, &m.PayloadID); err != nil {
			return nil, fmt.Errorf("%s: scan row: %w", operation, err)
		}
		if err := json.Unmarshal([]byte(blocksJSON), &m.Blocks); err != nil {
			return nil, fmt.Errorf("%s: decode blocks: %w", operation, err)
		}
		if len(m.Blocks) == 0 {
			m.Blocks = nil
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: iterate rows: %w", operation, err)
	}
	return result, nil
}
