package core

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
	"github.com/ryotapoi/somniloq/internal/ingest/cursoragent"
)

type ImportResult struct {
	FilesScanned  int
	FilesImported int
	FilesSkipped  int
	FilesFailed   int
	// UnparsedLines counts lines that could not be parsed or normalized and
	// were dropped (broken JSON, malformed payloads). Record types a source
	// deliberately ignores are not counted.
	UnparsedLines int
	// UnparsedDiagnostics holds the first five parse or normalization errors
	// encountered by this import run, in source/file/line encounter order.
	UnparsedDiagnostics []error
	Errors              []error
}

type ImportOptions struct {
	Full       bool
	Inputs     []Input
	InputPaths []string
	Source     ImportSource
}

type ImportSource string

const (
	ImportSourceAll         ImportSource = "all"
	ImportSourceClaudeCode  ImportSource = "claude-code"
	ImportSourceCodex       ImportSource = "codex"
	ImportSourceCursorAgent ImportSource = "cursor-agent"
)

// importSourceSpec ties a concrete ImportSource to its adapter constructor
// used to scan each configured input root.
// ImportSourceAll is intentionally not listed: it means "every entry in this
// table".
type importSourceSpec struct {
	source     ImportSource
	newAdapter func() ingest.Adapter
}

var importSourceSpecs = []importSourceSpec{
	{
		source:     ImportSourceClaudeCode,
		newAdapter: func() ingest.Adapter { return claudecode.NewAdapter(ResolveRepoPath) },
	},
	{
		source:     ImportSourceCodex,
		newAdapter: func() ingest.Adapter { return codex.NewAdapter(ResolveRepoPath) },
	},
	{
		source:     ImportSourceCursorAgent,
		newAdapter: func() ingest.Adapter { return cursoragent.NewAdapter() },
	},
}

// Import uses an already opened DB. Operation callers must hold LockOperation
// from before opening it until all import work has finished.
func Import(db *DB, opts ImportOptions) (*ImportResult, error) {
	source := opts.Source
	if source == "" {
		source = ImportSourceAll
	}
	if !source.Valid() {
		return nil, fmt.Errorf("unknown import source: %s", source)
	}
	// One import time keeps all writes in this run comparable without inventing source timestamps.
	importedAt := timeNow()
	selected := []struct {
		input   Input
		id      int64
		adapter ingest.Adapter
	}{}
	seen := map[string]bool{}
	paths := map[string]bool{}
	for _, path := range opts.InputPaths {
		canonical, err := CanonicalPath(path, "")
		if err != nil {
			return nil, err
		}
		paths[canonical] = true
	}
	for _, input := range opts.Inputs {
		if !validSource(input.Source) || input.Root == "" {
			return nil, fmt.Errorf("invalid input source/root")
		}
		root, err := CanonicalPath(input.Root, "")
		if err != nil {
			return nil, err
		}
		input.Root = root
		if len(paths) > 0 && !paths[root] {
			continue
		}
		cliSource := ImportSource(strings.ReplaceAll(string(input.Source), "_", "-"))
		if source != ImportSourceAll && source != cliSource {
			continue
		}
		key := InputKey(input.Source, root)
		if seen[key] {
			continue
		}
		seen[key] = true
		id, err := db.EnsureInput(input)
		if err != nil {
			return nil, err
		}
		for _, spec := range importSourceSpecs {
			if spec.source == cliSource {
				selected = append(selected, struct {
					input   Input
					id      int64
					adapter ingest.Adapter
				}{input, id, spec.newAdapter()})
				break
			}
		}
	}
	result := &ImportResult{}
	for _, item := range selected {
		var r *ImportResult
		var err error
		if a, ok := item.adapter.(codex.Adapter); ok {
			r, err = importCodexGroups(db, item.id, item.input.Root, a, importedAt, opts.Full)
		} else if a, ok := item.adapter.(claudecode.Adapter); ok {
			r, err = importClaudeSnapshots(db, item.id, item.input.Root, a, importedAt, opts.Full)
		} else if a, ok := item.adapter.(ingest.FileAdapter); ok {
			if opts.Full {
				r, err = replaceCursorInput(db, item.id, item.input.Root, a, importedAt)
			} else {
				r, err = importWithAdapter(db, item.id, item.input.Root, a, importedAt)
			}
		}
		if r != nil {
			result.add(r)
		}
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// ImportSourceChoices returns the valid --source values in CLI display order.
func ImportSourceChoices() []string {
	choices := make([]string, 0, len(importSourceSpecs)+1)
	choices = append(choices, string(ImportSourceAll))
	for _, spec := range importSourceSpecs {
		choices = append(choices, string(spec.source))
	}
	return choices
}

// Valid reports whether s is ImportSourceAll or one of the sources listed in
// importSourceSpecs.
func (s ImportSource) Valid() bool {
	if s == ImportSourceAll {
		return true
	}
	for _, spec := range importSourceSpecs {
		if s == spec.source {
			return true
		}
	}
	return false
}

func (r *ImportResult) add(other *ImportResult) {
	r.FilesScanned += other.FilesScanned
	r.FilesImported += other.FilesImported
	r.FilesSkipped += other.FilesSkipped
	r.FilesFailed += other.FilesFailed
	r.UnparsedLines += other.UnparsedLines
	r.addUnparsedDiagnostics(other.UnparsedDiagnostics)
	r.Errors = append(r.Errors, other.Errors...)
}

func (r *ImportResult) addUnparsedDiagnostics(diagnostics []error) {
	remaining := ingest.MaxUnparsedDiagnostics - len(r.UnparsedDiagnostics)
	if remaining <= 0 {
		return
	}
	if len(diagnostics) > remaining {
		diagnostics = diagnostics[:remaining]
	}
	r.UnparsedDiagnostics = append(r.UnparsedDiagnostics, diagnostics...)
}

func importWithAdapter(db *DB, inputID int64, rootDir string, adapter ingest.FileAdapter, importedAt string) (*ImportResult, error) {
	files, scanErrs := adapter.ScanFiles(rootDir)

	// Scan errors already carry their "scan <path>:" context from the adapter.
	result := &ImportResult{FilesScanned: len(files)}
	result.Errors = append(result.Errors, scanErrs...)
	for _, path := range files {
		state, err := db.GetImportState(inputID, path)
		if err != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: get state: %w", path, err))
			continue
		}

		fi, err := os.Stat(path)
		if err != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: stat: %w", path, err))
			continue
		}

		var offset int64
		if state != nil {
			switch {
			case state.FileSize == fi.Size():
				result.FilesSkipped++
				continue
			case state.FileSize < fi.Size():
				offset = state.LastOffset
			default:
				// File shrunk — re-read from start
				offset = 0
			}
		}

		newTransaction := func() (ingest.ImportTransaction, error) {
			tx, err := db.Begin()
			if err != nil {
				return nil, err
			}
			// Validate the cursor in the same SQLite snapshot as its writes.
			// A full import may have deleted its supporting body since the
			// initial read. Later writes to that snapshot must also fail if
			// another process commits a deletion after this validation.
			current, err := getImportState(tx, inputID, path)
			if err != nil {
				tx.Rollback()
				return nil, err
			}
			if (state == nil) != (current == nil) || (state != nil && current != nil && *state != *current) {
				tx.Rollback()
				return nil, fmt.Errorf("import state changed during import")
			}
			return importTx{tx: tx, inputID: inputID}, nil
		}
		pr, perr := adapter.ProcessFile(newTransaction, path, offset, fi.Size(), importedAt)
		if perr != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", path, perr))
			continue
		}
		result.UnparsedLines += pr.UnparsedLines
		result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
		result.FilesImported++
	}

	return result, nil
}

