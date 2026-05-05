package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/ryotapoi/somniloq/internal/core"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	defaultDB := filepath.Join(homeDir, ".somniloq", "somniloq.db")
	defaultProjectsDir := filepath.Join(homeDir, ".claude", "projects")
	defaultCodexSessionsDir := filepath.Join(homeDir, ".codex", "sessions")

	dbPath := flag.String("db", defaultDB, "path to SQLite database")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `Session log viewer for Claude Code and Codex

Usage:
  somniloq [flags] <command>

Commands:
  import        Import Claude Code session logs from JSONL files
  import-codex  Import Codex session logs from JSONL files
  backfill      Correct legacy session data
  sessions      List sessions
  show          Show session content in Markdown
  projects      List projects

Flags:
`)
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("somniloq version %s\n", getVersion())
		os.Exit(0)
	}

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		runImport(*dbPath, defaultProjectsDir, args[1:])
	case "import-codex":
		runImportCodex(*dbPath, defaultCodexSessionsDir, args[1:])
	case "backfill":
		runBackfill(*dbPath, args[1:])
	case "sessions":
		runSessions(*dbPath, args[1:])
	case "show":
		runShow(*dbPath, args[1:])
	case "projects":
		runProjects(*dbPath, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
}

func openDB(dbPath string) *core.DB {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating db directory: %v\n", err)
		os.Exit(1)
	}
	db, err := core.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
	return db
}

func runImport(dbPath, projectsDir string, args []string) {
	runImportWith(dbPath, projectsDir, args, "import", "Import Claude Code session logs from JSONL files", "somniloq import [flags]", core.Import)
}

func runImportCodex(dbPath, sessionsDir string, args []string) {
	runImportWith(dbPath, sessionsDir, args, "import-codex", "Import Codex session logs from JSONL files", "somniloq import-codex [flags]", core.ImportCodex)
}

func runImportWith(dbPath, rootDir string, args []string, name, description, usage string, importFunc func(*core.DB, core.ImportOptions) (*core.ImportResult, error)) {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	full := fs.Bool("full", false, "full re-import (delete all and re-import)")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	setUsage(fs, description, usage)
	fs.Parse(args)

	if *full && !*yes {
		if !isatty.IsTerminal(os.Stdin.Fd()) {
			fmt.Fprintln(os.Stderr, "error: --full requires confirmation; use --yes to skip in non-interactive mode")
			os.Exit(1)
		}
		if !confirmFullImport(os.Stdin, os.Stderr) {
			return
		}
	}

	db := openDB(dbPath)
	defer db.Close()

	result, err := importFunc(db, core.ImportOptions{
		Full:        *full,
		ProjectsDir: rootDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Imported %d files (%d scanned, %d skipped, %d failed)\n",
		result.FilesImported, result.FilesScanned, result.FilesSkipped, result.FilesFailed)

	for _, e := range result.Errors {
		fmt.Fprintf(os.Stderr, "  error: %v\n", e)
	}

	if result.FilesFailed > 0 {
		os.Exit(1)
	}
}

func runBackfill(dbPath string, args []string) {
	isTTY := isatty.IsTerminal(os.Stdin.Fd())
	open := func() (*core.DB, error) {
		return openDB(dbPath), nil
	}
	code, err := backfillCmd(args, open, os.Stdin, os.Stdout, os.Stderr, isTTY)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	os.Exit(code)
}

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
	setUsage(fs, "Correct legacy session data (migrate v0.3→v0.4 schema, delete orphan sessions, resolve repo_path)", "somniloq backfill")
	fs.SetOutput(errOut)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0, nil
		}
		return 1, err
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

