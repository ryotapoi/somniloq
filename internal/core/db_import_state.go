package core

import (
	"database/sql"
	"errors"
	"fmt"
)

func (d *DB) GetImportState(jsonlPath string) (*ImportState, error) {
	return getImportState(d.execer(), jsonlPath)
}

func getImportState(e execer, jsonlPath string) (*ImportState, error) {
	var s ImportState
	var src string
	err := e.QueryRow(
		"SELECT jsonl_path, source, file_size, last_offset, imported_at FROM import_state WHERE jsonl_path=?",
		jsonlPath,
	).Scan(&s.JSONLPath, &src, &s.FileSize, &s.LastOffset, &s.ImportedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get import state: scan row: %w", err)
	}
	s.Source = Source(src)
	return &s, nil
}
