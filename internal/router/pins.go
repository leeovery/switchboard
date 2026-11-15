package router

import (
	"maps"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/status"
)

// pins are each pool's pin, by the id of its pool as the pools spell it, a
// pool without one absent: "" for the one pin of a config from before pools,
// and, till fitted settles it, for the one a state file from before pools
// kept. They're never changed once made, only replaced whole, so a choice
// reads them as they stood.
type pins map[string]status.Pin

// with returns the pins with the pin of the pool with the given id set to
// pin.
func (ps pins) with(pool string, pin status.Pin) pins {
	next := maps.Clone(ps)
	if next == nil {
		next = make(pins)
	}
	next[pool] = pin
	return next
}

// without returns the pins without the pin of the pool with the given id,
// and that pin as it was, zero where the pool had none.
func (ps pins) without(pool string) (pins, status.Pin) {
	next := maps.Clone(ps)
	was := next[pool]
	delete(next, pool)
	return next, was
}

// gonePin is a pin fitted dropped whole, as the pin of the pool with the id
// pool.
type gonePin struct {
	pool string
	pin  status.Pin
}

// unpinned is the gone pin as the router tells it: routing in its pool given
// back to the router, by the router itself.
func (g gonePin) unpinned() Unpinned {
	return Unpinned{Accounts: g.pin.Accounts, By: ByRouter, Pool: g.pool}
}

// fitted returns the pins fitted to the pools as t has them, each pin it
// dropped whole, and whether that changed them: where t has pools, the pin
// from before pools is the pin of the pool it's home in, as home says,
// settled there once and for all, so it never follows its accounts into
// another pool; each pin names only the accounts its pool holds, as holds
// says, going with the last of them, so an account that leaves a pool leaves
// its pin, and doesn't come back to it with the account; and the pin of a
// pool t doesn't have goes. Where two come to be one pool's, the pin from
// before pools beside the pool's own, the later stands, the pool's own where
// they're as late. It notes in the log each account a pin leaves out, and
// each pin that goes. A closed table, which can't tell which pool any account
// is in, fits nothing: the pins stand as they are while nothing is routed,
// so none is lost before the pools come back.
func (ps pins) fitted(t *poolTable) (next pins, gone []gonePin, changed bool) {
	if t.closed != "" {
		return ps, nil, false
	}
	next = make(pins, len(ps))
	keep := func(from, into string, pin status.Pin) {
		if g, went := next.fit(t, from, into, pin); went {
			gone = append(gone, g)
		}
	}
	for _, pool := range slices.Sorted(maps.Keys(ps)) {
		if pool != "" || !t.pooled() {
			keep(pool, pool, ps[pool])
		}
	}
	if before, ok := ps[""]; ok && t.pooled() {
		keep("", t.home(before.Accounts), before)
	}
	return next, gone, !maps.EqualFunc(next, ps, samePin)
}

// fit has the pool with the id into pinned by pin, kept as the pin of the
// pool with the id from, as fitted fits it: but where t doesn't have the
// pool, or it holds none of the pin's accounts, or it has a pin as late, or
// later, as take says. It returns the pin that goes whole, reporting false
// where none does.
func (ps pins) fit(t *poolTable, from, into string, pin status.Pin) (gonePin, bool) {
	if !t.within(into) {
		logger.Warn("pin dropped: its pool is no longer configured", "pool", from, "accounts", strings.Join(pin.Accounts, ","))
		return gonePin{pool: from, pin: pin}, true
	}
	held := slices.DeleteFunc(slices.Clone(pin.Accounts), func(id string) bool { return !t.holds(into, id) })
	for _, id := range except(pin.Accounts, held) {
		logger.Warn("dropped from its pool's pin: the account is no longer in the pool", "pool", into, "account", id)
	}
	if len(held) == 0 {
		return gonePin{pool: into, pin: pin}, true
	}
	pin.Accounts = held
	return ps.take(into, pin)
}

// take has the pool with the given id pinned by pin, but where it has a pin
// as late, or later, which stands, noting in the log the one that goes. It
// returns the pin that goes, reporting false where none does.
func (ps pins) take(pool string, pin status.Pin) (gonePin, bool) {
	held, ok := ps[pool]
	if !ok {
		ps[pool] = pin
		return gonePin{}, false
	}
	stands, goes := pin, held
	if !pin.Since.After(held.Since) {
		stands, goes = held, pin
	}
	logger.Warn("pin dropped: a later one holds in its pool", "pool", pool, "accounts", strings.Join(goes.Accounts, ","))
	ps[pool] = stands
	return gonePin{pool: pool, pin: goes}, true
}

// samePin reports whether a and b are the same pin.
func samePin(a, b status.Pin) bool {
	return slices.Equal(a.Accounts, b.Accounts) && a.Since.Equal(b.Since) && a.Move == b.Move
}

// spends reports whether the pin of the pool the account with the given id
// is in, as t has the pools, names the account, which spends its reserve.
func (ps pins) spends(t *poolTable, id string) bool {
	pool, ok := t.in(id)
	return ok && ps[pool].Has(id)
}

// said is the pins as the log gives them: each pin's accounts, after the id
// of its pool and a colon, but for the pin of a config from before pools,
// in the order of the pools' ids.
func (ps pins) said() string {
	var each []string
	for _, pool := range slices.Sorted(maps.Keys(ps)) {
		accounts := strings.Join(ps[pool].Accounts, ",")
		if pool != "" {
			accounts = pool + ":" + accounts
		}
		each = append(each, accounts)
	}
	return strings.Join(each, " ")
}

// home returns the id of the pool a pin from before pools naming the
// accounts with the given ids is the pin of, where t has pools: the one
// those of them in a pool are in, as in says, where that's one, else the
// first, as where they're in none or several.
func (t *poolTable) home(ids []string) string {
	var in []string
	for _, id := range ids {
		if pool, ok := t.in(id); ok {
			in = append(in, pool)
		}
	}
	slices.Sort(in)
	if in = slices.Compact(in); len(in) == 1 {
		return in[0]
	}
	return t.first
}

// poolSaid is how the log names the pool with the given id: as given, but
// for the one pool of a config from before pools, which it leaves out.
func poolSaid(pool string) []any {
	if pool == "" {
		return nil
	}
	return []any{"pool", pool}
}
