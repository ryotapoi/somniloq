package ingest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanFilesRecursive_PreservesWalkOrderAndCollectsDescendantFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply to root")
	}
	root := t.TempDir()
	firstBlocked := filepath.Join(root, "a-blocked")
	secondBlocked := filepath.Join(root, "b-blocked")
	firstAccepted := filepath.Join(root, "c-accepted.jsonl")
	secondAccepted := filepath.Join(root, "d-accepted.jsonl")
	for _, path := range []string{firstBlocked, secondBlocked} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, 0o000); err != nil {
			t.Fatal(err)
		}
		path := path
		t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	}
	for _, path := range []string{firstAccepted, secondAccepted} {
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, errs := ScanFilesRecursive(root, func(path string) bool {
		return strings.HasSuffix(path, ".jsonl")
	})
	if len(files) != 2 || files[0] != firstAccepted || files[1] != secondAccepted {
		t.Fatalf("files = %v, want [%s %s]", files, firstAccepted, secondAccepted)
	}
	if len(errs) != 2 {
		t.Fatalf("errs = %v, want two errors", errs)
	}
	for i, path := range []string{firstBlocked, secondBlocked} {
		if !strings.Contains(errs[i].Error(), "scan "+path+":") {
			t.Errorf("errs[%d] = %v, want scan error for %s", i, errs[i], path)
		}
		if !errors.Is(errs[i], fs.ErrPermission) {
			t.Errorf("errs[%d] = %v, want permission error", i, errs[i])
		}
	}
}
