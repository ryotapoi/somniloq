package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

const importHelpDetails = `Output:
  Imported <imported> files (<scanned> scanned, <skipped> skipped, <failed> failed, <unparsed> unparsed lines)

  scanned: JSONL files discovered for the selected source(s).
  skipped: unchanged files skipped by differential import.
  failed: files that were discovered but could not be imported.
  unparsed lines: broken JSON or malformed payload lines. Deliberately ignored record types are not counted.

Notes:
  Default import is differential. Use --full to delete the whole somniloq DB and re-import from the selected source(s).
  With --source codex --full, existing Claude Code rows are deleted too, then only Codex rows are imported.
  Non-fatal scan/file errors are printed to stderr; import continues and exits 1 if any occurred.

Examples:
  somniloq import
  somniloq import --source codex
  somniloq import --full --yes`

// importCmd runs the import subcommand without calling os.Exit, so it can be
// tested directly. openDB is invoked only after argument parsing and
// confirmation succeed.
func importCmd(args []string, openDB func() (*core.DB, error), projectsDir, codexSessionsDir string, in io.Reader, out, errOut io.Writer, isTTY bool) (int, error) {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	full := fs.Bool("full", false, "full re-import (delete all and re-import)")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	sourceValue := fs.String("source", string(core.ImportSourceAll), "source to import: "+importSourceCommaList())
	setUsage(fs, "Import Claude Code and Codex session logs from JSONL files", "somniloq import [--source "+importSourcePipeList()+"] [flags]", importHelpDetails)
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

	fmt.Fprintf(out, "Imported %d files (%d scanned, %d skipped, %d failed, %d unparsed lines)\n",
		result.FilesImported, result.FilesScanned, result.FilesSkipped, result.FilesFailed, result.UnparsedLines)

	for _, e := range result.Errors {
		fmt.Fprintf(errOut, "  error: %v\n", e)
	}

	// Errors covers failed files and non-fatal scan failures alike.
	if len(result.Errors) > 0 {
		return 1, nil
	}
	return 0, nil
}

func parseImportSource(value string) (core.ImportSource, error) {
	source := core.ImportSource(value)
	if !source.Valid() {
		return "", fmt.Errorf("invalid --source %q (want %s)", value, importSourceSentenceList())
	}
	return source, nil
}

func importSourcePipeList() string {
	return strings.Join(core.ImportSourceChoices(), "|")
}

func importSourceCommaList() string {
	return strings.Join(core.ImportSourceChoices(), ", ")
}

func importSourceSentenceList() string {
	choices := core.ImportSourceChoices()
	switch len(choices) {
	case 0:
		return ""
	case 1:
		return choices[0]
	case 2:
		return choices[0] + " or " + choices[1]
	default:
		return strings.Join(choices[:len(choices)-1], ", ") + ", or " + choices[len(choices)-1]
	}
}
