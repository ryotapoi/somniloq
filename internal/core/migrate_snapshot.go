package core

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const legacySnapshotSchema = `
CREATE TABLE sessions (source TEXT NOT NULL CHECK(source <> ''), session_id TEXT NOT NULL, cwd TEXT, repo_path TEXT, git_branch TEXT, custom_title TEXT, agent_name TEXT, version TEXT, started_at TEXT, ended_at TEXT, imported_at TEXT NOT NULL, PRIMARY KEY (source, session_id));
CREATE TABLE messages (uuid TEXT PRIMARY KEY, source TEXT NOT NULL CHECK(source <> ''), session_id TEXT NOT NULL, parent_uuid TEXT, role TEXT NOT NULL, content TEXT NOT NULL, timestamp TEXT NOT NULL, is_sidechain BOOLEAN DEFAULT FALSE, FOREIGN KEY (source, session_id) REFERENCES sessions(source, session_id));
CREATE INDEX messages_session_idx ON messages(source, session_id);
CREATE TABLE import_state (jsonl_path TEXT PRIMARY KEY, source TEXT NOT NULL CHECK(source <> ''), file_size INTEGER, last_offset INTEGER, imported_at TEXT NOT NULL);
`

func migrationPath(path string, existing bool) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	path, err = CanonicalPath(path, cwd)
	if err != nil {
		return "", err
	}
	if existing {
		if _, err := os.Stat(path); err != nil {
			return "", err
		}
	}
	return path, nil
}

func snapshotDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Normalize only SQL spelling; quoted values remain exact.
func snapshotSQL(sqlText string) string {
	var out strings.Builder
	var quote rune
	for _, r := range sqlText {
		if quote != 0 {
			out.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			out.WriteRune(r)
			continue
		}
		if !unicode.IsSpace(r) {
			out.WriteRune(unicode.ToLower(r))
		}
	}
	return out.String()
}

func validateLegacySnapshot(db *sql.DB) error {
	var revision int
	if err := db.QueryRow("PRAGMA user_version").Scan(&revision); err != nil {
		return err
	}
	objects, err := schemaObjects(db)
	if err != nil {
		return err
	}
	expected, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return err
	}
	defer expected.Close()
	if _, err = expected.Exec(legacySnapshotSchema); err != nil {
		return err
	}
	want, err := schemaObjects(expected)
	if err != nil {
		return err
	}
	mismatch := revision != 0 || len(objects) != len(want)
	if !mismatch {
		for i, item := range objects {
			if item[0] != want[i][0] || item[1] != want[i][1] || snapshotSQL(item[2]) != snapshotSQL(want[i][2]) {
				mismatch = true
				break
			}
		}
	}
	if mismatch {
		return &SchemaError{Revision: revision, Reason: "expected legacy-v013 snapshot"}
	}
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return err
	}
	for rows.Next() {
		var result string
		if err = rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if result != "ok" {
			rows.Close()
			return fmt.Errorf("snapshot integrity check failed: %s", result)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	bad := rows.Next()
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if bad {
		return fmt.Errorf("snapshot foreign key check failed")
	}
	for _, query := range []string{
		`SELECT COUNT(*) FROM sessions WHERE source NOT IN ('codex','claude_code','cursor_agent') OR source='' OR session_id=''`,
		`SELECT COUNT(*) FROM messages WHERE source NOT IN ('codex','claude_code','cursor_agent') OR source='' OR session_id='' OR uuid IS NULL OR uuid=''`,
		`SELECT COUNT(*) FROM import_state WHERE source NOT IN ('codex','claude_code','cursor_agent') OR source=''`,
	} {
		var count int
		if err = db.QueryRow(query).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("snapshot contains invalid source or identity")
		}
	}
	return nil
}

