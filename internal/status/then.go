package status

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
)

// WasKind is how an account stood at a moment gone, as a Was says.
type WasKind int

// The kinds, in the order Then tries them, which is ROUTING's, as an
// account's Routing says how it stands at a moment: the first that holds is
// the one said.
const (
	// WasTokenless is an account without a usable token now, which had no
	// room at any moment, as Runway draws it.
	WasTokenless WasKind = iota + 1
	// WasLimited is an account a limit held back from every request.
	WasLimited
	// WasRefused is an account whose token the upstream refused.
	WasRefused
	// WasUnread is an account nothing was read of yet.
	WasUnread
	// WasCapped is an account at its cap, which held it back.
	WasCapped
	// WasRoom is an account with room.
	WasRoom
)

// Was is how an account stood at a moment gone, as Then has it: its kind,
// and, of a hold, when it began and when it lifts, each zero where what Then
// is given doesn't tell.
type Was struct {
	Kind         WasKind
	Since, Until time.Time
}

// Span is a hold of an account the router told of: of Kind WasLimited, a
// limit; of Kind WasRefused, the upstream refusing its token; of Kind
// WasCapped, its cap. It holds from From, zero where nothing tells when it
// began, as from before any moment asked of, up to Until, zero where that
// isn't known. Windows are, of a limit or a cap naming windows, the one
// window it holds, as such an event is a span a window, each from when that
// window came to be held, as the event's holds has them: such a hold the
// readings history shows itself, so the span tells only when one the
// readings show began, as heldFrom has it; a limit naming none holds back
// every request, and the readings can't show it.
type Span struct {
	Kind        WasKind
	From, Until time.Time
	Windows     []string
}

// holds reports whether the span holds at t.
func (s Span) holds(t time.Time) bool {
	return !t.Before(s.From) && s.Until.After(t)
}

// heldFrom is when the span s has the window w, last read, held, reporting
// false where it doesn't: of a span naming no window, as a limit naming
// none holds them all whatever their periods, from its start; of one naming
// w, from its start, where that came in w's period, as its last reading has
// it, as a hold of w told of from before then was of an earlier period, and
// says nothing of when w's hold now began. It's the one rule a hold of a
// window is dated by, from the router's events.
func (s Span) heldFrom(w quota.Window) (time.Time, bool) {
	switch {
	case len(s.Windows) == 0:
		return s.From, true
	case !slices.Contains(s.Windows, w.Key):
		return time.Time{}, false
	}
	if start, _, spanned := w.Span(); spanned && s.From.Before(start) {
		return time.Time{}, false
	}
	return s.From, true
}

// holds are the holds the router's event e, of a limit or a cap, tells, as
// spans of no kind yet, each till e's until: of one naming no window, as a
// limit can, one from when it began; else one each window it names, from
// when that window came to be held by it, as joinedAt has it, as a window
// can join a hold after it began.
func (e Event) holds() []Span {
	if len(e.Windows) == 0 {
		return []Span{{From: e.At, Until: e.Until}}
	}
	spans := make([]Span, len(e.Windows))
	for i, key := range e.Windows {
		spans[i] = Span{From: e.joinedAt(key), Until: e.Until, Windows: []string{key}}
	}
	return spans
}

// joinedAt is when the window with the given key came to be held by the
// router's event e, of a limit or a cap naming it: as its Joined has it,
// else as e began.
func (e Event) joinedAt(key string) time.Time {
	if at, ok := e.Joined[key]; ok {
		return at
	}
	return e.At
}

// heldFrom is when the router's event e, of a limit or a cap, has the
// window w, last read, held, as its holds have it, by Span's heldFrom,
// reporting false where it doesn't.
func (e Event) heldFrom(w quota.Window) (time.Time, bool) {
	for _, s := range e.holds() {
		if from, ok := s.heldFrom(w); ok {
			return from, true
		}
	}
	return time.Time{}, false
}

// Told are the holds of the account with the given id that the router told
// of, as Then takes them: those its events tell, as toldBy has them, newest
// first; then its limit naming no window, and the refusal of its token,
// where either holds at Now and no span told of its kind naming no window
// holds then, as where the router no longer keeps its event, from before
// anything.
func (m Moment) Told(id string) []Span {
	told := m.doc.toldBy(id)
	a, _ := m.doc.Account(id)
	if a.Limit.holds(m.Now) && len(a.Limit.Windows) == 0 {
		told = untold(told, Span{Kind: WasLimited, Until: a.Limit.Until}, m.Now)
	}
	if a.Refused.holds(m.Now) && a.Refused.Family == "" {
		told = untold(told, Span{Kind: WasRefused, Until: a.Refused.Until}, m.Now)
	}
	return told
}

