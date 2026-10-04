package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

func resolveSessionREF(db *core.DB, ref string, source *core.Source, diagnostics io.Writer) (core.SessionRow, int, error) {
	resolved, err := db.ResolveSession(ref)
	if err != nil {
		var refError *core.REFError
		if errors.As(err, &refError) {
			return core.SessionRow{}, 2, err
		}
		return core.SessionRow{}, 1, err
	}
	if resolved == nil || source != nil && resolved.Self.Source != *source {
		return core.SessionRow{}, 2, fmt.Errorf("session not found: %s", ref)
	}
	for _, diagnostic := range resolved.Diagnostics {
		if diagnostics != nil {
			fmt.Fprintln(diagnostics, diagnostic)
		}
	}
	return resolved.Self, 0, nil
}
