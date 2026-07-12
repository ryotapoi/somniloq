package claudecode

import (
	"bytes"
	"errors"
	"fmt"
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
	return ingest.SourceClaudeCode
}

func (a Adapter) ScanFiles(projectsDir string) ([]ingest.File, []error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("scan %s: %w", projectsDir, err)}
	}

	var files []ingest.File
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projDir := entry.Name()
		subPath := filepath.Join(projectsDir, projDir)
		subEntries, err := os.ReadDir(subPath)
		if err != nil {
			errs = append(errs, fmt.Errorf("scan %s: %w", subPath, err))
			continue
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
	return files, errs
}

// SessionMetaWriter is the claude-code-specific persistence surface for
// custom-title / agent-name records buffered during a file pass. The shared
// ingest.ImportTransaction stays source-neutral; the concrete transaction in
// internal/core implements this interface and Flush asserts for it.
type SessionMetaWriter interface {
	UpdateSessionTitle(source ingest.Source, sessionID, title, importedAt string) error
	UpdateSessionAgentName(source ingest.Source, sessionID, agentName, importedAt string) error
}

// fileHandler holds the per-file state of one ProcessFile pass.
type fileHandler struct {
	resolveRepoPath ingest.RepoResolver
	importedAt      string
	path            string
	lineNumber      int
	diagnostic      error
	repoCache       map[string]string
	titles          map[string]string
	agentNames      map[string]string
}

func (a Adapter) ProcessFile(newTransaction ingest.NewImportTransaction, file ingest.File, offset, fileSize int64, importedAt string) (ingest.ProcessResult, error) {
	if a.resolveRepoPath == nil {
		return ingest.ProcessResult{NewOffset: offset}, errors.New("resolve repo path is nil")
	}
	h := &fileHandler{
		resolveRepoPath: a.resolveRepoPath,
		importedAt:      importedAt,
		repoCache:       map[string]string{},
		titles:          map[string]string{},
		agentNames:      map[string]string{},
	}
	return ingest.ProcessJSONL(newTransaction, ingest.SourceClaudeCode, h, file, offset, fileSize, importedAt)
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

	lineNumber, err := ingest.CountLineFeeds(f, offset)
	if err != nil {
		return err
	}
	h.lineNumber = lineNumber
	return nil
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

	switch rec.Type {
	case "user", "assistant":
		repo, ok := h.repoCache[rec.CWD]
		if !ok {
			repo = h.resolveRepoPath(rec.CWD)
			h.repoCache[rec.CWD] = repo
		}
		normalized, perr := NormalizeRecord(rec, repo)
		if perr != nil {
			h.setUnparsedDiagnostic(perr)
			return ingest.LineUnparsed, nil
		}
		if err := ingest.PersistMessage(tx, normalized, h.importedAt); err != nil {
			return ingest.LineIgnored, err
		}
		return ingest.LineWroteBody, nil
	case "custom-title":
		h.titles[rec.SessionID] = rec.CustomTitle
	case "agent-name":
		h.agentNames[rec.SessionID] = rec.AgentName
	}
	return ingest.LineIgnored, nil
}

func (h *fileHandler) UnparsedDiagnostic() error {
	return h.diagnostic
}

func (h *fileHandler) setUnparsedDiagnostic(err error) {
	h.diagnostic = fmt.Errorf("%s:%d: %w", h.path, h.lineNumber, err)
}

func (h *fileHandler) Flush(tx ingest.ImportTransaction) error {
	if len(h.titles) == 0 && len(h.agentNames) == 0 {
		return nil
	}
	mw, ok := tx.(SessionMetaWriter)
	if !ok {
		return fmt.Errorf("transaction %T does not implement claudecode.SessionMetaWriter", tx)
	}
	for sid, t := range h.titles {
		if uerr := mw.UpdateSessionTitle(ingest.SourceClaudeCode, sid, t, h.importedAt); uerr != nil {
			return fmt.Errorf("flush title: %w", uerr)
		}
	}
	for sid, name := range h.agentNames {
		if uerr := mw.UpdateSessionAgentName(ingest.SourceClaudeCode, sid, name, h.importedAt); uerr != nil {
			return fmt.Errorf("flush agent name: %w", uerr)
		}
	}
	return nil
}
