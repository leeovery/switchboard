package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Taken is a read of the ledger's tail as the store takes it in, each line
// decoded once, as Took decodes it: the requests among today's lines it
// gained, and among the lines of each day before today it read again whole,
// by its date. Every other line is passed over.
type Taken struct {
	gained takenLines
	days   map[string]takenLines
}

// takenLines are the requests among a read's lines, by their sessions' ids,
// each session's in the order every story takes them in, as inOrder has it,
// each once, as distinct has it, unpriced.
type takenLines map[string][]heldRequest

// Took decodes a read of the ledger's tail for the store, off the event
// loop: gained, today's lines read since the tail's mark, as Ledger's Today
// gives them; and days, the lines of the days before today read again
// whole, by their dates, as Ledger's Day gives them. Each line is decoded
// once, and only a session's request kept, as isRequest says, each once.
func Took(gained []ledger.Held, days map[string][]ledger.Held) Taken {
	t := Taken{gained: takenOf(gained), days: make(map[string]takenLines, len(days))}
	for date, lines := range days {
		t.days[date] = takenOf(lines)
	}
	return t
}

// takenOf is the requests among lines, by their sessions' ids, as
// takenLines has them.
func takenOf(lines []ledger.Held) takenLines {
	taken := make(takenLines)
	for i := range lines {
		if l := &lines[i].Line; isRequest(l) {
			taken[l.Session] = append(taken[l.Session], heldRequestOf(l))
		}
	}
	for id, requests := range taken {
		taken[id] = distinct(requests)
	}
	return taken
}

// distinct are requests, a session's, as a read gives them, each once, in
// the order every story takes them in, as inOrder has it: a request is
// known by its id and when it arrived, as keyOf knows its line, so one of
// the same id that arrived at the same moment as one before it is the same
// line given again, and passed over. Requests without ids are each kept.
func distinct(requests []heldRequest) []heldRequest {
	seen := make(map[lineKey]bool, len(requests))
	kept := slices.DeleteFunc(requests, func(r heldRequest) bool {
		k, ok := r.key()
		if !ok {
			return false
		}
		given := seen[k]
		seen[k] = true
		return given
	})
	slices.SortStableFunc(kept, inOrder)
	return kept
}
