package core

import (
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenDB_CreatesSchema(t *testing.T) {
	db := testDB(t)

	tables := []string{"sessions", "messages", "import_state"}
	for _, table := range tables {
		var name string
		err := db.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", table, err)
		}
	}
}

func TestOpenDB_HasRepoPathColumn(t *testing.T) {
	db := testDB(t)

	present, err := tableColumnPresent(db.db, "sessions", "repo_path")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if !present {
		t.Fatal("repo_path column missing from sessions table")
	}
}

func TestOpenDB_ReturnsErrorForUnopenablePath(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "missing", "somniloq.db")

	db, err := OpenDB(dsn)
	if err == nil {
		if db != nil {
			db.Close()
		}
		t.Fatal("OpenDB succeeded for a DB path whose parent directory does not exist")
	}
	if db != nil {
		t.Fatalf("OpenDB returned db=%v with err=%v, want nil DB on error", db, err)
	}
}

// legacySessionsSchema is the v0.2.x sessions table shape: before the
// repo_path column was added and before project_dir was dropped. Used by
// migration tests that exercise ensureSessionsRepoPathColumn and
// ensureSessionsProjectDirColumnDropped against a DB that still has the old
// shape.
const legacySessionsSchema = `
CREATE TABLE sessions (
    session_id TEXT PRIMARY KEY,
    project_dir TEXT NOT NULL,
    cwd TEXT,
    git_branch TEXT,
    custom_title TEXT,
    agent_name TEXT,
    version TEXT,
    started_at TEXT,
    ended_at TEXT,
    imported_at TEXT NOT NULL
);`

func openLegacyMemoryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(legacySessionsSchema); err != nil {
		t.Fatalf("legacy schema exec failed: %v", err)
	}
	return db
}

func TestEnsureSessionsRepoPathColumn_AddsColumn(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsRepoPathColumn(db); err != nil {
		t.Fatalf("ensureSessionsRepoPathColumn failed: %v", err)
	}

	present, err := tableColumnPresent(db, "sessions", "repo_path")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if !present {
		t.Fatal("repo_path column should have been added")
	}
}

func TestEnsureSessionsRepoPathColumn_Idempotent(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsRepoPathColumn(db); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := ensureSessionsRepoPathColumn(db); err != nil {
		t.Fatalf("second call should be a no-op, got: %v", err)
	}
}

type alterRaceDB struct {
	*sql.DB
	match string
	err   error
}

func (d alterRaceDB) Exec(query string, args ...any) (sql.Result, error) {
	res, err := d.DB.Exec(query, args...)
	if err != nil {
		return res, err
	}
	if strings.Contains(query, d.match) {
		return nil, d.err
	}
	return res, nil
}

func TestEnsureSessionsRepoPathColumn_RaceRecheckTreatsConcurrentAddAsSuccess(t *testing.T) {
	db := openLegacyMemoryDB(t)

	err := ensureSessionsRepoPathColumn(alterRaceDB{
		DB:    db,
		match: "ALTER TABLE sessions ADD COLUMN repo_path",
		err:   errors.New("injected alter failure after concurrent add"),
	})
	if err != nil {
		t.Fatalf("ensureSessionsRepoPathColumn should accept add race after re-check, got: %v", err)
	}
}

func TestEnsureSessionsProjectDirColumnDropped_DropsColumn(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		t.Fatalf("ensureSessionsProjectDirColumnDropped failed: %v", err)
	}

	present, err := tableColumnPresent(db, "sessions", "project_dir")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if present {
		t.Fatal("project_dir column should have been dropped")
	}
}

func TestEnsureSessionsProjectDirColumnDropped_RaceRecheckTreatsConcurrentDropAsSuccess(t *testing.T) {
	db := openLegacyMemoryDB(t)

	err := ensureSessionsProjectDirColumnDropped(alterRaceDB{
		DB:    db,
		match: "ALTER TABLE sessions DROP COLUMN project_dir",
		err:   errors.New("injected alter failure after concurrent drop"),
	})
	if err != nil {
		t.Fatalf("ensureSessionsProjectDirColumnDropped should accept drop race after re-check, got: %v", err)
	}
}

func TestEnsureSessionsProjectDirColumnDropped_Idempotent(t *testing.T) {
	db := openLegacyMemoryDB(t)

	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		t.Fatalf("second call should be a no-op, got: %v", err)
	}
}

