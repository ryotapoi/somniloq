package ingest

import (
	"fmt"
	"strings"
)

// PersistMessage writes the source-neutral persistence sequence for one
// normalized message record. Empty message content still registers the session
// but does not write a message row.
func PersistMessage(tx ImportTransaction, record *NormalizedRecord, importedAt string) error {
	if err := tx.UpsertSession(record.Session, importedAt); err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}
	if strings.TrimSpace(record.Message.Content) == "" {
		return nil
	}
	if err := tx.InsertMessage(record.Message); err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}
