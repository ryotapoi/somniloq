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
			failures := []ParseFailure{
				{Offset: 0, Diagnostic: errors.New("first")},
				{Offset: 4, Diagnostic: errors.New("second")},
			}
			got := SnapshotDiagnostics([]byte(tc.data), failures, tc.old)
			wantCount := len(failures) - tc.wantFirst
			if got.UnparsedLines != wantCount || len(got.UnparsedDiagnostics) != wantCount {
				t.Fatalf("diagnostics = %+v, want %d failures", got, wantCount)
			}
			for i, diagnostic := range got.UnparsedDiagnostics {
				if diagnostic != failures[tc.wantFirst+i].Diagnostic {
					t.Fatalf("diagnostic %d = %v, want %v", i, diagnostic, failures[tc.wantFirst+i].Diagnostic)
				}
			}
		})
	}
}

func TestSnapshotDiagnosticsCapsRetainedErrors(t *testing.T) {
	var failures []ParseFailure
	for i := 0; i < MaxUnparsedDiagnostics+1; i++ {
		failures = append(failures, ParseFailure{Offset: int64(i), Diagnostic: fmt.Errorf("failure %d", i)})
	}
	got := SnapshotDiagnostics(nil, failures, nil)
	if got.UnparsedLines != len(failures) || len(got.UnparsedDiagnostics) != MaxUnparsedDiagnostics {
		t.Fatalf("diagnostics = %+v", got)
	}
	for i, diagnostic := range got.UnparsedDiagnostics {
		if diagnostic != failures[i].Diagnostic {
			t.Fatalf("diagnostic %d = %v, want %v", i, diagnostic, failures[i].Diagnostic)
		}
	}
}