func TestEnsureSessionsProjectDirColumnDropped_NoOpWhenAbsent(t *testing.T) {
	db := testDB(t)

	if err := ensureSessionsProjectDirColumnDropped(db.db); err != nil {
		t.Fatalf("call against fresh DB should be a no-op, got: %v", err)
	}
}

func TestOpenDB_HasNoProjectDirColumn(t *testing.T) {
	db := testDB(t)

	present, err := tableColumnPresent(db.db, "sessions", "project_dir")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if present {
		t.Fatal("project_dir column should not exist on a fresh DB")
	}
}

func TestOpenDB_ReopenIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "test.db")

	db1, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("first OpenDB failed: %v", err)
	}
	if err := db1.UpsertSession(SessionMeta{Source: SourceClaudeCode, SessionID: "s1", StartedAt: "2026-03-28T10:00:00Z"}, "2026-03-28T15:00:00Z"); err != nil {
		t.Fatalf("UpsertSession failed: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	db2, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("second OpenDB failed: %v", err)
	}
	t.Cleanup(func() { db2.Close() })

	// Second open must hit the "column already present" fast path without error.
	if present, err := tableColumnPresent(db2.db, "sessions", "repo_path"); err != nil || !present {
		t.Fatalf("repo_path not present after reopen: present=%v err=%v", present, err)
	}

	rows, err := db2.ListSessions(SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions failed: %v", err)
	}
	if len(rows) != 1 || rows[0].SessionID != "s1" {
		t.Errorf("session not retained across reopen: %+v", rows)
	}
}

