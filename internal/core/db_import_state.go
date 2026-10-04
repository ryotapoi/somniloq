package core

import (
	"database/sql"
	"errors"
	"fmt"
)

func (d *DB) GetImportState(inputID int64, jsonlPath string) (*ImportState, error) {
	return getImportState(d.execer(), inputID, jsonlPath)
}

func getImportState(e execer, inputID int64, jsonlPath string) (*ImportState, error) {
	var s ImportState
	var src string
	err := e.QueryRow(
		"SELECT jsonl_path, source, file_size, last_offset, imported_at FROM import_state WHERE input_id=? AND jsonl_path=?",
		inputID, jsonlPath,
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
