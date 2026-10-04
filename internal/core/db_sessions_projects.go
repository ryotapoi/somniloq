package core

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type SessionRow struct {
	InputID         int64
	REF             string
	Source          Source
	Identity        string
	RootIdentity    string
	RootREF         string
	SessionID       string
	ParentSessionID string
	ParentIdentity  string
	ParentREF       string
	CWD             string
	RepoPath        string
	StartedAt       string
	EndedAt         string
	CustomTitle     string
	MessageCount    int
	// BodySize is the total content size in bytes (UTF-8, not runes) of the
	// session's body messages: approximately what `show` would
	// print, excluding the Markdown headers show adds.
	BodySize int
}

type SessionFilter struct {
	Since         string // RFC3339 UTC string. Empty = no filter.
	Until         string // RFC3339 UTC string. Empty = no filter. Exclusive upper bound.
	ImportedSince string // RFC3339 UTC string. Empty = no filter. Inclusive lower bound.
	// Projects holds repo_path literal substrings; a row matches when ANY
	// pattern matches (project aliases expand one --project value into the
	// whole alias group). Empty = no filter.
	Projects []string
}

// Keep these selected columns in the same order as scanSessionRow. The
// body-size sum counts bytes (OCTET_LENGTH; LENGTH on TEXT would count
// characters) and counts only body rows, matching show and search.
const sessionRowSelect = `
	SELECT s.input_id, i.input_key, s.source, s.session_id, s.identity, s.root_identity, s.parent_session_id, s.parent_identity,
	       COALESCE(p.source, ''), COALESCE(p.identity, ''), COALESCE(r.source, ''), COALESCE(r.identity, ''), COALESCE(s.cwd, ''), COALESCE(s.repo_path, ''),
	       COALESCE(s.started_at, ''), COALESCE(s.ended_at, ''), COALESCE(s.custom_title, ''),
	       COUNT(m.uuid) FILTER (WHERE m.membership = 'body'),
	       COALESCE(SUM(OCTET_LENGTH(m.content)) FILTER (WHERE m.membership = 'body' AND (m.source IN ('codex','claude_code') OR m.is_sidechain = 0)), 0)
	FROM sessions s
 JOIN inputs i ON s.input_id=i.id
	LEFT JOIN sessions p ON p.input_id=s.input_id AND p.source=s.source AND p.identity=s.parent_identity
	LEFT JOIN sessions r ON r.input_id=s.input_id AND r.source=s.source AND r.identity=s.root_identity
 LEFT JOIN messages m ON s.input_id=m.input_id AND s.source = m.source AND s.identity = m.identity`

