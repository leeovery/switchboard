package views

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// busyWithin is how lately a session was seen at work for it to be busy, as
// the Overview's SESSIONS and Accounts' Detail say it: in the last minute.
const busyWithin = time.Minute

// Activity is what a session is doing at a moment, or its models on an
// account, as the router's listing and the request stream tell, or, of one
// no router lists, its lines.
type Activity struct {
	// Doing is what it's doing.
	Doing Doing
	// Chars counts the characters its answer has streamed so far, while
	// it's Answering: zero till the stream counts any.
	Chars int
	// Status is the answer that refused or throttled its request, while
	// it's Refused or Throttled.
	Status int
	// Since is, in flight, when its request went out; Idle, when it was last
	// at work, as its last answer ended: when the router or the stream last
	// saw it, or its line gives its answer's end, whichever is latest; and
	// Ended, when its last request came. It's zero where none of that is
	// known.
	Since time.Time
	// LastSeen is when the router, or the stream, last saw it at work, or,
	// of a session's, its last request came, as its line gives it.
	LastSeen time.Time
}

// Doing is what a session is doing, or its models on an account.
type Doing int

const (
	// Idle is no request of it in flight: the zero Doing.
	Idle Doing = iota
	// Asking is its request gone out, with nothing back yet.
	Asking
	// Answering is its answer streaming back.
	Answering
	// Throttled is its request throttled, to go out again on its account.
	Throttled
	// Refused is its request refused on its account, its limit reached or
	// not, before it goes out again elsewhere.
	Refused
	// Ended is a session the router no longer lists, an IdleHour after its
	// last request.
	Ended
)

// rank ranks what's done by how busy it is, as the busiest of a session's
// models says what the session's doing: answering, asking, throttled,
// refused, idle, then ended.
func (d Doing) rank() int {
	switch d {
	case Answering:
		return 5
	case Asking:
		return 4
	case Throttled:
		return 3
	case Refused:
		return 2
	case Idle:
		return 1
	}
	return 0
}

// InFlight reports whether a request of it is in flight: asking,
// answering, throttled, or refused before it goes out again.
func (a Activity) InFlight() bool {
	return a.Doing != Idle && a.Doing != Ended
}

// Busy reports whether it's busy at now, as the Overview's SESSIONS and
// Accounts' Detail say it: with a request in flight, or seen at work in the
// last minute.
func (a Activity) Busy(now time.Time) bool {
	return a.InFlight() || now.Sub(a.LastSeen) < busyWithin
}

// Works reports whether it works, as Sessions' switchboard says it: while it
// asks, while it's answering, and while a request of its is throttled, to be
// sent again.
func (a Activity) Works() bool {
	return a.Doing == Asking || a.Doing == Answering || a.Doing == Throttled
}

// Tokens are how many tokens its answer has streamed so far, while it's
// Answering, estimated at CharsPerToken characters a token.
func (a Activity) Tokens() int {
	return a.Chars / CharsPerToken
}

// inUTC is a, its times in UTC, as the List gives its times.
func (a Activity) inUTC() Activity {
	a.Since, a.LastSeen = a.Since.UTC(), a.LastSeen.UTC()
	return a
}

// with is a, or b where b's busier, as its doing ranks it, or, both in
// flight alike, sent later; seen as late as either was, and where neither's
// in flight and both do alike, since the later of the two.
func (a Activity) with(b Activity) Activity {
	seen := score.Later(a.LastSeen, b.LastSeen)
	switch r, s := a.Doing.rank(), b.Doing.rank(); {
	case s > r, s == r && b.InFlight() && b.Since.After(a.Since):
		a = b
	case s == r && !a.InFlight():
		a.Since = score.Later(a.Since, b.Since)
	}
	a.LastSeen = seen
	return a
}

// busiest is the busiest of the activities, as with has them, the first
// where they tie: zero, Idle, where there's none.
func busiest(activities []Activity) Activity {
	if len(activities) == 0 {
		return Activity{}
	}
	a := activities[0]
	for _, b := range activities[1:] {
		a = a.with(b)
	}
	return a
}

// doing is what the request is doing, as the stream told: while it's in
// flight, refused, where its answer refused it on its account, its limit
// reached or not, before it goes out again; else answering, once its
// answer's first byte has come, which a throttling passed on gives too;
// else throttled, to go out again; else asking, since it went. It's idle
// once it ended, or was lost.
func (t Told) doing() Activity {
	if !t.InFlight() {
		return Activity{}
	}
	a := Activity{Doing: Asking, Since: t.Sent}
	switch {
	case t.Verdict == VerdictLimited || t.Verdict == VerdictRefused:
		a.Doing, a.Status = Refused, t.Status
	case !t.First.IsZero():
		a.Doing, a.Chars = Answering, t.Chars
	case t.Verdict == VerdictThrottled:
		a.Doing, a.Status = Throttled, t.Status
	}
	return a
}

