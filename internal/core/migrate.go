package core

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

// MigrationResult reports copied history separately from owner replacements.
type MigrationResult struct {
	SnapshotSHA256              string  `json:"snapshot_sha256"`
	CopyPerformed               bool    `json:"copy_performed"`
	GroupsReplaced              int     `json:"groups_replaced"`
	GroupsFailed                int     `json:"groups_failed"`
	LegacyMessagesRemoved       int     `json:"legacy_messages_removed"`
	LegacyMessagesRetained      int     `json:"legacy_messages_retained"`
	LegacyConversationsRetained int     `json:"legacy_conversations_retained"`
	LegacyReplacementFailures   int     `json:"legacy_replacement_failures"`
	Errors                      []error `json:"-"`
}

// Migrate copies a fixed legacy snapshot, then independently replaces proven
// Codex owner groups. A non-nil result means the initial copy is committed.
func Migrate(from, destination string, inputs []Input) (*MigrationResult, error) {
	db, digest, copied, err := prepareMigration(from, destination)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	result := &MigrationResult{SnapshotSHA256: digest, CopyPerformed: copied}
	importedAt := timeNow()
	adapter := codex.NewAdapter(ResolveRepoPath)
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
		groups, readErrors := adapter.BuildMigrationGroups(input.Root, files, importedAt)
		scan := migrationInput{Input: input, Files: files, Groups: groups}
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
	Files  []string
	Groups []codex.Group
	Errors []error
}
type migrationEvidence struct {
	Input int
	Group int
	Path  string
	Line  int
}