// rowScanner abstracts *sql.Row and *sql.Rows so scanSessionRow serves both
// single-row and multi-row queries.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanSessionRow(row rowScanner) (SessionRow, error) {
	var r SessionRow
	var inputKey string
	var parentSource, parentIdentity, rootSource, rootID string
	if err := row.Scan(
		&r.InputID,
		&inputKey,
		&r.Source,
		&r.SessionID,
		&r.Identity,
		&r.RootIdentity,
		&r.ParentSessionID,
		&r.ParentIdentity,
		&parentSource,
		&parentIdentity,
		&rootSource,
		&rootID,
		&r.CWD,
		&r.RepoPath,
		&r.StartedAt,
		&r.EndedAt,
		&r.CustomTitle,
		&r.MessageCount,
		&r.BodySize,
	); err != nil {
		return SessionRow{}, err
	}
	r.REF = savedREF(inputKey, r.Source, r.Identity)
	if parentIdentity != "" {
		r.ParentREF = IdentityREF(inputKey, Source(parentSource), parentIdentity)
	}
	if rootID != "" {
		r.RootREF = IdentityREF(inputKey, Source(rootSource), rootID)
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
// only callers; user-provided values remain query parameters. Both operands are
// normalized so RFC3339 timestamps with variable fractional-second widths sort
// by instant rather than text representation.
func timeFilterConditions(filter SessionFilter, column timestampColumn) (conditions []string, args []any) {
	if filter.Since != "" || filter.Until != "" {
		// Unknown source timestamps are stored as NULL or empty strings. They
		// remain visible without a time filter, but must not match a range.
		conditions = append(conditions, string(column)+" <> ''")
	}
	if filter.Since != "" {
		conditions = append(conditions, "rfc3339_utc_nanos("+string(column)+") >= rfc3339_utc_nanos(?)")
		args = append(args, filter.Since)
	}
	if filter.Until != "" {
		conditions = append(conditions, "rfc3339_utc_nanos("+string(column)+") < rfc3339_utc_nanos(?)")
		args = append(args, filter.Until)
	}
	return conditions, args
}

// escapeLikeLiteral makes a value safe for SQLite LIKE with a backslash ESCAPE
// clause while preserving it as a literal substring.
func escapeLikeLiteral(value string) string {
	return strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(value)
}

// projectsCondition builds the repo_path literal substring condition for
// filter.Projects: one LIKE per value, OR-joined so any alias-group name
// matches. Returns "" when no patterns are given.
func projectsCondition(projects []string) (condition string, args []any) {
	if len(projects) == 0 {
		return "", nil
	}
	likes := make([]string, len(projects))
	for i, p := range projects {
		likes[i] = "s.repo_path LIKE '%' || ? || '%' ESCAPE '\\'"
		args = append(args, escapeLikeLiteral(p))
	}
	return "s.repo_path <> '' AND (" + strings.Join(likes, " OR ") + ")", args
}

// sessionFilterConditions composes the supported SessionFilter conditions in
// their stable parameter order. Callers select the timestamp for their primary
// record: sessions use started_at, while search uses message timestamp (ADR
// 0013).
func sessionFilterConditions(filter SessionFilter, column timestampColumn) (conditions []string, args []any) {
	conditions, args = timeFilterConditions(filter, column)
	if filter.ImportedSince != "" {
		conditions = append(conditions, "s.imported_at >= ?")
		args = append(args, filter.ImportedSince)
	}
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

	query += " GROUP BY s.input_id, s.source, s.identity ORDER BY rfc3339_utc_nanos(s.started_at) DESC"

	rows, err := d.execer().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list sessions: query: %w", err)
	}
	result, err := scanSessionRows(rows, "list sessions")
	if err != nil {
		return nil, err
	}
	query = legacySessionRowSelect
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " GROUP BY s.snapshot_sha256,s.source,s.session_id"
	rows, err = d.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	legacy, err := scanSessionRows(rows, "list legacy sessions")
	result = append(result, legacy...)
	sort.SliceStable(result, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, result[i].StartedAt)
		right, _ := time.Parse(time.RFC3339Nano, result[j].StartedAt)
		return left.After(right)
	})
	return result, err
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
	FROM (SELECT repo_path,started_at FROM sessions UNION ALL SELECT repo_path,started_at FROM legacy_sessions) s`

	conditions, args := timeFilterConditions(filter, sessionStartedAtColumn)
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY COALESCE(s.repo_path, '') ORDER BY MAX(rfc3339_utc_nanos(s.started_at)) DESC"

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

func (d *DB) GetSession(inputID int64, source Source, sessionID string) (*SessionRow, error) {
	if inputID == LegacyInputID {
		matches, err := d.LookupSessionsByID(sessionID)
		if err != nil {
			return nil, err
		}
		for _, r := range matches {
			if r.InputID == LegacyInputID && r.Source == source {
				return &r, nil
			}
		}
		return nil, nil
	}
	row := d.execer().QueryRow(sessionRowSelect+`
		WHERE s.input_id = ? AND s.source = ? AND s.identity = ?
		GROUP BY s.input_id, s.source, s.identity`,
		inputID, string(source), rootIdentity(sessionID),
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
		GROUP BY s.input_id, s.source, s.identity
		ORDER BY s.source ASC`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("lookup sessions by ID: query: %w", err)
	}
	result, err := scanSessionRows(rows, "lookup sessions by ID")
	if err != nil {
		return nil, err
	}
	rows, err = d.execer().Query(legacySessionRowSelect+` WHERE s.session_id=? GROUP BY s.snapshot_sha256,s.source,s.session_id`, sessionID)
	if err != nil {
		return nil, err
	}
	legacy, err := scanSessionRows(rows, "lookup legacy sessions by ID")
	return append(result, legacy...), err
}

func (d *DB) LookupSessionREF(ref string) (*SessionRow, error) {
	key, source, id, err := parseREF(ref)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(key, "legacy:") {
		row := d.execer().QueryRow(legacySessionRowSelect+` WHERE s.snapshot_sha256=? AND s.source=? AND s.session_id=? GROUP BY s.snapshot_sha256,s.source,s.session_id`, strings.TrimPrefix(key, "legacy:"), source, id)
		r, e := scanSessionRow(row)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, nil
		}
		if e != nil {
			return nil, e
		}
		return &r, nil
	}
	row := d.execer().QueryRow(sessionRowSelect+` WHERE i.input_key=? AND s.source=? AND s.identity=? GROUP BY s.input_id,s.source,s.identity`, key, source, id)
	r, err := scanSessionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}