func TestOpenDB_MigratesLegacyFile(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "legacy.db")

	// Create a legacy-shaped DB file directly, without going through OpenDB.
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	if _, err := raw.Exec(legacySessionsSchema); err != nil {
		raw.Close()
		t.Fatalf("legacy schema exec failed: %v", err)
	}
	if _, err := raw.Exec(
		`INSERT INTO sessions (session_id, project_dir, imported_at) VALUES (?, ?, ?)`,
		"legacy-s1", "-test", "2026-03-28T15:00:00Z",
	); err != nil {
		raw.Close()
		t.Fatalf("legacy INSERT failed: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("raw.Close failed: %v", err)
	}

	db, err := OpenDB(dsn)
	if err != nil {
		t.Fatalf("OpenDB on legacy file failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	present, err := tableColumnPresent(db.db, "sessions", "repo_path")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if !present {
		t.Fatal("repo_path column should have been added to legacy DB")
	}

	projectDirPresent, err := tableColumnPresent(db.db, "sessions", "project_dir")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if projectDirPresent {
		t.Fatal("project_dir column should have been dropped from legacy DB")
	}

	// OpenDB must NOT add the v0.4 source column on a legacy file: the
	// v0.3 → v0.4 migration is gated on `somniloq backfill`, not OpenDB.
	// If this assertion ever fails, OpenDB has started doing migration work
	// it shouldn't (see ADR 0004).
	sourcePresent, err := tableColumnPresent(db.db, "sessions", "source")
	if err != nil {
		t.Fatalf("tableColumnPresent failed: %v", err)
	}
	if sourcePresent {
		t.Fatal("source column must not be added by OpenDB; it is added by Backfill (v0.4 migration)")
	}

	var (
		importedAt string
		isNull     int
	)
	if err := db.db.QueryRow(
		"SELECT imported_at, repo_path IS NULL FROM sessions WHERE session_id='legacy-s1'",
	).Scan(&importedAt, &isNull); err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	if importedAt != "2026-03-28T15:00:00Z" {
		t.Errorf("imported_at mutated by migration: got %q", importedAt)
	}
	if isNull != 1 {
		t.Errorf("existing row should have repo_path IS NULL after migration")
	}
}

type tableColumnDef struct {
	Name    string
	Type    string
	NotNull int
	Default sql.NullString
	PK      int
}

func tableColumns(t *testing.T, db *sql.DB, table string) []tableColumnDef {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()

	var columns []tableColumnDef
	for rows.Next() {
		var (
			cid int
			col tableColumnDef
		)
		if err := rows.Scan(&cid, &col.Name, &col.Type, &col.NotNull, &col.Default, &col.PK); err != nil {
			t.Fatalf("scan PRAGMA table_info(%s): %v", table, err)
		}
		columns = append(columns, col)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate PRAGMA table_info(%s): %v", table, err)
	}
	return columns
}

func normalizedTableDDL(table, sql string) string {
	ddl := sql
	if withoutIfNotExists := strings.TrimPrefix(sql, "CREATE TABLE IF NOT EXISTS "); withoutIfNotExists != sql {
		ddl = "CREATE TABLE " + withoutIfNotExists
	}
	quotedTable := `CREATE TABLE "` + table + `"`
	if withoutQuotes := strings.TrimPrefix(ddl, quotedTable); withoutQuotes != ddl {
		ddl = "CREATE TABLE " + table + withoutQuotes
	}
	return normalizeSQLWhitespaceOutsideQuotes(ddl)
}

func normalizeSQLWhitespaceOutsideQuotes(sql string) string {
	var normalized strings.Builder
	normalized.Grow(len(sql))
	var quote byte
	spacePending := false
	for i := 0; i < len(sql); i++ {
		ch := sql[i]
		if quote != 0 {
			normalized.WriteByte(ch)
			if ch == quote {
				if quote != '[' && i+1 < len(sql) && sql[i+1] == quote {
					normalized.WriteByte(sql[i+1])
					i++
					continue
				}
				quote = 0
			}
			continue
		}

		if isSQLWhitespace(ch) {
			spacePending = normalized.Len() > 0
			continue
		}
		if spacePending {
			normalized.WriteByte(' ')
			spacePending = false
		}
		normalized.WriteByte(ch)
		switch ch {
		case '\'', '"', '`':
			quote = ch
		case '[':
			quote = ']'
		}
	}
	return normalized.String()
}

func isSQLWhitespace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func TestNormalizedTableDDL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "fresh table prefix and whitespace",
			in:   "CREATE TABLE IF NOT EXISTS sessions (\n  id TEXT\n)",
			want: "CREATE TABLE sessions ( id TEXT )",
		},
		{
			name: "normalizes only SQLite renamed table quoting",
			in:   "CREATE TABLE \"sessions\" (\"kind\" TEXT DEFAULT 'CamelCase')",
			want: "CREATE TABLE sessions (\"kind\" TEXT DEFAULT 'CamelCase')",
		},
		{
			name: "preserves repeated whitespace in quoted tokens",
			in:   "CREATE TABLE sessions (kind TEXT DEFAULT 'a  b'  )",
			want: "CREATE TABLE sessions (kind TEXT DEFAULT 'a  b' )",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizedTableDDL("sessions", tt.in); got != tt.want {
				t.Errorf("normalizedTableDDL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
	if one, two := normalizedTableDDL("sessions", "CREATE TABLE sessions (kind TEXT DEFAULT 'a b')"), normalizedTableDDL("sessions", "CREATE TABLE sessions (kind TEXT DEFAULT 'a  b')"); one == two {
		t.Fatalf("quoted string whitespace was normalized: one=%q two=%q", one, two)
	}
}

func tableDDL(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var ddl string
	err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&ddl)
	if err != nil {
		t.Fatalf("sqlite_master %s: %v", table, err)
	}
	return normalizedTableDDL(table, ddl)
}

func TestMigrateToV04_SchemaMatchesFreshOpenDB(t *testing.T) {
	migrated := setupV03DB(t)
	if _, _, _, err := migrateToV04(migrated); err != nil {
		t.Fatalf("migrateToV04: %v", err)
	}
	fresh := testDB(t)

	for _, table := range []string{"sessions", "messages", "import_state"} {
		gotDDL := tableDDL(t, migrated.db, table)
		wantDDL := tableDDL(t, fresh.db, table)
		if gotDDL != wantDDL {
			t.Errorf("%s sqlite_master DDL after migrateToV04 differs from fresh OpenDB\n got: %s\nwant: %s", table, gotDDL, wantDDL)
		}

		got := tableColumns(t, migrated.db, table)
		want := tableColumns(t, fresh.db, table)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s PRAGMA table_info after migrateToV04 differs from fresh OpenDB\n got: %#v\nwant: %#v", table, got, want)
		}
	}
}