func replaceMigrationGroups(db *DB, scans []migrationInput, adapter codex.Adapter, importedAt string, result *MigrationResult) error {
	evidence := map[string][]migrationEvidence{}
	fileInputs := map[string]map[int]bool{}
	for i, scan := range scans {
		for j, g := range scan.Groups {
			for _, report := range g.Reports {
				if fileInputs[report.Path] == nil {
					fileInputs[report.Path] = map[int]bool{}
				}
				fileInputs[report.Path][i] = true
				for _, line := range report.Lines {
					evidence[line.UUID] = append(evidence[line.UUID], migrationEvidence{i, j, report.Path, line.Line})
				}
			}
		}
	}
	successful := map[string]map[int]bool{}
	for i, scan := range scans {
		if len(scan.Errors) > 0 {
			result.Errors = append(result.Errors, scan.Errors...)
			result.GroupsFailed += max(1, len(scan.Groups))
			continue
		}
		for j, g := range scan.Groups {
			err := g.Err
			hasBody := false
			for _, m := range g.Messages {
				if m.Membership == "body" && strings.TrimSpace(m.Content) != "" {
					hasBody = true
					break
				}
			}
			if err == nil && g.Session.SessionID != "" && !hasBody {
				err = fmt.Errorf("%s: no_own_messages", g.Session.SessionID)
			}
			if err == nil && g.Session.SessionID == "" {
				continue
			}
			for _, report := range g.Reports {
				if len(fileInputs[report.Path]) > 1 {
					err = fmt.Errorf("%s: input_membership_conflict", report.Path)
				}
			}
			if err == nil {
				err = checkMigrationSnapshot(adapter, scan, g)
			}
			removed := 0
			if err == nil {
				removed, err = replaceMigrationGroup(db, scan.Input, g, i, j, evidence, adapter, scan, importedAt)
			}
			if err != nil {
				result.GroupsFailed++
				result.Errors = append(result.Errors, err)
				continue
			}
			result.GroupsReplaced++
			result.LegacyMessagesRemoved += removed
			if successful[g.Session.SessionID] == nil {
				successful[g.Session.SessionID] = map[int]bool{}
			}
			successful[g.Session.SessionID][i] = true
		}
	}
	// Same-named old history does not become attributable merely because a new
	// owner was saved. Diagnose remaining unproven or competing row evidence.
	rows, err := db.db.Query(`SELECT session_id,uuid FROM legacy_messages WHERE source='codex'`)
	if err != nil {
		return err
	}
	unknown := map[string]bool{}
	for rows.Next() {
		var id, uuid string
		if err = rows.Scan(&id, &uuid); err != nil {
			rows.Close()
			return err
		}
		owners := successful[id]
		if len(owners) == 0 {
			continue
		}
		matches := evidence[uuid]
		if len(matches) != 1 || !owners[matches[0].Input] {
			unknown[id] = true
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(unknown))
	for id := range unknown {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		result.LegacyReplacementFailures++
		result.Errors = append(result.Errors, fmt.Errorf("%s: old_input_membership_unknown", id))
	}
	if err = db.db.QueryRow(`SELECT count(*) FROM legacy_messages`).Scan(&result.LegacyMessagesRetained); err != nil {
		return err
	}
	return db.db.QueryRow(`SELECT count(*) FROM legacy_sessions`).Scan(&result.LegacyConversationsRetained)
}

func checkMigrationSnapshot(adapter codex.Adapter, scan migrationInput, group codex.Group) error {
	files, errs := adapter.ScanFiles(scan.Input.Root)
	if len(errs) > 0 {
		return errs[0]
	}
	expected := append([]string(nil), scan.Files...)
	sort.Strings(expected)
	sort.Strings(files)
	if !slices.Equal(files, expected) {
		return fmt.Errorf("%s: rollout set changed during migration", scan.Input.Root)
	}
	for _, report := range group.Reports {
		data, err := os.ReadFile(report.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", report.Path, err)
		}
		if !bytes.Equal(data, report.Data) {
			return fmt.Errorf("%s: rollout changed during migration", report.Path)
		}
	}
	return nil
}

func replaceMigrationGroup(db *DB, input Input, g codex.Group, inputIndex, groupIndex int, evidence map[string][]migrationEvidence, adapter codex.Adapter, scan migrationInput, importedAt string) (int, error) {
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
	t := importTx{tx: tx, inputID: inputID}
	if err = t.ReplaceSession(SourceCodex, g.Session.SessionID); err != nil {
		return 0, err
	}
	if err = t.UpsertSession(g.Session, importedAt); err != nil {
		return 0, err
	}
	for _, m := range g.Messages {
		meta := g.Session
		if m.Membership == "body" {
			meta.StartedAt = m.Timestamp
			meta.EndedAt = m.Timestamp
		}
		if err = ingest.PersistMessage(t, &ingest.NormalizedRecord{Session: meta, Message: m}, importedAt); err != nil {
			return 0, err
		}
	}
	for _, s := range g.States {
		if err = t.UpsertImportState(s); err != nil {
			return 0, err
		}
	}
	rows, err := tx.Query(`SELECT legacy_rowid,uuid,session_id FROM legacy_messages WHERE source='codex'`)
	if err != nil {
		return 0, err
	}
	type removal struct {
		rowid     int64
		sessionID string
	}
	var removals []removal
	for rows.Next() {
		var row removal
		var uuid string
		if err = rows.Scan(&row.rowid, &uuid, &row.sessionID); err != nil {
			rows.Close()
			return 0, err
		}
		matches := evidence[uuid]
		if len(matches) == 1 && matches[0].Input == inputIndex && matches[0].Group == groupIndex {
			removals = append(removals, row)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	for _, row := range removals {
		if _, err = tx.Exec(`DELETE FROM legacy_messages WHERE legacy_rowid=?`, row.rowid); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(`DELETE FROM legacy_sessions WHERE source='codex' AND session_id=? AND NOT EXISTS(SELECT 1 FROM legacy_messages WHERE source='codex' AND session_id=?)`, row.sessionID, row.sessionID); err != nil {
			return 0, err
		}
	}
	if err = checkMigrationSnapshot(adapter, scan, g); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(removals), nil
}
