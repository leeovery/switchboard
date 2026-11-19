package status

import (
	"hash/fnv"
	"math"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
)

// ShownPool is the pool a dashboard shows, as the config stood as it was
// last read: its id and label, its id where nothing of the label shows, as
// prose.LabelOr has it; its directories, as the config writes them,
// none where it's unfenced; how many accounts the config put in it; and how
// many pools there are, as no screen names a pool till there's a second.
// One with no id is no pool: a config without pools, or one of several,
// every one fenced, with none named. Routed is set once the router's
// document of it is read, as AsRead has it, and Routing is how many
// accounts that document routes in it. Refusing is how the router answered
// where it refuses to read the pool, as Refused has it: nil where it
// doesn't. Turn is how far the pool's hue is turned from the badge's, as
// PoolTurn gives it.
type ShownPool struct {
	ID, Label string
	Dirs      []string
	Accounts  int
	Pools     int
	Routed    bool
	Routing   int
	Refusing  *RouterAnswer
	Turn      float64
}

// RouterAnswer is how the router answered its health check: its health, and
// the restart it has due, none where none is.
type RouterAnswer struct {
	Health  Health
	Restart Restart
}

// Moment is the moment of a router answering so, routing nothing in the
// pool read, for a status line to say its state as of a pool it knows.
func (a RouterAnswer) Moment() Moment {
	return Moment{Routed: true, Health: a.Health, Restart: a.Restart}
}

// PoolShown is the pool of pools with the id given, ignoring case, as a
// dashboard shows it, its accounts those PoolAccounts gives of accounts: no
// pool, but for how many there are, where none of pools has the id.
func PoolShown(pools config.Pools, accounts config.Accounts, id string) ShownPool {
	p, ok := pools.ByID(id)
	if !ok {
		return ShownPool{Pools: len(pools)}
	}
	return ShownPool{ID: p.ID, Label: prose.LabelOr(p.Label, p.ID), Dirs: slices.Clone(p.WrittenDirs), Accounts: len(PoolAccounts(pools, accounts, p.ID)), Pools: len(pools), Turn: PoolTurn(pools.IDs(), p.ID)}
}

// PoolTurn is how far, in degrees, the hue of the pool with the id given,
// ignoring case, is turned from the badge's round the colour wheel, among
// pools with the ids given, in the config's order: the turn its id hashes
// to, unless that's within poolTurnApart of a pool's before it, when it
// takes the nearest turn that isn't, as spreadTurn has it. So a pool's
// colour follows from its id, holds as pools are added after it, and stands
// apart from the others', always for three pools, mostly for four, as far as
// the band allows for more; and none comes within poolTurnClear of the
// badge's, either way, so every pool's reads against the badge.
func PoolTurn(ids []string, id string) float64 {
	var taken []float64
	for _, each := range ids {
		turn := spreadTurn(hashedTurn(each), taken)
		if strings.EqualFold(each, id) {
			return turn
		}
		taken = append(taken, turn)
	}
	return hashedTurn(id)
}

// The band a pool's turn lies in, from poolTurnClear past the badge's hue to
// poolTurnClear short of it, and how far apart pools' turns are kept.
const (
	poolTurnClear = 75
	poolTurnBand  = 360 - 2*poolTurnClear
	poolTurnApart = 50
)

// hashedTurn is the turn a pool's id hashes to, ignoring case, in the band.
func hashedTurn(id string) float64 {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(id)))
	return poolTurnClear + float64(h.Sum32()%(poolTurnBand*10))/10
}

// spreadTurn is turn where it's poolTurnApart from every one taken; else the
// nearest turn in the band that is, which lies poolTurnApart from one taken
// or at an end of the band; else, where none is, the turn in the band
// farthest from them, at an end or midway between two.
func spreadTurn(turn float64, taken []float64) float64 {
	nearest := func(c float64) float64 {
		near := math.Inf(1)
		for _, t := range taken {
			near = min(near, math.Abs(c-t))
		}
		return near
	}
	// The slack lets a turn set exactly poolTurnApart from one taken count
	// as apart, as the subtraction can land a hair short.
	const slack = 1e-9
	if nearest(turn) >= poolTurnApart-slack {
		return turn
	}
	lo, hi := float64(poolTurnClear), float64(poolTurnClear+poolTurnBand)
	candidates := []float64{lo, hi}
	for _, t := range taken {
		candidates = append(candidates, t-poolTurnApart, t+poolTurnApart)
	}
	best, found := 0.0, false
	for _, c := range candidates {
		if c < lo || c > hi || nearest(c) < poolTurnApart-slack {
			continue
		}
		if !found || math.Abs(c-turn) < math.Abs(best-turn) || math.Abs(c-turn) == math.Abs(best-turn) && c < best {
			best, found = c, true
		}
	}
	if found {
		return best
	}
	sorted := slices.Sorted(slices.Values(taken))
	points := []float64{lo, hi}
	for i := 1; i < len(sorted); i++ {
		points = append(points, (sorted[i-1]+sorted[i])/2)
	}
	best = points[0]
	for _, c := range points[1:] {
		if far, was := nearest(c), nearest(best); far > was || far == was && math.Abs(c-turn) < math.Abs(best-turn) {
			best = c
		}
	}
	return best
}

// PoolAccounts are the accounts of accounts a dashboard shows of the pool of
// pools with the id given, ignoring case, as its Accounts tab, History and
// Graphs list them: every one in a config without pools; those in the pool;
// and none where none of pools has the id, as where every pool is fenced
// and none is shown. So no screen lists one pool's accounts in another's.
func PoolAccounts(pools config.Pools, accounts config.Accounts, id string) config.Accounts {
	if len(pools) == 0 {
		return accounts
	}
	p, ok := pools.ByID(id)
	if !ok {
		return nil
	}
	return accounts.InPool(p.ID)
}

