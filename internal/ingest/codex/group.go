package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type ParseFailure struct {
	Offset     int64
	Diagnostic error
}

type PhysicalLine struct {
	UUID string
	Line int
}

type FileReport struct {
	Lines    []PhysicalLine
	Path     string
	Data     []byte
	Hash     [sha256.Size]byte
	Failures []ParseFailure
}

// Group is a canonical snapshot of every discovered rollout for one owner.
// No SQLite work happens until all of its records have been validated.
type Group struct {
	Session  ingest.SessionMeta
	Messages []ingest.NormalizedMessage
	States   []ingest.ImportState
	// EmptyPaths retain valid owner evidence without advancing bodyless cursors.
	EmptyPaths []string
	Reports    []FileReport
	Err        error
	Files      int
}

type collector struct {
	meta           *ingest.SessionMeta
	messages       []ingest.NormalizedMessage
	state          ingest.ImportState
	indexOnly      bool
	unresolvedLine int
}

func (c *collector) UpsertSession(meta ingest.SessionMeta, _ string) error {
	if c.meta == nil {
		m := meta
		m.StartedAt = ""
		m.EndedAt = ""
		c.meta = &m
	}
	return nil
}
func (c *collector) InsertMessage(m ingest.NormalizedMessage) error {
	if c.indexOnly {
		if m.Membership == "unresolved" && c.unresolvedLine == 0 {
			c.unresolvedLine = m.OriginLine
		}
		return nil
	}
	c.messages = append(c.messages, m)
	return nil
}
func (c *collector) UpsertImportState(s ingest.ImportState) error { c.state = s; return nil }
func (c *collector) Commit() error                                { return nil }
func (c *collector) Rollback() error                              { return nil }

// BuildGroups reads full snapshots so edits to earlier rollouts renumber the
// entire owner rather than append records in import encounter order.
func (a Adapter) BuildGroups(root string, paths []string, importedAt string) ([]Group, []error) {
	return a.buildGroups(root, paths, importedAt, false, false)
}

// BuildMigrationGroups preserves every rollout and rejects incomplete ownership evidence.
// Ordinary imports intentionally continue to accept partially parsed snapshots.
func (a Adapter) BuildMigrationGroups(root string, paths []string, importedAt string) ([]Group, []error) {
	return a.buildGroups(root, paths, importedAt, true, false)
}

// BuildMigrationIndex retains owner and physical-line evidence, but releases
// each file's body before the next file is read.
func (a Adapter) BuildMigrationIndex(root string, paths []string, importedAt string) ([]Group, []error) {
	return a.buildGroups(root, paths, importedAt, true, true)
}

func (a Adapter) buildGroups(root string, paths []string, importedAt string, strict, indexOnly bool) ([]Group, []error) {
	resolveRepoPath := memoizeRepoResolver(a.resolveRepoPath)
	paths = append([]string(nil), paths...)
	boundaries := map[string]*int{}
	sort.Slice(paths, func(i, j int) bool {
		x, _ := filepath.Rel(root, paths[i])
		y, _ := filepath.Rel(root, paths[j])
		return x < y
	})
	var groups []Group
	indexes := map[string]int{}
	var errs []error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		c := &collector{indexOnly: indexOnly}
		h := &fileHandler{resolveRepoPath: resolveRepoPath, importedAt: importedAt}
		h.path = path
		hasBody := false
		var unfinishedTail int64
		var lines []PhysicalLine
		var strictErr error
		_, err = ingest.ForEachLine(bytes.NewReader(data), -1, func(line []byte) error {
			var outcome ingest.LineResult
			var err error
			if strict {
				lines = append(lines, PhysicalLine{UUID: messageUUID(path, len(lines)+1), Line: len(lines) + 1})
				trimmed := bytes.TrimSpace(line)
				var rec *RawRecord
				var parseErr error
				if len(trimmed) != 0 {
					rec, parseErr = ParseRecord(trimmed)
				}
				if e := h.validateMigrationRecord(rec); e != nil && strictErr == nil {
					strictErr = fmt.Errorf("%s:%d: %w", path, len(lines), e)
				}
				outcome, err = h.handleParsedLine(c, line, rec, parseErr)
			} else {
				outcome, err = h.HandleLine(c, line)
			}
			if err != nil {
				return err
			}
			if outcome.Outcome == ingest.LineWroteBody {
				hasBody = true
			}
			if outcome.Outcome == ingest.LineUnparsed && line[len(line)-1] != '\n' {
				unfinishedTail = int64(len(line))
			}
			return nil
		})
		if hasBody || (strict && h.meta != nil) {
			c.state = ingest.ImportState{JSONLPath: path, Source: ingest.SourceCodex, FileSize: int64(len(data)), LastOffset: int64(len(data)) - unfinishedTail, ImportedAt: importedAt}
		}
		if c.meta == nil {
			c.meta = h.meta
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if strict && len(h.failures) > 0 && strictErr == nil {
			strictErr = h.failures[0].Diagnostic
		}
		sum := sha256.Sum256(data)
		report := FileReport{Path: path, Hash: sum, Failures: h.failures, Lines: lines}
		if !indexOnly {
			report.Data = data
		}
		if c.meta == nil || (!strict && c.state.JSONLPath == "" && len(h.failures) > 0) {
			groups = append(groups, Group{Files: 1, Err: strictErr, Reports: []FileReport{report}})
			continue
		}
		id := c.meta.SessionID
		index, ok := indexes[id]
		if !ok {
			index = len(groups)
			indexes[id] = index
			groups = append(groups, Group{Session: *c.meta})
		}
		g := &groups[index]
		g.Files++
		if strictErr != nil {
			g.Err = strictErr
		}
		if strict {
			if old, ok := boundaries[id]; ok {
				if (old == nil) != (h.historyStart == nil) || (old != nil && h.historyStart != nil && *old != *h.historyStart) {
					g.Err = fmt.Errorf("%s: conflicting inheritance boundaries", path)
				}
			} else {
				boundaries[id] = h.historyStart
			}
		}
		if g.Session.ParentSessionID != c.meta.ParentSessionID {
			g.Err = fmt.Errorf("%s: conflicting explicit parent references", id)
		}
		c.state.ContentHash = hex.EncodeToString(sum[:])
		if c.state.JSONLPath != "" {
			g.States = append(g.States, c.state)
		} else {
			g.EmptyPaths = append(g.EmptyPaths, path)
		}
		g.Reports = append(g.Reports, report)
		if c.unresolvedLine != 0 {
			g.Err = fmt.Errorf("%s:%d: explicit inheritance boundary with missing ordinal", path, c.unresolvedLine)
		}
		for _, m := range c.messages {
			if strict && m.Membership == "unresolved" {
				g.Err = fmt.Errorf("%s:%d: explicit inheritance boundary with missing ordinal", path, m.OriginLine)
			}
			rel, _ := filepath.Rel(root, path)
			m.OriginPath = filepath.ToSlash(rel)
			g.Messages = append(g.Messages, m)
		}
	}
	if indexOnly {
		return groups, errs
	}
	for i := range groups {
		g := &groups[i]
		seen := map[string]int{}
		number := 0
		messages := g.Messages[:0]
		for _, m := range g.Messages {
			// Context and unresolved records retain their physical provenance. Only
			// owner body participates in payload-ID deduplication and numbering.
			if m.Membership == "body" {
				if m.PayloadID != "" {
					if index, ok := seen[m.PayloadID]; ok {
						if !samePayload(messages[index], m) {
							g.Err = fmt.Errorf("%s: conflicting payload ID %q", g.Session.SessionID, m.PayloadID)
						}
						continue
					}
					seen[m.PayloadID] = len(messages)
				}
				number++
				m.Number = number
			} else {
				m.Number = 0
			}
			messages = append(messages, m)
		}
		g.Messages = messages
	}
	return groups, errs
}

