package cursoragent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFilesAcceptsOnlyCursorTranscriptPaths(t *testing.T) {
	root := t.TempDir()
	accepted := filepath.Join(root, "private-tmp", "agent-transcripts", "session", "session.jsonl")
	for _, path := range []string{
		accepted,
		filepath.Join(root, "project", "session.jsonl"),
		filepath.Join(root, "project", "agent-transcripts", "session", "other.jsonl"),
		filepath.Join(root, "project", "agent-transcripts", "nested", "session", "session.jsonl"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	files, errs := NewAdapter().ScanFiles(root)
	if len(errs) != 0 {
		t.Fatalf("ScanFiles errors: %v", errs)
	}
	if len(files) != 1 || files[0] != accepted {
		t.Fatalf("ScanFiles files = %v, want [%s]", files, accepted)
	}
}

func TestScanFilesMissingRootIsUnusedSource(t *testing.T) {
	files, errs := NewAdapter().ScanFiles(filepath.Join(t.TempDir(), "missing"))
	if len(files) != 0 || len(errs) != 0 {
		t.Fatalf("ScanFiles = %v, %v; want empty", files, errs)
	}
}
