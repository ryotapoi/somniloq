package core

import (
	"fmt"
	"strings"
)

// SearchRow is one message that matched a search query.
type SearchRow struct {
	InputID   int64
	REF       string
	Source    Source
	UUID      string
	Identity  string
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

// SearchMessages returns body messages whose content contains the
// query, newest first. Matching uses SQLite LIKE with literal query text and
// ASCII-only case-insensitivity. filter.Since/Until apply to the
// message timestamp, not the session start, because the search target is the
// message. rowid breaks timestamp ties like GetMessages, inverted to follow
// the DESC order.
func (d *DB) SearchMessages(filter SessionFilter, query string, pagination SearchPagination) ([]SearchRow, error) {
	q := `
 SELECT m.input_id, m.input_key, m.source, m.uuid, m.session_id, m.identity, COALESCE(s.repo_path,''), COALESCE(m.timestamp,''), m.content
 FROM (
 SELECT m.*,i.input_key,m.rowid AS saved_rowid FROM messages m JOIN inputs i ON m.input_id=i.id
 UNION ALL
 SELECT -1,uuid,source,session_id,session_id,parent_uuid,role,content,COALESCE(blocks_json,'null'),timestamp,is_sidechain,number,'',0,'body','', 'legacy:' || snapshot_sha256,legacy_rowid FROM legacy_messages
 ) m JOIN (
 SELECT input_id,source,identity,repo_path,imported_at FROM sessions
 UNION ALL SELECT -1,source,session_id,repo_path,imported_at FROM legacy_sessions
 ) s ON m.input_id=s.input_id AND m.source=s.source AND m.identity=s.identity
 WHERE m.membership='body' AND (m.source IN ('codex','claude_code') OR COALESCE(m.is_sidechain,0)=0)
 AND m.content LIKE '%' || ? || '%' ESCAPE '\'`
	args := []any{escapeLikeLiteral(query)}
	conditions, filterArgs := sessionFilterConditions(filter, messageTimestampColumn)
	if len(conditions) > 0 {
		q += " AND " + strings.Join(conditions, " AND ")
		args = append(args, filterArgs...)
	}
	q += " ORDER BY rfc3339_utc_nanos(m.timestamp) DESC, m.saved_rowid DESC"
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
		var src, inputKey string
		if err := rows.Scan(&r.InputID, &inputKey, &src, &r.UUID, &r.SessionID, &r.Identity, &r.RepoPath, &r.Timestamp, &r.Content); err != nil {
			return nil, fmt.Errorf("search messages: scan row: %w", err)
		}
		r.Source = Source(src)
		r.REF = savedREF(inputKey, r.Source, r.Identity)
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search messages: iterate rows: %w", err)
	}
	return result, nil
}
