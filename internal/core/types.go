package core

import "github.com/ryotapoi/somniloq/internal/ingest"

type Source = ingest.Source

const (
	SourceClaudeCode  = ingest.SourceClaudeCode
	SourceCodex       = ingest.SourceCodex
	SourceCursorAgent = ingest.SourceCursorAgent
)

type ImportState = ingest.ImportState
type NormalizedMessage = ingest.NormalizedMessage
type SessionMeta = ingest.SessionMeta
