package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// DigestVersion is the version of what a day's digest holds of each session
// that ran on the day: its state at the day's end, as storedSession has it,
// and its page's part of the day, as storedPart has it. A release that
// stores a session otherwise, or tells a story, or tallies a page, otherwise
// of what's stored, writes another, and a digest of another version doesn't
// stand: its sessions are read whole from their lines.
const DigestVersion = 2

// storedSession is a session the store holds as a day's digest stores it,
// sealed at the start of the day after its own, none of its requests open:
// the date of its day; its story of its requests; how many of them arrived
// on each day, by date, for a day read again whole; and, where its page is
// tallied, what the page carries, as storedPage has it, put as JSON and
// read only where the page is restored, as restoredPaged has it, so a
// reader of its story alone decodes none of it. Its horizon isn't stored,
// as its date says it, nor anything priced, its requests on its day, or the
// flights of its requests: it's restored onto a later day, to be priced and
// counted there, as restored has it. Its keys are short and its zeros left
// out.
type storedSession struct {
	Date   string          `json:"d"`
	Story  storedStory     `json:"s"`
	Counts map[string]int  `json:"c,omitempty"`
	Page   json.RawMessage `json:"p,omitempty"`
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
// story told of every request it holds, and what its page carries, where
// it's tallied, as pageTally's put puts it: left out where JSON can't carry
// it, the session's state standing without it. It's refused, as its caller's
// fault, where s isn't so: sealed elsewhere than at the start of the day
// after its own, or a request of it held open.
func (s *heldSession) stored() (storedSession, error) {
	day := s.day()
	switch end := endOfDay(day); {
	case s.horizon != end:
		return storedSession{}, fmt.Errorf("store a session of %s: sealed at %v, not at the day's end, %v", day.Date, s.horizon.time(), end.time())
	case len(s.open) > 0:
		return storedSession{}, fmt.Errorf("store a session of %s: %d of its requests open", day.Date, len(s.open))
	}
	return storedSession{Date: day.Date, Story: s.sealed.stored(), Counts: maps.Clone(s.counts), Page: s.page.put()}, nil
}

// put is st as JSON.
func (st *storedSession) put() (json.RawMessage, error) {
	data, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("put the state of a session of %s: %w", st.Date, err)
	}
	return data, nil
}

// pageRead is how much of its page a session read from its state holds.
type pageRead uint8

const (
	// pageNone holds none of it.
	pageNone pageRead = iota
	// pageToFold holds all of it but its times, counted unheld, as
	// upstreamTimes has them: a fold resting on it needs only how many
	// there are, so its parts are read without them.
	pageToFold
	// pageWhole holds all of it.
	pageWhole
)

// restoredFrom is the session whose state from rests on, as folded puts it,
// restored onto the day given, as restored has it: where page says, its
// page with it, from what the state carries and from's chain of parts, as
// restoredPaged has it, its times with it where page reads it whole. It
// fails where the state doesn't decode as one, or is of another day than the
// digest holding it, as the lines after it would be others than those it was
// told of, or where restored, or restoredPaged, refuses it.
func restoredFrom(from ledger.SessionFrom, prices ledger.Table, day LedgerDay, page pageRead) (*heldSession, error) {
	base := from.Base
	var st storedSession
	if err := json.Unmarshal(base.State, &st); err != nil {
		return nil, fmt.Errorf("read the state of a session in the digest of %s: %w", base.Date, err)
	}
	if st.Date != base.Date {
		return nil, fmt.Errorf("read the state of a session in the digest of %s: a state of %q", base.Date, st.Date)
	}
	if page == pageNone {
		return st.restored(prices, day)
	}
	parts := make([]json.RawMessage, len(from.Parts))
	for i, p := range from.Parts {
		parts[i] = p.Part
	}
	return st.restoredPaged(prices, day, parts, page == pageWhole)
}

// endOfDay is the horizon a session's state at the end of the day d is
// sealed at: the start of the day after it.
func endOfDay(d LedgerDay) instant {
	return instantOf(dayfile.DayStart(d.Start, 1))
}

// restored is the session st stores, as the store would hold it on the day
// given, a later one than st's: sealed at the end of st's day, as endOfDay
// has it; its story put back on st's day, as story's restored has it, then
// rolled on to the day given as the store rolls a session on, as
// heldSession's on has it, so it's priced and counted there. It holds none
// of its requests open, nor the flights of any, so a line of a request it
// holds, given again, has its session read whole, as gained can't place it:
// but for a day before the one before the day given, read again whole, as
// many of its requests as it holds of that day, which gained places by
// their count. It's refused on a day not later than st's, and where st's
// date isn't one.
func (st *storedSession) restored(prices ledger.Table, day LedgerDay) (*heldSession, error) {
	start, _, ok := dayfile.Day(st.Date)
	switch {
	case !ok:
		return nil, fmt.Errorf("restore a session of %q: not a date", st.Date)
	case day.Date <= st.Date:
		return nil, fmt.Errorf("restore a session of %s on %s: not a later day", st.Date, day.Date)
	}
	own := LedgerDay{Start: start, Date: st.Date}
	sealed := st.Story.restored(prices, own)
	counts := make(map[string]int, len(st.Counts))
	maps.Copy(counts, st.Counts)
	s := &heldSession{horizon: endOfDay(own), sealed: sealed, story: sealed, flights: make(map[string]heldFlights), counts: counts}
	if sealed.begun {
		// Empty, not none, as sealedAt leaves a session it sealed every
		// request of; a read of no request holds none.
		s.open = []heldRequest{}
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