// toldBy are the holds of the account with the given id that the router's
// events tell, as the document gives them, newest first: each limit, and
// each cap, as its holds have it, a span each window it names, till its
// until, which the router brings forward as a limit lifts early, or the
// account's room after the span's start, as roomAfter has it, where that
// comes first, as a router from before kept a limit's until as it lifted
// early, telling of the room alone; and each refusal of its token, till its
// until, which the router brings forward as a refusal lifts early.
func (d Document) toldBy(id string) []Span {
	var told []Span
	for _, e := range d.Events {
		switch {
		case e.Account != id:
		case e.Kind == EventLimit:
			told = d.heldBy(told, id, e, WasLimited)
		case e.Kind == EventCap:
			told = d.heldBy(told, id, e, WasCapped)
		case e.Kind == EventRefused && e.Family == "":
			told = append(told, Span{Kind: WasRefused, From: e.At, Until: e.Until})
		}
	}
	return told
}

// heldBy is told, with the holds of the account with the given id that the
// event e, of a limit or a cap, tells, as its holds have them, each of the
// given kind, and till the account's room after it began, as roomAfter has
// it, where that comes first.
func (d Document) heldBy(told []Span, id string, e Event, kind WasKind) []Span {
	for _, s := range e.holds() {
		s.Kind, s.Until = kind, d.roomAfter(id, s.From, s.Until)
		told = append(told, s)
	}
	return told
}

// roomAfter is when the router told of the account with the given id having
// room again first after from and before until, else until.
func (d Document) roomAfter(id string, from, until time.Time) time.Time {
	for _, e := range d.Events {
		if e.Kind == EventRoom && e.Account == id && e.At.After(from) && e.At.Before(until) {
			until = e.At
		}
	}
	return until
}

// untold is the spans told, with s, a hold naming no window holding at now,
// added where none of its kind told naming none holds then.
func untold(told []Span, s Span, now time.Time) []Span {
	if slices.ContainsFunc(told, func(held Span) bool { return held.Kind == s.Kind && len(held.Windows) == 0 && held.holds(now) }) {
		return told
	}
	return append(told, s)
}

// Then is how the moment's account a stood at t: one rule for every moment
// gone, live or stepped back to, which at Now agrees with how the moment has
// the account stand. It goes by the readings history's readings given,
// oldest first, as the history reads them, of the windows the moment's
// policy says every model shares, each by its last reading by t, a window
// whose reset has passed by then counting as reset, unused; and by the holds
// the router told of, as Told has them, those naming windows telling only
// when a hold the readings show began. Its kind is the first that holds of:
// without a usable token now, none at any moment; a limit told of naming no
// window, or a window read spent, holding at t; its token refused, as told
// of; nothing read by t; its cap, where its reserve holds now, a window read
// at where its reserve, as configured now, starts, as the readings keep no
// reserve; and room. Of a hold, Since is when the first part of what holds
// it at t began: a told hold's From; of a window's, the router's event of
// it, as reachedAt finds it, else the first reading, of those given, of the
// unbroken run of the window's readings that shows it; a limit's or a cap's
// then begun as the run of the holds reaching back from it did, told of or
// read, as ranBack has it. Until is when the last part lifts, zero where one
// lifts at no known time: a told hold's Until; a window's at its reset, or
// sooner, where a reading of it after t, before that reset, no longer shows
// it, as a reset made by hand starts the window again, as liftedBy has it.
// A run begun at either edge of what the readings reach tells no start, as
// Reach says.
func (m Moment) Then(a AccountNow, read []readings.Reading, reach Reach, told []Span, t time.Time) Was {
	if !a.TokenSet {
		return Was{Kind: WasTokenless}
	}
	windows := m.readBy(a, read, reach, t)
	if was, ok := heldAs(WasLimited, slices.Concat(toldAt(told, WasLimited, t), shownAt(windows, told, WasLimited, t))); ok {
		was.Since = m.ranBack(a, read, reach, told, WasLimited, was.Since)
		return was
	}
	if was, ok := heldAs(WasRefused, toldAt(told, WasRefused, t)); ok {
		return was
	}
	if len(windows) == 0 {
		return Was{Kind: WasUnread}
	}
	if was, ok := heldAs(WasCapped, shownAt(windows, told, WasCapped, t)); ok && a.ReserveHolds {
		was.Since = m.ranBack(a, read, reach, told, WasCapped, was.Since)
		return was
	}
	return Was{Kind: WasRoom}
}

