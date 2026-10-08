package core

import (
	"crypto/sha256"
	"testing"

	"github.com/ryotapoi/somniloq/internal/ingest/codex"
)

func TestSameMigrationReportsRejectsReloadedBody(t *testing.T) {
	before := codex.FileReport{Path: "rollout.jsonl", Hash: sha256.Sum256([]byte("before"))}
	after := codex.FileReport{Path: "rollout.jsonl", Hash: sha256.Sum256([]byte("after"))}
	if sameMigrationReports([]codex.FileReport{before}, []codex.FileReport{after}) {
		t.Fatal("changed body passed the index snapshot check")
	}
	if !sameMigrationReports([]codex.FileReport{before}, []codex.FileReport{before}) {
		t.Fatal("identical body rejected")
	}
}
