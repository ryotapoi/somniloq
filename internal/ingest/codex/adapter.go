package codex

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type Adapter struct {
	resolveRepoPath ingest.RepoResolver
}

func NewAdapter(resolveRepoPath ingest.RepoResolver) Adapter {
	return Adapter{resolveRepoPath: resolveRepoPath}
}

// NewMigrationAdapter shares repository resolutions across the index and
// per-owner body passes of one Migrate call.
func NewMigrationAdapter(resolveRepoPath ingest.RepoResolver) Adapter {
	return Adapter{resolveRepoPath: memoizeRepoResolver(resolveRepoPath)}
}

// ForImportPass shares repository lookups across per-owner body loads while
// keeping a later import free to observe repository changes.
func (a Adapter) ForImportPass() Adapter {
	a.resolveRepoPath = memoizeRepoResolver(a.resolveRepoPath)
	return a
}

func (a Adapter) ScanFiles(rootDir string) ([]string, []error) {
	return ingest.ScanFilesRecursive(rootDir, func(path string) bool {
		return strings.HasSuffix(path, ".jsonl")
	})
}

// fileHandler holds per-file state while building an owner group. Line numbers
// count every physical line (blank ones included) because message UUIDs are
// derived from file path + line number.
type fileHandler struct {
	resolveRepoPath ingest.RepoResolver
	importedAt      string
	path            string
	meta            *ingest.SessionMeta
	lineNumber      int
	historyStart    *int
	number          int
	byteOffset      int64
	failures        []ingest.ParseFailure
}

func (h *fileHandler) HandleLine(tx ingest.ImportTransaction, line []byte) (ingest.LineResult, error) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return h.handleParsedLine(tx, line, nil, nil)
	}
	rec, perr := ParseRecord(trimmed)
	return h.handleParsedLine(tx, line, rec, perr)
}

func (h *fileHandler) handleParsedLine(tx ingest.ImportTransaction, line []byte, rec *RawRecord, perr error) (ingest.LineResult, error) {
	h.lineNumber++
	defer func() { h.byteOffset += int64(len(line)) }()
	if rec == nil && perr == nil {
		return ingest.LineResult{Outcome: ingest.LineIgnored}, nil
	}
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
	if h.historyStart != nil {
		if rec.Ordinal == nil {
			normalized.Message.Membership = "unresolved"
		} else if *rec.Ordinal < *h.historyStart {
			normalized.Message.Membership = "context"
		}
	}
	if normalized.Message.Membership != "body" {
		normalized.Session.StartedAt = ""
		normalized.Session.EndedAt = ""
	} else if strings.TrimSpace(normalized.Message.Content) != "" {
		h.number++
		normalized.Message.Number = h.number
	}
	if err := ingest.PersistMessage(tx, normalized, h.importedAt); err != nil {
		return ingest.LineResult{}, err
	}
	if normalized.Message.Membership == "unresolved" {
		return ingest.LineResult{Outcome: ingest.LineWroteBody, Diagnostic: fmt.Errorf("%s:%d: explicit inheritance boundary with missing ordinal; retained unresolved", h.path, h.lineNumber)}, nil
	}
	return ingest.LineResult{Outcome: ingest.LineWroteBody}, nil
}

func (h *fileHandler) unparsed(err error) ingest.LineResult {
	diagnostic := fmt.Errorf("%s:%d: %w", h.path, h.lineNumber, err)
	h.failures = append(h.failures, ingest.ParseFailure{Offset: h.byteOffset, Diagnostic: diagnostic})
	return ingest.LineResult{
		Outcome:    ingest.LineUnparsed,
		Diagnostic: fmt.Errorf("%s:%d: %w", h.path, h.lineNumber, err),
	}
}

func (h *fileHandler) applySessionMeta(rec *RawRecord) error {
	meta, err := parseSessionMeta(rec)
	if err != nil {
		return err
	}
	if h.meta != nil {
		return nil
	}
	var payload SessionMetaPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return err
	}
	meta.RepoPath = h.resolveRepoPath(meta.CWD)
	h.historyStart = payload.HistoryStart
	h.meta = meta
	return nil
}

// Cache both repository roots and failure fallbacks only for one processing
// pass. A later pass retries Git, so repository changes cannot leak across runs.
func memoizeRepoResolver(resolve ingest.RepoResolver) ingest.RepoResolver {
	paths := map[string]string{}
	return func(cwd string) string {
		if path, ok := paths[cwd]; ok {
			return path
		}
		path := resolve(cwd)
		paths[cwd] = path
		return path
	}
}
