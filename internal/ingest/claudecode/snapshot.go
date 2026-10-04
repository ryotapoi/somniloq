package claudecode

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

// FileSnapshot separates physical-file relation evidence from numbered text.
// Every pass reads the complete file, including unchanged non-text records.
type FileSnapshot struct {
	State      ingest.ImportState
	Records    []ingest.NormalizedRecord
	Titles     map[string]string
	AgentNames map[string]string
	Failures   []ParseFailure
	Data       []byte
	Parents    map[string]string
}

type ParseFailure struct {
	Offset     int64
	Diagnostic error
}

func (f FileSnapshot) Diagnostics(old *ingest.ImportState) ingest.ProcessResult {
	start := int64(0)
	if old != nil && int64(len(f.Data)) >= old.FileSize {
		sum := sha256.Sum256(f.Data[:old.FileSize])
		if hex.EncodeToString(sum[:]) == old.ContentHash {
			start = old.LastOffset
			if start > 0 && start == old.FileSize && f.Data[start-1] != '\n' {
				start = int64(bytes.LastIndexByte(f.Data[:start], '\n') + 1)
			}
		}
	}
	r := ingest.ProcessResult{}
	for _, failure := range f.Failures {
		if failure.Offset < start {
			continue
		}
		r.UnparsedLines++
		if len(r.UnparsedDiagnostics) < ingest.MaxUnparsedDiagnostics {
			r.UnparsedDiagnostics = append(r.UnparsedDiagnostics, failure.Diagnostic)
		}
	}
	return r
}

