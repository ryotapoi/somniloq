package main

import (
	"errors"
	"fmt"

	"github.com/ryotapoi/somniloq/internal/core"
)

func resolveSessionREF(db *core.DB, ref string, source *core.Source) (core.SessionRow, int, error) {
	session, err := db.LookupSessionREF(ref)
	if err != nil {
		var refError *core.REFError
		if errors.As(err, &refError) {
			return core.SessionRow{}, 2, err
		}
		return core.SessionRow{}, 1, err
	}
	if session == nil || source != nil && session.Source != *source {
		return core.SessionRow{}, 2, fmt.Errorf("session not found: %s", ref)
	}
	return *session, 0, nil
}
