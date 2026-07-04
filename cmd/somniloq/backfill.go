package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

const backfillHelpDetails = `Output:
  Migrated to v0.4: sessions=<n> messages=<n> import_states=<n>
  Backfilled: deleted=<n> resolved=<n> unresolved=<n>

  migrated counts: rows copied during v0.3 -> v0.4 schema migration; omitted when no migration is needed.
  deleted: sessions rows with no messages that were removed.
  resolved: sessions whose repo_path was filled from cwd.
  unresolved: sessions still missing repo_path after resolution.

Notes:
  Run after upgrading an old DB and before importing new data. It is safe to re-run.
  If rows will be deleted, non-interactive runs must pass --yes.

Examples:
  somniloq backfill
  somniloq backfill --yes`

// backfillCmd runs the backfill subcommand without calling os.Exit, so it can
// be tested directly. openDB is invoked only after argument parsing succeeds,
// so `--help` and validation errors do not require a real DB. backfillCmd
// closes the DB it opens via the factory; tests that need to inspect the same
// DB after backfillCmd returns must hand out a wrapper that survives Close
// (or simply skip Close inside the factory).
//
// Order of operations (preflight first so v0.3 → v0.4 migration completes
// before any v0.4-only SQL is executed):
//  1. MigrateToV04IfNeeded — runs once on a v0.3 DB, no-op afterwards.
//  2. Migration counts are emitted immediately so the user sees them even if
//     the subsequent confirmation prompt is declined or hits a non-TTY error.
//  3. CountOrphanSessions — requires the v0.4 schema (source column).
//  4. Confirmation prompt (only if orphans exist and --yes not given).
//  5. Backfill — orphan delete + repo_path resolve.
func backfillCmd(args []string, openDB func() (*core.DB, error), in io.Reader, out, errOut io.Writer, isTTY bool) (int, error) {
	fs := flag.NewFlagSet("backfill", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	setUsage(fs, "Correct legacy session data (migrate v0.3→v0.4 schema, delete orphan sessions, resolve repo_path)", "somniloq backfill", backfillHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}
	if fs.NArg() != 0 {
		return 1, errors.New("unexpected arguments")
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	ms, mm, mi, migrateErr := core.MigrateToV04IfNeeded(db)
	if ms > 0 || mm > 0 || mi > 0 {
		// Emit before checking migrateErr: the migration tx may have committed
		// successfully and only the foreign_keys PRAGMA restore in the deferred
		// cleanup failed. The user must still see the migration counts so the
		// next backfill (which will report 0) does not silently hide them.
		fmt.Fprintf(out, "Migrated to v0.4: sessions=%d messages=%d import_states=%d\n", ms, mm, mi)
	}
	if migrateErr != nil {
		return 1, migrateErr
	}

	count, err := core.CountOrphanSessions(db)
	if err != nil {
		return 1, err
	}
	if count > 0 && !*yes {
		if !isTTY {
			return 1, errors.New("backfill requires confirmation when deleting sessions; use --yes to skip in non-interactive mode")
		}
		if !confirmBackfillDelete(in, errOut, count) {
			return 0, nil
		}
	}

	result, err := core.Backfill(db)
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(out, "Backfilled: deleted=%d resolved=%d unresolved=%d\n", result.Deleted, result.Resolved, result.Unresolved)
	return 0, nil
}
