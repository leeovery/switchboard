// Package views builds each view's data from what's read: what a page of the
// dashboard draws, and its data verb prints, so the two never disagree. It
// opens nothing itself; it reads through what it's given.
package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Ends reports whether an answer on a turn's main thread that stopped at stop
// ends its turn: one at end_turn, max_tokens, stop_sequence or refusal does.
// Any other leaves it going, as tool_use and pause_turn do, each continued by
// the client sending the answer back, and as one never seen does.
func Ends(stop string) bool {
	switch stop {
	case "end_turn", "max_tokens", "stop_sequence", "refusal":
		return true
	}
	return false
}

// Request is a request of a turn: its line, and whether it was on the turn's
// main thread.
type Request struct {
	ledger.Line
	Main bool
}

// Turn is a prompt to the answer that ended it, as a session's lines tell it.
type Turn struct {
	// Requests are its message lines, in the order they arrived.
	Requests []Request
	// Started is when its first request arrived, and Ended when the request
	// that ended it did: zero while it's going.
	Started, Ended time.Time
	// Stop is the stop reason of the answer that ended it, and Canceled
	// whether a main-thread request its client canceled did: neither while
	// it's going.
	Stop     string
	Canceled bool
}

// Going reports whether the turn has yet to end.
func (t Turn) Going() bool {
	return t.Ended.IsZero()
}

// Thread returns the length of the turn's main thread as it last stood: 0
// where none of its requests was on it.
func (t Turn) Thread() int {
	for _, r := range slices.Backward(t.Requests) {
		if r.Main {
			return r.Shape.Messages
		}
	}
	return 0
}

// Turns returns the turns a session's lines tell, oldest first, from its
// lines in the order they arrived: its message lines alone, as checks and
// counts are left out.
//
// A request is on its turn's main thread where its thread length is at least
// the main thread's last, and the turn ends at an answer on it whose stop
// Ends, or at one its client canceled; a side request ends nothing. The
// request after a turn's end starts the next, its main thread afresh, so a
// conversation compacted or cleared starts a turn short. A request whose
// thread length is unknown, its body unread, is on no thread.
func Turns(lines []ledger.Line) []Turn {
	var t teller
	for _, line := range lines {
		if line.Kind == ledger.KindMessage {
			t.tell(line)
		}
	}
	return t.turns
}

// teller tells a session's turns from its message lines, a line at a time.
type teller struct {
	turns []Turn
	// going is the last of turns, while it's going, else nil, and main the
	// length of its main thread as it last stood, 0 for none yet.
	going *Turn
	main  int
}

// tell takes line into the turn going, starting one where none is, and ends
// the turn where line does.
func (t *teller) tell(line ledger.Line) {
	if t.going == nil {
		t.turns = append(t.turns, Turn{Started: line.At})
		t.going, t.main = &t.turns[len(t.turns)-1], 0
	}
	length := line.Shape.Messages
	main := length > 0 && length >= t.main
	if main {
		t.main = length
	}
	t.going.Requests = append(t.going.Requests, Request{Line: line, Main: main})
	switch {
	case !main:
		return
	case Ends(line.Answer.Stop):
		t.going.Stop = line.Answer.Stop
	case line.Canceled:
		t.going.Canceled = true
	default:
		return
	}
	t.going.Ended = line.At.Add(time.Duration(line.TotalMS) * time.Millisecond)
	t.going = nil
}
