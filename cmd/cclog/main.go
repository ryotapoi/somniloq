package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ryotapoi/cclog/internal/core"
)

func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	defaultDB := filepath.Join(homeDir, ".cclog", "cclog.db")
	defaultProjectsDir := filepath.Join(homeDir, ".claude", "projects")

	dbPath := flag.String("db", defaultDB, "path to SQLite database")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: cclog [--db path] <command>")
		fmt.Fprintln(os.Stderr, "commands: import, sessions")
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		runImport(*dbPath, defaultProjectsDir, args[1:])
	case "sessions":
		runSessions(*dbPath, args[1:])
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
	fs.Parse(args)

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
	since := fs.String("since", "", "filter sessions by age (e.g. 24h, 7d)")
	project := fs.String("project", "", "filter sessions by project name (substring match)")
	fs.Parse(args)

	db := openDB(dbPath)
	defer db.Close()

	var filter core.SessionFilter
	if *since != "" {
		d, err := core.ParseDuration(*since)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		filter.Since = time.Now().UTC().Add(-d).Format("2006-01-02T15:04:05.000Z")
	}
	filter.Project = *project

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

var tsvReplacer = strings.NewReplacer("\t", " ", "\n", " ", "\r", " ")

// sanitizeTSV replaces tabs and newlines with spaces to keep TSV output intact.
func sanitizeTSV(s string) string {
	return tsvReplacer.Replace(s)
}
