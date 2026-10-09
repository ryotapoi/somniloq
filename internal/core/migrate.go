package core

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

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
	candidates := migrationGroupCandidates(evidence)
	successful := map[string]map[int]bool{}
	for i, scan := range scans {
		if len(scan.Errors) > 0 {
			result.Errors = append(result.Errors, scan.Errors...)
			result.GroupsFailed += max(1, len(scan.Groups))
			continue
		}
		for j, g := range scan.Groups {
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
				err = checkMigrationSnapshot(adapter, scan, g)
			}
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
			if err == nil {
				hasBody := false
				for _, m := range body.Messages {
					if m.Membership == "body" && strings.TrimSpace(m.Content) != "" {
						hasBody = true
						break
					}
				}
				if !hasBody {
					err = checkEmptyMigrationGroup(db, scan.Input, body)
					if err == nil {
						err = checkMigrationSnapshot(adapter, scan, g)
					}
					if err == nil {
						result.GroupsSkipped++
						result.Skips = append(result.Skips, fmt.Errorf("%s: no_own_messages", g.Session.SessionID))
						continue
					}
				}
			}
			removed := 0
			if err == nil {
				removed, err = replaceMigrationGroup(db, scan.Input, body, candidates[[2]int{i, j}], adapter, scan, g, importedAt)
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
	conflicting := map[string]bool{}
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
		if len(matches) > 1 {
			conflicting[id] = true
		}
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
		if conflicting[id] {
			result.LegacyReplacementFailures++
			result.Errors = append(result.Errors, fmt.Errorf("%s: input_membership_conflict", id))
		} else {
			result.LegacyRetentionWarnings++
			result.Warnings = append(result.Warnings, fmt.Errorf("%s: old_input_membership_unknown", id))
		}
	}
	if err = db.db.QueryRow(`SELECT count(*) FROM legacy_messages`).Scan(&result.LegacyMessagesRetained); err != nil {
		return err
	}
	return db.db.QueryRow(`SELECT count(*) FROM legacy_sessions`).Scan(&result.LegacyConversationsRetained)
}

// Empty input is a safe no-op only when neither its physical rows nor its
// owner has history to protect. Do not create a session or advance a cursor.
func checkEmptyMigrationGroup(db *DB, input Input, g codex.Group) error {
	var protected bool
	err := db.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM legacy_sessions WHERE source='codex' AND session_id=?) OR EXISTS(SELECT 1 FROM messages m JOIN inputs i ON i.id=m.input_id WHERE i.input_key=? AND m.source='codex' AND m.identity=?)`, g.Session.SessionID, InputKey(input.Source, input.Root), rootIdentity(g.Session.SessionID)).Scan(&protected)
	if err != nil {
		return err
	}
	if protected {
		return fmt.Errorf("%s: no_own_messages", g.Session.SessionID)
	}
	for _, report := range g.Reports {
		if err = db.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM import_state s JOIN inputs i ON i.id=s.input_id WHERE i.input_key=? AND s.jsonl_path=?)`, InputKey(input.Source, input.Root), report.Path).Scan(&protected); err != nil {
			return err
		}
		if protected {
			return fmt.Errorf("%s: no_own_messages", g.Session.SessionID)
		}
		for _, line := range report.Lines {
			if err = db.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM legacy_messages WHERE source='codex' AND uuid=?)`, line.UUID).Scan(&protected); err != nil {
				return err
			}
			if protected {
				return fmt.Errorf("%s: no_own_messages", g.Session.SessionID)
			}
		}
	}
	return nil
}

func sameMigrationReports(index, body []codex.FileReport) bool {
	if len(index) != len(body) {
		return false
	}
	for i := range index {
		if index[i].Path != body[i].Path || index[i].Hash != body[i].Hash {
			return false
		}
	}
	return true
}

// Candidates must be assigned only after evidence from all inputs, including
// failed groups, is complete. Physical duplicate/context lines remain evidence.
func migrationGroupCandidates(evidence map[string][]migrationEvidence) map[[2]int][]string {
	candidates := map[[2]int][]string{}
	for uuid, matches := range evidence {
		if len(matches) == 1 {
			owner := [2]int{matches[0].Input, matches[0].Group}
			candidates[owner] = append(candidates[owner], uuid)
		}
	}
	return candidates
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
	buf := make([]byte, 32*1024)
	for _, report := range group.Reports {
		file, err := os.Open(report.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", report.Path, err)
		}
		hash := sha256.New()
		// Hide File.WriteTo so CopyBuffer reuses the buffer across files.
		_, err = io.CopyBuffer(hash, struct{ io.Reader }{file}, buf)
		file.Close()
		if err != nil {
			return fmt.Errorf("%s: %w", report.Path, err)
		}
		if !slices.Equal(hash.Sum(nil), report.Hash[:]) {
			return fmt.Errorf("%s: rollout changed during migration", report.Path)
		}
	}
	return nil
}

func replaceMigrationGroup(db *DB, input Input, g codex.Group, uuids []string, adapter codex.Adapter, scan migrationInput, index codex.Group, importedAt string) (int, error) {
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
	type removal struct {
		rowid     int64
		sessionID string
	}
	var removals []removal
	for _, uuid := range uuids {
		var row removal
		err = tx.QueryRow(`SELECT legacy_rowid,session_id FROM legacy_messages WHERE uuid=? AND source='codex'`, uuid).Scan(&row.rowid, &row.sessionID)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return 0, err
		}
		removals = append(removals, row)
	}
	for _, row := range removals {
		if _, err = tx.Exec(`DELETE FROM legacy_messages WHERE legacy_rowid=?`, row.rowid); err != nil {
			return 0, err
		}
		if _, err = tx.Exec(`DELETE FROM legacy_sessions WHERE source='codex' AND session_id=? AND NOT EXISTS(SELECT 1 FROM legacy_messages WHERE source='codex' AND session_id=?)`, row.sessionID, row.sessionID); err != nil {
			return 0, err
		}
	}
	if err = checkMigrationSnapshot(adapter, scan, index); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return len(removals), nil
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