// ranBack is when the unbroken run of the holds of every request of the
// given kind, WasLimited or WasCapped, that holds the account a at from
// began: from, but where such a hold began before it and held on past it, as
// one window can take over a hold another began; and so on back. Such a hold
// is told of, as ranFrom has it, or shown by the readings given of a window
// every model shares, by then, as shownAt has them, those given reaching back
// as reach says: so the readings tell a run across windows as far as they
// reach, and not a part of it the window that began it read before the
// readings given. Zero stays zero, and so does a run reaching back to a hold
// whose start nothing tells.
func (m Moment) ranBack(a AccountNow, read []readings.Reading, reach Reach, told []Span, kind WasKind, from time.Time) time.Time {
	for !from.IsZero() {
		began := ranFrom(told, kind, m.policy.IsShared, from)
		for _, p := range shownAt(m.readBy(a, read, reach, from), told, kind, from) {
			began = firstHeld(began, p.from)
		}
		if !began.Before(from) {
			break
		}
		from = began
	}
	return from
}

// Reach is how far back what the readings Then is given read reaches, each
// edge zero where it reaches back past anything asked of: Kept, when the
// first day the readings history holds starts, those before it standing for
// the days its readings are gone from; and Seen, when the readings the
// dashboard read itself begin, as it probes, or the router doesn't answer,
// which the history holds none of, nor keeps once it's closed. A run whose
// first reading is its window's first from either edge on, its window begun
// before that edge, may have begun before anything read tells, so has no
// start.
type Reach struct {
	Kept, Seen time.Time
}

// edges are the reach's edges, Kept's and Seen's.
func (r Reach) edges() [2]time.Time {
	return [2]time.Time{r.Kept, r.Seen}
}

// windowRead is a window of an account as its readings by a moment have it:
// the last of them; the unbroken run of them ending with it that reads it
// spent, and that reads it at its account's cap; and whether one was read
// from each of the reach's edges on.
type windowRead struct {
	last          quota.Window
	spent, capped run
	reached       [2]bool
}

// run is an unbroken run of a window's readings that shows a hold: whether
// there's one, and when it began, zero where the readings don't tell; and
// when it lifted before the window's reset, as liftedBy has it, zero where
// no reading after the moment it's had by shows it did.
type run struct {
	shown        bool
	from, lifted time.Time
}

// readBy are the windows of the account a that the moment's policy says
// every model shares, by key, each as its readings of those given, oldest
// first, by t, have it, its cap where its reserve, as configured now, starts,
// the readings reaching back as reach says; each run of them lifted early as
// those given after t show, as readAfter has it, read only while one could
// still lift, as liftable says.
func (m Moment) readBy(a AccountNow, read []readings.Reading, reach Reach, t time.Time) map[string]windowRead {
	windows := make(map[string]windowRead)
	for _, r := range read {
		if r.Account != a.ID || !m.policy.IsShared(r.Key) {
			continue
		}
		if !r.At.After(t) {
			windows[r.Key] = windows[r.Key].readAgain(r.Window(), r.At, reach, a.Reserve)
			continue
		}
		if !anyLiftable(windows, r.At) {
			break
		}
		if w := windows[r.Key]; w.liftable(r.At) {
			windows[r.Key] = w.readAfter(r.Window(), r.At, a.Reserve)
		}
	}
	return windows
}

// anyLiftable reports whether a reading at at could still lift a run of any
// of the windows given early, as liftable says.
func anyLiftable(windows map[string]windowRead, at time.Time) bool {
	for _, w := range windows {
		if w.liftable(at) {
			return true
		}
	}
	return false
}

// liftable reports whether a reading of the window w at at could still lift
// one of its runs early, as liftedBy has it: one shown and not lifted, w's
// last reset not passed by then.
func (w windowRead) liftable(at time.Time) bool {
	return !w.last.ResetBy(at) && (w.spent.unlifted() || w.capped.unlifted())
}

