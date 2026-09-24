package codex

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type Adapter struct {
	resolveRepoPath ingest.RepoResolver
}

func NewAdapter(resolveRepoPath ingest.RepoResolver) Adapter {
	return Adapter{resolveRepoPath: resolveRepoPath}
}

func (a Adapter) ScanFiles(rootDir string) ([]string, []error) {
	return ingest.ScanFilesRecursive(rootDir, func(path string) bool {
		return strings.HasSuffix(path, ".jsonl")
	})
}

// fileHandler holds the per-file state of one ProcessFile pass. Line numbers
// count every physical line (blank ones included) because message UUIDs are
// derived from file path + line number.
type fileHandler struct {
	resolveRepoPath ingest.RepoResolver
	importedAt      string
	path            string
	meta            *sessionMetaCursor
	lineNumber      int
}

func (a Adapter) ProcessFile(newTransaction ingest.NewImportTransaction, path string, offset, fileSize int64, importedAt string) (ingest.ProcessResult, error) {
	if a.resolveRepoPath == nil {
		return ingest.ProcessResult{}, errors.New("resolve repo path is nil")
	}
	h := &fileHandler{
		resolveRepoPath: a.resolveRepoPath,
		importedAt:      importedAt,
	}
	return ingest.ProcessJSONL(newTransaction, ingest.SourceCodex, h, path, offset, fileSize, importedAt)
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

	_, err = ingest.ForEachLine(io.LimitReader(f, offset), -1, func(line []byte) error {
		h.lineNumber += bytes.Count(line, []byte{'\n'})
		trimmed := bytes.TrimSpace(line)
		if len(trimmed) == 0 {
			return nil
		}
		rec, perr := ParseRecord(trimmed)
		if perr != nil || rec.Type != "session_meta" {
			return nil
		}
		_ = h.applySessionMeta(rec)
		return nil
	})
	return err
}

func (h *fileHandler) HandleLine(tx ingest.ImportTransaction, line []byte) (ingest.LineResult, error) {
	h.lineNumber++
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}

	rec, perr := ParseRecord(trimmed)
	if perr != nil {
		return h.unparsed(perr), nil
	}

	if rec.Type == "session_meta" {
		if err := h.applySessionMeta(rec); err != nil {
			return h.unparsed(err), nil
		}
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}

	if rec.Type != "response_item" {
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}
	payload, err := parseResponseItem(rec)
	if err != nil {
		return h.unparsed(err), nil
	}
	// A message arriving before session_meta is Ignored, not Unparsed: the line
	// parses fine, we just cannot attribute it to a session yet. Counting it as
	// unparsed would put a non-zero number on structurally valid rollouts and
	// drown out the real signal.
	if !isConversationMessage(payload) || h.meta == nil {
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}

	normalized, err := normalizeMessage(rec, payload, *h.meta, h.path, h.lineNumber)
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

func (h *fileHandler) Flush(tx ingest.ImportTransaction) error {
	return nil
}

func (h *fileHandler) applySessionMeta(rec *RawRecord) error {
	meta, err := parseSessionMetaCursor(rec, h.resolveRepoPath)
	if err != nil {
		return err
	}
	h.meta = meta
	return nil
}
