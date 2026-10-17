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
// Rewrote is set where one has, priced or not.
type EventMove struct {
	Request string
	At      time.Time
	Written int
	Cost    *ledger.Picodollars
	Rewrote bool
}

// CostSaid says what writing the session's context again cost, exactly,
// "$1.82", or "unpriced" where the price table can't price it: "" while no
// request has written it, or none will.
func (m EventMove) CostSaid() string {
	return rewriteSaid(m.Rewrote, m.Cost)
}

// moveOf is the move the move event e tells of, as the story s of its
// session tells it, nil where s tells of none. An event carries no
// request's id, so it's the move of e's model from e's From to e's To whose
// request was in flight as e happened, as inFlightAt has it, the one that
// arrived last where several were: the router tells of a move as it routes
// the request, after it arrived and before it ends. A move only comes into
// s as its request's line is written, as the request ends, so until then e
// is of none, never a like move before it whose request had ended. A move
// event naming no model, as one from a router from before events named it,
// is of its session's move of any model.
func moveOf(s *story, e status.Event) *move {
	var made *move
	for i := range s.moves {
		m := &s.moves[i]
		if m.from != e.From || m.to != e.To || e.Model != "" && m.model != e.Model || !m.by.inFlightAt(e.At) {
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
	made := &EventMove{Request: m.by.request, At: m.at, Cost: m.cost, Rewrote: m.rewrote != nil}
	if made.Rewrote {
		made.Written = m.written
	}
	return made
}
