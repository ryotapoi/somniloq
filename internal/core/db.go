package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"net/url"
	"os"
	"strings"
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
	db     *sql.DB
	readTx *sql.Tx
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
	// Apply foreign keys to replacement connections as well as the first one.
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	dsn += separator + "_pragma=foreign_keys%281%29"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// Each SQLite :memory: connection is a separate database. Keep schema checks and queries on one connection.
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

// writeTx owns its connection until SQLite has ended the transaction.
// sql.Tx is already done after a failed Commit, although SQLite may not be.
type writeTx struct {
	*sql.Tx
	conn *sql.Conn
	done bool
}

func (d *DB) Begin() (*writeTx, error) {
	conn, err := d.db.Conn(context.Background())
	if err != nil {
		return nil, err
	}
	tx, err := conn.BeginTx(context.Background(), nil)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return &writeTx{Tx: tx, conn: conn}, nil
}

func (t *writeTx) finish(err error) error {
	if err != nil {
		// Keep the connection reserved while clearing a transaction left by the driver.
		if _, cleanupErr := t.conn.ExecContext(context.Background(), "ROLLBACK"); cleanupErr != nil {
			// Never return a connection with uncertain transaction state to the pool.
			t.conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}
	t.conn.Close()
	return err
}

func (t *writeTx) Commit() error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	return t.finish(t.Tx.Commit())
}

func (t *writeTx) Rollback() error {
	if t.done {
		return sql.ErrTxDone
	}
	t.done = true
	return t.finish(t.Tx.Rollback())
}

func (d *DB) execer() execer {
	if d.readTx != nil {
		return d.readTx
	}
	return d.db
}

// ReadSnapshot runs related reads against one saved database state.
func (d *DB) ReadSnapshot(read func(*DB) error) error {
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := read(&DB{db: d.db, readTx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}
