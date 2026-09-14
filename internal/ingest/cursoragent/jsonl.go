package cursoragent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ryotapoi/somniloq/internal/ingest"
)

type rawRecord struct {
	Role    string          `json:"role"`
	Message json.RawMessage `json:"message"`
}

type messageEnvelope struct {
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string          `json:"type"`
	Text json.RawMessage `json:"text"`
}

func parseRecord(line []byte) (*rawRecord, error) {
	var record rawRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

func normalizeRecord(record *rawRecord, sessionID, path string, lineNumber int) (*ingest.NormalizedRecord, error) {
	var envelope messageEnvelope
	if err := json.Unmarshal(record.Message, &envelope); err != nil {
		return nil, err
	}

	content, err := extractText(envelope.Content)
	if err != nil {
		return nil, err
	}
	return &ingest.NormalizedRecord{
		Session: ingest.SessionMeta{
			Source:    ingest.SourceCursorAgent,
			SessionID: sessionID,
		},
		Message: ingest.NormalizedMessage{
			UUID:      messageUUID(path, lineNumber),
			Source:    ingest.SourceCursorAgent,
			SessionID: sessionID,
			Role:      record.Role,
			Content:   content,
		},
	}, nil
}

func extractText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", fmt.Errorf("content must be an array")
	}
	var blocks []contentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", err
	}

	var texts []string
	for _, block := range blocks {
		if block.Type != "text" {
			continue
		}
		if len(block.Text) == 0 || bytes.Equal(bytes.TrimSpace(block.Text), []byte("null")) {
			return "", fmt.Errorf("text block: text must be a string")
		}
		var text string
		if err := json.Unmarshal(block.Text, &text); err != nil {
			return "", fmt.Errorf("text block: %w", err)
		}
		if text != "" {
			texts = append(texts, text)
		}
	}
	return strings.Join(texts, "\n\n"), nil
}

func messageUUID(path string, lineNumber int) string {
	sum := sha256.Sum256([]byte(string(ingest.SourceCursorAgent) + "\x00" + path + "\x00" + strconv.Itoa(lineNumber)))
	return "cursor_agent:" + hex.EncodeToString(sum[:])
}