// busierThan reports whether the request t is busier than u, as what it's
// doing ranks it, else sent later, else, to settle it, its id sorting last.
func (t Told) busierThan(u Told) bool {
	return cmp.Or(
		cmp.Compare(t.doing().Doing.rank(), u.doing().Doing.rank()),
		t.Sent.Compare(u.Sent),
		strings.Compare(t.Request, u.Request),
	) > 0
}

// seatOf names a session's model on an account.
type seatOf struct {
	session, model, account string
}

// heard is what the request stream told of a session's model's requests on
// an account: the busiest of them in flight, as busierThan has it, where
// flying says any is; and when it last told of any, as one ended, or as it
// last told of one it lost, whose end came later still.
type heard struct {
	busiest Told
	flying  bool
	last    time.Time
}

// doing is what its requests in flight are doing, the busiest of them, seen
// as late as the stream told of any: idle, where none is in flight, since
// the stream last told of one.
func (h heard) doing() Activity {
	a := Activity{Since: h.last, LastSeen: h.last}
	if h.flying {
		a = a.with(h.busiest.doing())
	}
	return a
}

// hearing is what a request stream told of each session's model on each
// account, but of Claude Code's quota checks, which are no session's work;
// whether it's live; and each session's requests that spend quota, by its
// id, the first sent first, as its turns count them and what names it reads
// them.
type hearing struct {
	live  bool
	seats map[seatOf]heard
	told  map[string][]Told
}

// hearing is what s told of each session's model on each account, and of
// each session's requests, of those it knows, as known has them: so nothing
// is in flight while s isn't live.
func (s Stream) hearing() hearing {
	h := hearing{live: s.Live, seats: make(map[seatOf]heard), told: make(map[string][]Told)}
	for t := range s.known() {
		if t.Check || t.Session == "" {
			continue
		}
		k := seatOf{session: t.Session, model: t.Model, account: t.Account}
		got := h.seats[k]
		got.last = score.Later(got.last, t.Heard)
		if t.InFlight() && (!got.flying || t.busierThan(got.busiest)) {
			got.busiest, got.flying = t, true
		}
		h.seats[k] = got
		if t.Spends() {
			h.told[t.Session] = append(h.told[t.Session], t)
		}
	}
	for _, told := range h.told {
		slices.SortFunc(told, func(a, b Told) int { return cmp.Or(a.Sent.Compare(b.Sent), strings.Compare(a.Request, b.Request)) })
	}
	return h
}

// seat is what the session with the given id's model is doing on its
// account, as the router listed it in a: what the stream tells of its
// requests there, while it's live and has told of any, in flight, ended or
// lost; else what the router listed, as the stream's first events don't
// say when they're all in. It's seen as late as either saw it, and idle
// since then.
func (h hearing) seat(session string, a status.Assignment) Activity {
	got, told := h.seats[seatOf{session: session, model: a.Model, account: a.Account}]
	listed := Activity{Since: a.LastSeen, LastSeen: a.LastSeen}
	if h.live && told {
		return listed.with(got.doing())
	}
	return listed.with(got.doing()).with(listedDoing(a))
}

// listedDoing is what the router listed a's model's requests in flight
// doing, since it last saw them: answering, nothing counted, or asking; or
// idle, where none is.
func listedDoing(a status.Assignment) Activity {
	switch a.InFlight {
	case status.Answering:
		return Activity{Doing: Answering, Since: a.LastSeen, LastSeen: a.LastSeen}
	case status.Asking:
		return Activity{Doing: Asking, Since: a.LastSeen, LastSeen: a.LastSeen}
	}
	return Activity{}
}

// saw reports whether the stream, live, has seen the session with the given
// id at work within the IdleHour before now: told of a request of it in
// flight, or of any of its requests since.
func (h hearing) saw(id string, now time.Time) bool {
	if !h.live {
		return false
	}
	for k, got := range h.seats {
		if k.session == id && (got.flying || now.Sub(got.last) <= IdleHour) {
			return true
		}
	}
	return false
}

// session is what the session s is doing as a whole, as the router listed
// it: the busiest of its models on their accounts, as seat has each, and of
// those the stream told of that the router didn't list, as a model new to it
// or one a move took elsewhere, in the order of their names.
func (h hearing) session(s status.Session) Activity {
	activities := make([]Activity, 0, len(s.Assignments))
	for _, a := range s.Assignments {
		activities = append(activities, h.seat(s.ID, a))
	}
	var unlisted []seatOf
	for k := range h.seats {
		if k.session == s.ID && !slices.ContainsFunc(s.Assignments, func(a status.Assignment) bool { return a.Model == k.model && a.Account == k.account }) {
			unlisted = append(unlisted, k)
		}
	}
	slices.SortFunc(unlisted, func(x, y seatOf) int {
		return cmp.Or(strings.Compare(x.model, y.model), strings.Compare(x.account, y.account))
	})
	for _, k := range unlisted {
		activities = append(activities, h.seats[k].doing())
	}
	return busiest(activities)
}
