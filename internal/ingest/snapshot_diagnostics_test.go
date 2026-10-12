package ingest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
)

func TestSnapshotDiagnosticsCursorBoundary(t *testing.T) {
	state := func(prefix string) *ImportState {
		sum := sha256.Sum256([]byte(prefix))
		return &ImportState{FileSize: int64(len(prefix)), LastOffset: int64(len(prefix)), ContentHash: hex.EncodeToString(sum[:])}
	}
	for _, tc := range []struct {
		name      string
		data      string
		old       *ImportState
		wantFirst int
	}{
		{"unchanged prefix", "one\nbad\n", state("one\n"), 1},
		{"edited prefix", "two\nbad\n", state("one\n"), 0},
		{"unterminated previous line", "one\nbad\n", state("one"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failures := []error{errors.New("first"), errors.New("second")}
			summary := NewSnapshotFailures([]byte(tc.data), tc.old)
			for i, err := range failures {
				summary.Add(int64(i*4), func() error { return err })
			}
			got := summary.Diagnostics
			wantCount := len(failures) - tc.wantFirst
			if got.UnparsedLines != wantCount || len(got.UnparsedDiagnostics) != wantCount {
				t.Fatalf("diagnostics = %+v, want %d failures", got, wantCount)
			}
			for i, diagnostic := range got.UnparsedDiagnostics {
				if diagnostic != failures[tc.wantFirst+i] {
					t.Fatalf("diagnostic %d = %v, want %v", i, diagnostic, failures[tc.wantFirst+i])
				}
			}
		})
	}
}

func TestSnapshotDiagnosticsCapsRetainedErrors(t *testing.T) {
	summary := NewSnapshotFailures(nil, nil)
	var failures []error
	for i := 0; i < MaxUnparsedDiagnostics+1; i++ {
		err := fmt.Errorf("failure %d", i)
		failures = append(failures, err)
		summary.Add(int64(i), func() error { return err })
	}
	got := summary.Diagnostics
	if got.UnparsedLines != len(failures) || len(got.UnparsedDiagnostics) != MaxUnparsedDiagnostics {
		t.Fatalf("diagnostics = %+v", got)
	}
	for i, diagnostic := range got.UnparsedDiagnostics {
		if diagnostic != failures[i] {
			t.Fatalf("diagnostic %d = %v, want %v", i, diagnostic, failures[i])
		}
	}
}

func TestSnapshotFailuresReportsNewErrorsAfterHistoricalCap(t *testing.T) {
	data := []byte("bad\n" + "bad\n" + "bad\n" + "bad\n" + "bad\n" + "bad\n" + "new\n")
	sum := sha256.Sum256(data[:24])
	old := &ImportState{FileSize: 24, LastOffset: 24, ContentHash: hex.EncodeToString(sum[:])}
	summary := NewSnapshotFailures(data, old)
	for i := 0; i < 7; i++ {
		summary.Add(int64(i*4), func() error { return fmt.Errorf("line %d", i+1) })
	}
	if summary.Count != 7 || summary.First.Error() != "line 1" || summary.Diagnostics.UnparsedLines != 1 || len(summary.Diagnostics.UnparsedDiagnostics) != 1 || summary.Diagnostics.UnparsedDiagnostics[0].Error() != "line 7" {
		t.Fatalf("failures = %+v", summary)
	}
}
