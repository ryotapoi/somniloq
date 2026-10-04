package core

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"os"
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
		// Keep malformed source timestamps stored, but compare them as unknown.
		if err != nil {
			return nil, nil
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

func OpenDB(dsn string) (*DB, error) { return openDatabase(dsn, false) }

// OpenDBRead requires an existing supported DB and never initializes it.
func OpenDBRead(path string) (*DB, error) { return openDatabase(path, true) }

func openDatabase(path string, readOnly bool) (*DB, error) {
	dsn := path
	if readOnly {
		if _, err := os.Stat(path); err != nil {
			return nil, err
		}
		dsn = (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
	} else if path != ":memory:" && path != "" {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil && !os.IsExist(err) {
			return nil, err
		}
		if err == nil {
			if err = file.Close(); err != nil {
				return nil, err
			}
		}
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*DB, error) { db.Close(); return nil, err }
	empty, err := inspectSchema(db)
	if err != nil {
		return fail(err)
	}
	if empty {
		if readOnly {
			return fail(&SchemaError{Revision: 0, Reason: "empty database"})
		}
		tx, err := db.Begin()
		if err != nil {
			return fail(err)
		}
		// Recheck inside the same transaction that initializes the schema.
		empty, err = inspectSchema(tx)
		if err == nil && empty {
			_, err = tx.Exec(schema)
		}
		if err != nil {
			tx.Rollback()
			return fail(err)
		}
		if err = tx.Commit(); err != nil {
			return fail(err)
		}
	}
	if _, err = db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return fail(fmt.Errorf("enable foreign keys: %w", err))
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
