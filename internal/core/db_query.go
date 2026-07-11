package core

import (
	"database/sql"
	"errors"
	"strings"
)

func (d *DB) GetImportState(jsonlPath string) (*ImportState, error) {
	var s ImportState
	var src string
	err := d.execer().QueryRow(
		"SELECT jsonl_path, source, file_size, last_offset, imported_at FROM import_state WHERE jsonl_path=?",
		jsonlPath,
	).Scan(&s.JSONLPath, &src, &s.FileSize, &s.LastOffset, &s.ImportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.Source = Source(src)
	return &s, nil
}

type SessionRow struct {
	Source       Source
	SessionID    string
	CWD          string
	RepoPath     string
	StartedAt    string
	EndedAt      string
	CustomTitle  string
	MessageCount int
	// BodySize is the total content size in bytes (UTF-8, not runes) of the
	// session's non-sidechain messages: approximately what `show` would
	// print, excluding the Markdown headers show adds.
	BodySize int
}

type SessionFilter struct {
	Since string // RFC3339 UTC string. Empty = no filter.
	Until string // RFC3339 UTC string. Empty = no filter. Exclusive upper bound.
	// Projects holds repo_path substring patterns; a row matches when ANY
	// pattern matches (project aliases expand one --project value into the
	// whole alias group). Empty = no filter.
	Projects []string
}

// The body-size sum counts bytes (OCTET_LENGTH; LENGTH on TEXT would count
// characters) and skips sidechain rows so the value predicts what `show`
// prints. MessageCount keeps counting every row.
type sessionRowColumn struct {
	selectExpr string
	scanTarget func(*SessionRow) any
}

// sessionRowColumns is the single definition of the column order for every
// query that produces SessionRow values. sessionRowSelect and scanSessionRow
// are both derived from it, so their positions cannot drift apart.
var sessionRowColumns = []sessionRowColumn{
	{"s.source", func(r *SessionRow) any { return &r.Source }},
	{"s.session_id", func(r *SessionRow) any { return &r.SessionID }},
	{"COALESCE(s.cwd, '')", func(r *SessionRow) any { return &r.CWD }},
	{"COALESCE(s.repo_path, '')", func(r *SessionRow) any { return &r.RepoPath }},
	{"COALESCE(s.started_at, '')", func(r *SessionRow) any { return &r.StartedAt }},
	{"COALESCE(s.ended_at, '')", func(r *SessionRow) any { return &r.EndedAt }},
	{"COALESCE(s.custom_title, '')", func(r *SessionRow) any { return &r.CustomTitle }},
	{"COUNT(m.uuid)", func(r *SessionRow) any { return &r.MessageCount }},
	{"COALESCE(SUM(OCTET_LENGTH(m.content)) FILTER (WHERE m.is_sidechain = 0), 0)", func(r *SessionRow) any { return &r.BodySize }},
}

var sessionRowSelect = `
	SELECT ` + strings.Join(sessionRowSelectExpressions(), ", ") + `
	FROM sessions s
	LEFT JOIN messages m ON s.source = m.source AND s.session_id = m.session_id`

func sessionRowSelectExpressions() []string {
	expressions := make([]string, len(sessionRowColumns))
	for i, column := range sessionRowColumns {
		expressions[i] = column.selectExpr
	}
	return expressions
}

// rowScanner abstracts *sql.Row and *sql.Rows so scanSessionRow serves both
// single-row and multi-row queries.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSessionRow(row rowScanner) (SessionRow, error) {
	var r SessionRow
	targets := make([]any, len(sessionRowColumns))
	for i, column := range sessionRowColumns {
		targets[i] = column.scanTarget(&r)
	}
	if err := row.Scan(targets...); err != nil {
		return SessionRow{}, err
	}
	return r, nil
}

