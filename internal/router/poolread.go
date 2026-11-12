package router

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

// PoolQuery is the query parameter a reader of the control API names the
// pool it reads with, by its id: GET /status, /sessions, /sessions/{id},
// /history and /stream take it, and so do POST and DELETE /pin and POST
// /refresh, which answer with the status document.
const PoolQuery = "pool"

// RoutedPool is a pool the router routes for, as GET /health reports it.
type RoutedPool struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	// Dirs are the directories it's fenced to, absolute and clean, and
	// WrittenDirs the same as the config writes them: none for the unfenced
	// pool.
	Dirs        []string `json:"dirs,omitempty"`
	WrittenDirs []string `json:"written_dirs,omitempty"`
	// Accounts are the ids of its accounts the router has, in the order
	// configured: one the config has added, which waits for the router's
	// restart, isn't among them. Primary is its primary among them: "" where
	// it has none.
	Accounts []string `json:"accounts"`
	Primary  string   `json:"primary,omitempty"`
}

// poolRead is what a reader of the control API is answered for: one pool, as
// one reading of the pools has it, whose accounts, primary, pin, sessions
// and requests the router's answers hold, and no other's; or, for an answer
// of an account whatever pool it's in, every account. In a config from
// before pools, it's every account, and of a closed table none.
type poolRead struct {
	pools *poolTable
	// pool is the id of the pool read, as pools spells it: "" in a config
	// from before pools, and of every account.
	pool string
	// every is set where every account is read, whatever pool it's in.
	every bool
	// primaries are the ids of the primary of each pool, as primaryOf has
	// them, which the accounts read are marked by: nil in a config from
	// before pools, whose accounts keep the primary the router started with.
	primaries map[string]bool
}

// poolAsked returns what the reader of req asks to be answered for: the
// pool ?pool= names, else the one the verbs act on, as the pools stand now.
// It fails, naming the pools, where the pool named isn't configured, or none
// is named and every pool is fenced.
func (r *Router) poolAsked(req *http.Request) (poolRead, error) {
	t := r.state.pools.current()
	pool, err := t.asked(poolNamed(req))
	if err != nil {
		return poolRead{}, err
	}
	return r.state.poolReadOf(t, pool), nil
}

// poolNamed returns the id of the pool req names, as ?pool= gives it: ""
// where it names none, or gives the parameter no value.
func poolNamed(req *http.Request) string {
	return req.URL.Query().Get(PoolQuery)
}

// poolReadOf returns the poolRead of the pool with the given id, as t has the
// pools.
func (s *state) poolReadOf(t *poolTable, pool string) poolRead {
	return poolRead{pools: t, pool: pool, primaries: t.primaries(s.accounts)}
}

// everyAccount is a poolRead of every account the router has, whatever pool
// it's in, as the pools stand now: for an answer of one account, and for
// what the router judges of every account at once.
func (s *state) everyAccount() poolRead {
	t := s.pools.current()
	return poolRead{pools: t, every: true, primaries: t.primaries(s.accounts)}
}

// holds reports whether the account with the given id is read.
func (rd poolRead) holds(id string) bool {
	return rd.every || rd.pools.holds(rd.pool, id)
}

// primary reports whether the account with the given id is its pool's
// primary, as the pools stand, reporting false where the pools say nothing
// of it, in a config from before pools.
func (rd poolRead) primary(id string) (primary, known bool) {
	if rd.primaries == nil {
		return false, false
	}
	return rd.primaries[id], true
}

// whole reports whether the read holds all the router's work, every pool's:
// a read of every account's does, and so does one of a config from before
// pools, every account in its one pool; one of a closed table never does,
// as no account is in any pool.
func (rd poolRead) whole() bool {
	return rd.every || !rd.pools.pooled() && rd.pools.closed == ""
}

// routes reports whether work recorded as in the pool given, as a request's
// or a session's remembered pool names it, "" for none, is the pool read's:
// a request that named none is in the first. A whole read has every pool's
// work, and one of a closed table none.
func (rd poolRead) routes(pool string) bool {
	return rd.whole() || rd.pools.pooled() && rd.pools.of(pool) == rd.pool
}

// pin returns the pin of the pool read, of the pools' pins given, as fitted
// keeps each to its own pool's accounts. A read of every account's names
// those each pool's pin names among its own accounts, as spends says, of the
// accounts given, in their order.
func (rd poolRead) pin(ps pins, as accounts) status.Pin {
	if rd.every {
		return status.Pin{Accounts: as.held(func(id string) bool { return ps.spends(rd.pools, id) }).configured().IDs()}
	}
	return ps[rd.pool]
}

