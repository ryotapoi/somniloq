package codex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
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

func (a Adapter) ScanFiles(rootDir string) ([]ingest.File, []error) {
	var files []ingest.File
	var errs []error
	walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == rootDir {
				return err
			}
			errs = append(errs, fmt.Errorf("scan %s: %w", path, err))
			return nil
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
	if walkErr != nil {
		// A missing rootDir means the source is unused.
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil, nil
		}
		// Root scan failures are fatal for that source, matching the Claude
		// Code adapter. Descendant failures stay non-fatal via errs above.
		return nil, []error{fmt.Errorf("scan %s: %w", rootDir, walkErr)}
	}
	if len(files) == 0 && len(errs) == 0 {
		return nil, nil
	}
	return files, errs
}

// fileHandler holds the per-file state of one ProcessFile pass. Line numbers
// count every physical line (blank ones included) because message UUIDs are
// derived from file path + line number.
type fileHandler struct {
	resolveRepoPath ingest.RepoResolver
	importedAt      string
	path            string
	meta            sessionMetaCursor
	hasMeta         bool
	lineNumber      int
	diagnostic      error
}

func (a Adapter) ProcessFile(newTransaction ingest.NewImportTransaction, file ingest.File, offset, fileSize int64, importedAt string) (ingest.ProcessResult, error) {
	if a.resolveRepoPath == nil {
		return ingest.ProcessResult{NewOffset: offset}, errors.New("resolve repo path is nil")
	}
	h := &fileHandler{
		resolveRepoPath: a.resolveRepoPath,
		importedAt:      importedAt,
	}
	return ingest.ProcessJSONL(newTransaction, ingest.SourceCodex, h, file, offset, fileSize, importedAt)
}

// Begin recovers session_meta from the already-imported prefix so incremental
// imports can normalize messages that appear after the offset. Parse failures
// in this prefix are intentionally ignored: the initial import already counted
// them as unparsed, so resuming must not count them again (ADR 0009).
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

	lineNumber, err := ingest.CountLineFeeds(f, offset)
	if err != nil {
		return err
	}
	h.lineNumber = lineNumber
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	_, err = ingest.ForEachLine(io.LimitReader(f, offset), -1, func(line []byte) error {
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		rec, perr := ParseRecord(trimmed)
		if perr != nil || rec.Type != "session_meta" {
			return nil
		}
		if err := h.applySessionMeta(rec); err != nil {
			return nil
		}
		return nil
	})
	return err
}

func (h *fileHandler) HandleLine(tx ingest.ImportTransaction, line []byte) (ingest.LineOutcome, error) {
	h.lineNumber++
	h.diagnostic = nil
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return ingest.LineIgnored, nil
	}

	rec, perr := ParseRecord(trimmed)
	if perr != nil {
		h.setUnparsedDiagnostic(perr)
		return ingest.LineUnparsed, nil
	}

	if rec.Type == "session_meta" {
		if err := h.applySessionMeta(rec); err != nil {
			h.setUnparsedDiagnostic(err)
			return ingest.LineUnparsed, nil
		}
		return ingest.LineIgnored, nil
	}

	if rec.Type != "response_item" {
		return ingest.LineIgnored, nil
	}
	payload, err := parseResponseItem(rec)
	if err != nil {
		h.setUnparsedDiagnostic(err)
		return ingest.LineUnparsed, nil
	}
	if !isConversationMessage(payload) || !h.hasMeta {
		return ingest.LineIgnored, nil
	}

	normalized, err := normalizeMessage(rec, payload, h.meta, h.path, h.lineNumber)
	if err != nil {
		h.setUnparsedDiagnostic(err)
		return ingest.LineUnparsed, nil
	}
	if err := ingest.PersistMessage(tx, normalized, h.importedAt); err != nil {
		return ingest.LineIgnored, err
	}
	return ingest.LineWroteBody, nil
}

func (h *fileHandler) UnparsedDiagnostic() error {
	return h.diagnostic
}

func (h *fileHandler) setUnparsedDiagnostic(err error) {
	h.diagnostic = fmt.Errorf("%s:%d: %w", h.path, h.lineNumber, err)
}

func (h *fileHandler) Flush(tx ingest.ImportTransaction) error {
	return nil
}

func (h *fileHandler) applySessionMeta(rec *RawRecord) error {
	meta, err := parseSessionMetaCursor(rec, h.resolveRepoPath)
	if err != nil {
		return err
	}
	h.meta = *meta
	h.hasMeta = true
	return nil
}
