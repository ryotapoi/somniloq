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

// SearchMessages returns non-sidechain messages whose content contains the
// query, newest first. Matching uses SQLite LIKE: ASCII-only
// case-insensitivity, and `%`/`_` in the query act as wildcards (the same
// known limitation as the --project filter). filter.Since/Until apply to the
// message timestamp, not the session start, because the search target is the
// message. rowid breaks timestamp ties like GetMessages, inverted to follow
// the DESC order.
func (d *DB) SearchMessages(filter SessionFilter, query string) ([]SearchRow, error) {
	q := `
		SELECT m.source, m.uuid, m.session_id, COALESCE(s.repo_path, ''), m.timestamp, m.content
		FROM messages m
		JOIN sessions s ON m.source = s.source AND m.session_id = s.session_id
		WHERE m.is_sidechain = 0
		  AND m.content LIKE '%' || ? || '%'`
	args := []any{query}
	conditions, filterArgs := sessionFilterConditions(filter, messageTimestampColumn)
	if len(conditions) > 0 {
		q += " AND " + strings.Join(conditions, " AND ")
		args = append(args, filterArgs...)
	}
	q += " ORDER BY m.timestamp DESC, m.rowid DESC"

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