// demandOf returns the id of the pool whose demand the read carries: the
// pool read, the one there is in a config from before pools included;
// reporting false for a read of every account's where there are pools, as
// no pool's demand is another's.
func (rd poolRead) demandOf() (string, bool) {
	return rd.pool, !rd.every || !rd.pools.pooled()
}

// events returns those of the events given that the pool read shows: one
// that names a pool, where it's the pool read; one that names an account in
// a pool, where it's in the pool read now, as its windows go; and every one
// that names neither, as the router's own, its restarts and its exports do,
// or names only a pool or an account that has left the config. A whole read
// shows every event, and one of a closed table the router's own alone.
func (rd poolRead) events(all []status.Event) []status.Event {
	if rd.whole() {
		return all
	}
	var shown []status.Event
	for _, e := range all {
		if rd.shows(e) {
			shown = append(shown, e)
		}
	}
	return shown
}

// shows reports whether the pool read shows the event e, as events says.
func (rd poolRead) shows(e status.Event) bool {
	if rd.pools.closed != "" {
		return e.Pool == "" && befell(e) == ""
	}
	if _, in := rd.pools.pools.ByID(e.Pool); in {
		return rd.routes(e.Pool)
	}
	id := befell(e)
	if _, in := rd.pools.poolOf[id]; !in {
		return true
	}
	return rd.holds(id)
}

// befell returns the account the event e befell, as a pool read shows it by:
// the one it names; else the first a pool's pin names; else the one a
// session moved to, where its session stays, or from, where it moved to
// none; "" for none.
func befell(e status.Event) string {
	var pinned string
	if len(e.Accounts) > 0 {
		pinned = e.Accounts[0]
	}
	return cmp.Or(e.Account, pinned, e.To, e.From)
}

// named returns the id and the label of the pool read: both "" in a config
// from before pools.
func (rd poolRead) named() (id, label string) {
	p, ok := rd.pools.pools.ByID(rd.pool)
	if !ok {
		return "", ""
	}
	return p.ID, p.Label
}

// asked returns the id of the pool a reader naming the one given, "" for
// none, is answered for: the one named, else the one the verbs act on, as
// config.Pools' Implicit has it; "" in a config from before pools, whatever
// is named. It fails, naming the pools, where the pool named isn't
// configured, or none is named and there's none the verbs act on, as where
// every pool is fenced.
func (t *poolTable) asked(named string) (string, error) {
	if len(t.pools) == 0 {
		return "", nil
	}
	if named == "" {
		if p, ok := t.pools.Implicit(); ok {
			return p.ID, nil
		}
		return "", NeedPool("", t.pools)
	}
	if p, ok := t.pools.ByID(named); ok {
		return p.ID, nil
	}
	return "", NeedPool(named, t.pools)
}

// PoolNamed is a pool as a refusal for want of one names it.
type PoolNamed struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// PoolNeeded is why a read of a pool can't be answered without the pool
// named, as the router refuses one, and as a document built by probing
// can't be built: none is named, and there's none the verbs act on, as where
// every pool is fenced; or the one named, Named, isn't configured. Pools are
// those that are, to name one of. It's never a router that's down, nor one
// to read in its place.
type PoolNeeded struct {
	Named string
	Pools []PoolNamed
}

// NeedPool is the PoolNeeded of a read naming the pool given, "" for none,
// of pools.
func NeedPool(named string, pools config.Pools) PoolNeeded {
	need := PoolNeeded{Named: named, Pools: make([]PoolNamed, len(pools))}
	for i, p := range pools {
		need.Pools[i] = PoolNamed{ID: p.ID, Label: p.Label}
	}
	return need
}

func (e PoolNeeded) Error() string {
	named := make([]string, len(e.Pools))
	for i, p := range e.Pools {
		named[i] = prose.Clean(p.ID) + " (" + prose.Clean(p.Label) + ")"
	}
	if e.Named == "" {
		return "every pool is fenced: name a pool, " + prose.ListOr(named)
	}
	return fmt.Sprintf("there's no pool %q: name %s", redact.Text(e.Named), prose.ListOr(named))
}

// poolOfAll returns the id of the pool the accounts with the given ids are
// in, of as, the accounts the router has, passing over those it doesn't
// have, which pinning them refuses: "" in a config from before pools, and
// where it has none of them. It fails, naming each account by its id and its
// label with its pool, where they're in two pools, or one is in none, as one
// the config has dropped is till the router restarts.
func (t *poolTable) poolOfAll(ids []string, as accounts) (string, error) {
	if len(t.pools) == 0 {
		return "", nil
	}
	var pool string
	var each []string
	for _, id := range ids {
		a, ok := as.byID(id)
		if !ok {
			continue
		}
		in, pooled := t.in(id)
		if !pooled {
			return "", fmt.Errorf("%s is in no pool, as the config no longer has it, so it can't be pinned", accountSaid(a.Account))
		}
		each = append(each, accountSaid(a.Account)+" is in pool "+t.label(in))
		switch {
		case pool == "":
			pool = in
		case in != pool:
			return "", fmt.Errorf("pin accounts of one pool: %s", strings.Join(slices.Compact(each), ", "))
		}
	}
	return pool, nil
}