// readAfter is the window w, read again at at as window, after the moment
// it's had by, its account's reserve given: each of its runs lifted early,
// as liftedBy has it, the run that reads it spent where the reading holds
// the account back short of that, and the run that reads it at its cap where
// the reading holds it back at neither, as score.HoldOf has it: a cap turned
// to a limit hasn't lifted.
func (w windowRead) readAfter(window quota.Window, at time.Time, reserve float64) windowRead {
	hold := score.HoldOf(window, reserve)
	w.spent = w.spent.liftedBy(w.last, hold >= score.HeldAtLimit, at)
	w.capped = w.capped.liftedBy(w.last, hold >= score.HeldAtCap, at)
	return w
}

// liftedBy is the run r, of the window last read as last, read again at at,
// after the moment it's had by, as a reading that holds the account back as
// the run does, or not: lifted then, where it hadn't, and the reading, before
// last's reset, doesn't, as one of a window reset by hand doesn't, the window
// started again as the router read it; as it was otherwise, its hold lifting
// at the window's reset at the latest.
func (r run) liftedBy(last quota.Window, holds bool, at time.Time) run {
	if r.unlifted() && !holds && !last.ResetBy(at) {
		r.lifted = at
	}
	return r
}

// unlifted reports whether the run shows a hold that hasn't lifted early.
func (r run) unlifted() bool {
	return r.shown && r.lifted.IsZero()
}

// ResetsRead are the resets of the window with the given key that the
// readings given, of an account, oldest first, give: each as read; and each
// between two readings, as score.ResetBetween tells it, as a day's summary
// keeps it: a reset made by hand among them, at the reading that shows it,
// as a hold on the window lifts then, as Then has it.
func ResetsRead(read []readings.Reading, key string) []time.Time {
	var resets []time.Time
	var was quota.Window
	for _, r := range read {
		if r.Key != key {
			continue
		}
		w := r.Window()
		if !w.ResetsAt.IsZero() {
			resets = append(resets, w.ResetsAt)
		}
		if at, ok := score.ResetBetween(was, w, r.At); ok {
			resets = append(resets, at)
		}
		was = w
	}
	return resets
}

// readAgain is the window w, read again at at as window, the readings
// reaching back as reach says, its account's reserve given: a run of its
// readings runs on to it where the last one's reset hadn't passed by then;
// and one it begins began at at, but where it's the window's first read from
// an edge of the reach on, of a window begun before it, as untold says.
func (w windowRead) readAgain(window quota.Window, at time.Time, reach Reach, reserve float64) windowRead {
	ranOn, begun := !w.last.ResetBy(at), at
	if w.untold(window, at, reach) {
		begun = time.Time{}
	}
	read := windowRead{
		last:   window,
		spent:  w.spent.joined(ranOn, score.Spent(window), begun),
		capped: w.capped.joined(ranOn, score.HeldByReserve(window, reserve), begun),
	}
	for i, edge := range reach.edges() {
		read.reached[i] = w.reached[i] || !edge.IsZero() && !at.Before(edge)
	}
	return read
}

// untold reports whether a run of the window's readings begun by its reading
// at at, as window, began at no time the readings tell: the reading is the
// window's first from an edge of the reach on, and the window began before
// that edge, or when isn't known, so the run may have begun before anything
// read tells.
func (w windowRead) untold(window quota.Window, at time.Time, reach Reach) bool {
	began, _, spanned := window.Span()
	for i, edge := range reach.edges() {
		if !edge.IsZero() && !at.Before(edge) && !w.reached[i] && (!spanned || began.Before(edge)) {
			return true
		}
	}
	return false
}

// joined is the run r, once a reading, which shows its hold, or not, joins
// it, a run begun by the reading beginning at begun, zero where the readings
// don't tell when: where it shows it, r, where it ran on to the reading,
// else a run begun then; and none where it doesn't.
func (r run) joined(ranOn, shows bool, begun time.Time) run {
	switch {
	case !shows:
		return run{}
	case ranOn && r.shown:
		return r
	}
	return run{shown: true, from: begun}
}

