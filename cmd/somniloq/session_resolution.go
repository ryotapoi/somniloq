package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/ryotapoi/somniloq/internal/core"
)

func resolveSessionREF(db *core.DB, ref string, diagnostics io.Writer) (int, error) {
	resolved, err := db.ResolveSession(ref)
	if err != nil {
		var refError *core.REFError
		if errors.As(err, &refError) {
			return 2, err
		}
		return 1, err
	}
	if resolved == nil {
		return 2, fmt.Errorf("session not found: %s", ref)
	}
	for _, diagnostic := range resolved.Diagnostics {
		if diagnostics != nil {
			fmt.Fprintln(diagnostics, diagnostic)
		}
	}
	return 0, nil
}
