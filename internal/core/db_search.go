package core

import (
	"fmt"
	"strings"
)

// SearchRow is one message that matched a search query.
type SearchRow struct {
	Source    Source
	UUID      string
	SessionID string
	RepoPath  string
	Timestamp string
	Content   string
}

// SearchPagination narrows an ordered search result. A zero Limit leaves the
// result unlimited; Offset skips that many ordered rows.
type SearchPagination struct {
	Limit  int
	Offset int
}

// SearchMessages returns non-sidechain messages whose content contains the
// query, newest first. Matching uses SQLite LIKE with literal query text and
// ASCII-only case-insensitivity. filter.Since/Until apply to the
// message timestamp, not the session start, because the search target is the
// message. rowid breaks timestamp ties like GetMessages, inverted to follow
// the DESC order.
func (d *DB) SearchMessages(filter SessionFilter, query string, pagination SearchPagination) ([]SearchRow, error) {
	q := `
		SELECT m.source, m.uuid, m.session_id, COALESCE(s.repo_path, ''), m.timestamp, m.content
		FROM messages m
		JOIN sessions s ON m.source = s.source AND m.session_id = s.session_id
		WHERE m.is_sidechain = 0
		  AND m.content LIKE '%' || ? || '%' ESCAPE '\'`
	args := []any{escapeLikeLiteral(query)}
	conditions, filterArgs := sessionFilterConditions(filter, messageTimestampColumn)
	if len(conditions) > 0 {
		q += " AND " + strings.Join(conditions, " AND ")
		args = append(args, filterArgs...)
	}
	q += " ORDER BY rfc3339_utc_nanos(m.timestamp) DESC, m.rowid DESC"
	if pagination.Limit > 0 {
		q += " LIMIT ? OFFSET ?"
		args = append(args, pagination.Limit, pagination.Offset)
	} else if pagination.Offset > 0 {
		// SQLite requires LIMIT when OFFSET is present. -1 means no limit.
		q += " LIMIT -1 OFFSET ?"
		args = append(args, pagination.Offset)
	}

	rows, err := d.execer().Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("search messages: query: %w", err)
	}
	defer rows.Close()

	result := []SearchRow{}
	for rows.Next() {
		var r SearchRow
		var src string
		if err := rows.Scan(&src, &r.UUID, &r.SessionID, &r.RepoPath, &r.Timestamp, &r.Content); err != nil {
			return nil, fmt.Errorf("search messages: scan row: %w", err)
		}
		r.Source = Source(src)
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search messages: iterate rows: %w", err)
	}
	return result, nil
}
