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

type FileReport struct {
	Path     string
	Data     []byte
	Failures []ParseFailure
}

// Group is a canonical snapshot of every discovered rollout for one owner.
// No SQLite work happens until all of its records have been validated.
type Group struct {
	Session  ingest.SessionMeta
	Messages []ingest.NormalizedMessage
	States   []ingest.ImportState
	Reports  []FileReport
	Err      error
	Files    int
}

type collector struct {
	meta     *ingest.SessionMeta
	messages []ingest.NormalizedMessage
	state    ingest.ImportState
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
	c.messages = append(c.messages, m)
	return nil
}
func (c *collector) UpsertImportState(s ingest.ImportState) error { c.state = s; return nil }
func (c *collector) Commit() error                                { return nil }
func (c *collector) Rollback() error                              { return nil }

// BuildGroups reads full snapshots so edits to earlier rollouts renumber the
// entire owner rather than append records in import encounter order.
func (a Adapter) BuildGroups(root string, paths []string, importedAt string) ([]Group, []error) {
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
		c := &collector{}
		h := &fileHandler{resolveRepoPath: a.resolveRepoPath, importedAt: importedAt}
		h.path = path
		hasBody := false
		var unfinishedTail int64
		_, err = ingest.ForEachLine(bytes.NewReader(data), -1, func(line []byte) error {
			outcome, err := h.HandleLine(c, line)
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
		if hasBody {
			c.state = ingest.ImportState{JSONLPath: path, Source: ingest.SourceCodex, FileSize: int64(len(data)), LastOffset: int64(len(data)) - unfinishedTail, ImportedAt: importedAt}
		}
		if c.meta == nil {
			c.meta = h.meta
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			continue
		}
		if c.meta == nil || c.state.JSONLPath == "" {
			groups = append(groups, Group{Files: 1, Reports: []FileReport{{Path: path, Data: data, Failures: h.failures}}})
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
		if g.Session.ParentSessionID != c.meta.ParentSessionID {
			g.Err = fmt.Errorf("%s: conflicting explicit parent references", id)
		}
		sum := sha256.Sum256(data)
		c.state.ContentHash = hex.EncodeToString(sum[:])
		if c.state.JSONLPath != "" {
			g.States = append(g.States, c.state)
		}
		g.Reports = append(g.Reports, FileReport{Path: path, Data: data, Failures: h.failures})
		for _, m := range c.messages {
			rel, _ := filepath.Rel(root, path)
			m.OriginPath = filepath.ToSlash(rel)
			g.Messages = append(g.Messages, m)
		}
	}
	for i := range groups {
		g := &groups[i]
		seen := map[string]string{}
		number := 0
		messages := g.Messages[:0]
		for _, m := range g.Messages {
			// Context and unresolved records retain their physical provenance. Only
			// owner body participates in payload-ID deduplication and numbering.
			if m.Membership == "body" {
				signature, _ := json.Marshal(struct {
					Role      string
					Blocks    []string
					Timestamp string
				}{m.Role, m.Blocks, m.Timestamp})
				if m.PayloadID != "" {
					if old, ok := seen[m.PayloadID]; ok {
						if old != string(signature) {
							g.Err = fmt.Errorf("%s: conflicting payload ID %q", g.Session.SessionID, m.PayloadID)
						}
						continue
					}
					seen[m.PayloadID] = string(signature)
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
