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
	Full        bool
	ProjectsDir string
}

func Import(db *DB, opts ImportOptions) (*ImportResult, error) {
	return importWithAdapter(db, opts.Full, opts.ProjectsDir, claudecode.NewAdapter(ResolveRepoPath))
}

func ImportCodex(db *DB, opts ImportOptions) (*ImportResult, error) {
	return importWithAdapter(db, opts.Full, opts.ProjectsDir, codex.NewAdapter(ResolveRepoPath))
}

func importWithAdapter(db *DB, full bool, rootDir string, adapter ingest.Adapter) (*ImportResult, error) {
	if full {
		if err := db.DeleteAll(); err != nil {
			return nil, fmt.Errorf("delete all: %w", err)
		}
	}

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
