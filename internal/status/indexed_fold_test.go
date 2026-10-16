package status

import (
	"maps"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
)

// Judging how an account stood by its readings indexed, as Index has them,
// judges as the fold of every one of them in turn that came before did,
// kept here as the reference: each window every model shares, its last
// reading by a moment and its runs, their starts and their lifts; Then; and
// RoomThen in each window, a model's own among them. Over random histories,
// each from a seed of its own so a failure is judged again from its number:
// of reserves none, 5%, 10% and 20%, held or not; reaches from the keep and
// from the readings seen, or neither; readings rejected, readings without a
// reset, readings at one moment, and other accounts' readings; and the holds
// the router told of, of every kind, naming a window or none.
func TestTheIndexJudgesAsTheFoldOfEveryReadingDid(t *testing.T) {
	const histories, moments = 1500, 12
	m := Moment{policy: score.Policy{Shared: []string{"5h", "7d"}, Started: "5h", Pressure: "5h"}}
	for n := range uint64(histories) {
		h := randomHistory(n)
		indexed := Index(slices.Clone(h.read))
		for range moments {
			at := h.moment()
			got, want := m.readBy(h.account, indexed, h.reach, at), foldBy(m, h.account, h.read, h.reach, at)
			if !sameWindowsRead(got, want) {
				t.Fatalf("history %d at %s: the windows read are\n%+v\nwant, as the fold has them,\n%+v", n, at.Format(time.DateTime), got, want)
			}
			if got, want := m.Then(h.account, indexed, h.reach, h.told, at), foldThen(m, h.account, h.read, h.reach, h.told, at); !sameWas(got, want) {
				t.Fatalf("history %d at %s: Then = %+v, want %+v, as the fold has it", n, at.Format(time.DateTime), got, want)
			}
			for _, key := range []string{"5h", "7d", "7d_oi"} {
				got, known := m.RoomThen(h.account, indexed, h.told, key, at)
				want, wantKnown := foldRoomThen(m, h.account, h.read, h.told, key, at)
				if math.Abs(got-want) > 1e-12 || known != wantKnown {
					t.Fatalf("history %d at %s: RoomThen(%s) = %v, %t, want %v, %t, as the fold has it", n, at.Format(time.DateTime), key, got, known, want, wantKnown)
				}
			}
		}
	}
}

// history is a random history of readings, oldest first, of work, the
// account judged, and of side; what's judged of work besides; how far the
// readings reach; the holds the router told of; and the rand it was drawn
// from, which draws the moments it's judged at, across it and from before
// its first reading.
type history struct {
	read    []readings.Reading
	account AccountNow
	reach   Reach
	told    []Span
	from    time.Time
	length  time.Duration
	rng     *rand.Rand
}

// uses are the uses a history reads, about each reserve's cap, and at and
// past a limit.
var uses = []float64{0.1, 0.5, 0.79, 0.8, 0.85, 0.9, 0.95, 0.99, 1, 1.2}

// randomHistory is the history drawn from the seed given.
func randomHistory(seed uint64) history {
	rng := rand.New(rand.NewPCG(seed, 0x5eed))
	h := history{from: time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC), rng: rng}
	resets := map[string]time.Time{}
	at := h.from
	for range 1 + rng.IntN(60) {
		at = at.Add(time.Duration(rng.IntN(4)) * time.Duration(10+rng.IntN(80)) * time.Minute)
		key := []string{"5h", "7d", "7d_oi"}[rng.IntN(3)]
		length, _ := quota.Length(key)
		if resets[key].IsZero() {
			resets[key] = h.from.Add(length)
		}
		for !resets[key].After(at) && rng.IntN(3) > 0 {
			resets[key] = resets[key].Add(length)
		}
		r := readings.Reading{At: at, Account: "work", Key: key, Utilization: uses[rng.IntN(len(uses))], ResetsAt: resets[key]}
		if rng.IntN(8) == 0 {
			r.ResetsAt = time.Time{}
		}
		if rng.IntN(7) == 0 {
			r.Status = quota.StatusRejected
		}
		if rng.IntN(9) == 0 {
			r.Account = "side"
		}
		h.read = append(h.read, r)
	}
	h.length = at.Sub(h.from) + time.Hour
	reserve := []float64{0, 0.05, 0.1, 0.2}[rng.IntN(4)]
	h.account = AccountNow{ID: "work", TokenSet: true, Reserve: reserve, ReserveHolds: reserve > 0 && rng.IntN(4) > 0, Floor: 1 - reserve}
	h.reach = Reach{Kept: h.edge(), Seen: h.edge()}
	for range rng.IntN(4) {
		h.told = append(h.told, h.span())
	}
	return h
}

