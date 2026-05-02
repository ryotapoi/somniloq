package core

import (
	"encoding/json"
	"strings"
)

type RawRecord struct {
	Type        string          `json:"type"`
	UUID        string          `json:"uuid"`
	ParentUUID  *string         `json:"parentUuid"`
	SessionID   string          `json:"sessionId"`
	Timestamp   string          `json:"timestamp"`
	CWD         string          `json:"cwd"`
	GitBranch   string          `json:"gitBranch"`
	Version     string          `json:"version"`
	IsSidechain bool            `json:"isSidechain"`
	Message     json.RawMessage `json:"message"`
	CustomTitle string          `json:"customTitle"`
	AgentName   string          `json:"agentName"`
}

type MessageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ParsedMessage struct {
	UUID        string
	ParentUUID  *string
	SessionID   string
	Role        string
	Content     string
	Timestamp   string
	IsSidechain bool
}

type SessionMeta struct {
	SessionID string
	CWD       string
	RepoPath  string
	GitBranch string
	Version   string
	StartedAt string
	EndedAt   string
}

func ParseRecord(line []byte) (*RawRecord, error) {
	var rec RawRecord
	if err := json.Unmarshal(line, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

func ParseMessage(rec *RawRecord) (*ParsedMessage, error) {
	var env MessageEnvelope
	if err := json.Unmarshal(rec.Message, &env); err != nil {
		return nil, err
	}

	content, err := ExtractText(env.Content)
	if err != nil {
		return nil, err
	}

	return &ParsedMessage{
		UUID:        rec.UUID,
		ParentUUID:  rec.ParentUUID,
		SessionID:   rec.SessionID,
		Role:        env.Role,
		Content:     content,
		Timestamp:   rec.Timestamp,
		IsSidechain: rec.IsSidechain,
	}, nil
}

func ExtractText(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}

	var blocks []ContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", err
	}

	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n\n"), nil
}
