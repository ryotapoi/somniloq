package core

import (
	"database/sql"
	"database/sql/driver"
	"time"

	"modernc.org/sqlite"
)

const rfc3339UTCNanosLayout = "2006-01-02T15:04:05.000000000Z"

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("rfc3339_utc_nanos", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		if args[0] == nil {
			return nil, nil
		}
		value, ok := args[0].(string)
		if !ok || value == "" {
			return nil, nil
		}
		t, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, err
		}
		return t.UTC().Format(rfc3339UTCNanosLayout), nil
	})
}

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
