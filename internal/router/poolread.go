package router

import (
	"fmt"
	"net/http"
	"slices"

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
// and requests the router's answers hold, and no other's. In a config from
// before pools, it's every account.
type poolRead struct {
	pools *poolTable
	// pool is the id of the pool read, as pools spells it: "" in a config
	// from before pools.
	pool string
	// primary is the id of the pool's primary, as primaryOf has it: "" in a
	// config from before pools, whose accounts keep the primary the router
	// started with.
	primary string
}

// poolAsked returns what the reader of req asks to be answered for: the
// pool ?pool= names, else the one the verbs act on, as the pools stand now.
// It fails, naming the pools, where the pool named isn't configured, or none
// is named and every pool is fenced.
func (r *Router) poolAsked(req *http.Request) (poolRead, error) {
	t := r.state.pools.current()
	pool, err := t.asked(req.URL.Query().Get(PoolQuery))
	if err != nil {
		return poolRead{}, err
	}
	return r.poolReadOf(t, pool), nil
}

// poolReadOf returns the poolRead of the pool with the given id, as t has the
// pools.
func (r *Router) poolReadOf(t *poolTable, pool string) poolRead {
	return poolRead{pools: t, pool: pool, primary: t.primaryOf(pool, r.accounts)}
}

// everyAccount is a poolRead of every account the router has, as a config
// from before pools makes them: for an answer of one account, whatever pool
// it's in.
func everyAccount() poolRead {
	return poolRead{pools: newPoolTable(nil, nil, "")}
}

// holds reports whether the account with the given id is in the pool read.
func (rd poolRead) holds(id string) bool {
	return rd.pools.holds(rd.pool, id)
}

// routes reports whether work recorded as in the pool given, as a request's
// or a session's remembered pool names it, "" for none, is the pool read's:
// a request that named none is in the first.
func (rd poolRead) routes(pool string) bool {
	return rd.pools.of(pool) == rd.pool
}

// pin returns the global pin as it holds in the pool read: naming the
// pool's accounts alone, and zero where it names none of them, as it's no
// pin for a request in the pool.
func (rd poolRead) pin(global status.Pin) status.Pin {
	global.Accounts = slices.DeleteFunc(slices.Clone(global.Accounts), func(id string) bool { return !rd.holds(id) })
	if global.IsZero() {
		return status.Pin{}
	}
	return global
}

// events returns those of the events given that the pool read shows: every
// event that names no pool, and those that name it.
func (rd poolRead) events(all []status.Event) []status.Event {
	var shown []status.Event
	for _, e := range all {
		if e.Pool == "" || rd.routes(e.Pool) {
			shown = append(shown, e)
		}
	}
	return shown
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
		return "", fmt.Errorf("every pool is fenced: give the pool to read, %s", t.listed())
	}
	if p, ok := t.pools.ByID(named); ok {
		return p.ID, nil
	}
	return "", fmt.Errorf("there's no pool %q: give %s", redact.Text(named), t.listed())
}

// listed names every pool as a reader names one, as in "pool=b2n8
// (Default) or pool=q7mt (Acme)".
func (t *poolTable) listed() string {
	named := make([]string, len(t.pools))
	for i, p := range t.pools {
		named[i] = PoolQuery + "=" + prose.Clean(p.ID) + " (" + prose.Clean(p.Label) + ")"
	}
	return prose.ListOr(named)
}

// routed returns the accounts of as in the pool with the given id, in their
// order: those the router has, of those the pools put in it.
func (t *poolTable) routed(pool string, as accounts) accounts {
	return slices.DeleteFunc(slices.Clone(as), func(a account) bool { return !t.holds(pool, a.ID) })
}

// primaryOf returns the id of the primary of the pool with the given id, of
// as, the accounts the router has: the one the config makes it, where the
// router has it, else the pool's first, as the config's would be without
// it; "" where the pool has none of as, and in a config from before pools.
func (t *poolTable) primaryOf(pool string, as accounts) string {
	in := t.routed(pool, as)
	if len(t.pools) == 0 || len(in) == 0 {
		return ""
	}
	if i := slices.IndexFunc(in, func(a account) bool { return a.ID == t.primary[pool] }); i >= 0 {
		return in[i].ID
	}
	return in[0].ID
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
