package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

// Prevent the shared missing-database classification from changing migration
// filesystem failures into argument errors.
type migrationIOError struct{ err error }

func (e *migrationIOError) Error() string { return "migrate: " + e.err.Error() }
func (e *migrationIOError) Unwrap() error { return e.err }

func migrateCmd(args []string, cfg config, out, errOut io.Writer) (int, error) {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	from := fs.String("from", "", "fixed standalone legacy SQLite snapshot (required)")
	setUsage(fs, "Migrate a legacy snapshot without changing it", "somniloq migrate [--config NAME_OR_PATH] --from PATH", `The configured database must be new, empty, or a completed copy of the same snapshot.
All configured Codex inputs are processed; missing logs and other sources retain their saved history.
Output is one JSON summary. Successfully parsed owners replace same-ID Codex history, including empty bodies.
History with other IDs is retained even when physical lines match; other sources are retained.
Successful replacements allow exit 0. Failed groups retain their saved owner state and cause exit 1.`)
	if code, ok := parseFlags(fs, errOut, args); !ok {
		if code != 0 {
			code = 2
		}
		return code, nil
	}
	if fs.NArg() != 0 {
		return 2, fmt.Errorf("migrate: unexpected arguments")
	}
	if *from == "" {
		return 2, fmt.Errorf("migrate: missing --from PATH")
	}
	result, err := core.Migrate(*from, cfg.DB, cfg.Inputs)
	if result != nil {
		if encodeErr := json.NewEncoder(out).Encode(result); encodeErr != nil {
			return 1, encodeErr
		}
		for _, diagnostic := range result.Skips {
			if _, writeErr := fmt.Fprintf(errOut, "migrate: skip: %v\n", diagnostic); writeErr != nil {
				return 1, writeErr
			}
		}
		for _, diagnostic := range result.Warnings {
			if _, writeErr := fmt.Fprintf(errOut, "migrate: warning: %v\n", diagnostic); writeErr != nil {
				return 1, writeErr
			}
		}
		for _, diagnostic := range result.Errors {
			if _, writeErr := fmt.Fprintf(errOut, "migrate: error: %v\n", diagnostic); writeErr != nil {
				return 1, writeErr
			}
		}
	}
	if err != nil {
		var schemaErr *core.SchemaError
		if errors.As(err, &schemaErr) {
			return 2, err
		}
		return 1, &migrationIOError{err}
	}
	if result.GroupsFailed > 0 || result.LegacyReplacementFailures > 0 || len(result.Errors) > 0 {
		return 1, nil
	}
	return 0, nil
}
