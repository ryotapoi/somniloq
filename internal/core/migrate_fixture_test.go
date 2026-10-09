package core

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

const migrationFixtureDir = "../ingest/testdata/v0.14.0-migration"

type migrationOracle struct {
	MessageUUIDs map[string]string     `json:"message_uuids"`
	Cases        []migrationOracleCase `json:"cases"`
}
type migrationOracleCase struct {
	Name     string   `json:"name"`
	Files    []string `json:"files"`
	Mutation struct {
		SQL         string          `json:"sql"`
		File        string          `json:"file"`
		Text        string          `json:"text"`
		Append      string          `json:"append"`
		Prepend     json.RawMessage `json:"prepend"`
		RemoveLines []int           `json:"remove_lines"`
	} `json:"mutation"`
	Expected struct {
		NewIdentity     []string `json:"new_identity"`
		NewText         []string `json:"new_text"`
		Removed         []int    `json:"removed_legacy_rowids"`
		Retained        []int    `json:"retained_legacy_rowids"`
		Context         []string `json:"inherited_context"`
		Status          string   `json:"replacement_status"`
		Reason          string   `json:"reason"`
		LegacyPreserved bool     `json:"legacy_preserved"`
		Saved           bool     `json:"new_conversation_saved"`
		CopySkipped     bool     `json:"copy_skipped"`
		Reprocessed     bool     `json:"all_current_codex_groups_reprocessed"`
	} `json:"expected"`
}

func readMigrationOracle(t *testing.T) migrationOracle {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(migrationFixtureDir, "expected.json"))
	if err != nil {
		t.Fatal(err)
	}
	var oracle migrationOracle
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	return oracle
}

