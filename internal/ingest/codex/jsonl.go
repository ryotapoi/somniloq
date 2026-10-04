package codex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type RawRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Ordinal   *int            `json:"ordinal"`
	Payload   json.RawMessage `json:"payload"`
}

type SessionMetaPayload struct {
	ID           string          `json:"id"`
	HistoryStart *int            `json:"subagent_history_start_ordinal"`
	Source       json.RawMessage `json:"source"`
	CWD          string          `json:"cwd"`
	CLIVersion   string          `json:"cli_version"`
	Git          struct {
		Branch string `json:"branch"`
	} `json:"git"`
}

type ResponseItemPayload struct {
	ID      string          `json:"id"`
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
	if payload.ID == "" {
		return nil, errors.New("session_meta payload.id is missing or empty")
	}
	parent, err := explicitParentThreadID(payload.Source)
	if err != nil {
		return nil, err
	}
	return &ingest.SessionMeta{
		Source:          ingest.SourceCodex,
		SessionID:       payload.ID,
		CWD:             payload.CWD,
		RepoPath:        resolveRepoPath(payload.CWD),
		GitBranch:       payload.Git.Branch,
		Version:         payload.CLIVersion,
		ParentSessionID: parent,
	}, nil
}

func normalizeMessage(rec *RawRecord, payload *ResponseItemPayload, meta ingest.SessionMeta, rolloutPath string, lineNumber int) (*ingest.NormalizedRecord, error) {
	content, err := ExtractText(payload.Content)
	if err != nil {
		return nil, err
	}

	timestamp := rec.Timestamp
	meta.StartedAt = timestamp
	meta.EndedAt = timestamp

	return &ingest.NormalizedRecord{
		Session: meta,
		Message: ingest.NormalizedMessage{
			UUID:       messageUUID(rolloutPath, lineNumber),
			Blocks:     textBlocks(payload.Content),
			PayloadID:  payload.ID,
			OriginPath: rolloutPath,
			OriginLine: lineNumber,
			Membership: "body",
			Source:     ingest.SourceCodex,
			SessionID:  meta.SessionID,
			Role:       payload.Role,
			Content:    content,
			Timestamp:  timestamp,
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
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n\n"), nil
}

func messageUUID(rolloutPath string, lineNumber int) string {
	sum := sha256.Sum256([]byte(rolloutPath + "\x00" + strconv.Itoa(lineNumber)))
	return "codex:" + hex.EncodeToString(sum[:])
}

func textBlocks(raw json.RawMessage) []string {
	var blocks []ContentBlock
	_ = json.Unmarshal(raw, &blocks)
	var texts []string
	for _, b := range blocks {
		switch b.Type {
		case "input_text", "output_text", "text":
			texts = append(texts, b.Text)
		}
	}
	return texts
}

// source is a union: ordinary roots use strings, while thread_spawn parent
// evidence lives only in the explicitly nested object variant.
func explicitParentThreadID(source json.RawMessage) (string, error) {
	var fields map[string]json.RawMessage
	for _, key := range []string{"subagent", "thread_spawn"} {
		raw := bytes.TrimSpace(source)
		if len(raw) == 0 || raw[0] != '{' {
			return "", nil
		}
		if err := json.Unmarshal(raw, &fields); err != nil {
			return "", err
		}
		source = fields[key]
		fields = nil
	}
	raw := bytes.TrimSpace(source)
	if len(raw) == 0 || raw[0] != '{' {
		return "", nil
	}
	var spawn struct {
		ParentThreadID string `json:"parent_thread_id"`
	}
	if err := json.Unmarshal(raw, &spawn); err != nil {
		return "", err
	}
	return spawn.ParentThreadID, nil
}
