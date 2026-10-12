package core

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
)

func importClaudeSnapshots(db *DB, inputID int64, root string, adapter claudecode.Adapter, importedAt string, full bool) (returned *ImportResult, returnedErr error) {
	paths, scanErrors := adapter.ScanFiles(root)
	result := &ImportResult{FilesScanned: len(paths), Errors: scanErrors}
	defer func() {
		if returnedErr != nil {
			result.FilesImported = 0
			result.FilesFailed = result.FilesScanned - result.FilesSkipped
			returned = result
		}
	}()
	// Bind every scanned path to its cursor before constructing full snapshots.
	expected := make(map[string]*ImportState, len(paths))
	if !full {
		for _, path := range paths {
			old, err := db.GetImportState(inputID, path)
			if err != nil {
				return nil, err
			}
			expected[path] = old
		}
	}
	files, readErrors, diagnostics := adapter.BuildSnapshots(root, paths, importedAt, expected)
	result.Errors = append(result.Errors, readErrors...)
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
		old := expected[f.State.JSONLPath]
		if full {
			var err error
			old, err = db.GetImportState(inputID, f.State.JSONLPath)
			if err != nil {
				return nil, err
			}
		}
		oldStates[i] = old
		if full || old == nil || old.ContentHash != f.State.ContentHash {
			result.FilesImported++
			for _, r := range f.Records {
				changed[r.Session.Identity] = true
			}
			pr := f.Diagnostics
			result.UnparsedLines += pr.UnparsedLines
			result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
		} else {
			result.FilesSkipped++
		}
	}
	if !full && len(changed) > 0 {
		// Metadata-only files can still change an owner found in another file.
		owners := make(map[string]*claudeOwnerSnapshot, len(changed))
		for identity := range changed {
			owners[identity] = &claudeOwnerSnapshot{}
		}
		for i := range files {
			f := &files[i]
			for j := range f.Records {
				r := &f.Records[j]
				if owner := owners[r.Session.Identity]; owner != nil {
					owner.records = append(owner.records, r)
				}
			}
			for identity, title := range f.Titles {
				if owner := owners[identity]; owner != nil {
					owner.titles = append(owner.titles, title)
				}
			}
			for identity, name := range f.AgentNames {
				if owner := owners[identity]; owner != nil {
					owner.names = append(owner.names, name)
				}
			}
		}
		for identity := range changed {
			unchanged, err := claudeOwnerUnchanged(db, inputID, identity, owners[identity])
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
		} else if len(f.Records) == 0 && f.Failures.Count == 0 {
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

type claudeOwnerSnapshot struct {
	records []*ingest.NormalizedRecord
	titles  []string
	names   []string
}

// Non-text appendages may change a file hash without changing stored owner text
// or metadata. Their cursor and relation evidence still participate in the pass.
func claudeOwnerUnchanged(db *DB, inputID int64, identity string, owner *claudeOwnerSnapshot) (bool, error) {
	var sessionID, cwd, repo, branch, version, title, name string
	err := db.db.QueryRow(`SELECT session_id,COALESCE(cwd,''),COALESCE(repo_path,''),COALESCE(git_branch,''),COALESCE(version,''),COALESCE(custom_title,''),COALESCE(agent_name,'') FROM sessions WHERE input_id=? AND identity=?`, inputID, identity).Scan(&sessionID, &cwd, &repo, &branch, &version, &title, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var expected []MessageRow
	for _, r := range owner.records {
		meta := r.Session
		if meta.SessionID != sessionID || (meta.CWD != "" && meta.CWD != cwd) || (meta.RepoPath != "" && meta.RepoPath != repo) || (meta.GitBranch != "" && meta.GitBranch != branch) || (meta.Version != "" && meta.Version != version) {
			return false, nil
		}
		m := r.Message
		if strings.TrimSpace(m.Content) != "" {
			expected = append(expected, MessageRow{UUID: m.UUID, Role: m.Role, Content: m.Content, Timestamp: m.Timestamp, Blocks: m.Blocks, Number: m.Number, OriginPath: m.OriginPath, OriginLine: m.OriginLine, PayloadID: m.PayloadID, Provenance: "source_record"})
		}
	}
	for _, t := range owner.titles {
		if t != title {
			return false, nil
		}
	}
	for _, n := range owner.names {
		if n != name {
			return false, nil
		}
	}
	if len(owner.records) == 0 {
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
