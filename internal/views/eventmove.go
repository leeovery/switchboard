package views

import (
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// EventMove is what a move event's move came to, as its session's story
// settles the move, so the Log's Events say of it what its request's routing
// says: the request that made it, and when that arrived; and, once the
// session's own conversation's next request wrote its context again on the
// account it moved to, the tokens it wrote, and what that cost. Written is
// zero, and Cost nil, while no request has, and where the cache would have
// run out anyway; Cost is nil, too, where the price table can't price it.
type EventMove struct {
	Request string
	At      time.Time
	Written int
	Cost    *ledger.Picodollars
}

// moveOf is the move the move event e tells of, as the story s of its
// session tells it, nil where s is nil or tells of none. An event carries no
// request's id, so it's the move of e's model from e's From to e's To whose
// request arrived last at or before e happened: a request's line is filed
// as it arrived, to the millisecond cut down, and the router tells of the
// move once the request is routed, after it arrived, so the move's request
// is never after its event, and a like move later in the day never is
// before it. A move event naming no model, as one from a router from before
// events named it, is of its session's move of any model.
func moveOf(s *story, e status.Event) *move {
	if s == nil {
		return nil
	}
	var made *move
	for i := range s.moves {
		m := &s.moves[i]
		if m.from != e.From || m.to != e.To || e.Model != "" && m.model != e.Model || m.at.After(e.At) {
			continue
		}
		if made == nil || !m.at.Before(made.at) {
			made = m
		}
	}
	return made
}

// eventMove is what the move m came to, as EventMove has it: nil for none.
func eventMove(m *move) *EventMove {
	if m == nil {
		return nil
	}
	made := &EventMove{Request: m.by.Request, At: m.at, Cost: m.cost}
	if m.rewrote != nil {
		made.Written = m.written
	}
	return made
}
