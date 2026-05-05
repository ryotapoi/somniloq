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

func Import(db *DB, opts ImportOptions) (*ImportResult, error) {
	source := opts.Source
	if source == "" {
		source = ImportSourceAll
	}
	if !source.valid() {
		return nil, fmt.Errorf("unknown import source: %s", source)
	}
	if opts.Full {
		if err := db.DeleteAll(); err != nil {
			return nil, fmt.Errorf("delete all: %w", err)
		}
	}

	result := &ImportResult{}
	switch source {
	case ImportSourceAll:
		claudeResult, err := importWithAdapter(db, opts.ProjectsDir, claudecode.NewAdapter(ResolveRepoPath))
		if err != nil {
			return nil, err
		}
		result.add(claudeResult)
		codexResult, err := importWithAdapter(db, opts.CodexSessionsDir, codex.NewAdapter(ResolveRepoPath))
		if err != nil {
			return nil, err
		}
		result.add(codexResult)
	case ImportSourceClaudeCode:
		claudeResult, err := importWithAdapter(db, opts.ProjectsDir, claudecode.NewAdapter(ResolveRepoPath))
		if err != nil {
			return nil, err
		}
		result.add(claudeResult)
	case ImportSourceCodex:
		codexResult, err := importWithAdapter(db, opts.CodexSessionsDir, codex.NewAdapter(ResolveRepoPath))
		if err != nil {
			return nil, err
		}
		result.add(codexResult)
	}
	return result, nil
}

func (s ImportSource) valid() bool {
	switch s {
	case ImportSourceAll, ImportSourceClaudeCode, ImportSourceCodex:
		return true
	default:
		return false
	}
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
