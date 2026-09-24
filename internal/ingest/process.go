package ingest

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

const readBufferSize = 64 * 1024

type readSeekCloser interface {
	io.Reader
	io.Seeker
	io.Closer
}

// MaxUnparsedDiagnostics bounds retained parse/normalization diagnostics for
// one import run. It is intentionally fixed rather than user-configurable.
const MaxUnparsedDiagnostics = 5

// LineOutcome describes how a FileHandler consumed one line.
type LineOutcome int

const (
	// LineIgnored means the line was understood and intentionally not stored:
	// blank lines, metadata buffered for a later flush, or record types the
	// source deliberately does not import.
	LineIgnored LineOutcome = iota
	// LineWroteBody means the line wrote a sessions row. It is reported per
	// line, so returning it repeatedly is expected.
	LineWroteBody
	// LineUnparsed means the line could not be parsed or normalized and was
	// dropped. ProcessJSONL counts these so imports can report them.
	LineUnparsed
)

// LineResult reports how a FileHandler consumed one line and, when it was
// unparsed, the diagnostic for that line.
type LineResult struct {
	Outcome    LineOutcome
	Diagnostic error
}

// ProcessResult reports the outcome of processing one file.
type ProcessResult struct {
	// UnparsedLines counts lines dropped as LineUnparsed during this pass.
	UnparsedLines int
	// UnparsedDiagnostics holds up to five parse or normalization diagnostics
	// in encounter order. Each one identifies the physical JSONL line.
	UnparsedDiagnostics []error
}

// FileHandler is the source-specific part of processing one JSONL file.
// ProcessJSONL owns the shared skeleton (open, seek, offset tracking,
// transaction lifecycle, import_state advance); the handler owns record
// interpretation and any per-file state.
//
// Because that state is per-file, adapters must build a fresh handler for
// every ProcessFile call; reusing one leaks the previous file's state (line
// numbers, buffered metadata) into the next. The adapters themselves stay
// stateless.
type FileHandler interface {
	// Begin restores per-file state from the already-imported prefix before
	// any line is handled. Sources without resume state return nil.
	Begin(path string, offset int64) error
	// HandleLine receives each raw line including blank ones (some sources
	// derive line numbers from them) and reports how the line was consumed.
	// Diagnostic is used only when the outcome is LineUnparsed. On a non-nil
	// error the result carries no meaning: the runner aborts the file and rolls
	// back, discarding the entire result.
	HandleLine(tx ImportTransaction, line []byte) (LineResult, error)
	// Flush writes metadata buffered during HandleLine. It runs at EOF, only
	// when a body record has been written.
	Flush(tx ImportTransaction) error
}

// ProcessJSONL runs the shared skeleton of an incremental JSONL import: it
// feeds every line after offset to handler inside one transaction, then
// flushes buffered metadata, advances import_state, and commits. If no body
// record has ever been written for the file, it commits nothing and keeps the
// old offset so the next import re-reads the meta-only prefix once a body
// record finally appears.
func ProcessJSONL(newTransaction NewImportTransaction, source Source, handler FileHandler, path string, offset, fileSize int64, importedAt string) (ProcessResult, error) {
	return processJSONL(newTransaction, source, handler, path, offset, fileSize, importedAt, func(path string) (readSeekCloser, error) {
		return os.Open(path)
	})
}

func processJSONL(newTransaction NewImportTransaction, source Source, handler FileHandler, path string, offset, fileSize int64, importedAt string, openFile func(string) (readSeekCloser, error)) (ProcessResult, error) {
	var result ProcessResult

	if err := handler.Begin(path, offset); err != nil {
		return result, err
	}

	f, err := openFile(path)
	if err != nil {
		return result, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return result, err
		}
	}

	tx, err := newTransaction()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	// import_state only advances after a body record was committed, so a
	// positive offset proves a sessions row already exists for this file.
	hasBody := offset > 0
	consumed, err := ForEachLine(f, -1, func(line []byte) error {
		lineResult, herr := handler.HandleLine(tx, line)
		if herr != nil {
			// Per the FileHandler contract the result carries no meaning
			// alongside an error; discard it before it can touch any state.
			return herr
		}
		switch lineResult.Outcome {
		case LineWroteBody:
			hasBody = true
		case LineUnparsed:
			result.UnparsedLines++
			if lineResult.Diagnostic != nil && len(result.UnparsedDiagnostics) < MaxUnparsedDiagnostics {
				result.UnparsedDiagnostics = append(result.UnparsedDiagnostics, lineResult.Diagnostic)
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}

	if !hasBody {
		return result, nil
	}

	if err := handler.Flush(tx); err != nil {
		return result, err
	}

	if err := tx.UpsertImportState(ImportState{
		JSONLPath:  path,
		Source:     source,
		FileSize:   fileSize,
		LastOffset: offset + consumed,
		ImportedAt: importedAt,
	}); err != nil {
		return result, fmt.Errorf("upsert import state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

// ForEachLine feeds r's lines (newline included, blank lines included) to fn
// and returns the number of bytes consumed. Iteration stops at EOF, when fn
// returns an error, or — when limit >= 0 — once limit bytes have been
// consumed.
func ForEachLine(r io.Reader, limit int64, fn func(line []byte) error) (int64, error) {
	reader := bufio.NewReaderSize(r, readBufferSize)
	var consumed int64
	for limit < 0 || consumed < limit {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			consumed += int64(len(line))
			if ferr := fn(line); ferr != nil {
				return consumed, ferr
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return consumed, nil
			}
			return consumed, err
		}
	}
	return consumed, nil
}

// CountLineFeeds counts complete physical lines in the first limit bytes of r.
// Unlike ForEachLine with a byte limit, it never reads past that boundary; an
// unterminated line at the boundary is therefore counted when its continuation
// is processed on a later incremental import.
func CountLineFeeds(r io.Reader, limit int64) (int, error) {
	if limit <= 0 {
		return 0, nil
	}

	limited := io.LimitReader(r, limit)
	buf := make([]byte, readBufferSize)
	var lines int
	for {
		n, err := limited.Read(buf)
		for _, b := range buf[:n] {
			if b == '\n' {
				lines++
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, nil
			}
			return lines, err
		}
	}
}
