package main

import (
	"testing"

	"github.com/ryotapoi/somniloq/internal/core"
)

func TestAssignTurns(t *testing.T) {
	messages := []core.MessageRow{
		{UUID: "u1", Role: "user"},
		{UUID: "a1", Role: "assistant"},
		{UUID: "a2", Role: "assistant"},
		{UUID: "u2", Role: "user"},
		{UUID: "a3", Role: "assistant"},
		{UUID: "u3", Role: "user"},
	}

	got := assignTurns(messages)
	wantTurns := []int{1, 1, 1, 2, 2, 3}
	if len(got) != len(wantTurns) {
		t.Fatalf("len = %d, want %d", len(got), len(wantTurns))
	}
	for i, want := range wantTurns {
		if got[i].Turn != want {
			t.Errorf("messages[%d] (%s) turn = %d, want %d", i, got[i].Msg.UUID, got[i].Turn, want)
		}
		if got[i].Msg.UUID != messages[i].UUID {
			t.Errorf("messages[%d] uuid = %s, want %s", i, got[i].Msg.UUID, messages[i].UUID)
		}
	}
}

func TestAssignTurns_LeadingNonUserFoldedIntoTurnOne(t *testing.T) {
	messages := []core.MessageRow{
		{UUID: "a1", Role: "assistant"},
		{UUID: "u1", Role: "user"},
		{UUID: "a2", Role: "assistant"},
	}

	got := assignTurns(messages)
	wantTurns := []int{1, 1, 1}
	for i, want := range wantTurns {
		if got[i].Turn != want {
			t.Errorf("messages[%d] turn = %d, want %d", i, got[i].Turn, want)
		}
	}
}

func TestAssignTurns_Empty(t *testing.T) {
	if got := assignTurns(nil); len(got) != 0 {
		t.Errorf("assignTurns(nil) = %v, want empty", got)
	}
}
