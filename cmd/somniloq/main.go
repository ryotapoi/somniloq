package main

import (
	"flag"
	"fmt"
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

	dbPath := flag.String("db", defaultDB, "path to SQLite database")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: somniloq [--db path] <command>")
		fmt.Fprintln(os.Stderr, "commands: import, sessions, show, projects")
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		runImport(*dbPath, defaultProjectsDir, args[1:])
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
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	full := fs.Bool("full", false, "full re-import (delete all and re-import)")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
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

	result, err := core.Import(db, core.ImportOptions{
		Full:        *full,
		ProjectsDir: projectsDir,
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

func runSessions(dbPath string, args []string) {
	fs := flag.NewFlagSet("sessions", flag.ExitOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are UTC")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are UTC")
	project := fs.String("project", "", "filter sessions by project name (substring match)")
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
		fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\t%d\n",
			r.SessionID, r.StartedAt, r.ProjectDir, title, r.MessageCount)
	}
}

func runShow(dbPath string, args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are UTC")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are UTC")
	project := fs.String("project", "", "filter sessions by project name (substring match)")
	format := fs.String("format", "markdown", "output format (markdown)")
	fs.Parse(args)

	if *format != "markdown" {
		fmt.Fprintf(os.Stderr, "error: unknown format: %q\n", *format)
		os.Exit(1)
	}

	if fs.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "error: too many arguments")
		fmt.Fprintln(os.Stderr, "usage: somniloq show <session-id> | somniloq show [--since <time>] [--until <time>]")
		os.Exit(1)
	}

	sessionID := fs.Arg(0)

	if sessionID != "" && (*since != "" || *until != "") {
		fmt.Fprintln(os.Stderr, "error: specify either session-id or --since/--until, not both")
		os.Exit(1)
	}
	if sessionID == "" && *since == "" && *until == "" {
		fmt.Fprintln(os.Stderr, "usage: somniloq show <session-id> | somniloq show [--since <time>] [--until <time>]")
		os.Exit(1)
	}

	db := openDB(dbPath)
	defer db.Close()

	if sessionID != "" {
		session, err := db.GetSession(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if session == nil {
			fmt.Fprintf(os.Stderr, "error: session not found: %s\n", sessionID)
			os.Exit(1)
		}
		messages, err := db.GetMessages(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		formatSession(os.Stdout, *session, messages)
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

	if err := formatSessions(os.Stdout, sessions, db.GetMessages); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runProjects(dbPath string, args []string) {
	fs := flag.NewFlagSet("projects", flag.ExitOnError)
	since := fs.String("since", "", "filter by start time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are UTC")
	until := fs.String("until", "", "filter sessions started before this time (e.g. 24h, 7d, 2026-03-28, 2026-03-28T15:00); dates are UTC")
	fs.Parse(args)

	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "error: unexpected arguments")
		fmt.Fprintln(os.Stderr, "usage: somniloq projects [--since <time>] [--until <time>]")
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
		fmt.Fprintf(os.Stdout, "%s\t%d\n", r.ProjectDir, r.SessionCount)
	}
}

func buildSessionFilter(since, until, project string) (core.SessionFilter, error) {
	now := time.Now().UTC()
	var filter core.SessionFilter
	if since != "" {
		s, err := resolveTimeFlag(since, now, false)
		if err != nil {
			return filter, err
		}
		filter.Since = s
	}
	if until != "" {
		u, err := resolveTimeFlag(until, now, true)
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

func resolveTimeFlag(value string, now time.Time, isUntil bool) (string, error) {
	t, dateOnly, err := core.ParseTimeRef(value, now)
	if err != nil {
		return "", err
	}
	if isUntil && dateOnly {
		t = t.Add(24 * time.Hour)
	}
	return t.UTC().Format("2006-01-02T15:04:05.000Z"), nil
}

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}
