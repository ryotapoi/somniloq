package codex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type RawRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type SessionMetaPayload struct {
	ID         string `json:"id"`
	CWD        string `json:"cwd"`
	CLIVersion string `json:"cli_version"`
	Git        struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

type ResponseItemPayload struct {
	Type    string          `json:"type"`
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// ContentBlock intentionally remains separate from claudecode.ContentBlock.
// Codex accepts input_text, output_text, and text in ExtractText; the
// source-specific sets must not be unified (ADR 0005).
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func ParseRecord(line []byte) (*RawRecord, error) {
	var rec RawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func parseSessionMeta(rec *RawRecord, resolveRepoPath ingest.RepoResolver) (*ingest.SessionMeta, error) {
	var payload SessionMetaPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return nil, err
	}
	return &ingest.SessionMeta{
		Source:    ingest.SourceCodex,
		SessionID: payload.ID,
		CWD:       payload.CWD,
		RepoPath:  resolveRepoPath(payload.CWD),
		GitBranch: payload.Git.Branch,
		Version:   payload.CLIVersion,
		StartedAt: rec.Timestamp,
		EndedAt:   rec.Timestamp,
	}, nil
}

func normalizeMessage(rec *RawRecord, payload *ResponseItemPayload, meta ingest.SessionMeta, rolloutPath string, lineNumber int) (*ingest.NormalizedRecord, error) {
	content, err := ExtractText(payload.Content)
	if err != nil {
		return nil, err
	}

	timestamp := rec.Timestamp
	if timestamp == "" {
		timestamp = meta.StartedAt
	}
	meta.StartedAt = timestamp
	meta.EndedAt = timestamp

	return &ingest.NormalizedRecord{
		Session: meta,
		Message: ingest.NormalizedMessage{
			UUID:      messageUUID(rolloutPath, lineNumber),
			Source:    ingest.SourceCodex,
			SessionID: meta.SessionID,
			Role:      payload.Role,
			Content:   content,
			Timestamp: timestamp,
		},
	}, nil
}

func parseResponseItem(rec *RawRecord) (*ResponseItemPayload, error) {
	var payload ResponseItemPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func isConversationMessage(payload *ResponseItemPayload) bool {
	return payload.Type == "message" && (payload.Role == "user" || payload.Role == "assistant")
}

func ExtractText(raw json.RawMessage) (string, error) {
	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", err
	}

	var texts []string
	for _, b := range blocks {
		switch b.Type {
		case "input_text", "output_text", "text":
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		}
	}
	return strings.Join(texts, "\n\n"), nil
}

func messageUUID(rolloutPath string, lineNumber int) string {
	sum := sha256.Sum256([]byte(rolloutPath + "\x00" + strconv.Itoa(lineNumber)))
	return "codex:" + hex.EncodeToString(sum[:])
}
