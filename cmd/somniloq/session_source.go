package main

import (
	"fmt"

	"github.com/ryotapoi/somniloq/internal/core"
)

// parseSessionSource accepts source values emitted by search and the
// hyphenated forms used by import.
func parseSessionSource(value string) (core.Source, error) {
	switch value {
	case string(core.SourceClaudeCode), "claude-code":
		return core.SourceClaudeCode, nil
	case string(core.SourceCodex):
		return core.SourceCodex, nil
	case string(core.SourceCursorAgent), "cursor-agent":
		return core.SourceCursorAgent, nil
	default:
		return "", fmt.Errorf("invalid --source %q (want claude_code, claude-code, codex, cursor_agent, or cursor-agent)", value)
	}
}
