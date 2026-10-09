package core

import (
	"crypto/sha256"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func TestSameMigrationReportsComparesPathsWithoutSnapshotHashes(t *testing.T) {
	before := codex.FileReport{Path: "rollout.jsonl", Hash: sha256.Sum256([]byte("before"))}
	after := codex.FileReport{Path: "rollout.jsonl", Hash: sha256.Sum256([]byte("after"))}
	if !sameMigrationReports([]codex.FileReport{before}, []codex.FileReport{after}) {
		t.Fatal("body hash incorrectly constrained metadata index")
	}
	after.Path = "other.jsonl"
	if sameMigrationReports([]codex.FileReport{before}, []codex.FileReport{after}) {
		t.Fatal("different rollout accepted")
	}
}
