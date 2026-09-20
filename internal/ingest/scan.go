package ingest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// ScanFilesRecursive walks rootDir and returns the non-directory paths accepted
// by accept. Descendant scan failures are reported alongside files discovered
// from healthy siblings.
func ScanFilesRecursive(rootDir string, accept func(path string) bool) ([]string, []error) {
	var files []string
	var errs []error
	walkErr := filepath.WalkDir(rootDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == rootDir {
				return err
			}
			errs = append(errs, fmt.Errorf("scan %s: %w", path, err))
			return nil
		}
		if !d.IsDir() && accept(path) {
			files = append(files, path)
		}
		return nil
	})
	if walkErr != nil {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil, nil
		}
		return nil, []error{fmt.Errorf("scan %s: %w", rootDir, walkErr)}
	}
	if len(files) == 0 && len(errs) == 0 {
		return nil, nil
	}
	return files, errs
}
