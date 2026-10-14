package dashboard

import "slices"

// Traffic is what the router's request stream tells of the sessions'
// requests at a moment, as the watch reads it, for Sessions' Switchboard to
// draw: what travels each call's cord; and the cords moves no limit forced
// let go, while they hang loose. The zero Traffic is a watch without the
// stream, whose cords are drawn as each look reads them, nothing travelling.
type Traffic struct {
	// Calls are what travels each call's cord, by its row: a session on an
	// account, its every model's requests there travelling the one cord.
	Calls map[Plug]Call
	// Loose are the cords moves no limit forced let go, the oldest first,
	// while they fade.
	Loose []Loose
}

// Plug is where a call's cord is plugged in: a session's row on an account,
// each by its id.
type Plug struct {
	Session, Account string
}

// On is what travels the cord of the call of the session with the given id
// on the account with the given id: nothing, the zero Call, where the stream
// tells of none.
func (t Traffic) On(session, account string) Call {
	return t.Calls[Plug{Session: session, Account: account}]
}

// moves reports whether anything travels the cords t tells of: a pulse
// along one, a shimmer down one, or a cord a move let go, fading.
func (t Traffic) moves() bool {
	for _, c := range t.Calls {
		if c.Pulsing || c.Doing == Streaming {
			return true
		}
	}
	return len(t.Loose) > 0
}

// hangs reports whether t tells of a cord a move let go hanging loose from
// the jack its session left, at p.
func (t Traffic) hangs(p Plug) bool {
	return slices.ContainsFunc(t.Loose, func(l Loose) bool { return l.Session == p.Session && l.From == p.Account })
}

// Doing is what a call's requests are doing on its account, as the request
// stream tells of the latest of each.
type Doing int

const (
	// Asking is its request gone out, with nothing back yet.
	Asking Doing = iota + 1
	// Streaming is its answer streaming back.
	Streaming
	// Answered is its answer just ended, its pulse running home.
	Answered
	// Refused is the upstream refusing its request on the account: its
	// limit reached, or a refusal.
	Refused
	// Throttled is the upstream throttling its request, to be sent again.
	Throttled
)

// Busier reports whether a call doing d says more of its row than one doing
// e, as the busiest of a session's models says what it's doing: streaming,
// then asking, throttled, refused, and answered last.
func (d Doing) Busier(e Doing) bool {
	return d.rank() > e.rank()
}

// rank ranks what's done by how busy it is, as Busier has it: none ranks
// lowest.
func (d Doing) rank() int {
	switch d {
	case Streaming:
		return 5
	case Asking:
		return 4
	case Throttled:
		return 3
	case Refused:
		return 2
	case Answered:
		return 1
	}
	return 0
}

// Call is what travels a call's cord at a moment, as the request stream
// tells of its requests: what they're doing, the busiest of them, which its
// jack shows; the pulse travelling it, while Pulsing; and while an answer
// streams back, how many steps its shimmer has taken, each moving its lit
// cells a cell toward the call.
type Call struct {
	Doing   Doing
	Pulse   Pulse
	Pulsing bool
	Shimmer int
}

// Pulse is a pulse travelling a cord: how far along it is, from 0 at its
// start to 1 at its end; whether it travels back, from the jack to the call,
// as an answer does; and whether it's red, as a limit's or a refusal's is.
type Pulse struct {
	Along     float64
	Back, Red bool
}

// Loose is a cord a move no limit forced let go, hanging loose from the jack
// its session left on the account it left, each by its id: how far it has
// faded, from 0, just let go, to 1, gone.
type Loose struct {
	Session, From string
	Fade          float64
}
