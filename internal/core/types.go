package core

import "github.com/ryotapoi/somniloq/internal/ingest"

type Source = ingest.Source

const (
	SourceClaudeCode = ingest.SourceClaudeCode
)

type ImportState = ingest.ImportState
type ParsedMessage = ingest.NormalizedMessage
type SessionMeta = ingest.SessionMeta
