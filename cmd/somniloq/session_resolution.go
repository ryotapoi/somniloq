package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

func resolveSessionREF(db *core.DB, ref string, diagnostics io.Writer) (*core.SessionResolution, int, error) {
	resolved, err := db.ResolveSession(ref)
	if err != nil {
		var refError *core.REFError
		if errors.As(err, &refError) {
			return nil, 2, err
		}
		return nil, 1, err
	}
	if resolved == nil {
		return nil, 2, fmt.Errorf("session not found: %s", ref)
	}
	for _, diagnostic := range resolved.Diagnostics {
		if diagnostics != nil {
			fmt.Fprintln(diagnostics, diagnostic)
		}
	}
	return resolved, 0, nil
}