func samePayload(a, b ingest.NormalizedMessage) bool {
	if a.Role != b.Role || a.Timestamp != b.Timestamp || (a.Blocks == nil) != (b.Blocks == nil) || len(a.Blocks) != len(b.Blocks) {
		return false
	}
	for i := range a.Blocks {
		if a.Blocks[i] != b.Blocks[i] {
			return false
		}
	}
	return true
}

// Diagnostics reports only newly encountered failures when an unchanged prefix
// is resumed; a full edit reports its entire newly parsed snapshot.
func (r FileReport) Diagnostics(old *ingest.ImportState) ingest.ProcessResult {
	start := int64(0)
	if old != nil && int64(len(r.Data)) >= old.FileSize {
		sum := sha256.Sum256(r.Data[:old.FileSize])
		if hex.EncodeToString(sum[:]) == old.ContentHash {
			start = old.LastOffset
			if start > 0 && start == old.FileSize && r.Data[start-1] != '\n' {
				start = int64(bytes.LastIndexByte(r.Data[:start], '\n') + 1)
			}
		}
	}
	result := ingest.ProcessResult{}
	for _, f := range r.Failures {
		if f.Offset < start {
			continue
		}
		result.UnparsedLines++
		if len(result.UnparsedDiagnostics) < ingest.MaxUnparsedDiagnostics {
			result.UnparsedDiagnostics = append(result.UnparsedDiagnostics, f.Diagnostic)
		}
	}
	return result
}

func (h *fileHandler) validateMigrationRecord(rec *RawRecord) error {
	if rec == nil {
		return nil // The handler records malformed JSON, including unfinished tails.
	}
	// Only later metadata can conflict with an owner; the handler validates
	// the first record after this check, before applying it.
	if rec.Type == "session_meta" && h.meta != nil {
		meta, err := parseSessionMeta(rec)
		if err != nil {
			return nil
		}
		var payload SessionMetaPayload
		if err := json.Unmarshal(rec.Payload, &payload); err != nil {
			return nil
		}
		if meta.SessionID != h.meta.SessionID {
			if meta.SessionID != h.meta.ParentSessionID {
				return fmt.Errorf("conflicting owner metadata")
			}
			return nil // Embedded direct-parent metadata does not change the owner boundary.
		}
		if meta.ParentSessionID != h.meta.ParentSessionID {
			return fmt.Errorf("conflicting explicit parent references")
		}
		if (payload.HistoryStart == nil) != (h.historyStart == nil) || (payload.HistoryStart != nil && h.historyStart != nil && *payload.HistoryStart != *h.historyStart) {
			return fmt.Errorf("conflicting inheritance boundaries")
		}
	}
	if rec.Type == "response_item" && h.meta == nil {
		payload, err := parseResponseItem(rec)
		if err == nil && isConversationMessage(payload) {
			return fmt.Errorf("message_owner_unknown: conversation message before owner metadata")
		}
	}
	return nil
}
