package core

import (
	"database/sql"
	"errors"
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

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
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
		INSERT INTO sessions (session_id, project_dir, cwd, git_branch, version, started_at, ended_at, imported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(session_id) DO UPDATE SET
		  cwd = COALESCE(NULLIF(excluded.cwd, ''), sessions.cwd),
		  git_branch = COALESCE(NULLIF(excluded.git_branch, ''), sessions.git_branch),
		  version = COALESCE(NULLIF(excluded.version, ''), sessions.version),
		  started_at = COALESCE(MIN(sessions.started_at, excluded.started_at), excluded.started_at, sessions.started_at),
		  ended_at = COALESCE(MAX(sessions.ended_at, excluded.ended_at), excluded.ended_at, sessions.ended_at),
		  imported_at = excluded.imported_at`,
		meta.SessionID, meta.ProjectDir, meta.CWD, meta.GitBranch,
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

func (d *DB) UpdateSessionTitle(sessionID, projectDir, title, importedAt string) error {
	return updateSessionTitle(d.db, sessionID, projectDir, title, importedAt)
}

func updateSessionTitle(e execer, sessionID, projectDir, title, importedAt string) error {
	_, err := e.Exec(`
		INSERT INTO sessions (session_id, project_dir, imported_at) VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO NOTHING`,
		sessionID, projectDir, importedAt,
	)
	if err != nil {
		return err
	}
	_, err = e.Exec(`UPDATE sessions SET custom_title = ?, imported_at = ? WHERE session_id = ?`,
		title, importedAt, sessionID,
	)
	return err
}

func (d *DB) UpdateSessionAgentName(sessionID, projectDir, agentName, importedAt string) error {
	return updateSessionAgentName(d.db, sessionID, projectDir, agentName, importedAt)
}

func updateSessionAgentName(e execer, sessionID, projectDir, agentName, importedAt string) error {
	_, err := e.Exec(`
		INSERT INTO sessions (session_id, project_dir, imported_at) VALUES (?, ?, ?)
		ON CONFLICT(session_id) DO NOTHING`,
		sessionID, projectDir, importedAt,
	)
	if err != nil {
		return err
	}
	_, err = e.Exec(`UPDATE sessions SET agent_name = ?, imported_at = ? WHERE session_id = ?`,
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
	ProjectDir   string
	CWD          string
	StartedAt    string
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
		SELECT s.session_id, s.project_dir, COALESCE(s.cwd, ''), COALESCE(s.started_at, ''), COALESCE(s.custom_title, ''), COUNT(m.uuid)
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
		conditions = append(conditions, "s.project_dir LIKE '%' || ? || '%'")
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
		if err := rows.Scan(&r.SessionID, &r.ProjectDir, &r.CWD, &r.StartedAt, &r.CustomTitle, &r.MessageCount); err != nil {
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
	ProjectDir   string
	SessionCount int
}

func (d *DB) ListProjects(filter SessionFilter) ([]ProjectRow, error) {
	query := `SELECT s.project_dir, COUNT(*) FROM sessions s`

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

	query += " GROUP BY s.project_dir ORDER BY MAX(s.started_at) DESC"

	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []ProjectRow{}
	for rows.Next() {
		var r ProjectRow
		if err := rows.Scan(&r.ProjectDir, &r.SessionCount); err != nil {
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
		SELECT s.session_id, s.project_dir, COALESCE(s.cwd, ''), COALESCE(s.started_at, ''), COALESCE(s.custom_title, ''), COUNT(m.uuid)
		FROM sessions s
		LEFT JOIN messages m ON s.session_id = m.session_id
		WHERE s.session_id = ?
		GROUP BY s.session_id`,
		sessionID,
	).Scan(&r.SessionID, &r.ProjectDir, &r.CWD, &r.StartedAt, &r.CustomTitle, &r.MessageCount)
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

func (d *DB) GetSummaryMessages(sessionID string) ([]MessageRow, error) {
	rows, err := d.db.Query(`
		SELECT uuid, role, content, timestamp, is_sidechain
		FROM messages
		WHERE session_id = ? AND role = 'user' AND is_sidechain = 0
		ORDER BY timestamp ASC
		LIMIT 1`,
		sessionID,
	)
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
    project_dir TEXT NOT NULL,
    cwd TEXT,
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