func scanSessionRows(rows *sql.Rows) ([]SessionRow, error) {
	defer rows.Close()

	result := []SessionRow{}
	for rows.Next() {
		r, err := scanSessionRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// timeFilterConditions returns started_at range conditions for filter.Since /
// filter.Until. filter.Projects is intentionally not handled here: only
// ListSessions and SearchMessages support it (via projectsCondition).
func timeFilterConditions(filter SessionFilter) (conditions []string, args []any) {
	if filter.Since != "" {
		conditions = append(conditions, "s.started_at >= ?")
		args = append(args, filter.Since)
	}
	if filter.Until != "" {
		conditions = append(conditions, "s.started_at < ?")
		args = append(args, filter.Until)
	}
	return conditions, args
}

// projectsCondition builds the repo_path substring condition for
// filter.Projects: one LIKE per pattern, OR-joined so any alias-group name
// matches. Returns "" when no patterns are given.
func projectsCondition(projects []string) (condition string, args []any) {
	if len(projects) == 0 {
		return "", nil
	}
	likes := make([]string, len(projects))
	for i, p := range projects {
		likes[i] = "COALESCE(s.repo_path, '') LIKE '%' || ? || '%'"
		args = append(args, p)
	}
	return "(" + strings.Join(likes, " OR ") + ")", args
}

func (d *DB) ListSessions(filter SessionFilter) ([]SessionRow, error) {
	query := sessionRowSelect

	conditions, args := timeFilterConditions(filter)
	if cond, condArgs := projectsCondition(filter.Projects); cond != "" {
		conditions = append(conditions, cond)
		args = append(args, condArgs...)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY s.source, s.session_id ORDER BY s.started_at DESC"

	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanSessionRows(rows)
}

type ProjectRow struct {
	RepoPath     string
	SessionCount int
}

// ListProjects deliberately ignores filter.Projects and returns raw repo_path
// groups. The CLI applies project-alias display normalization and merges rows
// that collapse to the same canonical display name.
func (d *DB) ListProjects(filter SessionFilter) ([]ProjectRow, error) {
	query := `SELECT COALESCE(MIN(s.repo_path), ''), COUNT(*)
	FROM sessions s`

	conditions, args := timeFilterConditions(filter)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY COALESCE(s.repo_path, '') ORDER BY MAX(s.started_at) DESC"

	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []ProjectRow{}
	for rows.Next() {
		var r ProjectRow
		if err := rows.Scan(&r.RepoPath, &r.SessionCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// MessageRow deliberately has no IsSidechain field: every query that
// produces it excludes sidechain rows in SQL, so the value would always be
// false.
type MessageRow struct {
	UUID      string
	Role      string
	Content   string
	Timestamp string
}

func (d *DB) GetSession(source Source, sessionID string) (*SessionRow, error) {
	row := d.execer().QueryRow(sessionRowSelect+`
		WHERE s.source = ? AND s.session_id = ?
		GROUP BY s.source, s.session_id`,
		string(source), sessionID,
	)
	r, err := scanSessionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) LookupSessionsByID(sessionID string) ([]SessionRow, error) {
	rows, err := d.execer().Query(sessionRowSelect+`
		WHERE s.session_id = ?
		GROUP BY s.source, s.session_id
		ORDER BY s.source ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	return scanSessionRows(rows)
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
		return nil, err
	}
	return scanMessages(rows)
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
		return nil, err
	}
	return scanMessages(rows)
}

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
	if filter.Since != "" {
		q += " AND m.timestamp >= ?"
		args = append(args, filter.Since)
	}
	if filter.Until != "" {
		q += " AND m.timestamp < ?"
		args = append(args, filter.Until)
	}
	if cond, condArgs := projectsCondition(filter.Projects); cond != "" {
		q += " AND " + cond
		args = append(args, condArgs...)
	}
	q += " ORDER BY m.timestamp DESC, m.rowid DESC"

	rows, err := d.execer().Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []SearchRow{}
	for rows.Next() {
		var r SearchRow
		var src string
		if err := rows.Scan(&src, &r.UUID, &r.SessionID, &r.RepoPath, &r.Timestamp, &r.Content); err != nil {
			return nil, err
		}
		r.Source = Source(src)
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func scanMessages(rows *sql.Rows) ([]MessageRow, error) {
	defer rows.Close()

	result := []MessageRow{}
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.UUID, &m.Role, &m.Content, &m.Timestamp); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
