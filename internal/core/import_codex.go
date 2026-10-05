package core

import (
	"fmt"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func importCodexGroups(db *DB, inputID int64, root string, adapter codex.Adapter, importedAt string, full bool) (*ImportResult, error) {
	files, scanErrs := adapter.ScanFiles(root)
	result := &ImportResult{FilesScanned: len(files), Errors: scanErrs}
	groups, readErrs := adapter.BuildGroups(root, files, importedAt)
	result.Errors = append(result.Errors, readErrs...)
	result.FilesFailed += len(readErrs)
	if len(scanErrs) > 0 || len(readErrs) > 0 {
		// An incomplete scan cannot prove a complete canonical owner snapshot.
		result.FilesFailed = len(files)
		return result, nil
	}
	if full {
		return replaceCodexInput(db, inputID, groups, result, importedAt)
	}
	for _, g := range groups {
		if g.Err != nil {
			result.Errors = append(result.Errors, g.Err)
			result.FilesFailed += g.Files
			continue
		}
		if len(g.States) == 0 && len(g.EmptyPaths) == 0 {
			result.FilesImported += g.Files
			for _, report := range g.Reports {
				old, _ := db.GetImportState(inputID, report.Path)
				pr := report.Diagnostics(old)
				result.UnparsedLines += pr.UnparsedLines
				result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
			}
			continue
		}
		unchanged := true
		paths := make([]string, 0, len(g.States)+len(g.EmptyPaths))
		for _, s := range g.States {
			paths = append(paths, s.JSONLPath)
		}
		paths = append(paths, g.EmptyPaths...)
		oldStates := make([]*ImportState, len(paths))
		for i, path := range paths {
			old, err := db.GetImportState(inputID, path)
			if err != nil {
				return nil, err
			}
			oldStates[i] = old
			if (i < len(g.States) && (old == nil || old.ContentHash != g.States[i].ContentHash)) || (i >= len(g.States) && old != nil) {
				unchanged = false
			}
		}
		if unchanged {
			result.FilesSkipped += g.Files
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return nil, err
		}
		t := importTx{tx: tx, inputID: inputID}
		for i, path := range paths {
			current, e := getImportState(tx, inputID, path)
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
		if err == nil {
			err = t.ReplaceSession(ingest.SourceCodex, g.Session.SessionID)
		}
		if err == nil && len(g.States) > 0 {
			err = persistCodexGroup(t, g, importedAt)
		}
		for _, s := range g.States {
			if err != nil {
				break
			}
			err = t.UpsertImportState(s)
		}
		for _, path := range g.EmptyPaths {
			if err != nil {
				break
			}
			_, err = tx.Exec("DELETE FROM import_state WHERE input_id=? AND jsonl_path=?", inputID, path)
		}
		if err == nil {
			err = tx.Commit()
		}
		tx.Rollback()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", g.Session.SessionID, err))
			result.FilesFailed += g.Files
			continue
		}
		result.FilesImported += g.Files
		addCodexMembershipDiagnostics(result, g.Messages)
		for _, report := range g.Reports {
			var old *ImportState
			for i, path := range paths {
				if path == report.Path {
					old = oldStates[i]
					break
				}
			}
			pr := report.Diagnostics(old)
			result.UnparsedLines += pr.UnparsedLines
			result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
		}
	}
	return result, nil
}

// replaceCodexInput validates the complete snapshot before deleting a selected
// input, keeping full-import conflicts and write failures non-destructive.
func replaceCodexInput(db *DB, inputID int64, groups []codex.Group, result *ImportResult, importedAt string) (*ImportResult, error) {
	for _, g := range groups {
		if g.Err != nil {
			result.Errors = append(result.Errors, g.Err)
		}
	}
	if len(result.Errors) > 0 {
		result.FilesFailed = result.FilesScanned
		return result, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, table := range []string{"messages", "sessions", "import_state"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE input_id=?", inputID); err != nil {
			return nil, err
		}
	}
	t := importTx{tx: tx, inputID: inputID}
	for _, g := range groups {
		for _, report := range g.Reports {
			pr := report.Diagnostics(nil)
			result.UnparsedLines += pr.UnparsedLines
			result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
		}
		if len(g.States) == 0 {
			continue
		}
		if err = persistCodexGroup(t, g, importedAt); err != nil {
			break
		}
		for _, s := range g.States {
			if err = t.UpsertImportState(s); err != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		result.Errors = append(result.Errors, err)
		result.FilesFailed = result.FilesScanned
		return result, nil
	}
	for _, g := range groups {
		addCodexMembershipDiagnostics(result, g.Messages)
	}
	result.FilesImported = result.FilesScanned
	return result, nil
}

func persistCodexGroup(t importTx, g codex.Group, importedAt string) error {
	if err := t.UpsertSession(g.Session, importedAt); err != nil {
		return err
	}
	for _, m := range g.Messages {
		meta := g.Session
		if m.Membership == "body" {
			meta.StartedAt = m.Timestamp
			meta.EndedAt = m.Timestamp
		}
		if err := ingest.PersistMessage(t, &ingest.NormalizedRecord{Session: meta, Message: m}, importedAt); err != nil {
			return err
		}
	}
	return nil
}

func addCodexMembershipDiagnostics(result *ImportResult, messages []ingest.NormalizedMessage) {
	for _, m := range messages {
		if m.Membership == "unresolved" {
			result.addUnparsedDiagnostics([]error{fmt.Errorf("%s:%d: explicit inheritance boundary with missing ordinal; retained unresolved", m.OriginPath, m.OriginLine)})
		}
	}
}