// heldAt is the part of a hold of the given kind, WasLimited or WasCapped,
// the window's readings show at t, reporting false where they show none then,
// as where its reset has passed: from when the run of them showing it began,
// till it lifted early, as liftedBy has it, else till its reset.
func (w windowRead) heldAt(kind WasKind, t time.Time) (part, bool) {
	r := w.spent
	if kind == WasCapped {
		r = w.capped
	}
	if !r.shown || w.last.ResetBy(t) {
		return part{}, false
	}
	until := w.last.ResetsAt
	if !r.lifted.IsZero() {
		until = r.lifted
	}
	return part{from: r.from, until: until}, true
}

// part is a part of what holds an account back at a moment: from when it
// began, zero where that's before anything Then is given tells, until when
// it lifts, zero where that isn't known.
type part struct {
	from, until time.Time
}

// shownAt are the parts of a hold of the given kind, WasLimited or
// WasCapped, the windows' readings show at t, as heldAt has them, each from
// when the router's event of it, of the spans told, says it began, where one
// does, as reachedAt finds it.
func shownAt(windows map[string]windowRead, told []Span, kind WasKind, t time.Time) []part {
	var parts []part
	for _, w := range windows {
		if p, ok := w.heldAt(kind, t); ok {
			if at, ok := reachedAt(told, kind, w.last, t); ok {
				p.from = at
			}
			parts = append(parts, p)
		}
	}
	return parts
}

// reachedAt is when the hold of the given kind of the window, last read,
// holding at t, began, as the router's event of it tells, of the spans told
// naming windows: the newest of those that has the window held, as
// heldFrom has it, from by t, and lifts after t, where its lift is known.
// It reports false where none does, as a hold the router first read, or one
// whose event it no longer keeps.
func reachedAt(told []Span, kind WasKind, window quota.Window, t time.Time) (time.Time, bool) {
	var at time.Time
	var found bool
	for _, s := range told {
		from, held := s.heldFrom(window)
		switch {
		case s.Kind != kind || len(s.Windows) == 0 || !held || from.After(t):
		case !s.Until.IsZero() && !s.Until.After(t):
		case !found || from.After(at):
			at, found = from, true
		}
	}
	return at, found
}

// toldAt are the parts of a hold of the given kind the router told of that
// hold at t, of the spans given naming no window, as those naming one are
// the readings' to show.
func toldAt(told []Span, kind WasKind, t time.Time) []part {
	var parts []part
	for _, s := range told {
		if s.Kind == kind && len(s.Windows) == 0 && s.holds(t) {
			parts = append(parts, part{from: s.From, until: s.Until})
		}
	}
	return parts
}

// heldAs is how an account stood, held back as kind says by the parts
// given, reporting false where there are none: since the first of them
// began, as firstHeld has it; until the last of them lifts, zero where one
// lifts at no known time.
func heldAs(kind WasKind, parts []part) (Was, bool) {
	if len(parts) == 0 {
		return Was{}, false
	}
	was := Was{Kind: kind, Since: parts[0].from, Until: parts[0].until}
	for _, p := range parts[1:] {
		was.Since = firstHeld(was.Since, p.from)
		if was.Until.IsZero() || p.until.IsZero() {
			was.Until = time.Time{}
		} else {
			was.Until = score.Later(was.Until, p.until)
		}
	}
	return was, true
}

// ranFrom is when the unbroken run of the holds of every request of the
// given kind told of, the windows every model shares as shared says, that
// holds at from began: from, but where such a hold began before it and held
// on past it, as that one began, and so on back, as one window can take
// over a hold another began. A told hold holds on only till the account's
// room after it began, as toldBy has it, so room between ends a run, and
// one whose lift isn't known holds on past nothing. Zero stays zero.
func ranFrom(told []Span, kind WasKind, shared func(string) bool, from time.Time) time.Time {
	for {
		i := slices.IndexFunc(told, func(s Span) bool { return s.heldOnPast(kind, shared, from) })
		if i < 0 {
			return from
		}
		from = told[i].From
	}
}

// heldOnPast reports whether the span is a hold of every request of the
// given kind, the windows every model shares as shared says, that began
// before at and held on past it: one naming no window, or one naming such a
// window.
func (s Span) heldOnPast(kind WasKind, shared func(string) bool, at time.Time) bool {
	every := len(s.Windows) == 0 || slices.ContainsFunc(s.Windows, shared)
	return s.Kind == kind && every && s.From.Before(at) && s.Until.After(at)
}
