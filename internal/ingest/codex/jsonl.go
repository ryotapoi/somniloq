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

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type SessionMeta struct {
	SessionID string
	CWD       string
	RepoPath  string
	GitBranch string
	Version   string
	Timestamp string
}

func ParseRecord(line []byte) (*RawRecord, error) {
	var rec RawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func ParseSessionMeta(rec *RawRecord, repoPath string) (*SessionMeta, error) {
	var payload SessionMetaPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return nil, err
	}
	return &SessionMeta{
		SessionID: payload.ID,
		CWD:       payload.CWD,
		RepoPath:  repoPath,
		GitBranch: payload.Git.Branch,
		Version:   payload.CLIVersion,
		Timestamp: rec.Timestamp,
	}, nil
}

func NormalizeMessage(rec *RawRecord, meta SessionMeta, rolloutPath string, lineNumber int) (*ingest.NormalizedRecord, error) {
	var payload ResponseItemPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return nil, err
	}
	content, err := ExtractText(payload.Content)
	if err != nil {
		return nil, err
	}

	timestamp := rec.Timestamp
	if timestamp == "" {
		timestamp = meta.Timestamp
	}

	return &ingest.NormalizedRecord{
		Session: ingest.SessionMeta{
			Source:    ingest.SourceCodex,
			SessionID: meta.SessionID,
			CWD:       meta.CWD,
			RepoPath:  meta.RepoPath,
			GitBranch: meta.GitBranch,
			Version:   meta.Version,
			StartedAt: timestamp,
			EndedAt:   timestamp,
		},
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

func IsMessageRecord(rec *RawRecord) (bool, string, error) {
	if rec.Type != "response_item" {
		return false, "", nil
	}
	var payload ResponseItemPayload
	if err := json.Unmarshal(rec.Payload, &payload); err != nil {
		return false, "", err
	}
	if payload.Type != "message" {
		return false, payload.Role, nil
	}
	if payload.Role != "user" && payload.Role != "assistant" {
		return false, payload.Role, nil
	}
	return true, payload.Role, nil
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
