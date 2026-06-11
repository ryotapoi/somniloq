package core

import (
	"fmt"
	"os"
	"time"

	"github.com/ryotapoi/somniloq/internal/ingest"
	"github.com/ryotapoi/somniloq/internal/ingest/claudecode"
	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

type ImportResult struct {
	FilesScanned  int
	FilesImported int
	FilesSkipped  int
	FilesFailed   int
	Errors        []error
}

type ImportOptions struct {
	Full             bool
	ProjectsDir      string
	CodexSessionsDir string
	Source           ImportSource
}

type ImportSource string

const (
	ImportSourceAll        ImportSource = "all"
	ImportSourceClaudeCode ImportSource = "claude-code"
	ImportSourceCodex      ImportSource = "codex"
)

// importSourceSpec ties a concrete ImportSource to its adapter constructor
// and to the ImportOptions field that carries its scan root. Adding a new
// source means adding a constant, a table entry, an ImportOptions field, and
// the CLI side in cmd/somniloq (default directory wiring plus the hand-written
// source lists in usage/help/error strings).
// ImportSourceAll is intentionally not listed: it means "every entry in this
// table".
type importSourceSpec struct {
	source     ImportSource
	newAdapter func() ingest.Adapter
	rootDir    func(opts ImportOptions) string
}

var importSourceSpecs = []importSourceSpec{
	{
		source:     ImportSourceClaudeCode,
		newAdapter: func() ingest.Adapter { return claudecode.NewAdapter(ResolveRepoPath) },
		rootDir:    func(opts ImportOptions) string { return opts.ProjectsDir },
	},
	{
		source:     ImportSourceCodex,
		newAdapter: func() ingest.Adapter { return codex.NewAdapter(ResolveRepoPath) },
		rootDir:    func(opts ImportOptions) string { return opts.CodexSessionsDir },
	},
}

func Import(db *DB, opts ImportOptions) (*ImportResult, error) {
	source := opts.Source
	if source == "" {
		source = ImportSourceAll
	}
	if !source.Valid() {
		return nil, fmt.Errorf("unknown import source: %s", source)
	}
	if opts.Full {
		if err := db.DeleteAll(); err != nil {
			return nil, fmt.Errorf("delete all: %w", err)
		}
	}

	result := &ImportResult{}
	for _, spec := range importSourceSpecs {
		if source != ImportSourceAll && source != spec.source {
			continue
		}
		r, err := importWithAdapter(db, spec.rootDir(opts), spec.newAdapter())
		if err != nil {
			return nil, err
		}
		result.add(r)
	}
	return result, nil
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
	r.Errors = append(r.Errors, other.Errors...)
}

func importWithAdapter(db *DB, rootDir string, adapter ingest.Adapter) (*ImportResult, error) {
	files, err := adapter.ScanFiles(rootDir)
	if err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	result := &ImportResult{FilesScanned: len(files)}

	for _, file := range files {
		state, err := db.GetImportState(file.Path)
		if err != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: get state: %w", file.Path, err))
			continue
		}

		fi, err := os.Stat(file.Path)
		if err != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: stat: %w", file.Path, err))
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

		importedAt := timeNow()
		if _, perr := adapter.ProcessFile(db, file, offset, fi.Size(), importedAt); perr != nil {
			result.FilesFailed++
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", file.Path, perr))
			continue
		}
		result.FilesImported++
	}

	return result, nil
}

// timeNow returns the current time in RFC3339 UTC. Overridable for testing.
var timeNow = func() string {
	return time.Now().UTC().Format(time.RFC3339)
}
