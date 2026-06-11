package main

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

// importCmd runs the import subcommand without calling os.Exit, so it can be
// tested directly. openDB is invoked only after argument parsing and
// confirmation succeed.
func importCmd(args []string, openDB func() (*core.DB, error), projectsDir, codexSessionsDir string, in io.Reader, out, errOut io.Writer, isTTY bool) (int, error) {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	full := fs.Bool("full", false, "full re-import (delete all and re-import)")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	sourceValue := fs.String("source", string(core.ImportSourceAll), "source to import: all, claude-code, codex")
	setUsage(fs, "Import Claude Code and Codex session logs from JSONL files", "somniloq import [--source all|claude-code|codex] [flags]")
	if code, ok := parseFlags(fs, errOut, args); !ok {
		return code, nil
	}

	source, err := parseImportSource(*sourceValue)
	if err != nil {
		return 1, err
	}

	if *full && !*yes {
		if !isTTY {
			return 1, errors.New("--full requires confirmation; use --yes to skip in non-interactive mode")
		}
		if !confirmFullImport(in, errOut) {
			return 0, nil
		}
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	result, err := core.Import(db, core.ImportOptions{
		Full:             *full,
		ProjectsDir:      projectsDir,
		CodexSessionsDir: codexSessionsDir,
		Source:           source,
	})
	if err != nil {
		return 1, err
	}

	fmt.Fprintf(out, "Imported %d files (%d scanned, %d skipped, %d failed)\n",
		result.FilesImported, result.FilesScanned, result.FilesSkipped, result.FilesFailed)

	for _, e := range result.Errors {
		fmt.Fprintf(errOut, "  error: %v\n", e)
	}

	if result.FilesFailed > 0 {
		return 1, nil
	}
	return 0, nil
}

func parseImportSource(value string) (core.ImportSource, error) {
	source := core.ImportSource(value)
	if !source.Valid() {
		return "", fmt.Errorf("invalid --source %q (want all, claude-code, or codex)", value)
	}
	return source, nil
}
