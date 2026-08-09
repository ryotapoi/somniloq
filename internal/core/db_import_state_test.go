package core

import (
	"errors"
	"strings"
	"testing"
)

func TestGetImportState_ClosedDatabaseErrorIncludesOperationAndCause(t *testing.T) {
	db := testDB(t)
	must(t, db.Close())

	_, err := db.GetImportState("/tmp/session.jsonl")
	if err == nil {
		t.Fatal("expected query error from closed database")
	}
	if !strings.Contains(err.Error(), "get import state") {
		t.Errorf("error %q does not identify operation", err)
	}
	if errors.Unwrap(err) == nil {
		t.Errorf("error %q does not retain its cause", err)
	}
}
