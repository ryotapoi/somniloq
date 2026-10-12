package ingest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

// SnapshotFailures counts every failed line while retaining only the first error
// and the first five errors at or after the saved cursor.
type SnapshotFailures struct {
	Count       int
	First       error
	Diagnostics ProcessResult
	start       int64
}

// NewSnapshotFailures fixes the diagnostic boundary before parsing. An edited
// prefix reports all failures; an unterminated previous final line is retried.
func NewSnapshotFailures(data []byte, old *ImportState) SnapshotFailures {
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
	return SnapshotFailures{start: start}
}

// Add constructs a physical-line diagnostic only when it will be retained.
func (f *SnapshotFailures) Add(offset int64, diagnostic func() error) error {
	f.Count++
	report := offset >= f.start
	if report {
		f.Diagnostics.UnparsedLines++
	}
	var err error
	if f.First == nil || (report && len(f.Diagnostics.UnparsedDiagnostics) < MaxUnparsedDiagnostics) {
		err = diagnostic()
	}
	if f.First == nil {
		f.First = err
	}
	if report && len(f.Diagnostics.UnparsedDiagnostics) < MaxUnparsedDiagnostics {
		f.Diagnostics.UnparsedDiagnostics = append(f.Diagnostics.UnparsedDiagnostics, err)
	}
	return err
}
