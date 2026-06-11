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

func TestParseTurnRange(t *testing.T) {
	tests := []struct {
		in      string
		lo, hi  int
		wantErr bool
	}{
		{in: "40", lo: 40, hi: 40},
		{in: "40..60", lo: 40, hi: 60},
		{in: "1..1", lo: 1, hi: 1},
		{in: "0", wantErr: true},
		{in: "-1", wantErr: true},
		{in: "60..40", wantErr: true},
		{in: "a", wantErr: true},
		{in: "1..b", wantErr: true},
		{in: "1..2..3", wantErr: true},
		{in: "..", wantErr: true},
	}
	for _, tt := range tests {
		lo, hi, err := parseTurnRange(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseTurnRange(%q) = (%d, %d), want error", tt.in, lo, hi)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTurnRange(%q): %v", tt.in, err)
			continue
		}
		if lo != tt.lo || hi != tt.hi {
			t.Errorf("parseTurnRange(%q) = (%d, %d), want (%d, %d)", tt.in, lo, hi, tt.lo, tt.hi)
		}
	}
}

func TestFilterTurns_Range(t *testing.T) {
	messages := []core.MessageRow{
		{UUID: "u1", Role: "user"},
		{UUID: "a1", Role: "assistant"},
		{UUID: "u2", Role: "user"},
		{UUID: "a2", Role: "assistant"},
		{UUID: "u3", Role: "user"},
	}

	got := filterTurns(messages, 2, 2)
	wantUUIDs := []string{"u2", "a2"}
	if len(got) != len(wantUUIDs) {
		t.Fatalf("len = %d, want %d", len(got), len(wantUUIDs))
	}
	for i, want := range wantUUIDs {
		if got[i].UUID != want {
			t.Errorf("got[%d].UUID = %s, want %s", i, got[i].UUID, want)
		}
	}
}

func TestFilterTurns_Tail(t *testing.T) {
	messages := []core.MessageRow{
		{UUID: "u1", Role: "user"},
		{UUID: "a1", Role: "assistant"},
		{UUID: "u2", Role: "user"},
		{UUID: "u3", Role: "user"},
		{UUID: "a3", Role: "assistant"},
	}

	got := filterLastTurns(messages, 2)
	wantUUIDs := []string{"u2", "u3", "a3"}
	if len(got) != len(wantUUIDs) {
		t.Fatalf("len = %d, want %d", len(got), len(wantUUIDs))
	}
	for i, want := range wantUUIDs {
		if got[i].UUID != want {
			t.Errorf("got[%d].UUID = %s, want %s", i, got[i].UUID, want)
		}
	}
}

func TestFilterTurns_TailLargerThanSession(t *testing.T) {
	messages := []core.MessageRow{
		{UUID: "u1", Role: "user"},
		{UUID: "a1", Role: "assistant"},
	}
	if got := filterLastTurns(messages, 10); len(got) != 2 {
		t.Errorf("len = %d, want 2 (whole session)", len(got))
	}
}

func TestFilterTurns_RangeBeyondSession(t *testing.T) {
	messages := []core.MessageRow{
		{UUID: "u1", Role: "user"},
	}
	if got := filterTurns(messages, 5, 9); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestFilterTurns_TailEmpty(t *testing.T) {
	if got := filterLastTurns(nil, 3); len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
