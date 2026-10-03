package dashboard

import (
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Traffic is what the router's request stream tells of the sessions'
// requests at a moment, as the watch reads it: what each session's model is
// doing on its account, and what travels its cord; when the stream last saw
// each at work; and the moves the stream told of while they show. The zero
// Traffic is a watch without the stream, whose cords and cards' backs draw
// the sessions as each look reads them, with no requests travelling.
type Traffic struct {
	// Live is set while what the watch draws is the stream's: from when it
	// opens until the watch forgets what it told. The cards' backs then say
	// what each session is doing, or how long it has been idle.
	Live bool
	// Calls are what each session's model is doing on its account, where the
	// stream has told of it lately, by where it's plugged in.
	Calls map[Plug]Call
	// Seen are when the stream last told of each session's model on its
	// account, by where it's plugged in, kept once its calls are done with:
	// the router's sessions say when each was last seen as its request
	// arrived, which a long answer leaves far behind.
	Seen map[Plug]time.Time
	// Moves are the moves the stream told of, the oldest first, while each
	// shows: as a re-patch until a listing of the router's sessions shows it
	// done, and as the cord it left hanging loose until that's gone.
	Moves []Move
}

// Plug is where a seat is plugged in: the account its requests go to.
type Plug struct {
	Account string
	Seat    Seat
}

// Doing is what a session's model is doing on its account, as the request
// stream tells of its latest request there.
type Doing int

const (
	// Asking is its request gone out, with nothing back yet.
	Asking Doing = iota + 1
	// Streaming is its answer streaming back.
	Streaming
	// Answered is its answer just ended.
	Answered
	// Refused is the upstream refusing its request on the account: its
	// limit reached, or a refusal.
	Refused
	// Throttled is the upstream throttling its request, to be sent again.
	Throttled
)

// InFlight reports whether a request doing d is in flight on its account:
// asking, streaming, or throttled, to be sent again.
func (d Doing) InFlight() bool {
	return d == Asking || d == Streaming || d == Throttled
}

// Call is what a seat is doing on its account at a moment, as the request
// stream tells of it, and what travels its cord.
type Call struct {
	Doing Doing
	// Since is when its request went out, which it waits on while it asks.
	Since time.Time
	// Tokens are how many tokens its answer has streamed: estimated at four
	// characters a token, or, where Exact is set, as its closing usage counts
	// them.
	Tokens int
	Exact  bool
	// Status is the upstream's answer that refused or throttled it.
	Status int
	// New is set while it asks on an account a move has just brought it to.
	New bool
	// Pulse travels its cord while Pulsing is set.
	Pulse   Pulse
	Pulsing bool
	// Shimmer is how many steps the shimmer down its cord has taken while
	// its answer streams: each step, its lit cells move a cell toward the
	// call.
	Shimmer int
}

// Pulse is a pulse travelling a cord: how far along it is, from 0 at its
// start to 1 at its end; whether it travels back, from the jack to the call,
// as an answer does; and whether it's red, as a limit's or a refusal's is.
type Pulse struct {
	Along     float64
	Back, Red bool
}

// Move is a session's model moving from one account to another, as the
// stream told of it: the seat, the accounts, when, and why, as the router's
// reason gives it.
type Move struct {
	Seat     Seat
	From, To string
	At       time.Time
	Reason   string
	// Listed is set once a listing of the router's sessions shows the move
	// done, the seat where it went: it no longer re-patches, its cord alone
	// hanging loose.
	Listed bool
	// Held is set where a limit moved it: its cord hangs as a stub, unfaded,
	// while the move re-patches.
	Held bool
	// Fade is how far its cord, hanging loose, has faded: from 0, just let
	// go, to 1, gone.
	Fade float64
}

// call is what the seat plugged in at p is doing, as the stream tells of it,
// reporting false where it tells of nothing.
func (t Traffic) call(p Plug) (Call, bool) {
	c, ok := t.Calls[p]
	return c, ok
}

// repatching are the moves that re-patch at the moment: those no listing of
// the router's sessions has shown done.
func (t Traffic) repatching() []Move {
	var moves []Move
	for _, m := range t.Moves {
		if !m.Listed {
			moves = append(moves, m)
		}
	}
	return moves
}

// listing is the sessions as the router listed them, the request stream's
// moves among them, as the cards' backs, their dots and the plain list
// draw them: each seat a move re-patches put on the account it went to, as
// movedIn has it, and a session the router didn't list, which a move
// brought on, first, as the one seen last.
func (f Frame) listing() []status.Session {
	repatching := f.Traffic.repatching()
	if len(repatching) == 0 {
		return f.Sessions
	}
	listing := make([]status.Session, len(f.Sessions))
	for i, s := range f.Sessions {
		s.Assignments = slices.Clone(s.Assignments)
		listing[i] = s
	}
	moves := latestMoves(repatching)
	for _, m := range repatching {
		if moves[m.Seat] != m {
			continue
		}
		brought := f.movedIn(m).assignment
		i := slices.IndexFunc(listing, func(s status.Session) bool { return s.ID == m.Seat.Session })
		if i < 0 {
			listing = slices.Insert(listing, 0, status.Session{ID: m.Seat.Session, Assignments: []status.Assignment{brought}})
			continue
		}
		if j := slices.IndexFunc(listing[i].Assignments, func(a status.Assignment) bool { return a.Model == m.Seat.Model }); j >= 0 {
			listing[i].Assignments[j] = brought
		} else {
			listing[i].Assignments = append(listing[i].Assignments, brought)
		}
	}
	return listing
}

// event is the move as the router's events tell of one, for LOG, which tells
// of it before the router's document does.
func (m Move) event() status.Event {
	return status.Event{Kind: status.EventMoved, At: m.At, Session: m.Seat.Session, Model: m.Seat.Model, From: m.From, To: m.To, Reason: m.Reason}
}

// tokens counts n tokens as briefly as a row has room for: as it is under a
// thousand, then in thousands, to a tenth under ten thousand, as "1.2k", or
// whole, as "34k", then in millions, as "1.2M".
func tokens(n int) string {
	switch thousands := float64(n) / 1000; {
	case n < 1000:
		return fmt.Sprint(n)
	case thousands < 9.95:
		return fmt.Sprintf("%.1fk", thousands)
	case thousands < 999.5:
		return fmt.Sprintf("%.0fk", thousands)
	default:
		return fmt.Sprintf("%.1fM", thousands/1000)
	}
}

// streamed says how many tokens the call's answer has streamed, as in "↓
// ~1.2k", the tilde marking an estimate.
func (c Call) streamed() string {
	if c.Exact {
		return "↓ " + tokens(c.Tokens)
	}
	return "↓ ~" + tokens(c.Tokens)
}
