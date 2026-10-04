package core

import (
	"database/sql"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

type importTx struct {
	tx      *sql.Tx
	inputID int64
}

// importTx must keep satisfying the claude-code-specific extension interface
// that claudecode's Flush asserts for at runtime.
var _ claudecode.SessionMetaWriter = importTx{}

func (t importTx) UpsertSession(meta ingest.SessionMeta, importedAt string) error {
	return upsertSession(t.tx, t.inputID, meta, importedAt)
}

func (t importTx) InsertMessage(msg ingest.NormalizedMessage) error {
	return insertMessage(t.tx, t.inputID, msg)
}

func (t importTx) UpdateSessionTitle(source ingest.Source, sessionID, title, importedAt string) error {
	return updateSessionTitle(t.tx, t.inputID, source, sessionID, title, importedAt)
}

func (t importTx) UpdateSessionAgentName(source ingest.Source, sessionID, agentName, importedAt string) error {
	return updateSessionAgentName(t.tx, t.inputID, source, sessionID, agentName, importedAt)
}

func (t importTx) UpsertImportState(state ingest.ImportState) error {
	return upsertImportState(t.tx, t.inputID, state)
}

func (t importTx) Commit() error {
	return t.tx.Commit()
}

func (t importTx) Rollback() error {
	return t.tx.Rollback()
}

func (d *DB) UpsertSession(inputID int64, meta SessionMeta, importedAt string) error {
	return upsertSession(d.execer(), inputID, meta, importedAt)
}

func upsertSession(e execer, inputID int64, meta SessionMeta, importedAt string) error {
	// custom_title and agent_name have source-specific update paths; excluding
	// them here prevents ordinary message imports from overwriting that metadata.
	_, err := e.Exec(`
		INSERT INTO sessions (input_id, identity, source, session_id, cwd, repo_path, git_branch, version, started_at, ended_at, imported_at)
		VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
		ON CONFLICT(input_id, source, session_id) DO UPDATE SET
		  cwd = COALESCE(NULLIF(excluded.cwd, ''), sessions.cwd),
		  repo_path = COALESCE(NULLIF(excluded.repo_path, ''), sessions.repo_path),
		  git_branch = COALESCE(NULLIF(excluded.git_branch, ''), sessions.git_branch),
		  version = COALESCE(NULLIF(excluded.version, ''), sessions.version),
		  started_at = CASE
		    WHEN rfc3339_utc_nanos(sessions.started_at) IS NULL THEN
		      CASE WHEN rfc3339_utc_nanos(excluded.started_at) IS NULL THEN sessions.started_at ELSE excluded.started_at END
		    WHEN rfc3339_utc_nanos(excluded.started_at) IS NULL THEN sessions.started_at
		    WHEN rfc3339_utc_nanos(excluded.started_at) < rfc3339_utc_nanos(sessions.started_at) THEN excluded.started_at
		    ELSE sessions.started_at
		  END,
		  ended_at = CASE
		    WHEN rfc3339_utc_nanos(sessions.ended_at) IS NULL THEN
		      CASE WHEN rfc3339_utc_nanos(excluded.ended_at) IS NULL THEN sessions.ended_at ELSE excluded.ended_at END
		    WHEN rfc3339_utc_nanos(excluded.ended_at) IS NULL THEN sessions.ended_at
		    WHEN rfc3339_utc_nanos(excluded.ended_at) > rfc3339_utc_nanos(sessions.ended_at) THEN excluded.ended_at
		    ELSE sessions.ended_at
		  END,
		  imported_at = excluded.imported_at`,
		inputID, rootIdentity(meta.SessionID), string(meta.Source), meta.SessionID, meta.CWD, meta.RepoPath, meta.GitBranch,
		meta.Version, meta.StartedAt, meta.EndedAt, importedAt,
	)
	return err
}

func (d *DB) InsertMessage(inputID int64, msg NormalizedMessage) error {
	return insertMessage(d.execer(), inputID, msg)
}

func insertMessage(e execer, inputID int64, msg NormalizedMessage) error {
	_, err := e.Exec(`
		INSERT OR IGNORE INTO messages (input_id, uuid, source, session_id, parent_uuid, role, content, timestamp, is_sidechain)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inputID, msg.UUID, string(msg.Source), msg.SessionID, msg.ParentUUID, msg.Role,
		msg.Content, msg.Timestamp, msg.IsSidechain,
	)
	return err
}

func (d *DB) UpdateSessionTitle(inputID int64, source Source, sessionID, title, importedAt string) error {
	return updateSessionTitle(d.execer(), inputID, source, sessionID, title, importedAt)
}

func updateSessionTitle(e execer, inputID int64, source Source, sessionID, title, importedAt string) error {
	_, err := e.Exec(`UPDATE sessions SET custom_title = ?, imported_at = ? WHERE input_id = ? AND source = ? AND session_id = ?`,
		title, importedAt, inputID, string(source), sessionID,
	)
	return err
}

func (d *DB) UpdateSessionAgentName(inputID int64, source Source, sessionID, agentName, importedAt string) error {
	return updateSessionAgentName(d.execer(), inputID, source, sessionID, agentName, importedAt)
}

func updateSessionAgentName(e execer, inputID int64, source Source, sessionID, agentName, importedAt string) error {
	_, err := e.Exec(`UPDATE sessions SET agent_name = ?, imported_at = ? WHERE input_id = ? AND source = ? AND session_id = ?`,
		agentName, importedAt, inputID, string(source), sessionID,
	)
	return err
}

func (d *DB) UpsertImportState(inputID int64, state ImportState) error {
	return upsertImportState(d.execer(), inputID, state)
}

func upsertImportState(e execer, inputID int64, state ImportState) error {
	_, err := e.Exec(`
		INSERT INTO import_state (input_id, jsonl_path, source, file_size, last_offset, imported_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(input_id, jsonl_path) DO UPDATE SET
		  source = excluded.source,
		  file_size = excluded.file_size,
		  last_offset = excluded.last_offset,
		  imported_at = excluded.imported_at`,
		inputID, state.JSONLPath, string(state.Source), state.FileSize, state.LastOffset, state.ImportedAt,
	)
	return err
}

func (d *DB) DeleteAll() error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{"messages", "sessions", "import_state"} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteInputs rebuilds only the selected inputs, retaining their stable IDs.
func (d *DB) DeleteInputs(inputIDs []int64) error {
	if len(inputIDs) == 0 {
		return nil
	}
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, table := range []string{"messages", "sessions", "import_state"} {
		for _, id := range inputIDs {
			if _, err = tx.Exec("DELETE FROM "+table+" WHERE input_id=?", id); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}