// BuildSnapshots performs Claude-specific interpretation before storage writes.
// Direct-parent candidates never cross a physical file or a path root boundary.
func (a Adapter) BuildSnapshots(root string, paths []string, importedAt string) ([]FileSnapshot, []error, []error) {
	sort.Strings(paths)
	var files []FileSnapshot
	var readErrors, diagnostics []error
	candidates := map[string]map[string]bool{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			readErrors = append(readErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		sum := sha256.Sum256(data)
		f := FileSnapshot{Data: data, State: ingest.ImportState{JSONLPath: path, Source: ingest.SourceClaudeCode, FileSize: int64(len(data)), LastOffset: int64(len(data)), ContentHash: hex.EncodeToString(sum[:]), ImportedAt: importedAt}, Titles: map[string]string{}, AgentNames: map[string]string{}, Parents: map[string]string{}}
		rootID := strings.TrimSuffix(filepath.Base(path), ".jsonl")
		agent := ""
		if filepath.Base(filepath.Dir(path)) == "subagents" {
			rootID = filepath.Base(filepath.Dir(filepath.Dir(path)))
			agent = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "agent-"), ".jsonl")
		}
		calls := map[string]map[string]bool{}
		results := map[string][]string{}
		var offset int64
		lineNumber := 0
		_, err = ingest.ForEachLine(bytes.NewReader(data), -1, func(line []byte) error {
			lineNumber++
			at := offset
			offset += int64(len(line))
			fail := func(err error) {
				f.Failures = append(f.Failures, ParseFailure{at, fmt.Errorf("%s:%d: %w", path, lineNumber, err)})
				if line[len(line)-1] != '\n' {
					f.State.LastOffset = at
				}
			}
			if len(bytes.TrimSpace(line)) == 0 {
				return nil
			}
			rec, err := ParseRecord(line)
			if err != nil {
				fail(err)
				return nil
			}
			if agent != "" && (rec.Type == "user" || rec.Type == "assistant") && rec.AgentID != agent {
				fail(fmt.Errorf("agentId does not match physical subagent identity %q", agent))
				return nil
			}
			identity := ingest.Identity(rec.SessionID)
			if agent != "" {
				identity = ingest.Identity(rootID, agent)
			}
			switch rec.Type {
			case "custom-title":
				f.Titles[identity] = rec.CustomTitle
			case "agent-name":
				f.AgentNames[identity] = rec.AgentName
			case "user", "assistant":
				normalized, err := NormalizeRecord(rec, a.resolveRepoPath(rec.CWD))
				if err != nil {
					fail(err)
					return nil
				}
				if strings.TrimSpace(normalized.Message.Content) == "" {
					normalized.Session.StartedAt = ""
					normalized.Session.EndedAt = ""
				}
				normalized.Session.Identity = identity
				normalized.Message.Identity = identity
				if agent != "" {
					normalized.Session.RootIdentity = ingest.Identity(rootID)
				}
				rel, _ := filepath.Rel(root, path)
				normalized.Message.OriginPath = filepath.ToSlash(rel)
				normalized.Message.OriginLine = lineNumber
				normalized.Message.Membership = "body"
				f.Records = append(f.Records, *normalized)
				var envelope MessageEnvelope
				_ = json.Unmarshal(rec.Message, &envelope)
				var blocks []ContentBlock
				_ = json.Unmarshal(envelope.Content, &blocks)
				var result struct {
					AgentID string `json:"agentId"`
				}
				_ = json.Unmarshal(rec.ToolUseResult, &result)
				for _, block := range blocks {
					if rec.Type == "assistant" && block.Type == "tool_use" && (block.Name == "Agent" || block.Name == "Task") && block.ID != "" {
						if calls[block.ID] == nil {
							calls[block.ID] = map[string]bool{}
						}
						calls[block.ID][identity] = true
					}
					if rec.Type == "user" && block.Type == "tool_result" && block.ToolUseID != "" && result.AgentID != "" {
						results[block.ToolUseID] = append(results[block.ToolUseID], result.AgentID)
					}
				}
			}
			return nil
		})
		if err != nil {
			readErrors = append(readErrors, fmt.Errorf("%s: %w", path, err))
			continue
		}
		for call, agents := range results {
			parents, ok := calls[call]
			if !ok {
				continue
			}
			for _, childAgent := range agents {
				child := ingest.Identity(rootID, childAgent)
				if candidates[child] == nil {
					candidates[child] = map[string]bool{}
				}
				for parent := range parents {
					candidates[child][parent] = true
				}
			}
		}
		files = append(files, f)
	}
	parents, relationDiagnostics := resolveParents(candidates)
	diagnostics = append(diagnostics, relationDiagnostics...)
	numbers := map[string]int{}
	for i := range files {
		f := &files[i]
		for j := range f.Records {
			r := &f.Records[j]
			if r.Session.RootIdentity != "" {
				r.Session.ParentIdentity = parents[r.Session.Identity]
			}
			f.Parents[r.Session.Identity] = r.Session.ParentIdentity
			if strings.TrimSpace(r.Message.Content) != "" {
				numbers[r.Session.Identity]++
				r.Message.Number = numbers[r.Session.Identity]
			}
		}
	}
	return files, readErrors, diagnostics
}

func resolveParents(candidates map[string]map[string]bool) (map[string]string, []error) {
	parents := map[string]string{}
	var diagnostics []error
	keys := make([]string, 0, len(candidates))
	for child := range candidates {
		keys = append(keys, child)
	}
	sort.Strings(keys)
	for _, child := range keys {
		choices := candidates[child]
		if len(choices) != 1 {
			diagnostics = append(diagnostics, fmt.Errorf("%s: conflicting direct parents; retained unresolved", child))
			continue
		}
		for parent := range choices {
			parents[child] = parent
		}
	}
	cyclic := map[string]bool{}
	for _, child := range keys {
		visited := map[string]bool{}
		node := child
		for node != "" {
			if visited[node] {
				start := node
				for {
					cyclic[node] = true
					node = parents[node]
					if node == start {
						break
					}
				}
				break
			}
			visited[node] = true
			node = parents[node]
		}
	}
	for _, child := range keys {
		if cyclic[child] {
			delete(parents, child)
			diagnostics = append(diagnostics, fmt.Errorf("%s: cyclic direct parent; retained unresolved", child))
		}
	}
	return parents, diagnostics
}