func copySnapshotRows(source *sql.DB, tx *sql.Tx, digest string) error {
	for _, item := range []struct {
		query, insert string
		columns       int
	}{
		{`SELECT source,session_id,cwd,repo_path,git_branch,custom_title,agent_name,version,started_at,ended_at,imported_at FROM sessions`, `INSERT INTO legacy_sessions(snapshot_sha256,source,session_id,cwd,repo_path,git_branch,custom_title,agent_name,version,started_at,ended_at,imported_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, 11},
		{`SELECT rowid,uuid,source,session_id,parent_uuid,role,content,NULLIF(timestamp,''),is_sidechain,ROW_NUMBER() OVER (PARTITION BY source,session_id ORDER BY rowid) FROM messages ORDER BY rowid`, `INSERT INTO legacy_messages(snapshot_sha256,legacy_rowid,uuid,source,session_id,parent_uuid,role,content,timestamp,is_sidechain,number) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, 10},
	} {
		rows, err := source.Query(item.query)
		if err != nil {
			return err
		}
		for rows.Next() {
			values := make([]any, item.columns+1)
			values[0] = digest
			pointers := make([]any, item.columns)
			for i := range pointers {
				pointers[i] = &values[i+1]
			}
			if err = rows.Scan(pointers...); err == nil {
				_, err = tx.Exec(item.insert, values...)
			}
			if err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// prepareMigration validates a fixed legacy snapshot before touching the destination.
func prepareMigration(from, destination string) (*DB, string, bool, error) {
	from, err := migrationPath(from, true)
	if err != nil {
		return nil, "", false, err
	}
	destination, err = migrationPath(destination, false)
	if err != nil {
		return nil, "", false, err
	}
	sourceInfo, err := os.Stat(from)
	if err != nil {
		return nil, "", false, err
	}
	if !sourceInfo.Mode().IsRegular() {
		return nil, "", false, fmt.Errorf("snapshot is not a regular file")
	}
	if from == destination {
		return nil, "", false, fmt.Errorf("snapshot and destination are the same file")
	}
	if info, err := os.Stat(destination); err == nil && os.SameFile(sourceInfo, info) {
		return nil, "", false, fmt.Errorf("snapshot and destination are the same file")
	} else if err != nil && !os.IsNotExist(err) {
		return nil, "", false, err
	}
	checkSidecars := func() error {
		for _, suffix := range []string{"-wal", "-journal", "-shm"} {
			if _, err := os.Lstat(from + suffix); err == nil {
				return fmt.Errorf("snapshot has sidecar %s", suffix)
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	if err = checkSidecars(); err != nil {
		return nil, "", false, err
	}
	digest, err := snapshotDigest(from)
	if err != nil {
		return nil, "", false, err
	}
	source, err := sql.Open("sqlite", (&url.URL{Scheme: "file", Path: from, RawQuery: "mode=ro&immutable=1"}).String())
	if err != nil {
		return nil, "", false, err
	}
	defer source.Close()
	source.SetMaxOpenConns(1)
	if err = validateLegacySnapshot(source); err != nil {
		return nil, "", false, err
	}
	if err = os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		return nil, "", false, err
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil && !os.IsExist(err) {
		return nil, "", false, err
	}
	if err == nil {
		if err = file.Close(); err != nil {
			return nil, "", false, err
		}
	}
	target, err := sql.Open("sqlite", destination)
	if err != nil {
		return nil, "", false, err
	}
	target.SetMaxOpenConns(1)
	var tx *sql.Tx
	fail := func(err error) (*DB, string, bool, error) {
		if tx != nil {
			tx.Rollback()
		}
		target.Close()
		return nil, "", false, err
	}
	if _, err = target.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return fail(err)
	}
	tx, err = target.Begin()
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	empty, err := inspectSchema(tx)
	if err != nil {
		return fail(err)
	}
	if empty {
		if _, err = tx.Exec(schema); err != nil {
			return fail(err)
		}
		if err = copySnapshotRows(source, tx, digest); err != nil {
			return fail(err)
		}
		if _, err = tx.Exec(`INSERT INTO migration_origin(id,snapshot_sha256,legacy_shape,copy_complete,snapshot_path) VALUES(1,?,'legacy-v013',1,?)`, digest, from); err != nil {
			return fail(err)
		}
	} else {
		var count int
		if err = tx.QueryRow("SELECT COUNT(*) FROM migration_origin").Scan(&count); err != nil {
			return fail(err)
		}
		if count != 1 {
			return fail(fmt.Errorf("destination has no completed copy receipt"))
		}
		var id, complete int
		var saved, shape, path string
		if err = tx.QueryRow("SELECT id,snapshot_sha256,legacy_shape,copy_complete,snapshot_path FROM migration_origin").Scan(&id, &saved, &shape, &complete, &path); err != nil {
			return fail(fmt.Errorf("invalid copy receipt: %w", err))
		}
		decoded, decodeErr := hex.DecodeString(saved)
		if decodeErr != nil || len(decoded) != sha256.Size || saved != strings.ToLower(saved) {
			return fail(fmt.Errorf("invalid copy receipt"))
		}
		if id != 1 || complete != 1 || shape != "legacy-v013" || path == "" || !filepath.IsAbs(path) || len(saved) != 64 {
			return fail(fmt.Errorf("invalid copy receipt"))
		}
		if saved != digest {
			return fail(fmt.Errorf("snapshot digest mismatch"))
		}
	}
	if err = checkSidecars(); err != nil {
		return fail(err)
	}
	after, err := snapshotDigest(from)
	if err != nil {
		return fail(err)
	}
	if after != digest {
		return fail(fmt.Errorf("snapshot changed while reading"))
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	return &DB{db: target}, digest, empty, nil
}
