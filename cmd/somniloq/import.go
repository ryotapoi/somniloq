package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

const importHelpDetails = `Output:
  Imported <imported> files (<scanned> scanned, <skipped> skipped, <failed> failed, <unparsed> unparsed lines)

  scanned: JSONL files discovered for the selected source(s).
  skipped: unchanged files skipped by differential import.
  failed: files that were discovered but could not be imported.
  unparsed lines: broken JSON or malformed payload lines. Deliberately ignored record types are not counted.
  On failure after scanning starts, the partial summary is printed; only committed files count as imported.
  Import failure reasons are printed to stderr and the exit code is 1.
  Parse/normalization diagnostics: up to five file:line: error entries are printed to stderr.
  Codex records with a boundary but no ordinal are retained as unresolved, without a body number.

Notes:
  Default import is differential. Use --full to rebuild only selected inputs, preserving other inputs.
  With --source cursor-agent --full, only selected Cursor Agent inputs are rebuilt.
  Codex rebuilds each changed owner across all rollouts, ordered by relative path and physical line.
  Matching payload IDs deduplicate only identical role, text blocks, and raw timestamp; conflicts fail the group.
  Codex inheritance context is stored separately and excluded from show and body search.
  Missing record timestamps remain unknown; session metadata does not supply message time.
  An unparsed final line without a newline is retried if the file grows; completed malformed lines are skipped.
  Claude Code messages need non-empty session and message IDs; Codex session metadata needs a non-empty ID.
  Claude Code and Codex sessions from existing linked Git worktrees are grouped under the main repository.
  Previously stored invalid IDs or worktree paths are not repaired by differential import; --full
  can rebuild them if the original logs remain. Check selected inputs before rebuilding them.
  Non-fatal scan/file errors are printed to stderr; import continues and exits 1 if any occurred.

Examples:
  somniloq import --config default
  somniloq import --config default --source cursor-agent
  somniloq import --config default --full --yes`

// importConfiguredCmd opens the DB only after argument validation and confirmation.
func importConfiguredCmd(args []string, openDB func() (*core.DB, error), cfg config, in io.Reader, out, errOut io.Writer, isTTY bool) (int, error) {
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	full := fs.Bool("full", false, "rebuild only selected inputs")
	yes := fs.Bool("yes", false, "skip confirmation prompt")
	var inputs stringListFlag
	fs.Var(&inputs, "input", "input root path to import (repeatable; OR, intersected with --source)")
	sourceValue := fs.String("source", string(core.ImportSourceAll), "source to import: "+importSourceCommaList())
	setUsage(fs, "Import Claude Code, Codex, and Cursor Agent session logs from JSONL files", "somniloq import [--config NAME_OR_PATH] [--source "+importSourcePipeList()+"] [flags]", importHelpDetails)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		if code != 0 {
			code = 2
		}
		return code, nil
	}
	if fs.NArg() != 0 {
		writeUsageError(errOut, "unexpected arguments")
		fmt.Fprintln(errOut, "usage: somniloq import [--config NAME_OR_PATH] [flags]")
		return 2, nil
	}

	source, err := parseImportSource(*sourceValue)
	if err != nil {
		return 2, err
	}

	for i, path := range inputs {
		if path == "" {
			return 2, fmt.Errorf("--input requires a non-empty path")
		}
		inputs[i], err = core.CanonicalPath(path, filepath.Dir(cfg.Path))
		if err != nil {
			return 2, fmt.Errorf("invalid --input: %w", err)
		}
	}

	if *full && !*yes {
		if !isTTY {
			return 1, errors.New("--full requires confirmation; use --yes to skip in non-interactive mode")
		}
		confirmed, err := confirmFullImport(in, errOut)
		if err != nil {
			return 1, err
		}
		if !confirmed {
			return 0, nil
		}
	}

	db, err := openDB()
	if err != nil {
		return 1, err
	}
	defer db.Close()

	result, err := core.Import(db, core.ImportOptions{
		Full:       *full,
		Inputs:     cfg.Inputs,
		InputPaths: inputs,
		Source:     source,
	})
	if result == nil {
		return 1, err
	}
	importErr := err

	if _, err := fmt.Fprintf(out, "Imported %d files (%d scanned, %d skipped, %d failed, %d unparsed lines)\n",
		result.FilesImported, result.FilesScanned, result.FilesSkipped, result.FilesFailed, result.UnparsedLines); err != nil {
		return 1, err
	}

	for _, e := range result.Errors {
		if _, err := fmt.Fprintf(errOut, "  error: %v\n", e); err != nil {
			return 1, err
		}
	}
	for _, diagnostic := range result.UnparsedDiagnostics {
		if _, err := fmt.Fprintf(errOut, "  error: %v\n", diagnostic); err != nil {
			return 1, err
		}
	}

	if importErr != nil {
		return 1, importErr
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
