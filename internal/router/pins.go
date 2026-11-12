package router

import (
	"maps"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/status"
)

// pins are each pool's pin, by the id of its pool as the pools spell it, a
// pool without one absent: "" for the one pin of a config from before pools,
// and for the one a state file from before pools kept, which is its pool's
// as of says. They're never changed once made, only replaced whole, so a
// choice reads them as they stood.
type pins map[string]status.Pin

// of returns the pin of the pool with the given id, as t has the pools: its
// own, else, where t has pools, the pin from before them, where it's the
// pool's, as home says; zero where the pool has none.
func (ps pins) of(t *poolTable, pool string) status.Pin {
	if p, ok := ps[pool]; ok {
		return p
	}
	if p, ok := ps[""]; ok && t.pooled() && t.home(p.Accounts) == pool {
		return p
	}
	return status.Pin{}
}

// with returns the pins with the pin of the pool with the given id, as t has
// the pools, set to pin, as settled leaves the others.
func (ps pins) with(t *poolTable, pool string, pin status.Pin) pins {
	next := ps.settled(t)
	next[pool] = pin
	return next
}

// without returns the pins without the pin of the pool with the given id, as
// t has the pools, as settled leaves the others, and that pin as it was,
// zero where the pool had none.
func (ps pins) without(t *poolTable, pool string) (pins, status.Pin) {
	next := ps.settled(t)
	was := next[pool]
	delete(next, pool)
	return next, was
}

// settled returns a copy of the pins, the pin from before pools kept, where t
// has pools, as the pin of the pool it's home in, as home says, but where
// that pool has a pin of its own, which stands.
func (ps pins) settled(t *poolTable) pins {
	next := maps.Clone(ps)
	if next == nil {
		next = make(pins)
	}
	before, ok := next[""]
	if !ok || !t.pooled() {
		return next
	}
	delete(next, "")
	if home := t.home(before.Accounts); next[home].IsZero() {
		next[home] = before
	}
	return next
}

// spends reports whether the pin of the pool the account with the given id
// is in, as t has the pools, names the account, which spends its reserve.
func (ps pins) spends(t *poolTable, id string) bool {
	pool, ok := t.in(id)
	return ok && ps.of(t, pool).Has(id)
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
// accounts with the given ids is the pin of: the one those of them in a pool
// are in, where that's one, else the first, as where they're in none or
// several.
func (t *poolTable) home(ids []string) string {
	var in []string
	for _, id := range ids {
		if pool, ok := t.poolOf[id]; ok {
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
