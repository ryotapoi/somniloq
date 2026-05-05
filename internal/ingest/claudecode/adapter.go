package claudecode

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type RepoResolver func(cwd string) string

type Adapter struct {
	resolveRepoPath RepoResolver
}

func NewAdapter(resolveRepoPath RepoResolver) Adapter {
	return Adapter{resolveRepoPath: resolveRepoPath}
}

func (a Adapter) Source() ingest.Source {
	return ingest.SourceClaudeCode
}

func (a Adapter) ScanFiles(projectsDir string) ([]ingest.File, error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var files []ingest.File
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
			files = append(files, ingest.File{
				Path:      filepath.Join(subPath, name),
				SessionID: sessionID,
			})
		}
	}
	return files, nil
}

func (a Adapter) ProcessFile(store ingest.Store, file ingest.File, offset, fileSize int64, importedAt string) (int64, error) {
	if a.resolveRepoPath == nil {
		return offset, errors.New("resolve repo path is nil")
	}

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

	tx, err := store.BeginImport()
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
				repo, ok := repoCache[rec.CWD]
				if !ok {
					repo = a.resolveRepoPath(rec.CWD)
					repoCache[rec.CWD] = repo
				}
				normalized, perr := NormalizeRecord(rec, repo)
				if perr != nil {
					continue
				}
				if uerr := tx.UpsertSession(normalized.Session, importedAt); uerr != nil {
					return offset, fmt.Errorf("upsert session: %w", uerr)
				}
				hasBody = true
				if strings.TrimSpace(normalized.Message.Content) == "" {
					continue
				}
				if ierr := tx.InsertMessage(normalized.Message); ierr != nil {
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
		if uerr := tx.UpdateSessionTitle(ingest.SourceClaudeCode, sid, t, importedAt); uerr != nil {
			return offset, fmt.Errorf("flush title: %w", uerr)
		}
	}
	for sid, name := range agentNames {
		if uerr := tx.UpdateSessionAgentName(ingest.SourceClaudeCode, sid, name, importedAt); uerr != nil {
			return offset, fmt.Errorf("flush agent name: %w", uerr)
		}
	}

	if uerr := tx.UpsertImportState(ingest.ImportState{
		JSONLPath:  file.Path,
		Source:     ingest.SourceClaudeCode,
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
