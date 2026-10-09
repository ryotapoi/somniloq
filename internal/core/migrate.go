package core

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

// MigrationResult reports copied history separately from owner replacements.
type MigrationResult struct {
	SnapshotSHA256              string  `json:"snapshot_sha256"`
	CopyPerformed               bool    `json:"copy_performed"`
	GroupsReplaced              int     `json:"groups_replaced"`
	GroupsSkipped               int     `json:"groups_skipped"`
	GroupsFailed                int     `json:"groups_failed"`
	LegacyMessagesRemoved       int     `json:"legacy_messages_removed"`
	LegacyMessagesRetained      int     `json:"legacy_messages_retained"`
	LegacyConversationsRetained int     `json:"legacy_conversations_retained"`
	LegacyReplacementFailures   int     `json:"legacy_replacement_failures"`
	LegacyRetentionWarnings     int     `json:"legacy_retention_warnings"`
	Skips                       []error `json:"-"`
	Warnings                    []error `json:"-"`
	Errors                      []error `json:"-"`
}

// Migrate copies a fixed legacy snapshot, then independently replaces parsed
// Codex owner groups. A non-nil result means the initial copy is committed.
func Migrate(from, destination string, inputs []Input) (*MigrationResult, error) {
	db, digest, copied, err := prepareMigration(from, destination)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	result := &MigrationResult{SnapshotSHA256: digest, CopyPerformed: copied}
	importedAt := timeNow()
	adapter := codex.NewMigrationAdapter(ResolveRepoPath)
	var scans []migrationInput
	seen := map[string]bool{}
	for _, input := range inputs {
		if input.Source != SourceCodex {
			continue
		}
		cwd, err := os.Getwd()
		if err != nil {
			result.Errors = append(result.Errors, err)
			result.GroupsFailed++
			continue
		}
		input.Root, err = CanonicalPath(input.Root, cwd)
		if err != nil {
			result.Errors = append(result.Errors, err)
			result.GroupsFailed++
			continue
		}
		key := InputKey(input.Source, input.Root)
		if seen[key] {
			continue
		}
		seen[key] = true
		files, scanErrors := adapter.ScanFiles(input.Root)
		groups, readErrors := adapter.BuildMigrationIndex(input.Root, files, importedAt)
		scan := migrationInput{Input: input, Groups: groups}
		scan.Errors = append(scanErrors, readErrors...)
		// Unknown-owner failures invalidate the input's complete owner snapshot.
		for _, group := range groups {
			if group.Session.SessionID == "" && group.Err != nil {
				scan.Errors = append(scan.Errors, group.Err)
			}
		}
		scans = append(scans, scan)
	}
	if err := replaceMigrationGroups(db, scans, adapter, importedAt, result); err != nil {
		return result, err
	}
	return result, nil
}

type migrationInput struct {
	Input  Input
	Groups []codex.Group
	Errors []error
}

func replaceMigrationGroups(db *DB, scans []migrationInput, adapter codex.Adapter, importedAt string, result *MigrationResult) error {
	fileInputs := map[string]map[int]bool{}
	for i, scan := range scans {
		for _, g := range scan.Groups {
			for _, report := range g.Reports {
				if fileInputs[report.Path] == nil {
					fileInputs[report.Path] = map[int]bool{}
				}
				fileInputs[report.Path][i] = true
			}
		}
	}
	for _, scan := range scans {
		if len(scan.Errors) > 0 {
			result.Errors = append(result.Errors, scan.Errors...)
			result.GroupsFailed += max(1, len(scan.Groups))
			continue
		}
		for _, g := range scan.Groups {
			err := g.Err
			if err == nil && g.Session.SessionID == "" {
				continue
			}
			for _, report := range g.Reports {
				if len(fileInputs[report.Path]) > 1 {
					err = fmt.Errorf("%s: input_membership_conflict", report.Path)
				}
			}
			var body codex.Group
			if err == nil {
				paths := make([]string, len(g.Reports))
				for k, report := range g.Reports {
					paths[k] = report.Path
				}
				groups, readErrors := adapter.BuildMigrationGroups(scan.Input.Root, paths, importedAt)
				if len(readErrors) > 0 {
					err = readErrors[0]
				} else if len(groups) != 1 || groups[0].Session.SessionID != g.Session.SessionID || !sameMigrationReports(g.Reports, groups[0].Reports) {
					err = fmt.Errorf("%s: rollout changed during migration", g.Session.SessionID)
				} else {
					body = groups[0]
					err = body.Err
				}
			}
			removed := 0
			if err == nil {
				removed, err = replaceMigrationGroup(db, scan.Input, body, result.SnapshotSHA256, importedAt)
			}
			if err != nil {
				result.GroupsFailed++
				result.Errors = append(result.Errors, err)
				continue
			}
			result.GroupsReplaced++
			result.LegacyMessagesRemoved += removed
		}
	}
	var err error
	if err = db.db.QueryRow(`SELECT count(*) FROM legacy_messages`).Scan(&result.LegacyMessagesRetained); err != nil {
		return err
	}
	return db.db.QueryRow(`SELECT count(*) FROM legacy_sessions`).Scan(&result.LegacyConversationsRetained)
}

