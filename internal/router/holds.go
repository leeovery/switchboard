package router

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// capped is how an account stood at its reserve as last looked at: the
// windows its reserve held back, none when it held back none, and the id of
// its cap's event while the cap holds, 0 where none was told, as for an
// account at its cap as it's first looked at.
type capped struct {
	windows []string
	event   int
}

// refusedUntil is a request the upstream refused on an account, whose
// refusal holds until until.
type refusedUntil struct {
	request string
	until   time.Time
}

// awaiting is what forced a move, while the event the move is to be counted
// in is yet to be told: a limit, by its identity, or the cap or the refusal
// of the account the move left, which held back the requests of family.
type awaiting struct {
	held   Hold
	limit  int
	family string
}

// reached keeps a limit an account reached at now, by the limit's identity,
// whatever order its news comes in, as the news of a limit can come after
// the news of it reached again: one whose event is kept, and can change,
// joins it, and one newer than every limit heard of the account is news of
// its own, which counts the moves it forced that came first. One heard of
// before, whose event is no longer kept, or can't change, is dropped.
func (r *recent) reached(e LimitReached, now time.Time) {
	if h, ok := r.limitEvent(e.Limit, now); ok {
		r.join(h, e.Windows, e.Until)
		return
	}
	if e.Limit <= r.limits[e.Account] {
		return
	}
	r.limits[e.Account] = e.Limit
	h := r.keep(happening{Kind: status.EventLimit, Account: e.Account, Windows: slices.Clone(e.Windows), Until: e.Until, Limit: e.Limit}, now)
	r.countWaiting(h)
}

// refused keeps a refusal of an account at now: one renewed while the event
// of the account's refusal with its status and family holds, and can change,
// joins it, holding as long as the latest it's told of; any other is news of
// its own. Either way, it counts the moves it forced that came first.
func (r *recent) refused(e Refused, now time.Time) {
	refusal := refusedUntil{request: e.Request, until: e.Until}
	holds := refusalHolding(e.Account, e.Family, now)
	h, renewed := r.find(func(h *happening) bool { return holds(h) && h.Status == e.Status })
	if renewed {
		h.requests = append(h.requests, refusal)
		r.join(h, nil, e.Until)
	} else {
		h = r.keep(happening{Kind: status.EventRefused, Account: e.Account, Until: e.Until, Status: e.Status, Family: e.Family, requests: []refusedUntil{refusal}}, now)
	}
	r.countWaiting(h)
}

// lifted ends early the refusals a lifting tells of, kept, in force at now
// and able to change, as lift says.
func (r *recent) lifted(e RefusalLifted, now time.Time) {
	holds := refusalHolding(e.Account, e.Family, now)
	for i := range r.kept {
		if h := &r.kept[i]; holds(h) {
			h.lift(e.Request, now)
		}
	}
}

// lift ends early, at now, the refusal of every request its event joined, for
// a refusal of the account's token; else the refusal of the request with the
// given id, if it joined it, the event then holding as long as the latest of
// the others it joined, if any holds longer.
func (h *happening) lift(request string, now time.Time) {
	if h.Family == "" {
		h.requests = nil
	}
	h.requests = slices.DeleteFunc(h.requests, func(q refusedUntil) bool { return q.request == request })
	until := now
	for _, q := range h.requests {
		until = score.Later(until, q.until)
	}
	if !until.Equal(h.Until) {
		h.Until = until
		h.mark()
	}
}

// capping keeps what the accounts' reserves, as they stand at now, call for
// since they were last looked at: a cap reached by an account whose reserve
// held back none of its windows before, as news, counting the moves it forced
// that came first; and a cap reached again while it holds, in other windows
// perhaps, joining its event. A cap holds while the account's reserve holds
// back a window of it, and the global pin doesn't name it, which spends its
// reserve. The first look at an account read calls for none, as there's
// nothing yet to compare it with. r.mu must be held.
func (r *recent) capping(accounts []status.Account, pin status.Pin, now time.Time) {
	for _, a := range accounts {
		was, seen := r.caps[a.ID]
		if len(a.Windows) == 0 {
			delete(r.caps, a.ID)
			continue
		}
		is := capped{windows: a.AtReserve}
		if pin.Has(a.ID) {
			is.windows = nil
		}
		switch {
		case len(is.windows) == 0:
		case len(was.windows) == 0 && seen:
			is.event = r.cap(a, is.windows, now)
		default:
			is.event = was.event
			if h, ok := r.capEvent(a.ID, now); ok {
				r.join(h, is.windows, lastReset(a, is.windows))
			}
		}
		r.caps[a.ID] = is
	}
}

// cap keeps the cap the account reached at now in windows, its reserve as it
// came, counting the moves it forced that came first, and returns its
// event's id. r.mu must be held.
func (r *recent) cap(a status.Account, windows []string, now time.Time) int {
	h := r.keep(happening{Kind: status.EventCap, Account: a.ID, Windows: slices.Clone(windows), Until: lastReset(a, windows), Reserve: a.Reserve}, now)
	r.countWaiting(h)
	return h.ID
}

// lastReset returns when the last of the account's windows with the given
// keys resets, zero when none's reset is known.
func lastReset(a status.Account, keys []string) time.Time {
	var last time.Time
	for _, key := range keys {
		if w, ok := a.Window(key); ok {
			last = score.Later(last, w.ResetsAt)
		}
	}
	return last
}

