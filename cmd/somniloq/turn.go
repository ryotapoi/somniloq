package main

import "github.com/ryotapoi/somniloq/internal/core"

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
