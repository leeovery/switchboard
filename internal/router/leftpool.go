package router

import (
	"context"
	"log/slog"
	"net/http"

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

// strayed answers the routed request ex, sent on the token of the account
// with the id client, where that account isn't in the pool the request is in,
// as the pools stand now, and reports whether it did, as refuseStrayed says:
// the pool's own accounts would serve it, while what Claude Code sends that
// never reaches the router goes out on the account the session started on.
func (p *proxy) strayed(w http.ResponseWriter, ex *exchange, client string) bool {
	pools := p.pools.current()
	if pools.serves(ex.req.Pool, client) {
		return false
	}
	ex.pool, ex.status = pools.of(ex.req.Pool), leftPool.status
	p.refuseStrayed(w, pools, ex.pool, client, ex.req.Session, "id", ex.id)
	return true
}

// strayedThrough answers r, a request that isn't routed, where it carries the
// token of an account that isn't in the pool it's in, as the pools stand now,
// and reports whether it did, as refuseStrayed says: passed through, it would
// go out on another pool's subscription. One whose token is no account's goes
// as it came, as in a config from before pools.
func (p *proxy) strayedThrough(w http.ResponseWriter, r *http.Request) bool {
	pools := p.pools.current()
	if !pools.pooled() {
		return false
	}
	client, ok := p.accounts.byToken(bearer(r.Header), p.now())
	named := namedPool(r.Header)
	if !ok || pools.serves(named, client.ID) {
		return false
	}
	p.refuseStrayed(w, pools, pools.of(named), client.ID, p.provider.Session(r.Header), "method", r.Method, "path", r.URL.Path)
	return true
}

// refuseStrayed answers a request of session, in the pool with the given id,
// as pools has it, sent on the token of the account with the id client, which
// isn't in it: as leftPool says, saying why, as strayedWords does, nothing
// going upstream. The log warns of it once for each session, and notes it at
// debug after, with attrs, which name the request.
func (p *proxy) refuseStrayed(w http.ResponseWriter, pools *poolTable, pool, client, session string, attrs ...any) {
	why := "session's account has left its pool; refusing the request"
	if !pools.configured(pool) {
		why = "session's pool is no longer configured; refusing the request"
	}
	level := slog.LevelDebug
	if p.left.first(client, session) {
		level = slog.LevelWarn
	}
	logger.Log(context.Background(), level, why, append(attrs, "session", status.ShortID(session), "account", client, "pool", pool)...)
	writeError(w, leftPool.status, leftPool.kind, strayedWords(pools, pool))
}

// strayedWords says why a request in the pool with the given id, as pools has
// it, is refused, its session's token having left the pool, and what puts it
// right: the pool gone, else the account having left it, naming the pool
// where there are pools to tell apart.
func strayedWords(pools *poolTable, pool string) string {
	label := pools.shown(pool)
	switch {
	case !pools.configured(pool):
		return "switchboard: this session's pool has been removed" + resumeToCarryOn
	case label != "":
		return "switchboard: this session's account has left pool " + label + resumeToCarryOn
	}
	return "switchboard: this session's account has left its pool" + resumeToCarryOn
}
