package core

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

type ImportState struct {
	JSONLPath  string
	FileSize   int64
	LastOffset int64
	ImportedAt string
}

// execer abstracts *sql.DB and *sql.Tx for shared query methods.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

func OpenDB(dsn string) (*DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite treats each connection to ":memory:" as a separate DB
	// instance, so a shared *sql.DB can otherwise see different in-memory DBs
	// across queries. Pinning to one physical connection avoids that.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSessionsRepoPathColumn(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

// ensureSessionsProjectDirColumnDropped removes the legacy project_dir column
// if it is still present. Required when upgrading from v0.2.x DBs.
// Precondition: the sessions table exists. SQLite 3.35+ required for
// DROP COLUMN.
func ensureSessionsProjectDirColumnDropped(db *sql.DB) error {
	_, present, err := sessionsColumnType(db, "project_dir")
	if err != nil {
		return fmt.Errorf("inspect sessions table: %w", err)
	}
	if !present {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE sessions DROP COLUMN project_dir"); err != nil {
		// Race re-check: if the column is now absent, another instance dropped
		// it between our inspect and ALTER — treat as success. Any other state
		// (re-check failed, or column still present) returns the original
		// ALTER error.
		if _, present2, pErr := sessionsColumnType(db, "project_dir"); pErr == nil && !present2 {
			return nil
		}
		return fmt.Errorf("migrate drop project_dir column: %w", err)
	}
	return nil
}

// ensureSessionsRepoPathColumn adds sessions.repo_path if it is missing.
// Precondition: the sessions table exists.
func ensureSessionsRepoPathColumn(db *sql.DB) error {
	_, present, err := sessionsColumnType(db, "repo_path")
	if err != nil {
		return fmt.Errorf("inspect sessions table: %w", err)
	}
	if present {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE sessions ADD COLUMN repo_path TEXT"); err != nil {
		// Belt-and-suspenders: re-check state rather than match driver-specific
		// error strings. Covers the narrow cross-process race where another
		// instance added the column between our inspect and ALTER. Only treat
		// it as success when the column is actually present now — otherwise
		// surface the original ALTER error.
		if _, present2, pErr := sessionsColumnType(db, "repo_path"); pErr == nil && present2 {
			return nil
		}
		return fmt.Errorf("migrate repo_path column: %w", err)
	}
	return nil
}

// sessionsColumnType looks up a column on the sessions table via PRAGMA
// table_info and returns its declared type. Precondition: the sessions table
// exists. The returned bool reports whether the column was found.
func sessionsColumnType(db *sql.DB, column string) (string, bool, error) {
	rows, err := db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid       int
			name      string
			colType   string
			notnull   int
			dfltValue sql.NullString
			pk        int
		)
		if err := rows.Scan(&cid, &name, &colType, &notnull, &dfltValue, &pk); err != nil {
			return "", false, err
		}
		if name == column {
			return colType, true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, err
	}
	return "", false, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

func (d *DB) UpsertSession(meta SessionMeta, importedAt string) error {
	return upsertSession(d.db, meta, importedAt)
}

func upsertSession(e execer, meta SessionMeta, importedAt string) error {
	_, err := e.Exec(`
		INSERT INTO sessions (session_id, cwd, repo_path, git_branch, version, started_at, ended_at, imported_at)
		VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
		  cwd = COALESCE(NULLIF(excluded.cwd, ''), sessions.cwd),
		  repo_path = COALESCE(NULLIF(excluded.repo_path, ''), sessions.repo_path),
		  git_branch = COALESCE(NULLIF(excluded.git_branch, ''), sessions.git_branch),
		  version = COALESCE(NULLIF(excluded.version, ''), sessions.version),
		  started_at = COALESCE(MIN(sessions.started_at, excluded.started_at), excluded.started_at, sessions.started_at),
		  ended_at = COALESCE(MAX(sessions.ended_at, excluded.ended_at), excluded.ended_at, sessions.ended_at),
		  imported_at = excluded.imported_at`,
		meta.SessionID, meta.CWD, meta.RepoPath, meta.GitBranch,
		meta.Version, meta.StartedAt, meta.EndedAt, importedAt,
	)
	return err
}

func (d *DB) InsertMessage(msg ParsedMessage) error {
	return insertMessage(d.db, msg)
}

func insertMessage(e execer, msg ParsedMessage) error {
	_, err := e.Exec(`
		INSERT OR IGNORE INTO messages (uuid, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.UUID, msg.SessionID, msg.ParentUUID, msg.Role,
		msg.Content, msg.Timestamp, msg.IsSidechain,
	)
	return err
}

func (d *DB) UpdateSessionTitle(sessionID, title, importedAt string) error {
	return updateSessionTitle(d.db, sessionID, title, importedAt)
}

func updateSessionTitle(e execer, sessionID, title, importedAt string) error {
	_, err := e.Exec(`UPDATE sessions SET custom_title = ?, imported_at = ? WHERE session_id = ?`,
		title, importedAt, sessionID,
	)
	return err
}

func (d *DB) UpdateSessionAgentName(sessionID, agentName, importedAt string) error {
	return updateSessionAgentName(d.db, sessionID, agentName, importedAt)
}

func updateSessionAgentName(e execer, sessionID, agentName, importedAt string) error {
	_, err := e.Exec(`UPDATE sessions SET agent_name = ?, imported_at = ? WHERE session_id = ?`,
		agentName, importedAt, sessionID,
	)
	return err
}

func (d *DB) UpsertImportState(state ImportState) error {
	return upsertImportState(d.db, state)
}

func upsertImportState(e execer, state ImportState) error {
	_, err := e.Exec(`
		INSERT INTO import_state (jsonl_path, file_size, last_offset, imported_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(jsonl_path) DO UPDATE SET
		  file_size = excluded.file_size,
		  last_offset = excluded.last_offset,
		  imported_at = excluded.imported_at`,
		state.JSONLPath, state.FileSize, state.LastOffset, state.ImportedAt,
	)
	return err
}

func (d *DB) GetImportState(jsonlPath string) (*ImportState, error) {
	var s ImportState
	err := d.db.QueryRow(
		"SELECT jsonl_path, file_size, last_offset, imported_at FROM import_state WHERE jsonl_path=?",
		jsonlPath,
	).Scan(&s.JSONLPath, &s.FileSize, &s.LastOffset, &s.ImportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

type SessionRow struct {
	SessionID    string
	CWD          string
	RepoPath     string
	StartedAt    string
	EndedAt      string
	CustomTitle  string
	MessageCount int
}

type SessionFilter struct {
	Since   string // RFC3339 UTC string. Empty = no filter.
	Until   string // RFC3339 UTC string. Empty = no filter. Exclusive upper bound.
	Project string // Empty = no filter.
}

func (d *DB) ListSessions(filter SessionFilter) ([]SessionRow, error) {
	query := `
		SELECT s.session_id, COALESCE(s.cwd, ''), COALESCE(s.repo_path, ''), COALESCE(s.started_at, ''), COALESCE(s.ended_at, ''), COALESCE(s.custom_title, ''), COUNT(m.uuid)
		FROM sessions s
		LEFT JOIN messages m ON s.session_id = m.session_id`

	var conditions []string
	var args []any
	if filter.Since != "" {
		conditions = append(conditions, "s.started_at >= ?")
		args = append(args, filter.Since)
	}
	if filter.Until != "" {
		conditions = append(conditions, "s.started_at < ?")
		args = append(args, filter.Until)
	}
	if filter.Project != "" {
		conditions = append(conditions, "COALESCE(s.repo_path, '') LIKE '%' || ? || '%'")
		args = append(args, filter.Project)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY s.session_id ORDER BY s.started_at DESC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []SessionRow{}
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.SessionID, &r.CWD, &r.RepoPath, &r.StartedAt, &r.EndedAt, &r.CustomTitle, &r.MessageCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type ProjectRow struct {
	RepoPath     string
	SessionCount int
}

func (d *DB) ListProjects(filter SessionFilter) ([]ProjectRow, error) {
	query := `SELECT COALESCE(MIN(s.repo_path), ''), COUNT(*)
	FROM sessions s`

	var conditions []string
	var args []any
	if filter.Since != "" {
		conditions = append(conditions, "s.started_at >= ?")
		args = append(args, filter.Since)
	}
	if filter.Until != "" {
		conditions = append(conditions, "s.started_at < ?")
		args = append(args, filter.Until)
	}
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	query += " GROUP BY COALESCE(s.repo_path, '') ORDER BY MAX(s.started_at) DESC"

	rows, err := d.db.Query(query, args...)
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

type MessageRow struct {
	UUID        string
	Role        string
	Content     string
	Timestamp   string
	IsSidechain bool
}

func (d *DB) GetSession(sessionID string) (*SessionRow, error) {
	var r SessionRow
	err := d.db.QueryRow(`
		SELECT s.session_id, COALESCE(s.cwd, ''), COALESCE(s.repo_path, ''), COALESCE(s.started_at, ''), COALESCE(s.ended_at, ''), COALESCE(s.custom_title, ''), COUNT(m.uuid)
		FROM sessions s
		LEFT JOIN messages m ON s.session_id = m.session_id
		WHERE s.session_id = ?
		GROUP BY s.session_id`,
		sessionID,
	).Scan(&r.SessionID, &r.CWD, &r.RepoPath, &r.StartedAt, &r.EndedAt, &r.CustomTitle, &r.MessageCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (d *DB) GetMessages(sessionID string) ([]MessageRow, error) {
	rows, err := d.db.Query(`
		SELECT uuid, role, content, timestamp, is_sidechain
		FROM messages
		WHERE session_id = ?
		ORDER BY timestamp ASC`,
		sessionID,
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
func (d *DB) GetSummaryMessages(sessionID string, limit int, includeClear bool) ([]MessageRow, error) {
	if limit <= 0 {
		return nil, errors.New("limit must be >= 1")
	}

	query := `
		SELECT uuid, role, content, timestamp, is_sidechain
		FROM messages
		WHERE session_id = ?
		  AND role = 'user'
		  AND is_sidechain = 0`
	args := []any{sessionID}
	if !includeClear {
		query += `
		  AND content NOT LIKE ?
		  AND content NOT LIKE ?`
		args = append(args, clearCommandPrefix+"%", localCommandCaveatPrefix+"%")
	}
	query += `
		ORDER BY timestamp ASC
		LIMIT ?`
	args = append(args, limit)

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return scanMessages(rows)
}

func scanMessages(rows *sql.Rows) ([]MessageRow, error) {
	defer rows.Close()

	result := []MessageRow{}
	for rows.Next() {
		var m MessageRow
		if err := rows.Scan(&m.UUID, &m.Role, &m.Content, &m.Timestamp, &m.IsSidechain); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (d *DB) DeleteAll() error {
	for _, table := range []string{"messages", "sessions", "import_state"} {
		if _, err := d.db.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	return nil
}

const schema = `
CREATE TABLE IF NOT EXISTS sessions (
    session_id TEXT PRIMARY KEY,
    cwd TEXT,
    repo_path TEXT,
    git_branch TEXT,
    custom_title TEXT,
    agent_name TEXT,
    version TEXT,
    started_at TEXT,
    ended_at TEXT,
    imported_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
    uuid TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES sessions(session_id),
    parent_uuid TEXT,
    role TEXT NOT NULL,
    content TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    is_sidechain BOOLEAN DEFAULT FALSE,
    UNIQUE(uuid)
);

CREATE TABLE IF NOT EXISTS import_state (
    jsonl_path TEXT PRIMARY KEY,
    file_size INTEGER,
    last_offset INTEGER,
    imported_at TEXT NOT NULL
);
`
