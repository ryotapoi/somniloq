package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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
		fmt.Fprintln(os.Stderr, "commands: import")
		os.Exit(1)
	}

	switch args[0] {
	case "import":
		runImport(*dbPath, defaultProjectsDir, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		os.Exit(1)
	}
}

func runImport(dbPath, projectsDir string, args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	full := fs.Bool("full", false, "full re-import (delete all and re-import)")
	fs.Parse(args)

	// Ensure DB directory exists
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating db directory: %v\n", err)
		os.Exit(1)
	}

	db, err := core.OpenDB(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening database: %v\n", err)
		os.Exit(1)
	}
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
