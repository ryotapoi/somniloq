package core

import (
	"fmt"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

// Ordinary Codex imports retain raw parent evidence even when cyclic so the
// relation resolver can diagnose it without discarding valid owner bodies.
// Dedicated migration keeps the strict writer default.
func importCodexGroups(db *DB, inputID int64, root string, adapter codex.Adapter, importedAt string, full bool) (*ImportResult, error) {
	adapter = adapter.ForImportPass()
	files, scanErrs := adapter.ScanFiles(root)
	result := &ImportResult{FilesScanned: len(files), Errors: scanErrs}
	// Capture expected cursors before reading bytes; a later importer must not
	// become the expected state of this older snapshot.
	expected := make(map[string]*ImportState, len(files))
	if !full {
		for _, path := range files {
			old, err := db.GetImportState(inputID, path)
			if err != nil {
				result.FilesFailed++
				return result, fmt.Errorf("%s: get state: %w", path, err)
			}
			expected[path] = old
		}
	}
	groups, readErrs := adapter.BuildImportIndex(files)
	result.Errors = append(result.Errors, readErrs...)
	result.FilesFailed += len(readErrs)
	if len(scanErrs) > 0 || len(readErrs) > 0 {
		// An incomplete scan cannot prove a complete canonical owner snapshot.
		result.FilesFailed = len(files)
		return result, nil
	}
	if full {
		return replaceCodexInput(db, inputID, root, adapter, groups, result, importedAt)
	}
	for _, index := range groups {
		paths := codexGroupPaths(index)
		loaded, readErrs := adapter.BuildGroups(root, paths, importedAt)
		if len(readErrs) > 0 || !sameCodexImportIndex(index, loaded) {
			result.Errors = append(result.Errors, readErrs...)
			if len(readErrs) == 0 {
				result.Errors = append(result.Errors, fmt.Errorf("%s: rollout ownership changed during import", index.Session.SessionID))
			}
			result.FilesFailed += len(paths)
			continue
		}
		for _, g := range loaded {
			if g.Err != nil {
				result.Errors = append(result.Errors, g.Err)
				result.FilesFailed += g.Files
				continue
			}
			if len(g.States) == 0 && len(g.EmptyPaths) == 0 {
				result.FilesImported += g.Files
				for _, report := range g.Reports {
					old := expected[report.Path]
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
				old := expected[path]
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
				result.FilesFailed += g.Files
				return result, fmt.Errorf("%s: begin: %w", g.Session.SessionID, err)
			}
			t := importTx{tx: tx, inputID: inputID, allowCyclicParents: true}
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
				pr := report.Diagnostics(expected[report.Path])
				result.UnparsedLines += pr.UnparsedLines
				result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
			}
		}
	}
	return result, nil
}

// replaceCodexInput validates and writes one owner at a time inside one
// transaction, keeping full-import conflicts and write failures non-destructive.
func replaceCodexInput(db *DB, inputID int64, root string, adapter codex.Adapter, groups []codex.Group, result *ImportResult, importedAt string) (*ImportResult, error) {
	tx, err := db.Begin()
	if err != nil {
		result.FilesFailed = result.FilesScanned
		return result, err
	}
	defer tx.Rollback()
	for _, table := range []string{"messages", "sessions", "import_state"} {
		if _, err = tx.Exec("DELETE FROM "+table+" WHERE input_id=?", inputID); err != nil {
			result.FilesFailed = result.FilesScanned
			return result, err
		}
	}
	t := importTx{tx: tx, inputID: inputID, allowCyclicParents: true}
	var unparsedLines int
	var unparsedDiagnostics []error
	var membershipDiagnostics []error
	var writeErr error
	for _, index := range groups {
		loaded, readErrs := adapter.BuildGroups(root, codexGroupPaths(index), importedAt)
		if len(readErrs) > 0 || !sameCodexImportIndex(index, loaded) {
			result.Errors = append(result.Errors, readErrs...)
			if len(readErrs) == 0 {
				result.Errors = append(result.Errors, fmt.Errorf("%s: rollout ownership changed during import", index.Session.SessionID))
			}
			continue
		}
		for _, g := range loaded {
			if g.Err != nil {
				result.Errors = append(result.Errors, g.Err)
				continue
			}
			if writeErr == nil {
				for _, report := range g.Reports {
					pr := report.Diagnostics(nil)
					unparsedLines += pr.UnparsedLines
					for _, diagnostic := range pr.UnparsedDiagnostics {
						if len(unparsedDiagnostics) < ingest.MaxUnparsedDiagnostics {
							unparsedDiagnostics = append(unparsedDiagnostics, diagnostic)
						}
					}
				}
			}
			for _, message := range g.Messages {
				if message.Membership == "unresolved" && len(membershipDiagnostics) < ingest.MaxUnparsedDiagnostics {
					membershipDiagnostics = append(membershipDiagnostics, fmt.Errorf("%s:%d: explicit inheritance boundary with missing ordinal; retained unresolved", message.OriginPath, message.OriginLine))
				}
			}
			if len(g.States) == 0 || writeErr != nil || len(result.Errors) > 0 {
				continue
			}
			if writeErr = persistCodexGroup(t, g, importedAt); writeErr != nil {
				continue
			}
			for _, s := range g.States {
				if writeErr = t.UpsertImportState(s); writeErr != nil {
					break
				}
			}
		}
	}
	if writeErr == nil && len(result.Errors) == 0 {
		err = tx.Commit()
	}
	if writeErr != nil || err != nil || len(result.Errors) > 0 {
		if len(result.Errors) == 0 {
			result.UnparsedLines = unparsedLines
			result.addUnparsedDiagnostics(unparsedDiagnostics)
		}
		if writeErr != nil && len(result.Errors) == 0 {
			result.Errors = append(result.Errors, writeErr)
		}
		if err != nil {
			result.Errors = append(result.Errors, err)
		}
		result.FilesFailed = result.FilesScanned
		return result, nil
	}
	result.UnparsedLines = unparsedLines
	result.addUnparsedDiagnostics(unparsedDiagnostics)
	result.addUnparsedDiagnostics(membershipDiagnostics)
	result.FilesImported = result.FilesScanned
	return result, nil
}

func codexGroupPaths(g codex.Group) []string {
	paths := make([]string, len(g.Reports))
	for i, report := range g.Reports {
		paths[i] = report.Path
	}
	return paths
}

func sameCodexImportIndex(index codex.Group, loaded []codex.Group) bool {
	paths := codexGroupPaths(index)
	seen := make(map[string]bool, len(paths))
	for _, g := range loaded {
		if g.Session.SessionID != "" && g.Session.SessionID != index.Session.SessionID {
			return false
		}
		for _, report := range g.Reports {
			seen[report.Path] = true
		}
	}
	if len(seen) != len(paths) {
		return false
	}
	for _, path := range paths {
		if !seen[path] {
			return false
		}
	}
	return true
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
