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
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, `Session log viewer for Claude Code

Usage:
  somniloq [flags] <command>

Commands:
  import     Import session logs from JSONL files
  backfill   Resolve repo_path for legacy sessions
  sessions   List sessions
  show       Show session content in Markdown
  projects   List projects

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
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	full := fs.Bool("full", false, "full re-import (delete all and re-import)")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	setUsage(fs, "Import session logs from JSONL files", "somniloq import [flags]")
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

func runBackfill(dbPath string, args []string) {
	fs := flag.NewFlagSet("backfill", flag.ExitOnError)
	setUsage(fs, "Resolve repo_path for legacy sessions", "somniloq backfill")
	fs.Parse(args)

	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "error: unexpected arguments")
		os.Exit(1)
	}

	db := openDB(dbPath)
	defer db.Close()

	resolved, unresolved, err := core.BackfillRepoPaths(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Backfilled %d sessions (%d unresolved)\n", resolved, unresolved)
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

	getMessages := db.GetMessages
	if *summary >= 1 {
		n, ic := *summary, *includeClear
		getMessages = func(id string) ([]core.MessageRow, error) {
			return db.GetSummaryMessages(id, n, ic)
		}
	}

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
		proj := resolveDisplayName(session.RepoPath, *short)
		messages, err := getMessages(sessionID)
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
