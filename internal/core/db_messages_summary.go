package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// MessageRow carries the original block and physical provenance of a body row.
type MessageRow struct {
	UUID        string
	Role        string
	Content     string
	Timestamp   string
	Blocks      []string
	Number      int
	OriginPath  string
	OriginLine  int
	PayloadID   string
	Provenance  string
	LegacyRowID int64
}

// GetMessages returns source body messages by canonical number, with rowid
// as the order for sources that have no numbered stream.
func (d *DB) GetMessages(inputID int64, source Source, sessionID string) ([]MessageRow, error) {
	return d.GetIdentityMessages(inputID, source, rootIdentity(sessionID))
}

func (d *DB) GetIdentityMessages(inputID int64, source Source, identity string) ([]MessageRow, error) {
	return d.GetSessionMessagesFiltered(SessionRow{InputID: inputID, Source: source, Identity: identity}, MessageFilter{}, nil, 0)
}

type MessageFilter struct {
	Role                 string
	MinNumber, MaxNumber int
}

func messageFilterSQL(filter MessageFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.Role != "" {
		clauses = append(clauses, "role = ?")
		args = append(args, filter.Role)
	}
	if filter.MinNumber > 0 {
		clauses = append(clauses, "number >= ?")
		args = append(args, filter.MinNumber)
	}
	if filter.MaxNumber > 0 {
		clauses = append(clauses, "number <= ?")
		args = append(args, filter.MaxNumber)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func (d *DB) CountSessionMessages(session SessionRow, filter MessageFilter) (int, error) {
	condition, filterArgs := messageFilterSQL(filter)
	var query string
	args := []any{}
	if session.InputID == LegacyInputID {
		key, source, identity, err := parseREF(session.REF)
		if err != nil {
			return 0, err
		}

		query = `SELECT count(*) FROM legacy_messages WHERE snapshot_sha256=? AND source=? AND session_id=? AND (source IN ('codex','claude_code') OR COALESCE(is_sidechain,0)=0)`
		args = append(args, strings.TrimPrefix(key, "legacy:"), source, identity)
	} else {
		query = `SELECT count(*) FROM messages WHERE input_id=? AND source=? AND identity=? AND membership='body' AND (source IN ('codex','claude_code') OR is_sidechain=0)`
		args = append(args, session.InputID, session.Source, session.Identity)
	}
	args = append(args, filterArgs...)
	var count int
	if err := d.execer().QueryRow(query+condition, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count messages: %w", err)
	}
	return count, nil
}

func (d *DB) GetSessionMessagesFiltered(session SessionRow, filter MessageFilter, limit *int, offset int) ([]MessageRow, error) {
	condition, filterArgs := messageFilterSQL(filter)
	var query string
	args := []any{}
	operation := "get messages"
	if session.InputID == LegacyInputID {
		key, source, identity, err := parseREF(session.REF)
		if err != nil {
			return nil, err
		}

		query = `SELECT uuid,role,content,COALESCE(timestamp,''),'null',number,'',0,'','legacy_saved',legacy_rowid FROM legacy_messages WHERE snapshot_sha256=? AND source=? AND session_id=? AND (source IN ('codex','claude_code') OR COALESCE(is_sidechain,0)=0)`
		args = append(args, strings.TrimPrefix(key, "legacy:"), source, identity)
		query += condition + ` ORDER BY number`
		operation = "get legacy messages"
	} else {
		query = `SELECT uuid, role, content, timestamp, blocks_json, number, origin_path, origin_line, payload_id, 'source_record', 0 FROM messages WHERE input_id=? AND source=? AND identity=? AND membership='body' AND (source IN ('codex','claude_code') OR is_sidechain=0)`
		args = append(args, session.InputID, session.Source, session.Identity)
		query += condition + ` ORDER BY CASE WHEN number > 0 THEN 0 ELSE 1 END, number, rowid`
	}
	args = append(args, filterArgs...)
	if limit != nil {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, *limit, offset)
	} else if offset > 0 {
		query += ` LIMIT -1 OFFSET ?`
		args = append(args, offset)
	}
	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: query: %w", operation, err)
	}
	return scanMessages(rows, operation)
}

func scanMessages(rows *sql.Rows, operation string) ([]MessageRow, error) {
	defer rows.Close()

	result := []MessageRow{}
	for rows.Next() {
		var m MessageRow
		var blocksJSON string
		if err := rows.Scan(&m.UUID, &m.Role, &m.Content, &m.Timestamp, &blocksJSON, &m.Number, &m.OriginPath, &m.OriginLine, &m.PayloadID, &m.Provenance, &m.LegacyRowID); err != nil {
			return nil, fmt.Errorf("%s: scan row: %w", operation, err)
		}
		if err := json.Unmarshal([]byte(blocksJSON), &m.Blocks); err != nil {
			return nil, fmt.Errorf("%s: decode blocks: %w", operation, err)
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: iterate rows: %w", operation, err)
	}
	return result, nil
}