// pinStands fails, saying why, unless a pin of the accounts with the given
// ids, of as, stands whole as the pin of the pool with the given id, as t
// has the pools and fitted fits it: where t is closed, as shut says; where
// they're in another pool, as when the pools have moved them since the pool
// was read, or in several, or one is in none.
func (t *poolTable) pinStands(pool string, ids []string, as accounts) error {
	if err := t.shut(); err != nil {
		return err
	}
	in, err := t.poolOfAll(ids, as)
	if err != nil {
		return err
	}
	if in != pool {
		return errors.New("the accounts to pin have just moved pool, as the config changed: pin them again")
	}
	return nil
}

// shut fails, saying why, where t is closed: nothing is routed, as no account
// is in any pool, so no pin can be set, nor a pool's cleared. A session's own
// can still be cleared, as that needs no pool.
func (t *poolTable) shut() error {
	if t.closed == "" {
		return nil
	}
	return fmt.Errorf("%s: no pin can be set, nor a pool's cleared, till it can", t.closed)
}

// sessionMay fails, saying which pool each is in, unless a session in the
// pool given, as its request named it, "" for none, can be pinned to the
// account a: one in the session's pool, as every account is in a config
// from before pools; none, where t is closed, as shut says.
func (t *poolTable) sessionMay(pool string, a account) error {
	if err := t.shut(); err != nil {
		return err
	}
	in := t.of(pool)
	switch at, placed := t.in(a.ID); {
	case !placed:
		return fmt.Errorf("%s is in no pool, as the config no longer has it, so it can't be pinned", accountSaid(a.Account))
	case at != in:
		return fmt.Errorf("%s is in pool %s; this session is in pool %s", accountSaid(a.Account), t.label(at), t.label(in))
	}
	return nil
}

// label returns the label of the pool with the given id, else its id.
func (t *poolTable) label(pool string) string {
	if p, ok := t.pools.ByID(pool); ok {
		return prose.Clean(p.Label)
	}
	return prose.Clean(pool)
}

// accountSaid names the account a by its id, and its label beside where
// that differs, as in "b6s7 (Work)": one labelled by its id is named once.
func accountSaid(a config.Account) string {
	said := prose.Clean(a.ID)
	if label := status.ConfiguredName(a).Said(); !strings.EqualFold(label, said) {
		said += " (" + label + ")"
	}
	return said
}

// routed returns the accounts of as in the pool with the given id, in their
// order: those the router has, of those the pools put in it.
func (t *poolTable) routed(pool string, as accounts) accounts {
	return as.held(func(id string) bool { return t.holds(pool, id) })
}

// primaryOf returns the id of the primary of the pool with the given id, of
// as, the accounts the router has: the one the config makes it, where the
// router has it, else the pool's first, as config.PrimaryIndex has it; ""
// where the pool has none of as, and in a config from before pools.
func (t *poolTable) primaryOf(pool string, as accounts) string {
	in := t.routed(pool, as)
	if len(t.pools) == 0 || len(in) == 0 {
		return ""
	}
	return in[config.PrimaryIndex(in, func(a account) bool { return a.ID == t.primary[pool] })].ID
}

// primaries returns the ids of each pool's primary, of as, the accounts the
// router has, as primaryOf has them: nil in a config from before pools.
func (t *poolTable) primaries(as accounts) map[string]bool {
	if len(t.pools) == 0 {
		return nil
	}
	primaries := make(map[string]bool, len(t.pools))
	for _, p := range t.pools {
		if id := t.primaryOf(p.ID, as); id != "" {
			primaries[id] = true
		}
	}
	return primaries
}

// reported returns the pools as GET /health reports them, in the config's
// order, of as, the accounts the router has: none in a config from before
// pools.
func (t *poolTable) reported(as accounts) []RoutedPool {
	var pools []RoutedPool
	for _, p := range t.pools {
		pools = append(pools, RoutedPool{
			ID:          p.ID,
			Label:       p.Label,
			Dirs:        slices.Clone(p.Dirs),
			WrittenDirs: slices.Clone(p.WrittenDirs),
			Accounts:    t.routed(p.ID, as).configured().IDs(),
			Primary:     t.primaryOf(p.ID, as),
		})
	}
	return pools
}
