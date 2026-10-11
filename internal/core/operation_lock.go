package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrOperationBusy = errors.New("another import or migration is running for this database")

// LockOperation must be acquired before opening the database or reading inputs,
// and held until the operation finishes. Closing the file releases the lock;
// the OS also releases it on process exit.
func LockOperation(path string) (*os.File, error) {
	canonical, err := CanonicalPath(path, "")
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(canonical), 0o700); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	file, err := os.OpenFile(canonical+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open operation lock: %w", err)
	}
	if err = lockOperationFile(file); err != nil {
		file.Close()
		return nil, err
	}
	// Never unlink this sidecar: concurrent callers must lock the same inode.
	return file, nil
}