// edge is an edge of a history's reach: none, or a moment across it.
func (h history) edge() time.Time {
	if h.rng.IntN(2) == 0 {
		return time.Time{}
	}
	return h.across()
}

// span is a hold the router told of, of a kind drawn: a limit or a cap,
// naming no window or one, or a refusal; from a moment across the history,
// till a while after, or a time not known.
func (h history) span() Span {
	s := Span{Kind: []WasKind{WasLimited, WasCapped, WasRefused}[h.rng.IntN(3)], From: h.across()}
	if s.Kind != WasRefused {
		s.Windows = [][]string{nil, {"5h"}, {"7d"}, {"7d_oi"}}[h.rng.IntN(4)]
	}
	if h.rng.IntN(5) > 0 {
		s.Until = s.From.Add(time.Duration(1+h.rng.IntN(300)) * time.Minute)
	}
	return s
}

// across is a moment across the history.
func (h history) across() time.Time {
	return h.from.Add(time.Duration(h.rng.Int64N(int64(h.length))))
}

// moment is a moment a history is judged at: one of its readings', or one
// across it, from before the first.
func (h history) moment() time.Time {
	if h.rng.IntN(4) == 0 {
		return h.read[h.rng.IntN(len(h.read))].At
	}
	return h.across().Add(-30 * time.Minute)
}

// sameWindowsRead reports whether two accounts' windows read are the one.
func sameWindowsRead(a, b map[string]windowRead) bool {
	if len(a) != len(b) {
		return false
	}
	for key, w := range a {
		o, ok := b[key]
		if !ok || w.last != o.last || !sameRun(w.spent, o.spent) || !sameRun(w.capped, o.capped) {
			return false
		}
	}
	return true
}

// sameRun reports whether two runs are the one, instant for instant.
func sameRun(a, b run) bool {
	return a.shown == b.shown && a.from.Equal(b.from) && a.lifted.Equal(b.lifted)
}

// sameWas reports whether two Wases are the one, instant for instant.
func sameWas(a, b Was) bool {
	return a.Kind == b.Kind && a.Since.Equal(b.Since) && a.Until.Equal(b.Until)
}

// foldThen is Then as the fold judged it: by the windows read as foldBy has
// them, and a hold's start run back as foldRanBack has it.
func foldThen(m Moment, a AccountNow, read []readings.Reading, reach Reach, told []Span, t time.Time) Was {
	if !a.TokenSet {
		return Was{Kind: WasTokenless}
	}
	was := stoodAt(a, foldBy(m, a, read, reach, t), told, t)
	if was.Kind == WasLimited || was.Kind == WasCapped {
		was.Since = foldRanBack(m, a, read, reach, told, was.Kind, was.Since)
	}
	return was
}

// foldRanBack is ranBack as the fold had it, reading the windows back at
// each start as foldBy has them.
func foldRanBack(m Moment, a AccountNow, read []readings.Reading, reach Reach, told []Span, kind WasKind, from time.Time) time.Time {
	for !from.IsZero() {
		began := ranFrom(told, kind, m.policy.IsShared, from)
		for _, p := range shownAt(foldBy(m, a, read, reach, from), told, kind, from) {
			began = firstHeld(began, p.from)
		}
		if !began.Before(from) {
			break
		}
		from = began
	}
	return from
}

