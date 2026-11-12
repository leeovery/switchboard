package router

import (
	"cmp"
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/leeovery/switchboard/internal/status"
)

// leftPool is the status, and the API's type of error, switchboard answers a
// request with whose session's token has left its pool: one Claude Code
// neither retries, as it does a 408, 409, 429 or 5xx, nor takes for its login
// failing, as it does a 401 or 403.
var leftPool = struct {
	status int
	kind   string
}{http.StatusBadRequest, "invalid_request_error"}

// resumeToCarryOn is what a session whose token has left its pool is told to
// do: the token Claude Code started on can't change while it runs, and
// resumed, the session starts in the pool its directory is in now, on that
// pool's primary's token.
const resumeToCarryOn = " — exit and resume it to carry on in the pool its directory is in now"

// strayed answers the routed request ex, sent by s, where its session's token
// has left the pool the request is in, as strays says of the pools as they
// stand now, and reports whether it did, as refuseStrayed says, its ledger
// line and routed line giving the reason that says so: the pool's own
// accounts would serve it, while what Claude Code sends that never reaches
// the router goes out on the account the session started on.
func (p *proxy) strayed(w http.ResponseWriter, ex *exchange, s sender) bool {
	pools := p.pools.current()
	pool, left := p.strays(pools, ex.req.Pool, s)
	if !left {
		return false
	}
	ex.pool, ex.status, ex.reason = pool, leftPool.status, status.ReasonTokenLeftPool
	p.refuseStrayed(w, pools, pool, s, ex.req.Session, "id", ex.id)
	return true
}

// strayedThrough answers r, a request that isn't routed, where it names a
// pool its session's token has left, as strays says of the pools as they
// stand now, and reports whether it did, as refuseStrayed says: passed
// through, it would go out on another pool's subscription. One that names no
// pool, or whose token is no account's, goes as it came, as in a config from
// before pools.
func (p *proxy) strayedThrough(w http.ResponseWriter, r *http.Request) bool {
	named := namedPool(r.Header)
	if named == "" {
		return false
	}
	pools := p.pools.current()
	if !pools.pooled() {
		return false
	}
	s, ok := p.accounts.sender(bearer(r.Header), p.now())
	if !ok {
		return false
	}
	pool, left := p.strays(pools, named, s)
	if !left {
		return false
	}
	p.refuseStrayed(w, pools, pool, s, p.provider.Session(r.Header), "method", r.Method, "path", r.URL.Path)
	return true
}

// strays returns the id of the pool a request naming the pool given, "" for
// none, is in, as pools has it, and reports whether the token its sender s
// sent has left it, which a session's token does only as the pools change:
// the pool isn't configured; else, of an account the config configures, it's
// in another pool; else, of one that has left the config, it was in another
// pool last, as poolIn says, where that's known. Where nothing says so, the
// token of an account that has left the config is in the pool the request
// names, as its pool's primary's is, and the request is routed there. A
// config from before pools has none to leave.
func (p *proxy) strays(pools *poolTable, named string, s sender) (pool string, left bool) {
	pool = pools.of(named)
	switch {
	case !pools.pooled():
		return pool, false
	case !pools.configured(pool):
		return pool, true
	}
	removed := s.removed
	if removed == "" && !pools.placed(s.ID) {
		removed = s.ID
	}
	if removed == "" {
		return pool, !pools.holds(pool, s.ID)
	}
	was := p.pools.poolIn(pools, removed)
	return pool, was != "" && !strings.EqualFold(was, pool)
}

// refuseStrayed answers a request of session, in the pool with the given id,
// as pools has it, sent by s, whose token has left it: as leftPool says,
// saying why, as strayedWords does, nothing going upstream. The log warns of
// it once for each session, and notes it at debug after, with attrs, which
// name the request, and the account the token is of.
func (p *proxy) refuseStrayed(w http.ResponseWriter, pools *poolTable, pool string, s sender, session string, attrs ...any) {
	why := "session's account has left its pool; refusing the request"
	if !pools.configured(pool) {
		why = "session's pool isn't configured; refusing the request"
	}
	account := cmp.Or(s.removed, s.ID)
	level := slog.LevelDebug
	if p.left.first(account, session) {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, why, append(attrs, "session", status.ShortID(session), "account", account, "pool", pool)...)
	writeError(w, leftPool.status, leftPool.kind, strayedWords(pools, pool))
}

// strayedWords says why a request in the pool with the given id, as pools has
// it, is refused, its session's token having left the pool, and what puts it
// right: the pool not configured, else the account having left it, naming the
// pool where there are pools to tell apart.
func strayedWords(pools *poolTable, pool string) string {
	label := pools.shown(pool)
	switch {
	case !pools.configured(pool):
		return "switchboard: this session's pool isn't configured" + resumeToCarryOn
	case label != "":
		return "switchboard: this session's account has left pool " + label + resumeToCarryOn
	}
	return "switchboard: this session's account has left its pool" + resumeToCarryOn
}
