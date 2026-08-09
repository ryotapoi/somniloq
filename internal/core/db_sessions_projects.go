package core

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

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

func scanSessionRows(rows *sql.Rows, operation string) ([]SessionRow, error) {
	defer rows.Close()

	result := []SessionRow{}
	for rows.Next() {
		r, err := scanSessionRow(rows)
		if err != nil {
			return nil, fmt.Errorf("%s: scan row: %w", operation, err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: iterate rows: %w", operation, err)
	}
	return result, nil
}

type timestampColumn string

const (
	sessionStartedAtColumn timestampColumn = "s.started_at"
	messageTimestampColumn timestampColumn = "m.timestamp"
)

// timeFilterConditions returns range conditions for filter.Since / filter.Until
// on a trusted internal timestamp column. The column constants above are the
// only callers; user-provided values remain query parameters. Its lexical
// comparisons rely on cmd/somniloq/filter.go emitting three-digit UTC filter
// boundaries that remain ordered with source JSONL timestamps stored in
// sessions.started_at and messages.timestamp at RFC3339 second-or-finer
// precision (for example, …:05Z >= …:05.000Z).
func timeFilterConditions(filter SessionFilter, column timestampColumn) (conditions []string, args []any) {
	if filter.Since != "" {
		conditions = append(conditions, string(column)+" >= ?")
		args = append(args, filter.Since)
	}
	if filter.Until != "" {
		conditions = append(conditions, string(column)+" < ?")
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

// sessionFilterConditions composes the supported SessionFilter conditions in
// their stable parameter order. Callers select the timestamp for their primary
// record: sessions use started_at, while search uses message timestamp (ADR
// 0013).
func sessionFilterConditions(filter SessionFilter, column timestampColumn) (conditions []string, args []any) {
	conditions, args = timeFilterConditions(filter, column)
	if cond, condArgs := projectsCondition(filter.Projects); cond != "" {
		conditions = append(conditions, cond)
		args = append(args, condArgs...)
	}
	return conditions, args
}

func (d *DB) ListSessions(filter SessionFilter) ([]SessionRow, error) {
	query := sessionRowSelect

	conditions, args := sessionFilterConditions(filter, sessionStartedAtColumn)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY s.source, s.session_id ORDER BY s.started_at DESC"

	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: query: %w", err)
	}
	return scanSessionRows(rows, "list sessions")
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

	conditions, args := timeFilterConditions(filter, sessionStartedAtColumn)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY COALESCE(s.repo_path, '') ORDER BY MAX(s.started_at) DESC"

	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list projects: query: %w", err)
	}
	defer rows.Close()

	result := []ProjectRow{}
	for rows.Next() {
		var r ProjectRow
		if err := rows.Scan(&r.RepoPath, &r.SessionCount); err != nil {
			return nil, fmt.Errorf("list projects: scan row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list projects: iterate rows: %w", err)
	}
	return result, nil
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
		return nil, fmt.Errorf("get session: scan row: %w", err)
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
		return nil, fmt.Errorf("lookup sessions by ID: query: %w", err)
	}
	return scanSessionRows(rows, "lookup sessions by ID")
}
