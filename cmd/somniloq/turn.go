package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ryotapoi/somniloq/internal/core"
)

// turnMessage pairs a message with its conversation turn number.
type turnMessage struct {
	Turn int
	Msg  core.MessageRow
}

// assignTurns numbers messages by conversation turn: each user message starts
// a new turn (1-based), and following non-user messages belong to that turn.
// Messages before the first user message are folded into turn 1 so no message
// is unreachable through turn ranges.
//
// This numbering is the contract behind every turn-based view (`outline`,
// turn-range addressing in `show`): callers must pass the full GetMessages
// output (chronological, sidechain excluded) so all consumers derive
// identical numbers.
func assignTurns(messages []core.MessageRow) []turnMessage {
	result := make([]turnMessage, len(messages))
	turn := 0
	for i, m := range messages {
		if m.Role == "user" {
			turn++
		}
		result[i] = turnMessage{Turn: max(turn, 1), Msg: m}
	}
	return result
}

// parseTurnRange parses a --turn value: either a single turn number ("40") or
// an inclusive range ("40..60").
func parseTurnRange(s string) (lo, hi int, err error) {
	loStr, hiStr, isRange := strings.Cut(s, "..")
	if !isRange {
		hiStr = loStr
	}
	lo, err = strconv.Atoi(loStr)
	if err == nil {
		hi, err = strconv.Atoi(hiStr)
	}
	if err != nil {
		return 0, 0, fmt.Errorf("--turn must be N or N..M, got %q", s)
	}
	if lo < 1 {
		return 0, 0, fmt.Errorf("--turn numbers start at 1, got %d", lo)
	}
	if hi < lo {
		return 0, 0, fmt.Errorf("--turn range must not be reversed, got %q", s)
	}
	return lo, hi, nil
}

// filterTurns keeps the messages whose turn falls in the inclusive range
// [lo, hi].
func filterTurns(messages []core.MessageRow, lo, hi int) []core.MessageRow {
	var result []core.MessageRow
	for _, tm := range assignTurns(messages) {
		if tm.Turn >= lo && tm.Turn <= hi {
			result = append(result, tm.Msg)
		}
	}
	return result
}

// filterLastTurns keeps the messages of the session's last n turns.
func filterLastTurns(messages []core.MessageRow, n int) []core.MessageRow {
	turns := assignTurns(messages)
	if len(turns) == 0 {
		return nil
	}
	hi := turns[len(turns)-1].Turn
	return filterTurns(messages, max(hi-n+1, 1), hi)
}
