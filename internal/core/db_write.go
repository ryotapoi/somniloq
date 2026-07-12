package core

import (
	"database/sql"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

type importTx struct {
	tx *sql.Tx
}

// importTx must keep satisfying the claude-code-specific extension interface
// that claudecode's Flush asserts for at runtime.
var _ claudecode.SessionMetaWriter = importTx{}

func (t importTx) UpsertSession(meta ingest.SessionMeta, importedAt string) error {
	return upsertSession(t.tx, meta, importedAt)
}

func (t importTx) InsertMessage(msg ingest.NormalizedMessage) error {
	return insertMessage(t.tx, msg)
}

func (t importTx) UpdateSessionTitle(source ingest.Source, sessionID, title, importedAt string) error {
	return updateSessionTitle(t.tx, source, sessionID, title, importedAt)
}

func (t importTx) UpdateSessionAgentName(source ingest.Source, sessionID, agentName, importedAt string) error {
	return updateSessionAgentName(t.tx, source, sessionID, agentName, importedAt)
}

func (t importTx) UpsertImportState(state ingest.ImportState) error {
	return upsertImportState(t.tx, state)
}

func (t importTx) Commit() error {
	return t.tx.Commit()
}

func (t importTx) Rollback() error {
	return t.tx.Rollback()
}

func (d *DB) UpsertSession(meta SessionMeta, importedAt string) error {
	return upsertSession(d.execer(), meta, importedAt)
}

func upsertSession(e execer, meta SessionMeta, importedAt string) error {
	_, err := e.Exec(`
		INSERT INTO sessions (source, session_id, cwd, repo_path, git_branch, version, started_at, ended_at, imported_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
		ON CONFLICT(source, session_id) DO UPDATE SET
		  cwd = COALESCE(NULLIF(excluded.cwd, ''), sessions.cwd),
		  repo_path = COALESCE(NULLIF(excluded.repo_path, ''), sessions.repo_path),
		  git_branch = COALESCE(NULLIF(excluded.git_branch, ''), sessions.git_branch),
		  version = COALESCE(NULLIF(excluded.version, ''), sessions.version),
		  started_at = COALESCE(MIN(sessions.started_at, excluded.started_at), excluded.started_at, sessions.started_at),
		  ended_at = COALESCE(MAX(sessions.ended_at, excluded.ended_at), excluded.ended_at, sessions.ended_at),
		  imported_at = excluded.imported_at`,
		string(meta.Source), meta.SessionID, meta.CWD, meta.RepoPath, meta.GitBranch,
		meta.Version, meta.StartedAt, meta.EndedAt, importedAt,
	)
	return err
}

func (d *DB) InsertMessage(msg NormalizedMessage) error {
	return insertMessage(d.execer(), msg)
}

func insertMessage(e execer, msg NormalizedMessage) error {
	_, err := e.Exec(`
		INSERT OR IGNORE INTO messages (uuid, source, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.UUID, string(msg.Source), msg.SessionID, msg.ParentUUID, msg.Role,
		msg.Content, msg.Timestamp, msg.IsSidechain,
	)
	return err
}

func (d *DB) UpdateSessionTitle(source Source, sessionID, title, importedAt string) error {
	return updateSessionTitle(d.execer(), source, sessionID, title, importedAt)
}

func updateSessionTitle(e execer, source Source, sessionID, title, importedAt string) error {
	_, err := e.Exec(`UPDATE sessions SET custom_title = ?, imported_at = ? WHERE source = ? AND session_id = ?`,
		title, importedAt, string(source), sessionID,
	)
	return err
}

func (d *DB) UpdateSessionAgentName(source Source, sessionID, agentName, importedAt string) error {
	return updateSessionAgentName(d.execer(), source, sessionID, agentName, importedAt)
}

func updateSessionAgentName(e execer, source Source, sessionID, agentName, importedAt string) error {
	_, err := e.Exec(`UPDATE sessions SET agent_name = ?, imported_at = ? WHERE source = ? AND session_id = ?`,
		agentName, importedAt, string(source), sessionID,
	)
	return err
}

func (d *DB) UpsertImportState(state ImportState) error {
	return upsertImportState(d.execer(), state)
}

func upsertImportState(e execer, state ImportState) error {
	_, err := e.Exec(`
		INSERT INTO import_state (jsonl_path, source, file_size, last_offset, imported_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(jsonl_path) DO UPDATE SET
		  source = excluded.source,
		  file_size = excluded.file_size,
		  last_offset = excluded.last_offset,
		  imported_at = excluded.imported_at`,
		state.JSONLPath, string(state.Source), state.FileSize, state.LastOffset, state.ImportedAt,
	)
	return err
}

func (d *DB) DeleteAll() error {
	for _, table := range []string{"messages", "sessions", "import_state"} {
		if _, err := d.execer().Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	return nil
}