// join has the event of what holds an account back take in that reached
// again while it holds, in more windows perhaps, and till later, marking it
// where that changed it.
func (r *recent) join(h *happening, windows []string, until time.Time) {
	var changed bool
	if h.Windows, h.Until, changed = widen(h.Windows, h.Until, windows, until); changed {
		h.mark()
	}
}

// moved keeps a session's move at now, counting it among the sessions what
// forced it moved, in the event of that, which the move then names: the
// limit, cap or refusal that held its request back on the account it left.
// Where that event is yet to be told, the move waits for it, as waitFor says.
func (r *recent) moved(e Moved, now time.Time) {
	move := happening{Kind: status.EventMoved, Session: bounded(e.Session), Model: bounded(e.Model), From: e.From, To: e.To, Reason: e.Reason}
	if h, ok := r.forcing(e, now); ok {
		count(h, &move)
	} else {
		move.waits = r.waitFor(e)
	}
	r.keep(move, now)
}

// forcing returns the kept event of what forced a move, while it can change:
// the limit's, by its identity; the cap's of the account the move left, while
// it holds; or the refusal's of that account, while it holds back the move's
// model family, its token's before the family's. It reports false for a move
// by choice, and where that event is yet to be told, or is no longer kept.
// r.mu must be held.
func (r *recent) forcing(e Moved, now time.Time) (*happening, bool) {
	switch e.Held {
	case HeldByLimit:
		return r.limitEvent(e.Limit, now)
	case HeldByReserve:
		return r.capEvent(e.From, now)
	case HeldByRefusal:
		if h, ok := r.find(refusalHolding(e.From, "", now)); ok {
			return h, true
		}
		return r.find(refusalHolding(e.From, r.state.family(e.Model), now))
	}
	return nil, false
}

// waitFor returns what a move waits for, where the event of what forced it is
// yet to be told: a limit newer than every limit heard of the account it
// left, by its identity, which tells of it once it's heard of; the cap of
// that account, which a look at the accounts finds; or its refusal, holding
// back the move's model family, which tells of it as it comes. It returns
// zero for a move by choice, and for a limit heard of before, whose event is
// no longer kept.
func (r *recent) waitFor(e Moved) awaiting {
	switch e.Held {
	case HeldByLimit:
		if e.Limit > r.limits[e.From] {
			return awaiting{held: HeldByLimit, limit: e.Limit}
		}
	case HeldByReserve, HeldByRefusal:
		return awaiting{held: e.Held, family: r.state.family(e.Model)}
	}
	return awaiting{}
}

// heldBy is what each kind of event that counts the moves it forced holds an
// account back by.
var heldBy = map[string]Hold{status.EventLimit: HeldByLimit, status.EventCap: HeldByReserve, status.EventRefused: HeldByRefusal}

// waitsFor reports whether the move waits for h, newly told or joined, to be
// counted in: the limit's with the identity it waits for; or the cap's or
// the refusal's of the account it left, as it waits for, the refusal holding
// back its model family, every family for the token's.
func (move *happening) waitsFor(h *happening) bool {
	switch w := move.waits; {
	case w.held == 0 || w.held != heldBy[h.Kind]:
		return false
	case w.held == HeldByLimit:
		return w.limit == h.Limit
	default:
		return move.From == h.Account && (h.Family == "" || h.Family == w.family)
	}
}

// countWaiting counts in h, the event of what forced them, newly told or
// joined, the moves kept waiting for it. r.mu must be held.
func (r *recent) countWaiting(h *happening) {
	for i := range r.kept {
		if move := &r.kept[i]; move.waitsFor(h) {
			count(h, move)
			move.mark()
		}
	}
}

// unawaited has the moves heard up to the id heard that still wait for a cap
// or a refusal wait no more: a look reading the accounts after them told of
// the cap that forced them, if it could be told of, and a refusal is told of
// as it comes. r.mu must be held.
func (r *recent) unawaited(heard int) {
	for i := range r.kept {
		if move := &r.kept[i]; move.ID <= heard && (move.waits.held == HeldByReserve || move.waits.held == HeldByRefusal) {
			move.waits = awaiting{}
		}
	}
}

// count counts a move in h, the event of what forced it, which the move
// then names, a limit's as its limit too.
func count(h, move *happening) {
	if h.moves.add(move.Session, move.To) {
		h.Count, h.To = h.moves.moved()
		h.mark()
	}
	move.ForcedBy, move.waits = h.ID, awaiting{}
	if h.Kind == status.EventLimit {
		move.Limit = h.ID
	}
}

// limitEvent returns the kept event of the limit with the given identity,
// while it can change at now, reporting false when there's none. r.mu must
// be held.
func (r *recent) limitEvent(limit int, now time.Time) (*happening, bool) {
	if limit == 0 {
		return nil, false
	}
	return r.find(func(h *happening) bool {
		return h.Kind == status.EventLimit && h.Limit == limit && h.changes(now)
	})
}

// capEvent returns the kept event of the account's cap, while it holds, and
// can change at now, reporting false when there's none. r.mu must be held.
func (r *recent) capEvent(account string, now time.Time) (*happening, bool) {
	h, ok := r.byID(r.caps[account].event)
	return h, ok && h.changes(now)
}

// refusalHolding returns what picks out the events of the account's refusals
// that hold back the requests of family at now, every request when that's
// "", and can change.
func refusalHolding(account, family string, now time.Time) func(h *happening) bool {
	return func(h *happening) bool {
		return h.Kind == status.EventRefused && h.Account == account && h.Family == family && h.Until.After(now) && h.changes(now)
	}
}
