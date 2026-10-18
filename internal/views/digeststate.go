package views

import (
	"fmt"
	"maps"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// DigestVersion is the version of what a day's digest holds of each session
// that ran on the day: its state at the day's end, as storedSession has it.
// A release that stores a session otherwise, or tells a story otherwise of
// what's stored, writes another, and a digest of another version doesn't
// stand: its sessions are read whole from their lines.
const DigestVersion = 1

// storedSession is a session the store holds as a day's digest stores it,
// sealed at the start of the day after its own, none of its requests open
// and its page untallied: the date of its day; its horizon; its story of its
// requests; and how many of them arrived on each day, by date, for a day
// read again whole. Nothing priced is stored, nor its requests on its day,
// nor the flights of its requests: it's restored onto a later day, to be
// priced and counted there, as restored has it. Its keys are short and its
// zeros left out.
type storedSession struct {
	Date    string         `json:"d"`
	Horizon storedInstant  `json:"h"`
	Story   storedStory    `json:"s"`
	Counts  map[string]int `json:"c,omitempty"`
}

// storedStory is a story as a session's stored state holds it: whether it
// has taken in a request, its first and last; what names it; the directory
// it last named; its last request whose answer gave usage, and its last such
// of its own conversation, each none where none did, as metered says; whether
// a request of it said its class; each move a request of it made; and, of
// each of its threads, when its last request ended, and how long its writes
// to the cache last, in nanoseconds. Not its prices, its day, its requests on
// the day nor their worth.
type storedStory struct {
	Begun      bool                     `json:"b,omitzero"`
	First      storedFlight             `json:"f,omitzero"`
	Last       storedFlight             `json:"l,omitzero"`
	Main       storedSighting           `json:"mn,omitzero"`
	Latest     storedSighting           `json:"lt,omitzero"`
	Dir        string                   `json:"d,omitempty"`
	Used       *storedRequest           `json:"u,omitempty"`
	UsedMain   *storedRequest           `json:"um,omitempty"`
	Classed    bool                     `json:"c,omitzero"`
	Moves      []storedMove             `json:"mv,omitempty"`
	Ends       map[string]storedInstant `json:"e,omitempty"`
	LivesNanos map[string]int64         `json:"lv,omitempty"`
}

// storedSighting is a sighting as a stored story holds it: its model, the
// account it went to, and when it came.
type storedSighting struct {
	Model   string        `json:"m,omitempty"`
	Account string        `json:"a,omitempty"`
	At      storedInstant `json:"t,omitzero"`
}

// storedMove is a move as a stored story holds it: the request that made it,
// whose flight says when it came; the accounts it went out on that couldn't
// serve it, an empty list kept apart from none; the model it asked for, the
// account it left and the one it went to, and why; and whether it's settled,
// the tokens the request that settled it wrote to the cache, and that
// request, where it wrote the session's context again. Not its cost, worked
// out again as it's restored.
type storedMove struct {
	By      storedFlight   `json:"b,omitzero"`
	Tried   []storedTried  `json:"t,omitzero"`
	Model   string         `json:"m,omitempty"`
	From    string         `json:"f,omitempty"`
	To      string         `json:"to,omitempty"`
	Reason  string         `json:"r,omitempty"`
	Settled bool           `json:"s,omitzero"`
	Written int            `json:"w,omitzero"`
	Rewrote *storedRequest `json:"rw,omitempty"`
}

// stored is s as a day's digest stores it, at the end of its day, its sealed
// story told of every request it holds. It's refused, as its caller's fault,
// where s isn't so: sealed elsewhere than at the start of the day after its
// own, a request of it held open, or its page tallied.
func (s *heldSession) stored() (storedSession, error) {
	day := s.day()
	switch end := dayfile.DayStart(day.Start, 1); {
	case s.horizon != instantOf(end):
		return storedSession{}, fmt.Errorf("store a session of %s: sealed at %v, not at the day's end, %v", day.Date, s.horizon.time(), end)
	case len(s.open) > 0:
		return storedSession{}, fmt.Errorf("store a session of %s: %d of its requests open", day.Date, len(s.open))
	case s.page != nil:
		return storedSession{}, fmt.Errorf("store a session of %s: its page tallied", day.Date)
	}
	return storedSession{Date: day.Date, Horizon: s.horizon.stored(), Story: s.sealed.stored(), Counts: maps.Clone(s.counts)}, nil
}

// restored is the session st stores, as the store would hold it on the day
// given, a later one than st's: its story put back on st's day, as story's
// restored has it, then rolled on to the day given as the store rolls a
// session on, as heldSession's on has it, so it's priced and counted there.
// It holds none of its requests open, nor the flights of any, so a line of a
// request it holds, given again, has its session read whole, as gained can't
// place it: but for a day before the one before the day given, read again
// whole, as many of its requests as it holds of that day, which gained
// places by their count. It's refused on a day not later than st's, and
// where st's date isn't one.
func (st *storedSession) restored(prices ledger.Table, day LedgerDay) (*heldSession, error) {
	start, _, ok := dayfile.Day(st.Date)
	switch {
	case !ok:
		return nil, fmt.Errorf("restore a session of %q: not a date", st.Date)
	case day.Date <= st.Date:
		return nil, fmt.Errorf("restore a session of %s on %s: not a later day", st.Date, day.Date)
	}
	sealed := st.Story.restored(prices, LedgerDay{Start: start, Date: st.Date})
	counts := make(map[string]int, len(st.Counts))
	maps.Copy(counts, st.Counts)
	s := &heldSession{
		horizon: st.Horizon.instant(), sealed: sealed, story: sealed, flights: make(map[string]heldFlights), counts: counts,
		// Empty, not none, as sealedAt leaves a session it sealed every
		// request of.
		open: []heldRequest{},
	}
	return s.on(day, prices), nil
}

// stored is s as a session's stored state holds it.
func (s *story) stored() storedStory {
	st := storedStory{
		Begun: s.begun, First: s.first.stored(), Last: s.last.stored(), Main: s.naming.main.stored(), Latest: s.naming.latest.stored(),
		Dir: s.dir, Used: storedUsed(&s.used), UsedMain: storedUsed(&s.usedMain), Classed: s.classed,
		Ends: make(map[string]storedInstant, len(s.ends)), LivesNanos: make(map[string]int64, len(s.lives)),
	}
	for i := range s.moves {
		st.Moves = append(st.Moves, s.moves[i].stored())
	}
	for thread, end := range s.ends {
		st.Ends[thread] = instantOf(end).stored()
	}
	for thread, life := range s.lives {
		st.LivesNanos[thread] = int64(life)
	}
	return st
}

// restored is the story st stores, told on the day d, as newStory has it of
// prices and d: its requests unpriced, as heldRequestOf gives them, none of
// them counted on the day, and each settled move's cost worked out again at
// d's prices, as rewrites works it out.
func (st *storedStory) restored(prices ledger.Table, d LedgerDay) *story {
	s := newStory(prices, d)
	s.begun, s.first, s.last = st.Begun, st.First.flight(), st.Last.flight()
	s.naming = naming{main: st.Main.sighting(), latest: st.Latest.sighting()}
	s.dir, s.used, s.usedMain, s.classed = st.Dir, usedOf(st.Used), usedOf(st.UsedMain), st.Classed
	for i := range st.Moves {
		m := st.Moves[i].restored()
		if m.rewrote != nil {
			m.cost = s.rewriteCost(&m)
		}
		s.moves = append(s.moves, m)
	}
	for thread, end := range st.Ends {
		s.ends[thread] = end.instant().time()
	}
	for thread, life := range st.LivesNanos {
		s.lives[thread] = time.Duration(life)
	}
	return s
}

// storedUsed is r, a story's last request whose answer gave usage, as a
// stored story holds it: none where it's none, as metered says.
func storedUsed(r *heldRequest) *storedRequest {
	if !r.metered() {
		return nil
	}
	stored := r.stored()
	return &stored
}

// usedOf is the request a stored story holds as storedUsed stores it, the
// zero request of none.
func usedOf(stored *storedRequest) heldRequest {
	if stored == nil {
		return heldRequest{}
	}
	return stored.restored()
}

// stored is s as a stored story holds it.
func (s sighting) stored() storedSighting {
	return storedSighting{Model: s.model, Account: s.account, At: instantOf(s.at).stored()}
}

// sighting is the sighting st stores.
func (st storedSighting) sighting() sighting {
	return sighting{model: st.Model, account: st.Account, at: st.At.instant().time()}
}

// stored is m as a stored story holds it.
func (m *move) stored() storedMove {
	st := storedMove{
		By: m.by.stored(), Tried: storedTriedOf(m.tried), Model: m.model, From: m.from, To: m.to, Reason: m.reason, Settled: m.settled, Written: m.written,
	}
	if m.rewrote != nil {
		rewrote := m.rewrote.stored()
		st.Rewrote = &rewrote
	}
	return st
}

// restored is the move st stores, its rewriting request unpriced, and its
// cost none, for its story to work out.
func (st *storedMove) restored() move {
	m := move{by: st.By.flight(), tried: triedOf(st.Tried), model: st.Model, from: st.From, to: st.To, reason: st.Reason, settled: st.Settled, written: st.Written}
	m.at = m.by.arrived()
	if st.Rewrote != nil {
		rewrote := st.Rewrote.restored()
		m.rewrote = &rewrote
	}
	return m
}
