package cursoragent

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type Adapter struct{}

func NewAdapter() Adapter { return Adapter{} }

func (Adapter) ScanFiles(rootDir string) ([]string, []error) {
	var files []string
	var errs []error
	walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == rootDir {
				return err
			}
			errs = append(errs, fmt.Errorf("scan %s: %w", path, err))
			return nil
		}
		if !d.IsDir() && acceptedPath(rootDir, path) {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("scan %s: %w", rootDir, walkErr)}
	}
	return files, errs
}

func acceptedPath(rootDir, path string) bool {
	rel, err := filepath.Rel(rootDir, path)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	if len(parts) != 4 || parts[0] == "" || parts[1] != "agent-transcripts" || parts[2] == "" {
		return false
	}
	return parts[3] == parts[2]+".jsonl"
}

func (Adapter) ProcessFile(newTransaction ingest.NewImportTransaction, path string, offset, fileSize int64, importedAt string) (ingest.ProcessResult, error) {
	sessionID, ok := sessionIDFromPath(path)
	if !ok {
		return ingest.ProcessResult{}, fmt.Errorf("invalid Cursor Agent transcript path: %s", path)
	}
	h := &fileHandler{sessionID: sessionID, importedAt: importedAt}
	return ingest.ProcessJSONL(newTransaction, ingest.SourceCursorAgent, h, path, offset, fileSize, importedAt)
}

func sessionIDFromPath(path string) (string, bool) {
	name := filepath.Base(path)
	sessionID := strings.TrimSuffix(name, ".jsonl")
	if sessionID == "" || name != sessionID+".jsonl" || filepath.Base(filepath.Dir(path)) != sessionID || filepath.Base(filepath.Dir(filepath.Dir(path))) != "agent-transcripts" {
		return "", false
	}
	return sessionID, true
}

type fileHandler struct {
	path       string
	sessionID  string
	importedAt string
	lineNumber int
	diagnostic error
}

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
	lines, err := ingest.CountLineFeeds(f, offset)
	if err != nil {
		return err
	}
	h.lineNumber = lines
	return nil
}

func (h *fileHandler) HandleLine(tx ingest.ImportTransaction, line []byte) (ingest.LineOutcome, error) {
	h.lineNumber++
	h.diagnostic = nil
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return ingest.LineIgnored, nil
	}
	record, err := parseRecord(trimmed)
	if err != nil {
		h.setDiagnostic(err)
		return ingest.LineUnparsed, nil
	}
	if record.Role != "user" && record.Role != "assistant" {
		return ingest.LineIgnored, nil
	}
	normalized, err := normalizeRecord(record, h.sessionID, h.path, h.lineNumber)
	if err != nil {
		h.setDiagnostic(err)
		return ingest.LineUnparsed, nil
	}
	if err := ingest.PersistMessage(tx, normalized, h.importedAt); err != nil {
		return ingest.LineIgnored, err
	}
	return ingest.LineWroteBody, nil
}

func (h *fileHandler) UnparsedDiagnostic() error { return h.diagnostic }

func (h *fileHandler) setDiagnostic(err error) {
	h.diagnostic = fmt.Errorf("%s:%d: %w", h.path, h.lineNumber, err)
}

func (h *fileHandler) Flush(ingest.ImportTransaction) error { return nil }
