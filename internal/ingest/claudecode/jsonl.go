package claudecode

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type RawRecord struct {
	Type          string          `json:"type"`
	UUID          string          `json:"uuid"`
	ParentUUID    *string         `json:"parentUuid"`
	SessionID     string          `json:"sessionId"`
	Timestamp     string          `json:"timestamp"`
	CWD           string          `json:"cwd"`
	GitBranch     string          `json:"gitBranch"`
	Version       string          `json:"version"`
	IsSidechain   bool            `json:"isSidechain"`
	Message       json.RawMessage `json:"message"`
	CustomTitle   string          `json:"customTitle"`
	AgentID       string          `json:"agentId"`
	ToolUseResult json.RawMessage `json:"toolUseResult"`
	AgentName     string          `json:"agentName"`
}

type MessageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ContentBlock intentionally remains separate from codex.ContentBlock.
// Claude Code ExtractText accepts only text blocks; the source-specific sets
// must not be unified (ADR 0023).
type ContentBlock struct {
	Type      string `json:"type"`
	Text      string `json:"text"`
	ID        string `json:"id"`
	Name      string `json:"name"`
	ToolUseID string `json:"tool_use_id"`
}

func ParseRecord(line []byte) (*RawRecord, error) {
	var rec RawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func NormalizeRecord(rec *RawRecord, repoPath string) (*ingest.NormalizedRecord, error) {
	if rec.SessionID == "" {
		return nil, errors.New("sessionId is missing or empty")
	}
	if rec.UUID == "" {
		return nil, errors.New("uuid is missing or empty")
	}

	msg, err := ParseMessage(rec)
	if err != nil {
		return nil, err
	}

	return &ingest.NormalizedRecord{
		Session: ingest.SessionMeta{
			Source:    ingest.SourceClaudeCode,
			SessionID: rec.SessionID,
			CWD:       rec.CWD,
			RepoPath:  repoPath,
			GitBranch: rec.GitBranch,
			Version:   rec.Version,
			StartedAt: rec.Timestamp,
			EndedAt:   rec.Timestamp,
		},
		Message: *msg,
	}, nil
}

func ParseMessage(rec *RawRecord) (*ingest.NormalizedMessage, error) {
	var env MessageEnvelope
	if err := json.Unmarshal(rec.Message, &env); err != nil {
		return nil, err
	}

	blocks, err := ExtractTextBlocks(env.Content)
	if err != nil {
		return nil, err
	}

	return &ingest.NormalizedMessage{
		UUID:        rec.UUID,
		Source:      ingest.SourceClaudeCode,
		ParentUUID:  rec.ParentUUID,
		SessionID:   rec.SessionID,
		Role:        env.Role,
		Content:     strings.Join(blocks, "\n\n"),
		Blocks:      blocks,
		Timestamp:   rec.Timestamp,
		IsSidechain: rec.IsSidechain,
	}, nil
}

func ExtractText(raw json.RawMessage) (string, error) {
	blocks, err := ExtractTextBlocks(raw)
	return strings.Join(blocks, "\n\n"), err
}

func ExtractTextBlocks(raw json.RawMessage) ([]string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return []string{s}, nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, err
	}

	var texts []string
	for _, b := range blocks {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return texts, nil
}
