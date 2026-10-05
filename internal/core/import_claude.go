package core

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

func importClaudeSnapshots(db *DB, inputID int64, root string, adapter claudecode.Adapter, importedAt string, full bool) (*ImportResult, error) {
	paths, scanErrors := adapter.ScanFiles(root)
	files, readErrors, diagnostics := adapter.BuildSnapshots(root, paths, importedAt)
	result := &ImportResult{FilesScanned: len(paths), Errors: append(scanErrors, readErrors...)}
	result.addUnparsedDiagnostics(diagnostics)
	result.FilesFailed = len(readErrors)
	// Owner snapshots are complete only when every physical file was readable.
	// A missing file can belong to the same owner as a successfully read file.
	if len(result.Errors) > 0 {
		result.FilesFailed = len(paths)
		return result, nil
	}
	oldStates := make([]*ImportState, len(files))
	changed := map[string]bool{}
	for i, f := range files {
		old, err := db.GetImportState(inputID, f.State.JSONLPath)
		if err != nil {
			return nil, err
		}
		oldStates[i] = old
		if full || old == nil || old.ContentHash != f.State.ContentHash {
			result.FilesImported++
			for _, r := range f.Records {
				changed[r.Session.Identity] = true
			}
			if old != nil && (len(f.Records) > 0 || len(f.Failures) == 0) {
				relative, e := filepath.Rel(root, f.State.JSONLPath)
				if e != nil {
					return nil, e
				}
				rows, e := db.db.Query(`SELECT DISTINCT identity FROM messages WHERE input_id=? AND origin_path=?`, inputID, filepath.ToSlash(relative))
				if e != nil {
					return nil, e
				}
				for rows.Next() {
					var identity string
					if e = rows.Scan(&identity); e != nil {
						rows.Close()
						return nil, e
					}
					changed[identity] = true
				}
				e = rows.Err()
				rows.Close()
				if e != nil {
					return nil, e
				}
			}
			pr := f.Diagnostics(old)
			if full {
				pr = f.Diagnostics(nil)
			}
			result.UnparsedLines += pr.UnparsedLines
			result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
		} else {
			result.FilesSkipped++
		}
	}
	if !full {
		for identity := range changed {
			unchanged, err := claudeOwnerUnchanged(db, inputID, identity, files)
			if err != nil {
				return nil, err
			}
			if unchanged {
				delete(changed, identity)
			}
		}
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t := importTx{tx: tx, inputID: inputID}
	for i, f := range files {
		current, e := getImportState(tx, inputID, f.State.JSONLPath)
		if e != nil {
			err = e
			break
		}
		old := oldStates[i]
		if (old == nil) != (current == nil) || (old != nil && current != nil && *old != *current) {
			err = fmt.Errorf("import state changed during import")
			break
		}
	}
	if err == nil && full {
		for _, table := range []string{"messages", "sessions", "import_state"} {
			if _, err = tx.Exec("DELETE FROM "+table+" WHERE input_id=?", inputID); err != nil {
				break
			}
		}
	}
	if err == nil && !full {
		// Replace only changed owners; unchanged original text keeps imported_at.
		for identity := range changed {
			if _, err = tx.Exec(`DELETE FROM messages WHERE input_id=? AND identity=?`, inputID, identity); err != nil {
				break
			}
			if _, err = tx.Exec(`DELETE FROM sessions WHERE input_id=? AND identity=?`, inputID, identity); err != nil {
				break
			}
		}
	}
	for i, f := range files {
		if err != nil {
			break
		}
		for _, r := range f.Records {
			if !changed[r.Session.Identity] {
				continue
			}
			// Relations are replaced below after all conversations have been saved.
			r.Session.ParentIdentity = ""
			if err = ingest.PersistMessage(t, &r, importedAt); err != nil {
				break
			}
		}
		if err != nil {
			break
		}
		for identity, title := range f.Titles {
			if changed[identity] {
				_, err = tx.Exec(`UPDATE sessions SET custom_title=? WHERE input_id=? AND identity=?`, title, inputID, identity)
				if err != nil {
					break
				}
			}
		}
		if err != nil {
			break
		}
		for identity, name := range f.AgentNames {
			if changed[identity] {
				_, err = tx.Exec(`UPDATE sessions SET agent_name=? WHERE input_id=? AND identity=?`, name, inputID, identity)
				if err != nil {
					break
				}
			}
		}
		if err != nil {
			break
		}
		if len(f.Records) > 0 && (full || oldStates[i] == nil || oldStates[i].ContentHash != f.State.ContentHash) {
			err = t.UpsertImportState(f.State)
		} else if len(f.Records) == 0 && len(f.Failures) == 0 {
			_, err = tx.Exec(`DELETE FROM import_state WHERE input_id=? AND jsonl_path=?`, inputID, f.State.JSONLPath)
		}
	}
	if err == nil {
		for _, f := range files {
			for identity, parent := range f.Parents {
				// Clearing is essential when new evidence makes an earlier edge uncertain.
				if _, err = tx.Exec(`UPDATE sessions SET parent_identity=?, imported_at=CASE WHEN parent_identity<>? THEN ? ELSE imported_at END WHERE input_id=? AND identity=?`, parent, parent, importedAt, inputID, identity); err != nil {
					break
				}
			}
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		result.Errors = append(result.Errors, err)
		result.FilesFailed = result.FilesScanned
		result.FilesImported = 0
		result.FilesSkipped = 0
	}
	return result, nil
}

// Non-text appendages may change a file hash without changing stored owner text
// or metadata. Their cursor and relation evidence still participate in the pass.
func claudeOwnerUnchanged(db *DB, inputID int64, identity string, files []claudecode.FileSnapshot) (bool, error) {
	var sessionID, cwd, repo, branch, version, title, name string
	err := db.db.QueryRow(`SELECT session_id,COALESCE(cwd,''),COALESCE(repo_path,''),COALESCE(git_branch,''),COALESCE(version,''),COALESCE(custom_title,''),COALESCE(agent_name,'') FROM sessions WHERE input_id=? AND identity=?`, inputID, identity).Scan(&sessionID, &cwd, &repo, &branch, &version, &title, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var expected []MessageRow
	found := false
	for _, f := range files {
		for _, r := range f.Records {
			if r.Session.Identity != identity {
				continue
			}
			found = true
			meta := r.Session
			if meta.SessionID != sessionID || (meta.CWD != "" && meta.CWD != cwd) || (meta.RepoPath != "" && meta.RepoPath != repo) || (meta.GitBranch != "" && meta.GitBranch != branch) || (meta.Version != "" && meta.Version != version) {
				return false, nil
			}
			m := r.Message
			if strings.TrimSpace(m.Content) != "" {
				expected = append(expected, MessageRow{UUID: m.UUID, Role: m.Role, Content: m.Content, Timestamp: m.Timestamp, Blocks: m.Blocks, Number: m.Number, OriginPath: m.OriginPath, OriginLine: m.OriginLine, PayloadID: m.PayloadID, Provenance: "source_record"})
			}
		}
		if t, ok := f.Titles[identity]; ok && t != title {
			return false, nil
		}
		if n, ok := f.AgentNames[identity]; ok && n != name {
			return false, nil
		}
	}
	if !found {
		return false, nil
	}
	messages, err := db.GetIdentityMessages(inputID, SourceClaudeCode, identity)
	if err != nil {
		return false, err
	}
	if len(expected) == 0 && len(messages) == 0 {
		return true, nil
	}
	return reflect.DeepEqual(expected, messages), nil
}
