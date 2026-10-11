package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// ParseFailure records the physical byte offset and diagnostic of a failed line.
type ParseFailure struct {
	Offset     int64
	Diagnostic error
}

// SnapshotDiagnostics reports failures newly encountered after a saved cursor.
// An edited prefix reports all failures, while an unterminated previous final
// line is retried because appending bytes can change how it parses.
func SnapshotDiagnostics(data []byte, failures []ParseFailure, old *ImportState) ProcessResult {
	start := int64(0)
	if old != nil && int64(len(data)) >= old.FileSize {
		sum := sha256.Sum256(data[:old.FileSize])
		if hex.EncodeToString(sum[:]) == old.ContentHash {
			start = old.LastOffset
			if start > 0 && start == old.FileSize && data[start-1] != '\n' {
				start = int64(bytes.LastIndexByte(data[:start], '\n') + 1)
			}
		}
	}
	result := ProcessResult{}
	for _, failure := range failures {
		if failure.Offset < start {
			continue
		}
		result.UnparsedLines++
		if len(result.UnparsedDiagnostics) < MaxUnparsedDiagnostics {
			result.UnparsedDiagnostics = append(result.UnparsedDiagnostics, failure.Diagnostic)
		}
	}
	return result
}