func runSessions(dbPath string, args []string) {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	project := fs.String("project", "", "filter by repo path (substring match)")
	short := fs.Bool("short", false, "shorten project to repo basename")
	setUsage(fs, "List sessions", "somniloq sessions [flags]")
	fs.Parse(args)

	db := openDB(dbPath)
	defer db.Close()

	filter, err := buildSessionFilter(*since, *until, *project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	rows, err := db.ListSessions(filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, r := range rows {
		title := sanitizeTSV(r.CustomTitle)
		proj := resolveDisplayName(r.RepoPath, *short)
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\t%d\n",
			r.SessionID, formatTimeRange(r.StartedAt, r.EndedAt, time.Local), proj, title, r.MessageCount)
	}
}

const showUsageLine = "somniloq show [--summary <N>] [--include-clear] [--short] <session-id>\n" +
	"  somniloq show [--since <time>] [--until <time>] [--project <name>] [--summary <N>] [--include-clear] [--short]"

func runShow(dbPath string, args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	project := fs.String("project", "", "filter by repo path (substring match)")
	short := fs.Bool("short", false, "shorten project to repo basename")
	summary := fs.Int("summary", 0, "show first N user messages skipping /clear and local-command-caveat (0 disables)")
	includeClear := fs.Bool("include-clear", false, "keep /clear and local-command-caveat messages in --summary output (requires --summary >= 1)")
	format := fs.String("format", "markdown", "output format (markdown)")
	setUsage(fs, "Show session content in Markdown", showUsageLine)
	fs.Parse(args)

	if *summary < 0 {
		fmt.Fprintln(os.Stderr, "error: --summary must be >= 0")
		os.Exit(1)
	}
	if *includeClear && *summary == 0 {
		fmt.Fprintln(os.Stderr, "error: --include-clear requires --summary >= 1")
		os.Exit(1)
	}

	if *format != "markdown" {
		fmt.Fprintf(os.Stderr, "error: unknown format: %q\n", *format)
		os.Exit(1)
	}

	showUsage := "usage: " + showUsageLine

	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "error: too many arguments")
		fmt.Fprintln(os.Stderr, showUsage)
		os.Exit(1)
	}

	sessionID := fs.Arg(0)

	if sessionID != "" && (*since != "" || *until != "") {
		fmt.Fprintln(os.Stderr, "error: specify either session-id or --since/--until, not both")
		os.Exit(1)
	}
	if sessionID == "" && *since == "" && *until == "" {
		fmt.Fprintln(os.Stderr, showUsage)
		os.Exit(1)
	}

	db := openDB(dbPath)
	defer db.Close()

	getMessages := func(src core.Source, id string) ([]core.MessageRow, error) {
		return db.GetMessages(src, id)
	}
	if *summary >= 1 {
		n, ic := *summary, *includeClear
		getMessages = func(src core.Source, id string) ([]core.MessageRow, error) {
			return db.GetSummaryMessages(src, id, n, ic)
		}
	}

	if sessionID != "" {
		session, err := db.GetSession(core.SourceClaudeCode, sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if session == nil {
			fmt.Fprintf(os.Stderr, "error: session not found: %s\n", sessionID)
			os.Exit(1)
		}
		proj := resolveDisplayName(session.RepoPath, *short)
		messages, err := getMessages(core.SourceClaudeCode, sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		formatSession(os.Stdout, *session, proj, messages, time.Local)
		return
	}

	// --since/--until mode
	filter, err := buildSessionFilter(*since, *until, *project)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	sessions, err := db.ListSessions(filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(sessions) == 0 {
		return
	}

	displayNames := make([]string, len(sessions))
	for i := range sessions {
		displayNames[i] = resolveDisplayName(sessions[i].RepoPath, *short)
	}

	if err := formatSessions(os.Stdout, sessions, displayNames, getMessages, time.Local); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runProjects(dbPath string, args []string) {
	fs := flag.NewFlagSet("projects", flag.ExitOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are local time")
	short := fs.Bool("short", false, "shorten project names to repo basename")
	setUsage(fs, "List projects", "somniloq projects [flags]")
	fs.Parse(args)

	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "error: unexpected arguments")
		fmt.Fprintln(os.Stderr, "usage: somniloq projects [--since <time>] [--until <time>] [--short]")
		os.Exit(1)
	}

	db := openDB(dbPath)
	defer db.Close()

	filter, err := buildSessionFilter(*since, *until, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	rows, err := db.ListProjects(filter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	for _, r := range rows {
		name := resolveDisplayName(r.RepoPath, *short)
		fmt.Fprintf(os.Stdout, "%s\t%d\n", name, r.SessionCount)
	}
}

func buildSessionFilter(since, until, project string) (core.SessionFilter, error) {
	now := time.Now().UTC()
	var filter core.SessionFilter
	if since != "" {
		s, err := resolveTimeFlag(since, now, false, time.Local)
		if err != nil {
			return filter, err
		}
		filter.Since = s
	}
	if until != "" {
		u, err := resolveTimeFlag(until, now, true, time.Local)
		if err != nil {
			return filter, err
		}
		filter.Until = u
	}
	if filter.Since != "" && filter.Until != "" && filter.Since >= filter.Until {
		return filter, fmt.Errorf("--since must be before --until")
	}
	filter.Project = project
	return filter, nil
}

func resolveTimeFlag(value string, now time.Time, isUntil bool, loc *time.Location) (string, error) {
	t, dateOnly, err := core.ParseTimeRef(value, now, loc)
	if err != nil {
		return "", err
	}
	if isUntil && dateOnly {
		t = t.AddDate(0, 0, 1)
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
}

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}