// timeNow returns the current time in RFC3339 UTC. Overridable for testing.
var timeNow = func() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// cursorFullTx leaves transaction completion to the input replacement, while
// ProcessJSONL retains its per-file parsing and cursor bookkeeping.
type cursorFullTx struct{ importTx }

func (cursorFullTx) Commit() error   { return nil }
func (cursorFullTx) Rollback() error { return nil }

func replaceCursorInput(db *DB, inputID int64, rootDir string, adapter ingest.FileAdapter, importedAt string) (*ImportResult, error) {
	files, scanErrs := adapter.ScanFiles(rootDir)
	result := &ImportResult{FilesScanned: len(files), Errors: scanErrs}
	if len(scanErrs) > 0 {
		result.FilesFailed = len(files)
		return result, nil
	}
	tx, err := db.Begin()
	if err != nil {
		return result, fmt.Errorf("%s: begin: %w", rootDir, err)
	}
	defer tx.Rollback()
	fail := func(err error) (*ImportResult, error) {
		result.FilesFailed = len(files)
		result.Errors = append(result.Errors, err)
		return result, nil
	}
	for _, table := range []string{"messages", "sessions", "import_state"} {
		if _, err := tx.Exec("DELETE FROM "+table+" WHERE input_id=?", inputID); err != nil {
			return fail(fmt.Errorf("%s: delete: %w", rootDir, err))
		}
	}
	newTransaction := func() (ingest.ImportTransaction, error) {
		return cursorFullTx{importTx{tx: tx, inputID: inputID}}, nil
	}
	for _, path := range files {
		fi, err := os.Stat(path)
		if err != nil {
			return fail(fmt.Errorf("%s: stat: %w", path, err))
		}
		pr, err := adapter.ProcessFile(newTransaction, path, 0, fi.Size(), importedAt)
		if err != nil {
			return fail(fmt.Errorf("%s: %w", path, err))
		}
		result.UnparsedLines += pr.UnparsedLines
		result.addUnparsedDiagnostics(pr.UnparsedDiagnostics)
	}
	if err := tx.Commit(); err != nil {
		return fail(fmt.Errorf("%s: commit: %w", rootDir, err))
	}
	result.FilesImported = len(files)
	return result, nil
}
