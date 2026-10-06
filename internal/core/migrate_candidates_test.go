package core

import (
	"reflect"
	"testing"
)

func TestMigrationGroupCandidates(t *testing.T) {
	// Failed groups still compete; ownership does not depend on old session IDs
	// or whether the physical line survived canonical message deduplication.
	evidence := map[string][]migrationEvidence{
		"physical-duplicate": {{Input: 0, Group: 1, Path: "child", Line: 3}},
		"other-input":        {{Input: 1, Group: 0, Path: "other", Line: 2}},
		"failed-competitor":  {{Input: 0, Group: 1}, {Input: 1, Group: 2}},
		"repeated-physical":  {{Input: 0, Group: 1}, {Input: 0, Group: 1}},
		"no-evidence":        {},
	}
	want := map[[2]int][]string{
		{0, 1}: {"physical-duplicate"},
		{1, 0}: {"other-input"},
	}
	if got := migrationGroupCandidates(evidence); !reflect.DeepEqual(got, want) {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}
