package core

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
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

type JSONLFile struct {
	Path      string
	SessionID string
}

func Import(db *DB, opts ImportOptions) (*ImportResult, error) {
	if opts.Full {
		if err := db.DeleteAll(); err != nil {
			return nil, fmt.Errorf("delete all: %w", err)
		}
	}

	files, err := ScanJSONLFiles(opts.ProjectsDir)
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
		if _, perr := processFile(db, file, offset, fi.Size(), importedAt); perr != nil {
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

func processFile(db *DB, file JSONLFile, offset, fileSize int64, importedAt string) (int64, error) {
	f, err := os.Open(file.Path)
	if err != nil {
		return offset, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return offset, err
		}
	}

	reader := bufio.NewReaderSize(f, 64*1024)
	currentOffset := offset

	tx, err := db.Begin()
	if err != nil {
		return offset, err
	}
	defer tx.Rollback()

	repoCache := map[string]string{}
	titles := map[string]string{}
	agentNames := map[string]string{}
	// Tracks whether any user/assistant record has been seen in this file
	// (across all imports, since meta-only files don't advance import_state).
	// If false at EOF, no sessions row exists yet and the buffered title /
	// agent-name UPDATEs would all be 0-row no-ops, permanently losing the
	// values. In that case we skip the flush and import_state advance so the
	// next import re-reads from offset 0.
	hasBody := offset > 0

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			currentOffset += int64(len(line))
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}

			rec, perr := ParseRecord(trimmed)
			if perr != nil {
				continue
			}

			switch rec.Type {
			case "user", "assistant":
				msg, perr := ParseMessage(rec)
				if perr != nil {
					continue
				}
				repo, ok := repoCache[rec.CWD]
				if !ok {
					repo = ResolveRepoPath(rec.CWD)
					repoCache[rec.CWD] = repo
				}
				meta := SessionMeta{
					SessionID: rec.SessionID,
					CWD:       rec.CWD,
					RepoPath:  repo,
					GitBranch: rec.GitBranch,
					Version:   rec.Version,
					StartedAt: rec.Timestamp,
					EndedAt:   rec.Timestamp,
				}
				if uerr := upsertSession(tx, meta, importedAt); uerr != nil {
					return offset, fmt.Errorf("upsert session: %w", uerr)
				}
				hasBody = true
				if strings.TrimSpace(msg.Content) == "" {
					continue
				}
				if ierr := insertMessage(tx, *msg); ierr != nil {
					return offset, fmt.Errorf("insert message: %w", ierr)
				}
			case "custom-title":
				titles[rec.SessionID] = rec.CustomTitle
			case "agent-name":
				agentNames[rec.SessionID] = rec.AgentName
			}
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return offset, err
		}
	}

	if !hasBody {
		// Meta-only file: no sessions row exists yet, so flushing titles /
		// agent-names would silently no-op. Skip the flush and the import_state
		// advance so the next import re-reads these meta records from offset 0
		// after a body record finally appears.
		return offset, nil
	}

	for sid, t := range titles {
		if uerr := updateSessionTitle(tx, sid, t, importedAt); uerr != nil {
			return offset, fmt.Errorf("flush title: %w", uerr)
		}
	}
	for sid, name := range agentNames {
		if uerr := updateSessionAgentName(tx, sid, name, importedAt); uerr != nil {
			return offset, fmt.Errorf("flush agent name: %w", uerr)
		}
	}

	if uerr := upsertImportState(tx, ImportState{
		JSONLPath:  file.Path,
		FileSize:   fileSize,
		LastOffset: currentOffset,
		ImportedAt: importedAt,
	}); uerr != nil {
		return offset, fmt.Errorf("upsert import state: %w", uerr)
	}

	if err := tx.Commit(); err != nil {
		return offset, err
	}
	return currentOffset, nil
}

func ScanJSONLFiles(projectsDir string) ([]JSONLFile, error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var files []JSONLFile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projDir := entry.Name()
		subPath := filepath.Join(projectsDir, projDir)
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			return nil, fmt.Errorf("read dir %s: %w", subPath, err)
		}
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			name := se.Name()
			if !strings.HasSuffix(name, ".jsonl") {
				continue
			}
			sessionID := strings.TrimSuffix(name, ".jsonl")
			files = append(files, JSONLFile{
				Path:      filepath.Join(subPath, name),
				SessionID: sessionID,
			})
		}
	}
	return files, nil
}
