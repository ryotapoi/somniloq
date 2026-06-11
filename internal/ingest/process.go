package ingest

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
)

const readBufferSize = 64 * 1024

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

// ProcessResult reports the outcome of processing one file.
type ProcessResult struct {
	// NewOffset is the import cursor after this pass; on error or when no
	// body record exists yet it stays at the old offset.
	NewOffset int64
	// UnparsedLines counts lines dropped as LineUnparsed during this pass.
	UnparsedLines int
}

// FileHandler is the source-specific part of processing one JSONL file.
// ProcessJSONL owns the shared skeleton (open, seek, offset tracking,
// transaction lifecycle, import_state advance); the handler owns record
// interpretation and any per-file state.
type FileHandler interface {
	// Begin restores per-file state from the already-imported prefix before
	// any line is handled. Sources without resume state return nil.
	Begin(path string, offset int64) error
	// HandleLine receives each raw line including blank ones (some sources
	// derive line numbers from them) and reports how the line was consumed.
	// On a non-nil error the outcome carries no meaning: the runner aborts
	// the file and rolls back, discarding whatever outcome was returned.
	HandleLine(tx ImportTransaction, line []byte) (LineOutcome, error)
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
func ProcessJSONL(store Store, source Source, handler FileHandler, file File, offset, fileSize int64, importedAt string) (ProcessResult, error) {
	keep := ProcessResult{NewOffset: offset}

	if err := handler.Begin(file.Path, offset); err != nil {
		return keep, err
	}

	f, err := os.Open(file.Path)
	if err != nil {
		return keep, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return keep, err
		}
	}

	tx, err := store.BeginImport()
	if err != nil {
		return keep, err
	}
	defer tx.Rollback()

	// import_state only advances after a body record was committed, so a
	// positive offset proves a sessions row already exists for this file.
	hasBody := offset > 0
	var unparsed int
	consumed, err := ForEachLine(f, -1, func(line []byte) error {
		outcome, herr := handler.HandleLine(tx, line)
		if herr != nil {
			// Per the FileHandler contract the outcome carries no meaning
			// alongside an error; discard it before it can touch any state.
			return herr
		}
		switch outcome {
		case LineWroteBody:
			hasBody = true
		case LineUnparsed:
			unparsed++
		}
		return nil
	})
	keep.UnparsedLines = unparsed
	if err != nil {
		return keep, err
	}

	if !hasBody {
		return keep, nil
	}

	if err := handler.Flush(tx); err != nil {
		return keep, err
	}

	if err := tx.UpsertImportState(ImportState{
		JSONLPath:  file.Path,
		Source:     source,
		FileSize:   fileSize,
		LastOffset: offset + consumed,
		ImportedAt: importedAt,
	}); err != nil {
		return keep, fmt.Errorf("upsert import state: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return keep, err
	}
	keep.NewOffset = offset + consumed
	return keep, nil
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