// Physical UUIDs change when synthetic rollouts move into the temporary root.
// This encodes the historical UUID contract, rather than migration's ownership rule.
func migrationFixtureUUID(path string, line int) string {
	return fmt.Sprintf("codex:%x", sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", path, line))))
}

type migrationFixture struct {
	from, destination, root, shared string
	inputs                          []Input
	before                          []byte
}

func setupMigrationFixture(t *testing.T, oracle migrationOracle, c migrationOracleCase) migrationFixture {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := migrationFixture{root: filepath.Join(base, "input-a"), shared: filepath.Join(base, "shared"), destination: filepath.Join(base, "new.db")}
	for _, root := range []string{f.root, f.shared} {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.inputs = []Input{{Source: SourceCodex, Root: f.root}, {Source: SourceCodex, Root: f.shared}}
	script, err := os.ReadFile(filepath.Join(migrationFixtureDir, "legacy.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(script)
	mapping := map[string]struct {
		name string
		line int
	}{"1": {"01-child.jsonl", 3}, "2": {"01-child.jsonl", 4}, "3": {"absent.jsonl", 2}, "4": {"02-multi.jsonl", 2}, "5": {"03-multi.jsonl", 3}, "6": {"04-conflict.jsonl", 2}}
	for row, loc := range mapping {
		root := f.root
		if row == "6" {
			root = f.shared
		}
		text = strings.ReplaceAll(text, oracle.MessageUUIDs[row], migrationFixtureUUID(filepath.Join(root, loc.name), loc.line))
	}
	// Old cursor paths are deliberately not copied into the destination.
	text = strings.ReplaceAll(text, "/fixture/input-a", f.root)
	text = strings.ReplaceAll(text, "/fixture/shared", f.shared)
	text += c.Mutation.SQL
	f.from = makeLegacySnapshot(t, text)
	files := c.Files
	switch c.Name {
	case "rollout_payload_conflict", "invalid_json":
		files = []string{"02-multi.jsonl", "03-multi.jsonl"}
	case "unattributed_before_metadata":
		files = []string{"01-child.jsonl"}
	case "same_snapshot_retry":
		files = []string{"02-multi.jsonl", "03-multi.jsonl"}
	}
	for _, name := range files {
		data, err := os.ReadFile(filepath.Join(migrationFixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if c.Mutation.File == name {
			if c.Mutation.Text != "" {
				data = bytes.Replace(data, []byte(`"text":"first"`), []byte(`"text":"`+c.Mutation.Text+`"`), 1)
			}
			if len(c.Mutation.Prepend) > 0 {
				var compact bytes.Buffer
				if err := json.Compact(&compact, c.Mutation.Prepend); err != nil {
					t.Fatal(err)
				}
				data = append(append(compact.Bytes(), '\n'), data...)
			}
			if c.Mutation.Append != "" {
				data = append(data, []byte(c.Mutation.Append)...)
			}
			if len(c.Mutation.RemoveLines) > 0 {
				lines := strings.SplitAfter(string(data), "\n")
				var kept strings.Builder
				for i, line := range lines {
					remove := false
					for _, n := range c.Mutation.RemoveLines {
						if i+1 == n {
							remove = true
						}
					}
					if !remove {
						kept.WriteString(line)
					}
				}
				data = []byte(kept.String())
			}
		}
		root := f.root
		if name == "04-conflict.jsonl" {
			root = f.shared
		}
		if err = os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if c.Name == "conflicting_input_evidence" {
		f.inputs[0].Root = base
	}
	f.before, err = os.ReadFile(f.from)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func assertMigrationSourceUnchanged(t *testing.T, f migrationFixture) {
	t.Helper()
	after, err := os.ReadFile(f.from)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.before, after) {
		t.Fatal("source bytes changed")
	}
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err = os.Stat(f.from + suffix); !os.IsNotExist(err) {
			t.Fatalf("source sidecar %s: %v", suffix, err)
		}
	}
}

func migrationQueryStrings(t *testing.T, db *DB, query string, args ...any) []string {
	t.Helper()
	rows, err := db.db.Query(query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		out = append(out, value)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertMigrationOracle(t *testing.T, db *DB, c migrationOracleCase, result *MigrationResult) {
	t.Helper()
	for _, id := range c.Expected.Removed {
		var n int
		if err := db.db.QueryRow(`SELECT count(*) FROM legacy_messages WHERE legacy_rowid=?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("removed row %d remains", id)
		}
	}
	for _, id := range c.Expected.Retained {
		var n int
		if err := db.db.QueryRow(`SELECT count(*) FROM legacy_messages WHERE legacy_rowid=?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("retained row %d missing", id)
		}
	}
	if c.Expected.NewIdentity != nil {
		got := migrationQueryStrings(t, db, `SELECT session_id FROM sessions ORDER BY session_id`)
		if !reflect.DeepEqual(got, c.Expected.NewIdentity) {
			t.Errorf("identities=%v want=%v", got, c.Expected.NewIdentity)
		}
	}
	if c.Expected.NewText != nil {
		got := migrationQueryStrings(t, db, `SELECT content FROM messages WHERE membership='body' ORDER BY number`)
		if !reflect.DeepEqual(got, c.Expected.NewText) {
			t.Errorf("body=%v want=%v", got, c.Expected.NewText)
		}
	}
	if c.Expected.Context != nil {
		got := migrationQueryStrings(t, db, `SELECT content FROM messages WHERE membership='context' ORDER BY origin_path,origin_line`)
		if !reflect.DeepEqual(got, c.Expected.Context) {
			t.Errorf("context=%v want=%v", got, c.Expected.Context)
		}
	}
	if c.Expected.Status == "replaced" && (result.GroupsReplaced != 1 || result.GroupsSkipped != 0 || len(result.Errors) != 0 || len(result.Warnings) != 0) {
		t.Errorf("replacement result=%+v", result)
	}
	if c.Expected.Status == "failed" && len(result.Errors) == 0 {
		t.Error("failed replacement reported success")
	}
	if c.Expected.Reason != "" {
		found := false
		diagnostics := append(append(append([]error{}, result.Errors...), result.Warnings...), result.Skips...)
		for _, err := range diagnostics {
			if strings.Contains(err.Error(), c.Expected.Reason) {
				found = true
			}
		}
		if !found {
			t.Errorf("errors=%v missing %q", result.Errors, c.Expected.Reason)
		}
	}
	if c.Expected.Saved {
		var n int
		if err := db.db.QueryRow(`SELECT count(*) FROM sessions WHERE session_id='child'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Error("new owner not saved")
		}
	}
	if c.Expected.LegacyPreserved {
		id := "missing"
		if c.Name == "unattributed_old_same_id" {
			id = "child"
		}
		var n int
		if err := db.db.QueryRow(`SELECT count(*) FROM legacy_sessions WHERE session_id=? AND source='codex'`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("legacy %s missing", id)
		}
	}
}

func TestMigrateFixtureReplacementOracle(t *testing.T) {
	oracle := readMigrationOracle(t)
	excluded := map[string]bool{"initial_copy_interrupt": true, "different_snapshot_retry": true, "ordinary_new_destination": true, "unknown_schema": true}
	for _, c := range oracle.Cases {
		if excluded[c.Name] {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			f := setupMigrationFixture(t, oracle, c)
			result, err := Migrate(f.from, f.destination, f.inputs)
			if err != nil {
				t.Fatal(err)
			}
			if !result.CopyPerformed {
				t.Fatal("initial copy skipped")
			}
			assertMigrationSourceUnchanged(t, f)
			db, err := OpenDB(f.destination)
			if err != nil {
				t.Fatal(err)
			}
			assertMigrationOracle(t, db, c, result)
			if c.Name == "child_physical_not_owner" {
				var number int
				if err = db.db.QueryRow(`SELECT number FROM legacy_messages WHERE legacy_rowid=3`).Scan(&number); err != nil {
					t.Fatal(err)
				}
				if number != 3 {
					t.Fatalf("partial legacy renumbered to %d", number)
				}
			}
			if c.Name == "other_sources" {
				var timestamp any
				if err = db.db.QueryRow(`SELECT timestamp FROM legacy_messages WHERE legacy_rowid=8`).Scan(&timestamp); err != nil {
					t.Fatal(err)
				}
				if timestamp != nil {
					t.Fatalf("unknown timestamp promoted: %v", timestamp)
				}
			}
			if len(result.Errors) == 0 && result.GroupsSkipped == 0 {
				for _, input := range f.inputs {
					files, errs := codex.NewAdapter(ResolveRepoPath).ScanFiles(input.Root)
					if len(errs) > 0 {
						t.Fatal(errs)
					}
					for _, path := range files {
						var offset int64
						if err = db.db.QueryRow(`SELECT last_offset FROM import_state WHERE jsonl_path=?`, path).Scan(&offset); err != nil {
							t.Fatal(err)
						}
						info, err := os.Stat(path)
						if err != nil {
							t.Fatal(err)
						}
						if offset != info.Size() {
							t.Errorf("cursor %s=%d want=%d", path, offset, info.Size())
						}
					}
				}
			}
			db.Close()
			if c.Expected.CopySkipped {
				again, err := Migrate(f.from, f.destination, f.inputs)
				if err != nil {
					t.Fatal(err)
				}
				if again.CopyPerformed {
					t.Fatal("retry recopied legacy")
				}
				if c.Expected.Reprocessed && again.GroupsReplaced != result.GroupsReplaced {
					t.Fatalf("retry groups=%d first=%d", again.GroupsReplaced, result.GroupsReplaced)
				}
				retryDB, err := OpenDB(f.destination)
				must(t, err)
				assertMigrationOracle(t, retryDB, c, again)
				retryDB.Close()
				assertMigrationSourceUnchanged(t, f)
			}
		})
	}
}

func TestMigrateOwnerFailurePreservesPriorState(t *testing.T) {
	for _, kind := range []string{"partial_tail", "metadata_only_sibling", "save_failure", "changed_rollout", "missing_rollout", "unreadable_rollout", "cyclic_parent"} {
		t.Run(kind, func(t *testing.T) {
			oracle := readMigrationOracle(t)
			c := migrationOracleCase{Name: "multiple_rollouts", Files: []string{"02-multi.jsonl", "03-multi.jsonl"}}
			f := setupMigrationFixture(t, oracle, c)
			result, err := Migrate(f.from, f.destination, f.inputs)
			if err != nil || len(result.Errors) != 0 {
				t.Fatalf("seed: %v %v", result, err)
			}
			db, err := OpenDB(f.destination)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			beforeBody := migrationQueryStrings(t, db, `SELECT printf('%d:%s:%s',number,content,membership) FROM messages ORDER BY number`)
			beforeCursor := migrationQueryStrings(t, db, `SELECT printf('%s:%d:%d:%s',jsonl_path,file_size,last_offset,content_hash) FROM import_state ORDER BY jsonl_path`)
			// Restore unmatched same-ID history so rollback protects copied history as well.
			_, err = db.db.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,imported_at) SELECT snapshot_sha256,'codex','multi','' FROM migration_origin; INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,number) SELECT 4,snapshot_sha256,?,'codex','multi','user','saved',1 FROM migration_origin`, "unproven-old-row")
			if err != nil {
				t.Fatal(err)
			}
			adapter := codex.NewAdapter(ResolveRepoPath)
			files, errs := adapter.ScanFiles(f.root)
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			if kind == "partial_tail" {
				path := filepath.Join(f.root, "03-multi.jsonl")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte(`{"type":`)...)
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "metadata_only_sibling" {
				if err = os.WriteFile(filepath.Join(f.root, "04-meta.jsonl"), []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"multi\"}}\n"), 0600); err != nil {
					t.Fatal(err)
				}
				files, errs = adapter.ScanFiles(f.root)
				if len(errs) > 0 {
					t.Fatal(errs)
				}
			}
			if kind == "cyclic_parent" {
				inputID, err := db.EnsureInput(f.inputs[0])
				must(t, err)
				must(t, db.UpsertSession(inputID, SessionMeta{Source: SourceCodex, SessionID: "peer", ParentSessionID: "multi"}, "saved"))
				for _, path := range files {
					data, err := os.ReadFile(path)
					must(t, err)
					data = []byte(strings.ReplaceAll(string(data), `"id":"multi"`, `"id":"multi","source":{"subagent":{"thread_spawn":{"parent_thread_id":"peer"}}}`))
					must(t, os.WriteFile(path, data, 0600))
				}
			}
			groups, errs := adapter.BuildMigrationGroups(f.root, files, "2026-01-01T00:00:00Z")
			if len(errs) > 0 {
				t.Fatal(errs)
			}
			switch kind {
			case "save_failure":
				_, err = db.db.Exec(`CREATE TRIGGER reject_migration BEFORE INSERT ON messages BEGIN SELECT RAISE(ABORT,'forced save failure'); END;`)
			case "changed_rollout":
				err = os.WriteFile(files[0], []byte("{}\n"), 0600)
			case "missing_rollout":
				err = os.Remove(files[0])
			case "unreadable_rollout":
				if err = os.Remove(files[0]); err == nil {
					err = os.Symlink("missing-target", files[0])
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			r := &MigrationResult{SnapshotSHA256: result.SnapshotSHA256}
			err = replaceMigrationGroups(db, []migrationInput{{Input: f.inputs[0], Groups: groups}}, adapter, "2026-01-01T00:00:00Z", r)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "metadata_only_sibling" {
				if r.GroupsReplaced != 1 || r.GroupsFailed != 0 {
					t.Fatalf("metadata-only sibling: %+v", r)
				}
				var n int
				if err = db.db.QueryRow(`SELECT count(*) FROM import_state`).Scan(&n); err != nil {
					t.Fatal(err)
				}
				if n != 3 {
					t.Fatalf("group cursors=%d want 3", n)
				}
				return
			}
			if kind == "cyclic_parent" && (len(r.Errors) != 1 || !strings.Contains(r.Errors[0].Error(), "cycle")) {
				t.Fatalf("expected strict cycle rejection: %+v", r.Errors)
			}
			if kind == "unreadable_rollout" && (len(r.Errors) != 1 || !errors.Is(r.Errors[0], os.ErrNotExist) || !strings.Contains(r.Errors[0].Error(), files[0])) {
				t.Fatalf("expected rollout read error: %+v", r.Errors)
			}
			if r.GroupsFailed != 1 || r.GroupsReplaced != 0 {
				t.Fatalf("failure result=%+v", r)
			}
			if got := migrationQueryStrings(t, db, `SELECT printf('%d:%s:%s',number,content,membership) FROM messages ORDER BY number`); !reflect.DeepEqual(got, beforeBody) {
				t.Fatalf("body changed: %v", got)
			}
			if got := migrationQueryStrings(t, db, `SELECT printf('%s:%d:%d:%s',jsonl_path,file_size,last_offset,content_hash) FROM import_state ORDER BY jsonl_path`); !reflect.DeepEqual(got, beforeCursor) {
				t.Fatalf("cursors changed: %v", got)
			}
			var retained int
			if err = db.db.QueryRow(`SELECT count(*) FROM legacy_messages WHERE legacy_rowid=4`).Scan(&retained); err != nil {
				t.Fatal(err)
			}
			if retained != 1 {
				t.Fatal("legacy removed on failed group")
			}
			assertMigrationSourceUnchanged(t, f)
		})
	}
}

func TestMigrateMissingRolloutBeforeRetryPreservesPriorState(t *testing.T) {
	for _, membership := range []string{"body", "context"} {
		t.Run(membership, func(t *testing.T) {
			c := migrationOracleCase{Files: []string{"02-multi.jsonl", "03-multi.jsonl"}}
			c.Mutation.SQL = "DELETE FROM messages; DELETE FROM sessions; DELETE FROM import_state;"
			f := setupMigrationFixture(t, readMigrationOracle(t), c)
			a := filepath.Join(f.root, "02-multi.jsonl")
			b := filepath.Join(f.root, "03-multi.jsonl")
			if membership == "context" {
				for _, path := range []string{a, b} {
					data, err := os.ReadFile(path)
					must(t, err)
					text := strings.ReplaceAll(string(data), `"id":"multi"`, `"id":"multi","subagent_history_start_ordinal":1,"source":{"subagent":{"thread_spawn":{"parent_thread_id":"parent"}}}`)
					text = strings.ReplaceAll(text, `"type":"response_item",`, `"type":"response_item","ordinal":1,`)
					text = strings.ReplaceAll(text, `"ordinal":1,"payload":{"type":"message","role":"assistant","id":"m2"`, `"ordinal":0,"payload":{"type":"message","role":"assistant","id":"m2"`)
					must(t, os.WriteFile(path, []byte(text), 0600))
				}
			}
			// A second input has the same owner ID but a different rollout set.
			peer := filepath.Join(f.shared, "peer.jsonl")
			peerData := "{\"type\":\"session_meta\",\"payload\":{\"id\":\"multi\"}}\n{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"user\",\"content\":[{\"type\":\"text\",\"text\":\"peer\"}]}}\n"
			must(t, os.WriteFile(peer, []byte(peerData), 0600))
			result, err := Migrate(f.from, f.destination, f.inputs)
			if err != nil || len(result.Errors) != 0 || !result.CopyPerformed || result.GroupsReplaced != 2 {
				t.Fatalf("initial: %+v %v", result, err)
			}
			result, err = Migrate(f.from, f.destination, f.inputs)
			if err != nil || len(result.Errors) != 0 || result.CopyPerformed || result.GroupsReplaced != 2 {
				t.Fatalf("complete retry: %+v %v", result, err)
			}
			db, err := OpenDB(f.destination)
			must(t, err)
			defer db.Close()
			inputID, err := db.EnsureInput(f.inputs[0])
			must(t, err)
			gotMembership := migrationQueryStrings(t, db, `SELECT membership FROM messages WHERE input_id=? AND origin_path='03-multi.jsonl'`, inputID)
			if !reflect.DeepEqual(gotMembership, []string{membership}) {
				t.Fatalf("missing rollout membership: %v", gotMembership)
			}
			// Seed proven old rows after the first copy; receipt retry cannot restore them.
			_, err = db.db.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,imported_at) SELECT snapshot_sha256,'codex','multi','saved' FROM migration_origin; INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,number) SELECT 4,snapshot_sha256,?,'codex','multi','user','saved',1 FROM migration_origin`, migrationFixtureUUID(a, 2))
			must(t, err)
			queries := []string{
				`SELECT json_array(input_id,uuid,source,session_id,identity,parent_uuid,role,content,blocks_json,timestamp,is_sidechain,number,origin_path,origin_line,membership,payload_id) FROM messages WHERE input_id=? ORDER BY identity,uuid`,
				`SELECT json_array(input_id,source,session_id,identity,parent_session_id,parent_identity,root_identity,cwd,repo_path,git_branch,custom_title,agent_name,version,started_at,ended_at,imported_at) FROM sessions WHERE input_id=? ORDER BY identity`,
				`SELECT json_array(input_id,jsonl_path,source,file_size,last_offset,imported_at,content_hash) FROM import_state WHERE input_id=? ORDER BY jsonl_path`,
			}
			before := make([][]string, len(queries))
			for i, query := range queries {
				before[i] = migrationQueryStrings(t, db, query, inputID)
			}
			missing, err := os.ReadFile(b)
			must(t, err)
			must(t, os.Remove(b))
			data, err := os.ReadFile(a)
			must(t, err)
			must(t, os.WriteFile(a, bytes.ReplaceAll(data, []byte(`"text":"first"`), []byte(`"text":"updated first"`)), 0600))
			must(t, os.WriteFile(peer, []byte(strings.ReplaceAll(peerData, `"text":"peer"`, `"text":"updated peer"`)), 0600))
			result, err = Migrate(f.from, f.destination, f.inputs)
			if err != nil || result.CopyPerformed || result.GroupsFailed != 1 || result.GroupsReplaced != 1 || result.LegacyRetentionWarnings != 0 || result.LegacyMessagesRemoved != 1 || len(result.Errors) != 1 || !strings.Contains(result.Errors[0].Error(), "rollout missing") {
				t.Fatalf("missing retry: %+v %v", result, err)
			}
			for i, query := range queries {
				if got := migrationQueryStrings(t, db, query, inputID); !reflect.DeepEqual(got, before[i]) {
					t.Fatalf("owner state changed (%d): %v", i, got)
				}
			}
			// The independent successful owner has the same ID and therefore
			// removes same-ID legacy, while the failed input keeps its saved state.
			if got := migrationQueryStrings(t, db, `SELECT uuid FROM legacy_messages`); len(got) != 0 {
				t.Fatalf("same-ID legacy remains: %v", got)
			}
			if got := migrationQueryStrings(t, db, `SELECT content FROM messages WHERE input_id<>?`, inputID); !reflect.DeepEqual(got, []string{"updated peer"}) {
				t.Fatalf("independent same-ID owner did not progress: %v", got)
			}
			assertMigrationSourceUnchanged(t, f)
			must(t, os.WriteFile(b, bytes.ReplaceAll(missing, []byte(`"text":"first"`), []byte(`"text":"updated first"`)), 0600))
			result, err = Migrate(f.from, f.destination, f.inputs)
			if err != nil || len(result.Errors) != 0 || result.CopyPerformed || result.GroupsReplaced != 2 || result.LegacyMessagesRemoved != 0 {
				t.Fatalf("restored retry: %+v %v", result, err)
			}
			assertMigrationSourceUnchanged(t, f)
		})
	}
}

func TestMigrateEarlierRolloutAndNormalFullPreserveLegacy(t *testing.T) {
	oracle := readMigrationOracle(t)
	f := setupMigrationFixture(t, oracle, migrationOracleCase{Name: "multiple_rollouts", Files: []string{"02-multi.jsonl", "03-multi.jsonl"}})
	result, err := Migrate(f.from, f.destination, f.inputs)
	if err != nil || len(result.Errors) > 0 {
		t.Fatalf("seed: %v %v", result, err)
	}
	earlier := "{\"type\":\"session_meta\",\"payload\":{\"id\":\"multi\"}}\n{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"role\":\"assistant\",\"id\":\"m0\",\"content\":[{\"type\":\"output_text\",\"text\":\"earlier\"}]}}\n"
	if err = os.WriteFile(filepath.Join(f.root, "00-earlier.jsonl"), []byte(earlier), 0600); err != nil {
		t.Fatal(err)
	}
	result, err = Migrate(f.from, f.destination, f.inputs)
	if err != nil || len(result.Errors) > 0 {
		t.Fatalf("retry: %v %v", result, err)
	}
	db, err := OpenDB(f.destination)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	expected := []string{"1:earlier", "2:first", "3:second"}
	if got := migrationQueryStrings(t, db, `SELECT printf('%d:%s',number,content) FROM messages WHERE membership='body' ORDER BY number`); !reflect.DeepEqual(got, expected) {
		t.Fatalf("canonical=%v", got)
	}
	before := migrationQueryStrings(t, db, `SELECT printf('%d:%s:%d',legacy_rowid,content,number) FROM legacy_messages ORDER BY legacy_rowid`)
	runCodexImport(t, db, f.root, true)
	if got := migrationQueryStrings(t, db, `SELECT printf('%d:%s:%d',legacy_rowid,content,number) FROM legacy_messages ORDER BY legacy_rowid`); !reflect.DeepEqual(got, before) {
		t.Fatalf("normal full changed legacy: %v", got)
	}
	if got := migrationQueryStrings(t, db, `SELECT printf('%d:%s',number,content) FROM messages WHERE membership='body' ORDER BY number`); !reflect.DeepEqual(got, expected) {
		t.Fatalf("normal full canonical=%v", got)
	}
	assertMigrationSourceUnchanged(t, f)
}

func TestMigrateSameIDDuplicateUUIDAndIndependentFailure(t *testing.T) {
	oracle := readMigrationOracle(t)
	f := setupMigrationFixture(t, oracle, migrationOracleCase{Name: "multiple_rollouts", Files: []string{"01-child.jsonl", "02-multi.jsonl", "03-multi.jsonl"}})
	// The old row points at the payload duplicate that disappears from canonical
	// body. Same-ID replacement must not depend on that physical UUID.
	snapshot, err := sql.Open("sqlite", f.from)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = snapshot.Exec(`UPDATE messages SET uuid=? WHERE rowid=4`, migrationFixtureUUID(filepath.Join(f.root, "03-multi.jsonl"), 2)); err != nil {
		t.Fatal(err)
	}
	if err = snapshot.Close(); err != nil {
		t.Fatal(err)
	}
	f.before, err = os.ReadFile(f.from)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(f.root, "01-child.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(data, []byte("{broken\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := Migrate(f.from, f.destination, f.inputs)
	if err != nil {
		t.Fatal(err)
	}
	if result.GroupsReplaced != 1 || result.GroupsFailed != 1 {
		t.Fatalf("independent groups=%+v", result)
	}
	db, err := OpenDB(f.destination)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := migrationOracleCase{}
	c.Expected.Removed = []int{4, 5}
	c.Expected.Retained = []int{1, 2, 3}
	c.Expected.NewText = []string{"first", "second"}
	assertMigrationOracle(t, db, c, result)
	assertMigrationSourceUnchanged(t, f)
}

func TestMigrateEmptyOwnerReplacesSavedHistory(t *testing.T) {
	for _, kind := range []string{"body", "cursor", "physical_legacy", "same_id_legacy", "same_id_message", "metadata_only"} {
		t.Run(kind, func(t *testing.T) {
			c := migrationOracleCase{Files: []string{"05-inherited-only.jsonl"}}
			f := setupMigrationFixture(t, readMigrationOracle(t), c)
			path := filepath.Join(f.root, "05-inherited-only.jsonl")
			empty, err := os.ReadFile(path)
			must(t, err)
			must(t, os.WriteFile(path, bytes.ReplaceAll(empty, []byte(`"subagent_history_start_ordinal":82`), []byte(`"subagent_history_start_ordinal":3`)), 0600))
			result, err := Migrate(f.from, f.destination, f.inputs)
			must(t, err)
			if len(result.Errors) != 0 {
				t.Fatalf("seed: %+v", result)
			}
			db, err := OpenDB(f.destination)
			must(t, err)
			defer db.Close()
			switch kind {
			case "body":
				_, err = db.db.Exec(`DELETE FROM import_state`)
			case "cursor":
				_, err = db.db.Exec(`DELETE FROM messages`)
			case "physical_legacy":
				_, err = db.db.Exec(`UPDATE legacy_messages SET uuid=? WHERE legacy_rowid=1`, migrationFixtureUUID(path, 2))
			case "same_id_legacy", "same_id_message", "metadata_only":
				_, err = db.db.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,imported_at) SELECT snapshot_sha256,'codex','inherited-only','' FROM migration_origin`)
				if err == nil && kind != "same_id_legacy" {
					_, err = db.db.Exec(`INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,number) SELECT 9,snapshot_sha256,'old-unmatched','codex','inherited-only','user','old body',1 FROM migration_origin`)
				}
			}
			must(t, err)
			_, err = db.db.Exec(`INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,imported_at) SELECT snapshot_sha256,'claude_code','inherited-only','' FROM migration_origin; INSERT INTO legacy_messages(legacy_rowid,snapshot_sha256,uuid,source,session_id,role,content,number) SELECT 10,snapshot_sha256,'other-source-same-id','claude_code','inherited-only','user','other source',1 FROM migration_origin`)
			must(t, err)
			if kind == "metadata_only" {
				empty = bytes.SplitAfterN(empty, []byte("\n"), 2)[0]
			}
			must(t, os.WriteFile(path, empty, 0600))
			fresh, err := OpenDB(filepath.Join(t.TempDir(), "fresh.db"))
			must(t, err)
			defer fresh.Close()
			runCodexImport(t, fresh, f.root, false)
			membership := `SELECT json_array(content,membership,origin_path,origin_line) FROM messages ORDER BY origin_path,origin_line`
			want := migrationQueryStrings(t, fresh, membership)
			queries := []string{membership, `SELECT json_array(jsonl_path,last_offset,content_hash) FROM import_state ORDER BY jsonl_path`, `SELECT json_array(identity,parent_identity,root_identity) FROM sessions ORDER BY identity`}
			var before [][]string
			for run := 0; run < 2; run++ {
				result, err = Migrate(f.from, f.destination, f.inputs)
				must(t, err)
				if result.CopyPerformed || result.GroupsReplaced != 1 || result.GroupsFailed != 0 || result.GroupsSkipped != 0 || len(result.Errors) != 0 {
					t.Fatalf("empty replacement: %+v", result)
				}
				wantRemoved := 0
				if run == 0 && (kind == "same_id_message" || kind == "metadata_only") {
					wantRemoved = 1
				}
				if result.LegacyMessagesRemoved != wantRemoved {
					t.Fatalf("removed=%d want=%d", result.LegacyMessagesRemoved, wantRemoved)
				}
				if got := migrationQueryStrings(t, db, membership); !reflect.DeepEqual(got, want) {
					t.Fatalf("membership=%v fresh=%v", got, want)
				}
				var n int
				must(t, db.db.QueryRow(`SELECT count(*) FROM legacy_sessions WHERE source='codex' AND session_id='inherited-only'`).Scan(&n))
				if n != 0 {
					t.Fatal("same-ID legacy remains")
				}
				must(t, db.db.QueryRow(`SELECT count(*) FROM legacy_messages WHERE source='claude_code' AND session_id='inherited-only'`).Scan(&n))
				if n != 1 {
					t.Fatal("other source same-ID legacy removed")
				}
				must(t, db.db.QueryRow(`SELECT count(*) FROM sessions WHERE session_id='inherited-only' AND parent_session_id='parent' AND parent_identity='["parent"]'`).Scan(&n))
				if n != 1 {
					t.Fatal("owner relation missing")
				}
				must(t, db.db.QueryRow(`SELECT count(*) FROM messages WHERE membership='body'`).Scan(&n))
				if n != 0 {
					t.Fatal("old body remains")
				}
				var offset int64
				must(t, db.db.QueryRow(`SELECT last_offset FROM import_state WHERE jsonl_path=?`, path).Scan(&offset))
				if offset != int64(len(empty)) {
					t.Fatalf("cursor=%d", offset)
				}
				for i, q := range queries {
					got := migrationQueryStrings(t, db, q)
					if run == 0 {
						before = append(before, got)
					} else if !reflect.DeepEqual(got, before[i]) {
						t.Fatalf("retry changed state: %v", got)
					}
				}
			}
			assertMigrationSourceUnchanged(t, f)
		})
	}
}

func TestMigrateUnrelatedRolloutChangesDoNotInvalidateOwner(t *testing.T) {
	for _, mutation := range []string{"add", "remove"} {
		t.Run(mutation, func(t *testing.T) {
			f := setupMigrationFixture(t, readMigrationOracle(t), migrationOracleCase{Files: []string{"02-multi.jsonl", "03-multi.jsonl"}})
			unrelated := filepath.Join(f.root, "unrelated.jsonl")
			must(t, os.WriteFile(unrelated, []byte("{\"type\":\"session_meta\",\"payload\":{\"id\":\"unrelated\"}}\n"), 0600))
			db, digest, _, err := prepareMigration(f.from, f.destination)
			must(t, err)
			defer db.Close()
			adapter := codex.NewMigrationAdapter(ResolveRepoPath)
			files, errs := adapter.ScanFiles(f.root)
			if len(errs) != 0 {
				t.Fatal(errs)
			}
			groups, errs := adapter.BuildMigrationIndex(f.root, files, "now")
			if len(errs) != 0 {
				t.Fatal(errs)
			}
			if mutation == "add" {
				must(t, os.WriteFile(filepath.Join(f.root, "added.jsonl"), []byte("invalid\n"), 0600))
			} else {
				must(t, os.Remove(unrelated))
			}
			result := &MigrationResult{SnapshotSHA256: digest}
			must(t, replaceMigrationGroups(db, []migrationInput{{Input: f.inputs[0], Groups: groups}}, adapter, "now", result))
			wantFailed := 0
			if mutation == "remove" {
				wantFailed = 1
			}
			if result.GroupsFailed != wantFailed || result.GroupsReplaced != 2-wantFailed || result.LegacyMessagesRemoved != 2 {
				t.Fatalf("result=%+v", result)
			}
			if got := migrationQueryStrings(t, db, `SELECT content FROM messages WHERE membership='body' ORDER BY number`); !reflect.DeepEqual(got, []string{"first", "second"}) {
				t.Fatalf("body=%v", got)
			}
			assertMigrationSourceUnchanged(t, f)
		})
	}
}

func TestMigrateCommitFailureRollsBackOwnerAndKeepsPriorSuccess(t *testing.T) {
	f := setupMigrationFixture(t, readMigrationOracle(t), migrationOracleCase{Files: []string{"01-child.jsonl", "02-multi.jsonl", "03-multi.jsonl"}})
	db, digest, _, err := prepareMigration(f.from, f.destination)
	must(t, err)
	defer db.Close()
	// A deferred FK fails only at commit, after body, cursor and legacy deletion.
	_, err = db.db.Exec(`CREATE TABLE guard_parent(id TEXT PRIMARY KEY); CREATE TABLE commit_guard(owner TEXT REFERENCES guard_parent(id) DEFERRABLE INITIALLY DEFERRED); CREATE TRIGGER fail_multi AFTER INSERT ON sessions WHEN NEW.session_id='multi' BEGIN INSERT INTO commit_guard VALUES('missing'); END;`)
	must(t, err)
	adapter := codex.NewMigrationAdapter(ResolveRepoPath)
	files, errs := adapter.ScanFiles(f.root)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	groups, errs := adapter.BuildMigrationIndex(f.root, files, "now")
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	result := &MigrationResult{SnapshotSHA256: digest}
	must(t, replaceMigrationGroups(db, []migrationInput{{Input: f.inputs[0], Groups: groups}}, adapter, "now", result))
	if result.GroupsReplaced != 1 || result.GroupsFailed != 1 {
		t.Fatalf("result=%+v", result)
	}
	if !strings.Contains(result.Errors[0].Error(), "FOREIGN KEY") {
		t.Fatalf("not commit FK: %v", result.Errors)
	}
	for _, q := range []string{`SELECT count(*) FROM sessions WHERE session_id='multi'`, `SELECT count(*) FROM messages WHERE identity='["multi"]'`, `SELECT count(*) FROM import_state WHERE jsonl_path LIKE '%multi.jsonl'`, `SELECT count(*) FROM commit_guard`} {
		var n int
		must(t, db.db.QueryRow(q).Scan(&n))
		if n != 0 {
			t.Fatalf("partial state %s=%d", q, n)
		}
	}
	var n int
	must(t, db.db.QueryRow(`SELECT count(*) FROM legacy_messages WHERE session_id='multi'`).Scan(&n))
	if n != 2 {
		t.Fatalf("legacy=%d", n)
	}
	if got := migrationQueryStrings(t, db, `SELECT content FROM messages WHERE membership='body'`); !reflect.DeepEqual(got, []string{"child own"}) {
		t.Fatalf("prior success=%v", got)
	}
	_, err = db.db.Exec(`DROP TRIGGER fail_multi`)
	must(t, err)
	result = &MigrationResult{SnapshotSHA256: digest}
	must(t, replaceMigrationGroups(db, []migrationInput{{Input: f.inputs[0], Groups: groups}}, adapter, "now", result))
	if result.GroupsFailed != 0 {
		t.Fatalf("recovery=%+v", result)
	}
	assertMigrationSourceUnchanged(t, f)
}
