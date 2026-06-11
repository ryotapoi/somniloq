package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type Adapter struct {
	resolveRepoPath ingest.RepoResolver
}

func NewAdapter(resolveRepoPath ingest.RepoResolver) Adapter {
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

// fileHandler holds the per-file state of one ProcessFile pass. Line numbers
// count every physical line (blank ones included) because message UUIDs are
// derived from file path + line number.
type fileHandler struct {
	resolveRepoPath ingest.RepoResolver
	importedAt      string
	path            string
	meta            SessionMeta
	hasMeta         bool
	lineNumber      int
}

func (a Adapter) ProcessFile(store ingest.Store, file ingest.File, offset, fileSize int64, importedAt string) (ingest.ProcessResult, error) {
	if a.resolveRepoPath == nil {
		return ingest.ProcessResult{NewOffset: offset}, errors.New("resolve repo path is nil")
	}
	h := &fileHandler{
		resolveRepoPath: a.resolveRepoPath,
		importedAt:      importedAt,
	}
	return ingest.ProcessJSONL(store, ingest.SourceCodex, h, file, offset, fileSize, importedAt)
}

// Begin recovers session_meta from the already-imported prefix so incremental
// imports can normalize messages that appear after the offset.
func (h *fileHandler) Begin(path string, offset int64) error {
	h.path = path
	if offset <= 0 {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = ingest.ForEachLine(f, offset, func(line []byte) error {
		h.lineNumber++
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		rec, perr := ParseRecord(trimmed)
		if perr != nil || rec.Type != "session_meta" {
			return nil
		}
		nextMeta, perr := ParseSessionMeta(rec, h.resolveRepoPathFromRecord(rec))
		if perr != nil {
			return nil
		}
		h.meta = *nextMeta
		h.hasMeta = true
		return nil
	})
	return err
}

func (h *fileHandler) HandleLine(tx ingest.ImportTransaction, line []byte) (ingest.LineOutcome, error) {
	h.lineNumber++
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return ingest.LineIgnored, nil
	}

	rec, perr := ParseRecord(trimmed)
	if perr != nil {
		return ingest.LineUnparsed, nil
	}

	if rec.Type == "session_meta" {
		nextMeta, perr := ParseSessionMeta(rec, h.resolveRepoPathFromRecord(rec))
		if perr != nil {
			return ingest.LineUnparsed, nil
		}
		h.meta = *nextMeta
		h.hasMeta = true
		return ingest.LineIgnored, nil
	}

	ok, _, perr := IsMessageRecord(rec)
	if perr != nil {
		return ingest.LineUnparsed, nil
	}
	if !ok || !h.hasMeta {
		return ingest.LineIgnored, nil
	}

	normalized, perr := NormalizeMessage(rec, h.meta, h.path, h.lineNumber)
	if perr != nil {
		return ingest.LineUnparsed, nil
	}
	if uerr := tx.UpsertSession(normalized.Session, h.importedAt); uerr != nil {
		return ingest.LineIgnored, fmt.Errorf("upsert session: %w", uerr)
	}
	if strings.TrimSpace(normalized.Message.Content) == "" {
		return ingest.LineWroteBody, nil
	}
	if ierr := tx.InsertMessage(normalized.Message); ierr != nil {
		return ingest.LineWroteBody, fmt.Errorf("insert message: %w", ierr)
	}
	return ingest.LineWroteBody, nil
}

func (h *fileHandler) Flush(tx ingest.ImportTransaction) error {
	return nil
}

func (h *fileHandler) resolveRepoPathFromRecord(rec *RawRecord) string {
	var payload SessionMetaPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return ""
	}
	return h.resolveRepoPath(payload.CWD)
}
