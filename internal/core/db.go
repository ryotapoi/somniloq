package core

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type DB struct {
	db *sql.DB
}

// execer abstracts *sql.DB and *sql.Tx for shared query methods.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func OpenDB(dsn string) (*DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite treats each connection to ":memory:" as a separate DB
	// instance, so a shared *sql.DB can otherwise see different in-memory DBs
	// across queries. Pinning to one physical connection avoids that.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSessionsRepoPathColumn(db); err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureSessionsProjectDirColumnDropped(db); err != nil {
		db.Close()
		return nil, err
	}

	return &DB{db: db}, nil
}

func (d *DB) Close() error {
	return d.db.Close()
}

func (d *DB) Begin() (*sql.Tx, error) {
	return d.db.Begin()
}

func (d *DB) execer() execer {
	return d.db
}
