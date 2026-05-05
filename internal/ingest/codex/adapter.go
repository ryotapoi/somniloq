package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	return ingest.SourceCodex
}

func (a Adapter) ScanFiles(rootDir string) ([]ingest.File, error) {
	var files []ingest.File
	err := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		files = append(files, ingest.File{
			Path:      path,
			SessionID: strings.TrimSuffix(name, ".jsonl"),
		})
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return files, nil
}

func (a Adapter) ProcessFile(store ingest.Store, file ingest.File, offset, fileSize int64, importedAt string) (int64, error) {
	if a.resolveRepoPath == nil {
		return offset, errors.New("resolve repo path is nil")
	}

	meta, hasMeta, lineNumber, err := a.scanPrefix(file.Path, offset)
	if err != nil {
		return offset, err
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

	hasBody := offset > 0

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			lineNumber++
			currentOffset += int64(len(line))
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) == 0 {
				continue
			}

			rec, perr := ParseRecord(trimmed)
			if perr != nil {
				continue
			}

			if rec.Type == "session_meta" {
				nextMeta, perr := ParseSessionMeta(rec, a.resolveRepoPathFromRecord(rec))
				if perr != nil {
					continue
				}
				meta = *nextMeta
				hasMeta = true
				continue
			}

			ok, _, perr := IsMessageRecord(rec)
			if perr != nil || !ok {
				continue
			}
			if !hasMeta {
				continue
			}

			normalized, perr := NormalizeMessage(rec, meta, file.Path, lineNumber)
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
		}

		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return offset, err
		}
	}

	if !hasBody {
		return offset, nil
	}

	if uerr := tx.UpsertImportState(ingest.ImportState{
		JSONLPath:  file.Path,
		Source:     ingest.SourceCodex,
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

func (a Adapter) resolveRepoPathFromRecord(rec *RawRecord) string {
	var payload SessionMetaPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return ""
	}
	return a.resolveRepoPath(payload.CWD)
}

func (a Adapter) scanPrefix(path string, offset int64) (SessionMeta, bool, int, error) {
	var meta SessionMeta
	if offset <= 0 {
		return meta, false, 0, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return meta, false, 0, err
	}
	defer f.Close()

	reader := bufio.NewReaderSize(f, 64*1024)
	var currentOffset int64
	var lineNumber int
	var hasMeta bool

	for currentOffset < offset {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			currentOffset += int64(len(line))
			lineNumber++
			trimmed := bytes.TrimSpace(line)
			if len(trimmed) > 0 {
				rec, perr := ParseRecord(trimmed)
				if perr == nil && rec.Type == "session_meta" {
					nextMeta, perr := ParseSessionMeta(rec, a.resolveRepoPathFromRecord(rec))
					if perr == nil {
						meta = *nextMeta
						hasMeta = true
					}
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return meta, hasMeta, lineNumber, err
		}
	}

	return meta, hasMeta, lineNumber, nil
}