// foldRoomThen is RoomThen as the fold judged it: by the windows read as
// foldBy has them, and a model's own window by a scan of every reading.
func foldRoomThen(m Moment, a AccountNow, read []readings.Reading, told []Span, key string, t time.Time) (float64, bool) {
	if !a.TokenSet {
		return 0, true
	}
	windows := foldBy(m, a, read, Reach{}, t)
	switch was := stoodAt(a, windows, told, t); {
	case was.Kind == WasUnread:
		return 0, false
	case len(toldAt(told, WasRefused, t)) > 0, was.Kind != WasRoom && key == m.policy.Started:
		return 0, true
	}
	var w quota.Window
	var ok bool
	if m.policy.IsShared(key) {
		var read windowRead
		read, ok = windows[key]
		w = read.last
	} else {
		for _, r := range read {
			if r.At.After(t) {
				break
			}
			if r.Account == a.ID && r.Key == key {
				w, ok = r.Window(), true
			}
		}
	}
	if !ok {
		return 0, true
	}
	return score.Room(w, a.Floor, t), true
}

// folded is a window as the fold read it by a moment: the last of its
// readings; its runs; and whether one was read from each of the reach's
// edges on.
type folded struct {
	last          quota.Window
	spent, capped run
	reached       [2]bool
}

// foldBy are the windows of the account a every model shares, as the fold
// read them by t: each of the readings given in turn, those by t read again,
// those after lifting a run early while one could still lift.
func foldBy(m Moment, a AccountNow, read []readings.Reading, reach Reach, t time.Time) map[string]windowRead {
	windows := make(map[string]folded)
	for _, r := range read {
		if r.Account != a.ID || !m.policy.IsShared(r.Key) {
			continue
		}
		if !r.At.After(t) {
			windows[r.Key] = windows[r.Key].readAgain(r.Window(), r.At, reach, a.Reserve)
			continue
		}
		if !slices.ContainsFunc(slices.Collect(maps.Values(windows)), func(w folded) bool { return w.liftable(r.At) }) {
			break
		}
		if w := windows[r.Key]; w.liftable(r.At) {
			windows[r.Key] = w.readAfter(r.Window(), r.At, a.Reserve)
		}
	}
	byKey := make(map[string]windowRead, len(windows))
	for key, w := range windows {
		byKey[key] = windowRead{last: w.last, spent: w.spent, capped: w.capped}
	}
	return byKey
}

// liftable reports whether a reading at at could still lift a run of w.
func (w folded) liftable(at time.Time) bool {
	return !w.last.ResetBy(at) && (w.spent.unlifted() || w.capped.unlifted())
}

// readAfter is w read again at at, after the moment it's had by.
func (w folded) readAfter(window quota.Window, at time.Time, reserve float64) folded {
	hold := score.HoldOf(window, reserve)
	w.spent = w.spent.liftedBy(w.last, hold >= score.HeldAtLimit, at)
	w.capped = w.capped.liftedBy(w.last, hold >= score.HeldAtCap, at)
	return w
}

// liftedBy is r read again at at, after the moment it's had by.
func (r run) liftedBy(last quota.Window, holds bool, at time.Time) run {
	if r.unlifted() && !holds && !last.ResetBy(at) {
		r.lifted = at
	}
	return r
}

// unlifted reports whether r shows a hold that hasn't lifted early.
func (r run) unlifted() bool {
	return r.shown && r.lifted.IsZero()
}

// readAgain is w read again at at, by the moment it's had by.
func (w folded) readAgain(window quota.Window, at time.Time, reach Reach, reserve float64) folded {
	ranOn, begun := !w.last.ResetBy(at), at
	if w.untold(window, at, reach) {
		begun = time.Time{}
	}
	read := folded{
		last:   window,
		spent:  w.spent.joined(ranOn, score.Spent(window), begun),
		capped: w.capped.joined(ranOn, score.HeldByReserve(window, reserve), begun),
	}
	for i, edge := range reach.edges() {
		read.reached[i] = w.reached[i] || !edge.IsZero() && !at.Before(edge)
	}
	return read
}

// untold reports whether a run begun by w's reading at at began at no time
// the readings tell.
func (w folded) untold(window quota.Window, at time.Time, reach Reach) bool {
	for i, edge := range reach.edges() {
		if edge.IsZero() || at.Before(edge) || w.reached[i] {
			continue
		}
		if began, _, spanned := window.Span(); !spanned || began.Before(edge) {
			return true
		}
	}
	return false
}

// joined is r once a reading, which shows its hold, or not, joins it.
func (r run) joined(ranOn, shows bool, begun time.Time) run {
	switch {
	case !shows:
		return run{}
	case ranOn && r.shown:
		return r
	}
	return run{shown: true, from: begun}
}