func sameMigrationReports(index, body []codex.FileReport) bool {
	if len(index) != len(body) {
		return false
	}
	for i := range index {
		if index[i].Path != body[i].Path {
			return false
		}
	}
	return true
}

// Appends can be recovered by import using the parsed bytes' hash and cursor.
// Missing, replaced, truncated, or same-size edited files are not appends.
// Compare against parsed length because reads may include an append after fstat;
// compare mtime at the original size so a same-size edit cannot pass as growth.
func checkMigrationGroup(group codex.Group) error {
	for _, report := range group.Reports {
		info, err := os.Stat(report.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", report.Path, err)
		}
		if report.Info == nil || !os.SameFile(report.Info, info) || info.Size() < int64(len(report.Data)) || (info.Size() == report.Info.Size() && !info.ModTime().Equal(report.Info.ModTime())) {
			return fmt.Errorf("%s: rollout changed during migration", report.Path)
		}
	}
	return nil
}

func replaceMigrationGroup(db *DB, input Input, g codex.Group, digest, importedAt string) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// Input creation belongs to the same transaction as the replacement.
	key := InputKey(input.Source, input.Root)
	if _, err = tx.Exec(`INSERT INTO inputs(input_key,source,root) VALUES(?,?,?) ON CONFLICT(input_key) DO NOTHING`, key, input.Source, input.Root); err != nil {
		return 0, err
	}
	var inputID int64
	if err = tx.QueryRow(`SELECT id FROM inputs WHERE input_key=?`, key).Scan(&inputID); err != nil {
		return 0, err
	}
	if err = checkSavedMigrationRollouts(tx, inputID, input.Root, g); err != nil {
		return 0, err
	}
	t := importTx{tx: tx, inputID: inputID}
	if err = t.ReplaceSession(SourceCodex, g.Session.SessionID); err != nil {
		return 0, err
	}
	if err = persistCodexGroup(t, g, importedAt); err != nil {
		return 0, err
	}
	for _, s := range g.States {
		if err = t.UpsertImportState(s); err != nil {
			return 0, err
		}
	}
	// A successfully parsed owner replaces all same-ID Codex history, including
	// rows without surviving physical evidence. Other IDs remain legacy history.
	deleted, err := tx.Exec(`DELETE FROM legacy_messages WHERE snapshot_sha256=? AND source='codex' AND session_id=?`, digest, g.Session.SessionID)
	if err != nil {
		return 0, err
	}
	removed, err := deleted.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(`DELETE FROM legacy_sessions WHERE snapshot_sha256=? AND source='codex' AND session_id=?`, digest, g.Session.SessionID); err != nil {
		return 0, err
	}
	if err = checkMigrationGroup(g); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return int(removed), nil
}

// Saved body and context provenance must still belong to the replacement's
// rollout set. Checking inside the transaction protects the prior full owner.
func checkSavedMigrationRollouts(tx *writeTx, inputID int64, root string, g codex.Group) error {
	paths := make(map[string]bool, len(g.Reports))
	for _, report := range g.Reports {
		rel, err := filepath.Rel(root, report.Path)
		if err != nil {
			return err
		}
		paths[filepath.ToSlash(rel)] = true
	}
	identity := g.Session.Identity
	if identity == "" {
		identity = rootIdentity(g.Session.SessionID)
	}
	rows, err := tx.Query(`SELECT DISTINCT origin_path FROM messages WHERE input_id=? AND source=? AND identity=? ORDER BY origin_path`, inputID, SourceCodex, identity)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var path string
		if err = rows.Scan(&path); err != nil {
			return err
		}
		if !paths[path] {
			return fmt.Errorf("%s: rollout missing from migration group %s", filepath.Join(root, filepath.FromSlash(path)), g.Session.SessionID)
		}
	}
	return rows.Err()
}
