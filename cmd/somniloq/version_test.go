package main

import "testing"

func TestGetVersion_Default(t *testing.T) {
	// When BuildInfo returns "(devel)", getVersion should return "dev".
	got := getVersion()
	if got != "dev" {
		t.Errorf("getVersion() = %q, want %q", got, "dev")
	}
}
