package cursoragent

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type Adapter struct{}

func NewAdapter() Adapter { return Adapter{} }

func (Adapter) ScanFiles(rootDir string) ([]string, []error) {
	return ingest.ScanFilesRecursive(rootDir, func(path string) bool {
		return acceptedPath(rootDir, path)
	})
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

func (h *fileHandler) HandleLine(tx ingest.ImportTransaction, line []byte) (ingest.LineResult, error) {
	h.lineNumber++
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}
	record, err := parseRecord(trimmed)
	if err != nil {
		return h.unparsed(err), nil
	}
	if record.Role != "user" && record.Role != "assistant" {
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}
	normalized, err := normalizeRecord(record, h.sessionID, h.path, h.lineNumber)
	if err != nil {
		return h.unparsed(err), nil
	}
	if err := ingest.PersistMessage(tx, normalized, h.importedAt); err != nil {
		return ingest.LineResult{}, err
	}
	return ingest.LineResult{Outcome: ingest.LineWroteBody}, nil
}

func (h *fileHandler) unparsed(err error) ingest.LineResult {
	return ingest.LineResult{
		Outcome:    ingest.LineUnparsed,
		Diagnostic: fmt.Errorf("%s:%d: %w", h.path, h.lineNumber, err),
	}
}

func (h *fileHandler) Flush(ingest.ImportTransaction) error { return nil }