// NamesThePool reports whether a screen, or a verb's text, names the pool it
// tells of, of pools as many as given: once there's a second, as with one
// switchboard says what it said before pools.
func NamesThePool(pools int) bool {
	return pools > 1
}

// Named is the pool's label, as a screen names the pool it shows once
// there's a second, as NamesThePool has it: "" where there's one or none, or
// no pool is shown. PoolShown has put the id in place of a label that shows
// nothing, so Named needn't clean it: a session's page asks it at every
// scroll, and the label passed to prose.Clean, which keeps what it's given,
// would have the frame's contents escape to the heap at each.
func (p ShownPool) Named() string {
	if p.ID == "" || !p.Turns() {
		return ""
	}
	return p.Label
}

// Turns reports whether there's another pool to turn to, as the key line
// lists P: where there's a second, as NamesThePool has it.
func (p ShownPool) Turns() bool {
	return NamesThePool(p.Pools)
}

// Pooled reports whether there are pools at all, so P asks for the next
// however many there were as the pool was chosen: one added since, to a
// config of one, shows.
func (p ShownPool) Pooled() bool {
	return p.Pools > 0
}

// Empty reports whether the pool shown has no accounts to route, so a
// session started in it doesn't start, and the dashboard draws a page saying
// so in place of its views: none the router routes in it, once its document
// of the pool is read, as AsRead has it; else none the config puts in it.
func (p ShownPool) Empty() bool {
	if p.ID == "" {
		return false
	}
	if p.Routed {
		return p.Routing == 0
	}
	return p.Accounts == 0
}

// Waiting reports whether the pool shown is empty only as the router hasn't
// taken up the accounts the config puts in it, as it does as it restarts.
func (p ShownPool) Waiting() bool {
	return p.Empty() && p.Accounts > 0
}

// AsRead is the pool shown as the moment mo of the router's document has it,
// where the document is of the pool shown, by its id, ignoring case: the
// accounts it routes there, so an account the router takes up, or the last
// it lets go, shows at the next read. A probed document, whose accounts are
// the config's, or one of another pool or of none, as a router that can't
// tell its pools answers, leaves it as the config has it.
func (p ShownPool) AsRead(mo Moment) ShownPool {
	if mo.Routed && p.ID != "" && config.SamePool(mo.PoolID, p.ID) {
		p.Routed, p.Routing = true, len(mo.Accounts)
	}
	return p
}

// Refused is the pool shown as the router has it where it refuses a read
// of it, not knowing it, as a pool added since it started, answering as
// answer says: routing none in it, as AsRead has it of a document of it
// with none, till it takes the pool up as it restarts.
func (p ShownPool) Refused(answer RouterAnswer) ShownPool {
	if p.ID != "" {
		p.Routed, p.Routing, p.Refusing = true, 0, &answer
	}
	return p
}

// AccountsChanged reports whether d holds other accounts, by their ids, than
// before, the document read before it, where one was read: as an account the
// router took up as it restarted, or a config changed.
func (d Document) AccountsChanged(before Document) bool {
	if before.GeneratedAt.IsZero() {
		return false
	}
	return !slices.EqualFunc(before.Accounts, d.Accounts, func(a, b Account) bool { return a.ID == b.ID })
}

// Placement is how a config places its accounts in its pools, as a
// dashboard compares it as the config changes, starting the pool it shows
// afresh where it's changed: every pool's id, label and directories, in
// order, and every account, with its label, plan, primary mark, reserve and
// pool, as the router's events of every pool are placed by them.
type Placement struct {
	pools    config.Pools
	accounts config.Accounts
}

// Placed is the placement of accounts in pools.
func Placed(pools config.Pools, accounts config.Accounts) Placement {
	return Placement{pools: pools, accounts: accounts}
}

// Same reports whether p and q place alike: as many pools, each with the
// same id, label and directories, and the same accounts, each alike in
// everything the config gives it.
func (p Placement) Same(q Placement) bool {
	return slices.Equal(p.accounts, q.accounts) && slices.EqualFunc(p.pools, q.pools, func(a, b config.Pool) bool {
		return a.ID == b.ID && a.Label == b.Label && slices.Equal(a.Dirs, b.Dirs) && slices.Equal(a.WrittenDirs, b.WrittenDirs)
	})
}

// Fenced reports whether the pool shown is fenced to directories: else it's
// the pool of every session started outside every fence.
func (p ShownPool) Fenced() bool {
	return len(p.Dirs) > 0
}

// PoolAfter returns the pool P turns to from the one with the id given,
// ignoring case: the next of pools, in the config's order, the first after
// the last; else, where none of them has the id, as where every pool is
// fenced and none is shown, the first. It reports false where there are no
// pools.
func PoolAfter(pools config.Pools, id string) (config.Pool, bool) {
	if len(pools) == 0 {
		return config.Pool{}, false
	}
	i := slices.IndexFunc(pools, func(p config.Pool) bool { return config.SamePool(p.ID, id) })
	return pools[(i+1)%len(pools)], true
}

// PoolAgain returns the pool of pools with the id given, ignoring case, as
// the dashboard reads the pool it shows again as the config changes; else,
// where it's gone, the one P would turn to from it, as PoolAfter has it. It
// reports false where there are no pools.
func PoolAgain(pools config.Pools, id string) (config.Pool, bool) {
	if p, ok := pools.ByID(id); ok {
		return p, true
	}
	return PoolAfter(pools, id)
}
