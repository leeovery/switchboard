package router

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// The pools of the pools tests: work, the primary, and side in the first,
// Default; personal in Acme, fenced to a directory.
const (
	defaultPool = "b2n8"
	acmePool    = "q7mt"
)

var testPools = config.Pools{
	{ID: defaultPool, Label: "Default"},
	{ID: acmePool, Label: "Acme", Dirs: []string{"/work/acme"}, WrittenDirs: []string{"/work/acme"}},
}

// pooledAccounts are testConfigured in testPools.
func pooledAccounts() config.Accounts {
	return config.Accounts{
		{ID: "work", Label: "Work", Pool: defaultPool, Primary: true},
		{ID: "personal", Label: "Personal", Pool: acmePool, Primary: true},
		{ID: "side", Label: "Side", Pool: defaultPool},
	}
}

// poolTokens has every account's token, personal's among them.
var poolTokens = tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken}

// latestWeek is a week whose quota can wait longest: half used, resetting in
// six days.
var latestWeek = quota.Window{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: start.Add(6 * 24 * time.Hour)}

// spentWeek is a week with no room left.
var spentWeek = quota.Window{Key: "7d", Label: "Week", Utilization: 1, ResetsAt: start.Add(5 * 24 * time.Hour), Status: quota.StatusRejected}

// newPoolsRouter builds a router on now's time of pooledAccounts in
// testPools, or of testConfigured in no pools where pooled is unset, each
// account with a token, serving every request on any account.
func newPoolsRouter(t *testing.T, now func() time.Time, pooled bool) *Router {
	t.Helper()
	cfg := testConfig(now, &stubProber{}, poolTokens.Read)
	if pooled {
		cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
	}
	r := newTestRouterOf(t, cfg)
	r.proxy.transport = servingAll(served, served, served)
	return r
}

// servingAll is an upstream that answers work's requests, personal's and
// side's as given.
func servingAll(work, personal, side answer) *scriptedUpstream {
	return &scriptedUpstream{answers: map[string][]answer{workToken: {work}, personalToken: {personal}, sideToken: {side}}}
}

// readPersonalFirst has r read the accounts' weeks as of now: personal's
// quota needs using first, then work's, then side's.
func readPersonalFirst(r *Router) {
	readWeeks(r, map[string]quota.Window{"personal": soonWeek, "work": laterWeek, "side": latestWeek})
}

// readWeeks has r read each account's week as given, by its id.
func readWeeks(r *Router, weeks map[string]quota.Window) {
	for id, w := range weeks {
		r.state.record(id, []quota.Window{w}, r.state.mark())
	}
}

// routeIn has r's proxy route a messages request of Opus, on token, of the
// session given, "" for none, naming the pool given in its header, "" for
// none, with headers beside, and returns its answer.
func routeIn(t testing.TB, r *Router, token, session, pool string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"`+opus+`","max_tokens":1}`))
	req.Header.Set("Authorization", "Bearer "+token)
	if session != "" {
		req.Header.Set(claude.SessionHeader, session)
	}
	if pool != "" {
		req.Header.Set(PoolHeader, pool)
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	w := httptest.NewRecorder()
	r.Proxy().ServeHTTP(w, req)
	return w
}

// tokenIn returns the token a session started in the pool with the given id
// holds, its primary's, as testPools have them: personal's in Acme, else
// work's.
func tokenIn(pool string) string {
	if strings.EqualFold(pool, acmePool) {
		return personalToken
	}
	return workToken
}

// leavingAsChosen has r's pools put the account with the given id in the pool
// given as each request is checked, and as they stood before as its account
// is chosen, as a change to the pools taken up between the two: a request
// sent on the account's token passes the check, and finds the account outside
// its pool as it's chosen.
func leavingAsChosen(r *Router, id, pool string) {
	was := r.proxy.pools.current()
	checked := *was
	checked.poolOf = maps.Clone(was.poolOf)
	checked.poolOf[id] = pool
	r.proxy.pools.table.Store(checked.stored())
	r.proxy.chooser = choosingAfter{Chooser: r.proxy.chooser, first: func() { r.proxy.pools.table.Store(was) }}
}

// choosingAfter is a Chooser that calls first before each choice.
type choosingAfter struct {
	Chooser
	first func()
}

func (c choosingAfter) Choose(ctx context.Context, req Request) Choice {
	c.first()
	return c.Chooser.Choose(ctx, req)
}

// forbidOpus has the upstream have refused Opus on each account given lately.
func forbidOpus(r *Router, ids ...string) {
	for _, id := range ids {
		r.state.forbid(id, claude.Provider{}.Family(opus), http.StatusForbidden, someRequest)
	}
}

func TestEachRequestIsRoutedWithinItsPool(t *testing.T) {
	// Personal's quota needs using first, but it's in Acme alone.
	tests := []struct {
		name string
		// pooled has the config put the accounts in pools: else it has none.
		pooled bool
		// pool is what the request's header names, "" for none.
		pool string
		// want is the account the request goes out on, and wantPool the pool
		// its line names.
		want, wantPool string
	}{
		{name: "in the first pool", pooled: true, pool: defaultPool, want: "work", wantPool: defaultPool},
		{name: "in another", pooled: true, pool: acmePool, want: "personal", wantPool: acmePool},
		{name: "naming its pool in another case", pooled: true, pool: "Q7MT", want: "personal", wantPool: acmePool},
		{name: "naming none, from a session started before pools: the first", pooled: true, want: "work", wantPool: defaultPool},
		{name: "a config from before pools, the header passed over", pool: defaultPool, want: "personal"},
		{name: "a config from before pools, naming none", want: "personal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r := newPoolsRouter(t, at(start), tt.pooled)
			readPersonalFirst(r)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			if w := routeIn(t, r, tokenIn(tt.pool), "one", tt.pool, nil); w.Code != http.StatusOK {
				t.Fatalf("answered %d, want 200", w.Code)
			}

			if sent := upstream.sent(); !slices.Equal(sent, []string{tt.want}) {
				t.Errorf("the request went out on %q, want %s alone", sent, tt.want)
			}
			if got := poolsOf(lines()); !slices.Equal(got, []string{tt.wantPool}) {
				t.Errorf("the ledger's lines name the pools %q, want one line naming %q", got, tt.wantPool)
			}
		})
	}
}

func TestARequestNeverGoesOutOnAnotherPoolsAccount(t *testing.T) {
	// Personal, in Acme, is the account every choice would take, were the
	// pools not there. Each request is in Default, of session one.
	tests := []struct {
		name string
		// setUp readies the router beyond its readings.
		setUp func(r *Router)
		// work and side are their answers, served where they're nil; token
		// is the one the request carries, work's unless it says; headers are
		// what it carries beside.
		work, side answer
		token      string
		headers    map[string]string
		// wantSent is where the request went out, in turn, wantCode what the
		// client was answered, and wantReason why its line says it went
		// where it did, where it's given; wantLog is a line the log has,
		// where it's given.
		wantSent   []string
		wantCode   int
		wantReason string
		wantLog    []string
	}{
		{name: "a choice afresh", wantSent: []string{"work"}, wantCode: http.StatusOK, wantReason: reasonNew},
		{
			name: "a session on another pool's account, its cache warm: chosen afresh in its own",
			setUp: func(r *Router) {
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "personal", reason: reasonNew}, start.Add(-5*time.Minute))
			},
			wantSent: []string{"work"}, wantCode: http.StatusOK,
		},
		{
			name:     "the global pin naming another pool's account: no pin for it",
			setUp:    func(r *Router) { r.pinTo(status.Pin{Accounts: []string{"personal"}, Since: start}, false) },
			wantSent: []string{"work"}, wantCode: http.StatusOK, wantReason: reasonNew,
		},
		{
			name: "the global pin moving running sessions to another pool's account: the session stays",
			setUp: func(r *Router) {
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
				r.pinTo(status.Pin{Accounts: []string{"personal"}, Since: start, Move: true}, false)
			},
			wantSent: []string{"work"}, wantCode: http.StatusOK, wantReason: reasonSticky,
		},
		{
			name:     "the session launched pinned to another pool's account: no pin for it",
			headers:  map[string]string{PinHeader: "personal"},
			wantSent: []string{"work"}, wantCode: http.StatusOK, wantReason: reasonNew,
			wantLog: []string{"level=WARN", `msg="pin ignored: account isn't in the request's pool"`, "pin=personal", "session=one"},
		},
		{
			name: "the session pinned to another pool's account as it runs: no pin for it",
			setUp: func(r *Router) {
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
				r.sessions.pinSession("one", "personal")
			},
			wantSent: []string{"work"}, wantCode: http.StatusOK, wantReason: reasonSticky,
			wantLog: []string{"level=DEBUG", `msg="pin ignored: account isn't in the request's pool"`, "pin=personal", "pool=" + defaultPool},
		},
		{
			name: "the session pinned to another pool's account as it runs, launched pinned to one of its own: the one it launched with",
			setUp: func(r *Router) {
				assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
				r.sessions.pinSession("one", "personal")
			},
			headers:  map[string]string{PinHeader: "side"},
			wantSent: []string{"side"}, wantCode: http.StatusOK, wantReason: reasonPinned,
			wantLog: []string{"level=DEBUG", `msg="pin ignored: account isn't in the request's pool"`, "pin=personal", "pool=" + defaultPool},
		},
		{
			name:     "a replay after a limit: on the pool's next account, then the limit passed on",
			work:     limitHit,
			side:     limitHit,
			wantSent: []string{"work", "side"}, wantCode: http.StatusTooManyRequests,
		},
		{
			name:     "a replay after a refusal: on the pool's next account, then refused",
			work:     forbidden,
			side:     forbidden,
			wantSent: []string{"work", "side"}, wantCode: http.StatusBadGateway,
		},
		{
			name:     "no room in the pool: the client's account, in the pool, for the upstream to say why",
			setUp:    func(r *Router) { readWeeks(r, map[string]quota.Window{"work": spentWeek, "side": spentWeek}) },
			work:     limitHit,
			wantSent: []string{"work"}, wantCode: http.StatusTooManyRequests,
		},
		{
			name: "no room in the pool, the client's account another pool's as it's chosen: one of the pool's, for the upstream to say why",
			setUp: func(r *Router) {
				readWeeks(r, map[string]quota.Window{"work": spentWeek, "side": spentWeek})
				leavingAsChosen(r, "personal", defaultPool)
			},
			token:    personalToken,
			work:     limitHit,
			wantSent: []string{"work"}, wantCode: http.StatusTooManyRequests,
		},
		{
			name:     "every account of the pool refused it lately, the client's account in the pool: on that all the same",
			setUp:    func(r *Router) { forbidOpus(r, "work", "side") },
			work:     forbidden,
			wantSent: []string{"work"}, wantCode: http.StatusBadGateway,
		},
		{
			name: "every account of the pool refused it lately, the client's account another pool's as it's chosen: answered by switchboard",
			setUp: func(r *Router) {
				forbidOpus(r, "work", "side")
				leavingAsChosen(r, "personal", defaultPool)
			},
			token:    personalToken,
			wantSent: nil, wantCode: http.StatusTooManyRequests, wantReason: status.ReasonNoRoom,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r := newPoolsRouter(t, at(start), true)
			readPersonalFirst(r)
			if tt.setUp != nil {
				tt.setUp(r)
			}
			upstream := servingAll(orServed(tt.work), served, orServed(tt.side))
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			w := routeIn(t, r, cmp.Or(tt.token, workToken), "one", defaultPool, tt.headers)

			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request went out on %q, want %q", sent, tt.wantSent)
			}
			if w.Code != tt.wantCode {
				t.Errorf("answered %d, want %d", w.Code, tt.wantCode)
			}
			if a := r.sessions.lookup(key{session: "one", model: opus}).current; a.Account == "personal" {
				t.Errorf("the session is left on personal, another pool's account")
			}
			if got := reasonsOf(lines()); tt.wantReason != "" && !slices.Equal(got, []string{tt.wantReason}) {
				t.Errorf("the ledger's lines give the reasons %q, want one line, its reason %q", got, tt.wantReason)
			}
			if tt.wantLog != nil && !log.Has(tt.wantLog...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
		})
	}
}

// reasonsOf returns the reason each line gives, in order.
func reasonsOf(lines []ledger.Line) []string {
	return ofEachLine(lines, func(l ledger.Line) string { return l.Reason })
}

// poolsOf returns the pool each line names, in order.
func poolsOf(lines []ledger.Line) []string {
	return ofEachLine(lines, func(l ledger.Line) string { return l.Pool })
}

// ofEachLine returns what of gives of each line, in order.
func ofEachLine(lines []ledger.Line, of func(ledger.Line) string) []string {
	values := make([]string, len(lines))
	for i, l := range lines {
		values[i] = of(l)
	}
	return values
}

// orServed is a, else served where it's nil.
func orServed(a answer) answer {
	if a == nil {
		return served
	}
	return a
}

func TestAPoolWithNoAccountToTakeARequestIsAnsweredBySwitchboardWhileAnotherCarriesOn(t *testing.T) {
	tests := []struct {
		name string
		// read reads the accounts' tokens; out puts Acme's accounts out; why
		// is why switchboard says they are.
		read func(id string) (tokens.Token, error)
		out  func(r *Router)
		why  string
	}{
		{
			name: "its one account refused the request lately", read: poolTokens.Read, out: func(r *Router) { forbidOpus(r, "personal") },
			why: "account personal: the upstream refused it lately",
		},
		{
			name: "its one account without a token", read: testTokens.Read, out: func(*Router) {},
			why: "account personal: " + tokenstest.Missing("personal").Error(),
		},
		{
			name: "one account without a token, side, moved to it, refused the request lately: each named",
			read: testTokens.Read,
			out: func(r *Router) {
				moved := pooledAccounts()
				moved[2].Pool = acmePool
				r.proxy.pools.follow(moved, testPools)
				forbidOpus(r, "side")
			},
			why: "account personal: " + tokenstest.Missing("personal").Error() + "; account side: the upstream refused it lately",
		},
		{
			name: "only accounts the config added since the router started, personal leaving it",
			read: poolTokens.Read,
			out: func(r *Router) {
				added := slices.DeleteFunc(pooledAccounts(), func(a config.Account) bool { return a.ID == "personal" })
				added = append(added, config.Account{ID: "spare", Pool: acmePool}, config.Account{ID: "extra", Pool: acmePool})
				r.proxy.pools.follow(added, testPools)
			},
			why: "account extra: taken up when the router restarts; account spare: taken up when the router restarts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			cfg := testConfig(at(start), &stubProber{}, tt.read)
			cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
			r := newTestRouterOf(t, cfg)
			readPersonalFirst(r)
			tt.out(r)
			leavingAsChosen(r, "work", acmePool)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			// A session in Acme, holding work's token, which is Default's by
			// the time the request is chosen, as a change to the pools taken
			// up between its check and its choice can leave it.
			w := routeIn(t, r, workToken, "one", acmePool, nil)

			if w.Code != http.StatusTooManyRequests {
				t.Errorf("Acme's request was answered %d, want 429", w.Code)
			}
			want := "switchboard: no account in pool Acme can take the request; " + tt.why
			if body := w.Body.String(); !strings.Contains(body, `"rate_limit_error"`) || !strings.Contains(body, want) {
				t.Errorf("Acme's request was answered %s, want switchboard's rate limit error saying %q", body, want)
			}
			if got := w.Header().Get("Anthropic-Ratelimit-Unified-Status"); got != "rejected" {
				t.Errorf("Acme's answer's unified status is %q, want rejected, as Claude Code reads a limit off it", got)
			}
			if sent := upstream.sent(); len(sent) > 0 {
				t.Errorf("Acme's request went out on %q, want none: its client's own token is Default's", sent)
			}
			closed := []string{"level=WARN", `msg="no account in the request's pool can take it; answering 429"`, "pool=" + acmePool, fmt.Sprintf("why=%q", tt.why)}
			if !log.Has(closed...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, closed)
			}

			if w := routeIn(t, r, workToken, "two", defaultPool, nil); w.Code != http.StatusOK {
				t.Errorf("Default's request was answered %d, want 200: it carries on", w.Code)
			}
			if sent := upstream.sent(); !slices.Equal(sent, []string{"work"}) {
				t.Errorf("the requests went out on %q, want Default's on work alone", sent)
			}
			got := lines()
			if len(got) != 2 {
				t.Fatalf("the ledger holds %d lines, want Acme's and Default's", len(got))
			}
			if l := got[0]; l.Pool != acmePool || l.Status != http.StatusTooManyRequests || l.Reason != status.ReasonNoRoom || l.Account != "" {
				t.Errorf("Acme's line is of pool %q, answered %d for %q on %q, want of %s, answered 429 for %q on no account",
					l.Pool, l.Status, l.Reason, l.Account, acmePool, status.ReasonNoRoom)
			}
		})
	}
}

func TestAPoolLeftWithNoAccountsAsARequestIsChosenIsAnsweredBySwitchboard(t *testing.T) {
	logstest.Capture(t)
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.Pools = pooledAccounts(), append(slices.Clone(testPools), config.Pool{ID: "z5k2", Label: "Spare", Dirs: []string{"/work/spare"}})
	r := newTestRouterOf(t, cfg)
	leavingAsChosen(r, "work", "z5k2")
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	w := routeIn(t, r, workToken, "one", "z5k2", nil)

	want := "switchboard: no account in pool Spare can take the request; the pool has no accounts"
	if w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), want) {
		t.Errorf("answered %d %s, want 429 saying %q", w.Code, w.Body, want)
	}
	if sent := upstream.sent(); len(sent) > 0 {
		t.Errorf("the request went out on %q, want none", sent)
	}
}

func TestALaunchPinOutsideItsPoolIsWarnedOfOnceForEachSession(t *testing.T) {
	log := logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	pinned := map[string]string{PinHeader: "personal"}

	// Default's sessions one and two, launched pinned to personal, Acme's;
	// Acme's session three, pinned to it in its own pool, between them.
	for _, request := range []struct{ session, pool string }{
		{"one", defaultPool}, {"three", acmePool}, {"one", defaultPool}, {"one", defaultPool}, {"two", defaultPool},
	} {
		routeIn(t, r, tokenIn(request.pool), request.session, request.pool, pinned)
	}

	why := `msg="pin ignored: account isn't in the request's pool"`
	for level, want := range map[string]int{"WARN": 2, "DEBUG": 2} {
		if got := timesLogged(log, "level="+level, why); got != want {
			t.Errorf("log reads\n%s\nwant the pin ignored noted at %s %d times, not %d: warned of once for each session, at debug after, whatever another pool honours",
				log, level, want, got)
		}
	}
}

func TestALaunchPinOutsideItsPoolIsWarnedOfAgainAfterASpellInside(t *testing.T) {
	log := logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	pinned := map[string]string{PinHeader: "personal"}
	inDefault := pooledAccounts()
	inDefault[1].Pool = defaultPool

	// Default's session one, launched pinned to personal: outside its pool,
	// then inside it once personal joins Default, then outside again as it
	// goes back to Acme.
	for _, accounts := range []config.Accounts{pooledAccounts(), pooledAccounts(), inDefault, pooledAccounts(), pooledAccounts()} {
		r.proxy.pools.follow(accounts, testPools)
		routeIn(t, r, workToken, "one", defaultPool, pinned)
	}

	why := `msg="pin ignored: account isn't in the request's pool"`
	for level, want := range map[string]int{"WARN": 2, "DEBUG": 2} {
		if got := timesLogged(log, "level="+level, why); got != want {
			t.Errorf("log reads\n%s\nwant the pin ignored noted at %s %d times, not %d: warned of once for each spell outside the pool, at debug after",
				log, level, want, got)
		}
	}
}

func TestTheReserveThatHoldsARequestBackIsItsPools(t *testing.T) {
	// The request is Default's, on work's token. atReserve is a week used
	// past the 10% reserve of the account it's read of.
	atReserve := quota.Window{Key: "7d", Label: "Week", Utilization: 0.95, ResetsAt: start.Add(5 * 24 * time.Hour)}
	tests := []struct {
		name string
		// reserved is the account with the reserve, whose week is atReserve.
		reserved string
		// weeks are the other accounts' weeks, and refused those that have
		// refused the request lately.
		weeks   map[string]quota.Window
		refused []string
		// wantSent is where the request went out, wantCode what it was
		// answered, and wantBody what the answer says, where it's given.
		wantSent []string
		wantCode int
		wantBody string
	}{
		{
			name:     "Default's accounts refused it lately but work, at its reserve: switchboard's 429, Acme's room untouched",
			reserved: "work", weeks: map[string]quota.Window{"side": latestWeek, "personal": latestWeek}, refused: []string{"side"},
			wantSent: nil, wantCode: http.StatusTooManyRequests, wantBody: "switchboard: no account has room outside its reserve",
		},
		{
			name:     "Default's accounts refused it lately, Acme's at its reserve: not Default's to hold, so on the client's account",
			reserved: "personal", weeks: map[string]quota.Window{"work": latestWeek, "side": latestWeek}, refused: []string{"work", "side"},
			wantSent: []string{"work"}, wantCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
			cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
			cfg.Accounts[slices.IndexFunc(cfg.Accounts, func(a config.Account) bool { return a.ID == tt.reserved })].Reserve = 0.1
			r := newTestRouterOf(t, cfg)
			readWeeks(r, tt.weeks)
			readWeeks(r, map[string]quota.Window{tt.reserved: atReserve})
			forbidOpus(r, tt.refused...)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream

			w := routeIn(t, r, workToken, "one", defaultPool, nil)

			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request went out on %q, want %q", sent, tt.wantSent)
			}
			if w.Code != tt.wantCode || !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("answered %d %s, want %d saying %q", w.Code, w.Body, tt.wantCode, tt.wantBody)
			}
		})
	}
}

func TestACompactionNeverMovesASessionOutOfItsPool(t *testing.T) {
	// The session is on work, whose week is 90% used, resetting in five
	// days; personal's resets tomorrow, read rising at 0.6% an hour, its room
	// at risk; side's resets in six days, later than work's.
	tests := []struct {
		name   string
		pooled bool
		want   string
	}{
		{name: "personal in another pool: stays", pooled: true, want: "work"},
		{name: "a config from before pools: moves to personal", want: "personal"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			clock := &testClock{}
			r := newPoolsRouter(t, clock.read, tt.pooled)
			workWeek := quota.Window{Key: "7d", Label: "Week", Utilization: 0.9, ResetsAt: start.Add(5 * 24 * time.Hour)}
			for i, u := range []float64{0.5, 0.502} {
				clock.now = start.Add(time.Duration(i-1) * 20 * time.Minute)
				r.state.record("work", []quota.Window{risingSession, workWeek}, r.state.mark())
				r.state.record("side", []quota.Window{risingSession, latestWeek}, r.state.mark())
				r.state.record("personal", []quota.Window{risingSession, {Key: "7d", Label: "Week", Utilization: u, ResetsAt: sideSooner}}, r.state.mark())
			}
			assign(r.sessions, key{session: "one", model: opus}, "", decision{account: "work", reason: reasonNew}, start.Add(-5*time.Minute))
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream

			routeIn(t, r, workToken, "one", defaultPool, afterCompaction(ledger.ClassMain, "auto"))

			if sent := upstream.sent(); !slices.Equal(sent, []string{tt.want}) {
				t.Errorf("the request went out on %q, want %s alone", sent, tt.want)
			}
		})
	}
}

func TestTheProbesBeforeAChoiceReadThePoolsAccountsAlone(t *testing.T) {
	tests := []struct {
		pool string
		want map[string]int
	}{
		{pool: defaultPool, want: map[string]int{workToken: 1, sideToken: 1}},
		{pool: acmePool, want: map[string]int{personalToken: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.pool, func(t *testing.T) {
			logstest.Capture(t)
			prober := &stubProber{}
			cfg := testConfig(at(start), prober, poolTokens.Read)
			cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
			r := newTestRouterOf(t, cfg)
			r.proxy.transport = servingAll(served, served, served)

			routeIn(t, r, tokenIn(tt.pool), "one", tt.pool, nil)

			if got := prober.counts(); !maps.Equal(got, tt.want) {
				t.Errorf("probes = %v, want %v: the pool's accounts alone, none read yet", got, tt.want)
			}
		})
	}
}

func TestASessionsNextAccountIsJudgedWithinItsPool(t *testing.T) {
	// Work's quota needs using first. Each session's cache has gone cold, so
	// its next request would be chosen afresh.
	tests := []struct {
		name string
		// on is the account the session is on, and pool the pool its last
		// request was routed within.
		on, pool string
		want     string
	}{
		{name: "in Acme", on: "personal", pool: acmePool, want: "personal"},
		{name: "in Default", on: "side", pool: defaultPool, want: "work"},
		{name: "kept from before the router knew pools: the first", on: "side", want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newPoolsRouter(t, clock.read, true)
			readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": latestWeek, "side": laterWeek})
			assignFor(r.sessions, Request{Session: "one", Model: opus, Pool: tt.pool}, decision{account: tt.on, reason: reasonNew}, start)
			clock.now = start.Add(2 * time.Hour)

			if got := r.scheduler.auto("one", opus, clock.now); got != tt.want {
				t.Errorf("auto(one, %s) = %q, want %s", opus, got, tt.want)
			}
			if s, err := r.session("one", anyPool); err != nil || s.Assignments[0].Auto != tt.want {
				t.Errorf("the session is listed as %+v, %v, want its next request going to %s", s, err, tt.want)
			}
		})
	}
}

func TestARequestNamingNoPoolIsInTheFirstAsTheAccountsRecordNotesIt(t *testing.T) {
	tests := []struct {
		name string
		// first is the pool the accounts record notes as the first, "" for
		// none.
		first string
		want  string
	}{
		{name: "the record noting none: the config's first", want: "work"},
		{name: "the record noting another than the config's first", first: acmePool, want: "personal"},
		{name: "the record noting one no longer configured: the config's first", first: "z5k2", want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			stateDir := t.TempDir()
			if tt.first != "" {
				noteFirstPool(t, stateDir, tt.first)
			}
			cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
			cfg.Accounts, cfg.Pools, cfg.StateDir = pooledAccounts(), testPools, stateDir
			r := newTestRouterOf(t, cfg)
			readPersonalFirst(r)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream

			routeIn(t, r, tokenIn(tt.first), "one", "", nil)

			if sent := upstream.sent(); !slices.Equal(sent, []string{tt.want}) {
				t.Errorf("the request went out on %q, want %s alone", sent, tt.want)
			}
		})
	}
}

// noteFirstPool has the accounts record in stateDir note the pool with the
// given id as the first, as the move to pools notes it.
func noteFirstPool(t *testing.T, stateDir, id string) {
	t.Helper()
	err := identity.NewFile(stateDir).Update(func(r *identity.Record) error {
		r.NoteFirst(id, identity.PoolWritten{}, start)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestThePoolHeaderNeverGoesUpstream(t *testing.T) {
	logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	var mu sync.Mutex
	var got http.Header
	capturing := func(req *http.Request) *http.Response {
		mu.Lock()
		got = req.Header.Clone()
		mu.Unlock()
		return served(req)
	}
	r.proxy.transport = servingAll(capturing, capturing, capturing)

	routeIn(t, r, personalToken, "one", acmePool, nil)

	mu.Lock()
	defer mu.Unlock()
	for name := range got {
		if strings.EqualFold(name, PoolHeader) {
			t.Errorf("the upstream got %s, want it taken off, as every header of switchboard's own is", name)
		}
	}
}

func TestEveryLineNamesItsRequestsPool(t *testing.T) {
	logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	lines := keepingLedger(t, r)

	routeIn(t, r, personalToken, "one", acmePool, nil)
	unread := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/messages", iotest.ErrReader(errors.New("connection reset")))
	unread.Header.Set("Authorization", "Bearer "+personalToken)
	unread.Header.Set(PoolHeader, acmePool)
	r.Proxy().ServeHTTP(httptest.NewRecorder(), unread)

	got := lines()
	if len(got) != 2 {
		t.Fatalf("the ledger holds %d lines, want 2", len(got))
	}
	for _, line := range got {
		if line.Pool != acmePool {
			t.Errorf("a line answered %d names pool %q, want %s, whether its body was read or not", line.Status, line.Pool, acmePool)
		}
	}
}

// poolsTOML is a config file of pooledAccounts in testPools, but side in the
// pool given, and left out where that's "", and with each pool given after
// them as a table of its own.
func poolsTOML(sidePool string, pools ...string) string {
	var b strings.Builder
	b.WriteString("[[account]]\nid = \"work\"\nlabel = \"Work\"\npool = \"" + defaultPool + "\"\n")
	b.WriteString("\n[[account]]\nid = \"personal\"\nlabel = \"Personal\"\npool = \"" + acmePool + "\"\n")
	if sidePool != "" {
		b.WriteString("\n[[account]]\nid = \"side\"\nlabel = \"Side\"\npool = \"" + sidePool + "\"\n")
	}
	for _, pool := range pools {
		b.WriteString("\n" + pool)
	}
	return b.String()
}

// The pools of the config file poolsTOML writes, as it is given them.
const (
	defaultTOML = "[[pool]]\nid = \"" + defaultPool + "\"\nlabel = \"Default\"\n"
	acmeTOML    = "[[pool]]\nid = \"" + acmePool + "\"\nlabel = \"Acme\"\ndirs = [\"/work/acme\"]\n"
)

// sparePool is a pool with no accounts, labelled as given, as a config file
// writes it.
func sparePool(label string) string {
	return "[[pool]]\nid = \"z5k2\"\nlabel = \"" + label + "\"\ndirs = [\"/work/spare\"]\n"
}

// writeConfigFile writes content to the config file at path, in place of
// what it held, as a new file, so the router's look finds it changed however
// soon after the last.
func writeConfigFile(t *testing.T, path, content string) {
	t.Helper()
	written := path + ".new"
	writeFile(t, written, content)
	if err := os.Rename(written, path); err != nil {
		t.Fatal(err)
	}
}

// newWatchingPoolsRouter builds a router of pooledAccounts in testPools, every
// account with a token, started from a config file that makes them, which it
// returns the path of, keeping its accounts record in stateDir, "" for none,
// its config readied by each of configure.
func newWatchingPoolsRouter(t *testing.T, stateDir string, configure ...func(cfg *Config)) (*Router, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.Pools, cfg.ConfigFile, cfg.StateDir = pooledAccounts(), testPools, Watch(path), stateDir
	for _, c := range configure {
		c(&cfg)
	}
	r := newTestRouterOf(t, cfg)
	r.proxy.transport = servingAll(served, served, served)
	return r, path
}

func TestTheRouterTakesUpAChangeToThePoolsAtOnce(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	routeIn(t, r, workToken, "one", defaultPool, nil)

	// Side moves to Acme: Default's session on it moves off it at once, and
	// Acme's sessions are routed to it.
	writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
	r.upkeep.look()
	routeIn(t, r, workToken, "one", defaultPool, nil)
	routeIn(t, r, personalToken, "two", acmePool, nil)

	// Side leaves the config: it's in no pool at once.
	writeConfigFile(t, path, poolsTOML("", defaultTOML, acmeTOML))
	r.upkeep.look()
	routeIn(t, r, personalToken, "three", acmePool, nil)

	if sent, want := upstream.sent(), []string{"side", "work", "side", "personal"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q", sent, want)
	}
}

// The detector is what holds this test: a request reads the pools whole, as
// they stood as it read them, however they change under it.
func TestAChangeToThePoolsRacesTheRequestsBeingRouted(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var once sync.Once
	halt := func() {
		once.Do(func() { close(stop) })
		wg.Wait()
	}
	t.Cleanup(halt)
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					routeIn(t, r, workToken, "", defaultPool, nil)
				}
			}
		})
	}

	waitFor(t, "requests routed before the change", func() bool { return len(upstream.sent()) >= 10 })
	writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
	r.upkeep.look()
	looked := len(upstream.sent())
	waitFor(t, "requests routed after the change", func() bool { return len(upstream.sent()) >= looked+10 })
	halt()
	raced := len(upstream.sent())
	for range 3 {
		routeIn(t, r, workToken, "", defaultPool, nil)
	}

	sent := upstream.sent()
	if i := slices.Index(sent, "personal"); i >= 0 {
		t.Errorf("request %d of Default's went out on personal, Acme's", i)
	}
	if after := sent[raced:]; !slices.Equal(after, []string{"work", "work", "work"}) {
		t.Errorf("once the pools were taken up, Default's requests went out on %q, want work alone, side having moved to Acme", after)
	}
}

// unpooledTOML is a config file of the accounts with the ids given in no
// pools, as one put back from before pools.
func unpooledTOML(ids ...string) string {
	var b strings.Builder
	for _, id := range ids {
		b.WriteString("[[account]]\nid = \"" + id + "\"\nlabel = \"" + id + "\"\n\n")
	}
	return b.String()
}

// movedWithoutRecord returns a state directory with the move to pools marked
// done and no accounts record, so the pools of a config that loses them
// stand as the router last routed within them.
func movedWithoutRecord(t *testing.T) string {
	t.Helper()
	stateDir := t.TempDir()
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
	return stateDir
}

func TestAConfigThatLosesItsPoolsWhileTheRouterRunsKeepsEachPoolsWorkInIt(t *testing.T) {
	log := logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, movedWithoutRecord(t))
	// Personal's quota needs using first, then side's, then work's.
	readWeeks(r, map[string]quota.Window{"personal": soonWeek, "side": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	writeConfigFile(t, path, unpooledTOML("work", "personal"))
	r.upkeep.look()
	routeIn(t, r, workToken, "one", defaultPool, nil)
	routeIn(t, r, workToken, "two", "", nil)
	// Personal refuses Acme's next request, work's quota still waiting, and
	// side's, which is in no pool, having left the config.
	forbidOpus(r, "personal")
	routeIn(t, r, personalToken, "three", acmePool, nil)

	if sent, want := upstream.sent(), []string{"work", "work", "personal"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: Default's on work, side having left the config, and Acme's on personal, its client's, for the upstream to say why", sent, want)
	}
	if n := timesLogged(log, "level=WARN", "the config names no pools"); n != 1 {
		t.Errorf("the log says the config names no pools %d times, want once\n%s", n, log)
	}
}

func TestAConfigPutBackFromBeforeIDsAndPoolsKeepsEachPoolsWorkInIt(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, movedWithoutRecord(t), func(cfg *Config) { cfg.Formerly = formerlyOld })
	// Personal's quota needs using first, then side's, then work's.
	readWeeks(r, map[string]quota.Window{"personal": soonWeek, "side": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	writeConfigFile(t, path, unpooledTOML("old-work", "old-personal"))
	r.upkeep.look()
	defaults := routeIn(t, r, workToken, "one", defaultPool, nil)
	acmes := routeIn(t, r, personalToken, "two", acmePool, nil)

	if defaults.Code != http.StatusOK || acmes.Code != http.StatusOK {
		t.Errorf("Default's request was answered %d and Acme's %d, want 200 each", defaults.Code, acmes.Code)
	}
	if sent, want := upstream.sent(), []string{"work", "personal"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: each in its pool, the accounts known by the ids they had, side, left out, in none", sent, want)
	}
}

// formerlyOld gives each account's id by the one it had before switchboard
// made ids: old-work's is work, and so on.
func formerlyOld(id string) (string, bool) {
	return strings.CutPrefix(id, "old-")
}

func TestThePoolsComingBackToAConfigThatLostThemAreTakenUpAtOnce(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, movedWithoutRecord(t))
	// Side's quota needs using first, then personal's, then work's.
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "personal": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	r.upkeep.look()
	routeIn(t, r, workToken, "one", defaultPool, nil)
	// They come back, side moved to Acme.
	writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
	r.upkeep.look()
	routeIn(t, r, workToken, "two", defaultPool, nil)
	routeIn(t, r, personalToken, "three", acmePool, nil)

	if sent, want := upstream.sent(), []string{"side", "work", "side"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: Default's on side while the pools stood, then on work, side in Acme", sent, want)
	}
}

func TestAConfigPutBackMidMoveKeepsThePoolsTheRouterRoutedWithin(t *testing.T) {
	log := logstest.Capture(t)
	stateDir := t.TempDir()
	// The move under way: the first pool written to the config and taken
	// up, the move not yet marked done.
	noteFirstPool(t, stateDir, defaultPool)
	r, path := newWatchingPoolsRouter(t, stateDir)
	// Side's quota needs using first, then personal's, then work's.
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "personal": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	r.upkeep.look()
	routeIn(t, r, personalToken, "one", acmePool, nil)
	routeIn(t, r, workToken, "two", defaultPool, nil)

	if sent, want := upstream.sent(), []string{"personal", "side"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: Acme's on personal, never on side, Default's", sent, want)
	}
	if n := timesLogged(log, "level=WARN", "the move to pools isn't done"); n != 1 {
		t.Errorf("the log says the move to pools isn't done %d times, want once\n%s", n, log)
	}
}

func TestARouterRoutesAConfigNamingNoPoolsAsBeforePoolsTillTheMoveToPoolsIsDone(t *testing.T) {
	log := logstest.Capture(t)
	stateDir := t.TempDir()
	// The move under way: the record keeps the pools, the move not yet
	// marked done.
	if err := identity.NewFile(stateDir).Update(func(rec *identity.Record) error {
		*rec = recordOfThePools()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.StateDir = stateDir
	r := newTestRouterOf(t, cfg)
	readPersonalFirst(r)
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	defaults := routeIn(t, r, workToken, "one", defaultPool, nil)
	acmes := routeIn(t, r, workToken, "two", acmePool, nil)
	if sent, want := upstream.sent(), []string{"personal", "personal"}; !slices.Equal(sent, want) || defaults.Code != http.StatusOK || acmes.Code != http.StatusOK {
		t.Errorf("before the move is done, the requests were answered %d and %d, going out on %q; want 200 each, on %q, any account, as before pools",
			defaults.Code, acmes.Code, sent, want)
	}
	if n := timesLogged(log, "level=WARN", "the config names no pools"); n != 0 {
		t.Errorf("before the move is done, the log says the config names no pools %d times, want never\n%s", n, log)
	}

	// Done: the next look keeps the pools the record does.
	writeFile(t, filepath.Join(stateDir, "pools.moved"), "")
	r.upkeep.look()
	routeIn(t, r, workToken, "three", defaultPool, nil)
	if sent, want := upstream.sent(), []string{"personal", "personal", "work"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: once the move is done, Default's on work, personal in Acme, as the record keeps it", sent, want)
	}
}

func TestARequestSentAgainOnItsAccountGoesElsewhereOnceTheAccountHasLeftItsPool(t *testing.T) {
	// The request is Default's, and work's quota needs using first. Work's
	// answer would have the request sent on it again, but work moves to Acme
	// meanwhile, or leaves the config where Default is the one pool. reset is
	// the session as a reset by hand leaves it.
	reset := session
	reset.Utilization = 0.02
	tests := []struct {
		name string
		// first returns work's answer, given the router, what has work's file
		// hold a new token, and what moves work; side is side's.
		first func(r *Router, renew, move func()) answer
		side  answer
		// onePool has Default the one pool, which work leaves the config
		// from, rather than moving to Acme.
		onePool bool
		// wantSent is where the request went out, and wantCode what it was
		// answered; wantWhy is why it left work, "left the pool" where it's
		// "".
		wantSent []string
		wantCode int
		wantWhy  string
	}{
		{
			name:     "throttled, work moving in the wait: on side, the pool's next",
			first:    throttledThenMoving,
			side:     served,
			wantSent: []string{"work", "side"}, wantCode: http.StatusOK,
		},
		{
			name:     "throttled, then side refusing it, as every account that judged it did: its refusal taken back",
			first:    throttledThenMoving,
			side:     forbidden,
			wantSent: []string{"work", "side"}, wantCode: http.StatusBadGateway,
		},
		{
			name:  "throttled, work leaving the one pool in the wait: on side, work having no room",
			first: throttledThenMoving, side: served, onePool: true,
			wantSent: []string{"work", "side"}, wantCode: http.StatusOK, wantWhy: whyNoRoom,
		},
		{
			name:  "throttled, work leaving the one pool, then side refusing it: its refusal taken back",
			first: throttledThenMoving, side: forbidden, onePool: true,
			wantSent: []string{"work", "side"}, wantCode: http.StatusBadGateway, wantWhy: whyNoRoom,
		},
		{
			name: "at a limit from before a reset by hand: on side",
			first: func(r *Router, _, move func()) answer {
				return func(req *http.Request) *http.Response {
					r.state.record("work", []quota.Window{reset, soonWeek}, r.state.mark())
					move()
					return sessionRejected(req)
				}
			},
			side:     served,
			wantSent: []string{"work", "side"}, wantCode: http.StatusOK,
		},
		{
			name:     "refusing a token work has replaced since: on side",
			first:    func(_ *Router, renew, move func()) answer { return refusedAs(renew, move) },
			side:     served,
			wantSent: []string{"work", "side"}, wantCode: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				files := &changingFiles{files: poolTokens}
				cfg := testConfig(at(start), &stubProber{}, files.read)
				cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
				moved := pooledAccounts()
				moved[0].Pool = acmePool
				if tt.onePool {
					cfg.Accounts = slices.DeleteFunc(pooledAccounts(), func(a config.Account) bool { return a.Pool != defaultPool })
					cfg.Pools, moved = testPools[:1], cfg.Accounts[1:]
				}
				r := newTestRouterOf(t, cfg)
				for id, week := range map[string]quota.Window{"work": soonWeek, "side": laterWeek} {
					r.state.record(id, []quota.Window{session, week}, r.state.mark())
				}
				renew := func() {
					files.set(tokenstest.Files{"work": renewedToken, "personal": personalToken, "side": sideToken})
				}
				move := func() { r.proxy.pools.follow(moved, cfg.Pools) }
				upstream := servingAll(tt.first(r, renew, move), served, tt.side)
				upstream.answers[renewedToken] = []answer{served}
				r.proxy.transport = upstream
				lines := keepingLedger(t, r)

				w := routeIn(t, r, workToken, "one", defaultPool, nil)

				if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
					t.Errorf("the request went out on %q, want %q: never again on work, Acme's now", sent, tt.wantSent)
				}
				if w.Code != tt.wantCode {
					t.Errorf("answered %d, want %d", w.Code, tt.wantCode)
				}
				why := cmp.Or(tt.wantWhy, whyLeftPool)
				if want := []string{"level=INFO", "msg=replaying", "from=work", "to=side", `why="` + why + `"`}; !log.Has(want...) {
					t.Errorf("log reads\n%s\nwant a line with %q", log, want)
				}
				if got, want := reasonsOf(lines()), status.ReasonMovedOff+"work "+why; !slices.Equal(got, []string{want}) {
					t.Errorf("the ledger's lines give the reasons %q, want one line, its reason %q", got, want)
				}
				if side, _ := r.Status().Account("side"); side.Refused != (status.Refusal{}) {
					t.Errorf("side's refusal is %+v, want none: work, left for leaving the pool, judged nothing", side.Refused)
				}
			})
		})
	}
}

// throttledThenMoving is work's answer that throttles the request, moving
// work as given halfway through the wait.
func throttledThenMoving(_ *Router, _, move func()) answer {
	return func(req *http.Request) *http.Response {
		time.AfterFunc(throttleWait/2, move)
		return throttled("")(req)
	}
}

func TestARequestAndItsReplayEachFindItInThePoolsAsTheyStandAsTheyChoose(t *testing.T) {
	// The config moves to pools as work answers the request at its limit:
	// work's quota needed using first, then personal's.
	tests := []struct {
		name string
		// pool is what the request's header names, "" for none.
		pool string
		// wantSent is where it went out, in turn, and wantPool the pool its
		// line names.
		wantSent []string
		wantPool string
	}{
		{name: "naming Acme: on its account", pool: acmePool, wantSent: []string{"work", "personal"}, wantPool: acmePool},
		{name: "naming none: on the first's", wantSent: []string{"work", "side"}, wantPool: defaultPool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r := newPoolsRouter(t, at(start), false)
			readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": laterWeek, "side": latestWeek})
			limitThenPools := func(req *http.Request) *http.Response {
				r.proxy.pools.follow(pooledAccounts(), testPools)
				return limitHit(req)
			}
			upstream := servingAll(limitThenPools, served, served)
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			if w := routeIn(t, r, workToken, "one", tt.pool, nil); w.Code != http.StatusOK {
				t.Errorf("answered %d, want 200", w.Code)
			}

			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request went out on %q, want %q", sent, tt.wantSent)
			}
			if got := poolsOf(lines()); !slices.Equal(got, []string{tt.wantPool}) {
				t.Errorf("the ledger's lines name the pools %q, want one line naming %q", got, tt.wantPool)
			}
		})
	}
}

func TestTheAccountsRecordIsKeptInStepWithAChangeToThePoolsAlone(t *testing.T) {
	logstest.Capture(t)
	stateDir := t.TempDir()
	r, path := newWatchingPoolsRouter(t, stateDir)
	r.upkeep.keepRecord()
	// The config file written again as it was, so upkeep has found it making
	// the accounts it makes from here on.
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()

	// A pool with no accounts added: the record has it.
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML, sparePool("Spare")))
	r.upkeep.look()
	if p, ok := recordedPool(t, stateDir, "z5k2"); !ok || !p.Removed.IsZero() {
		t.Fatalf("the record holds %+v, %v, want the pool added, as it was configured", p, ok)
	}

	// Relabelled, then removed: the record keeps the label it had last.
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML, sparePool("Spare room")))
	r.upkeep.look()
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	if p, ok := recordedPool(t, stateDir, "z5k2"); !ok || p.Removed.IsZero() || p.Label != "Spare room" {
		t.Errorf("the record holds %+v, %v, want the pool removed, labelled Spare room, as it was last", p, ok)
	}
}

// recordedPool returns the pool with the given id as the accounts record in
// stateDir holds it, reporting false where it holds none.
func recordedPool(t *testing.T, stateDir, id string) (identity.PoolEntry, bool) {
	t.Helper()
	record, err := identity.NewFile(stateDir).Read()
	if err != nil {
		t.Fatal(err)
	}
	return record.PoolByID(id)
}

func TestThePoolsARequestIsIn(t *testing.T) {
	pools := newPoolTable(pooledAccounts(), testPools, "", sameID)
	none := newPoolTable(config.Accounts(testConfigured), nil, "", sameID)
	tests := []struct {
		name  string
		table *poolTable
		// named is what the request's header names.
		named string
		want  string
		// in and out are accounts the pool holds, and those it doesn't.
		in, out []string
	}{
		{name: "the first", table: pools, named: defaultPool, want: defaultPool, in: []string{"work", "side"}, out: []string{"personal", "gone"}},
		{name: "another", table: pools, named: acmePool, want: acmePool, in: []string{"personal"}, out: []string{"work", "side"}},
		{name: "in another case", table: pools, named: "Q7MT", want: acmePool, in: []string{"personal"}},
		{name: "none named", table: pools, want: defaultPool, in: []string{"work"}},
		{name: "one not configured", table: pools, named: "z5k2", want: "z5k2", out: []string{"work", "personal", "side"}},
		{name: "a config from before pools", table: none, named: acmePool, want: "", in: []string{"work", "personal", "side", "gone"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.table.of(tt.named)
			if got != tt.want {
				t.Errorf("of(%q) = %q, want %q", tt.named, got, tt.want)
			}
			for _, id := range tt.in {
				if !tt.table.holds(got, id) {
					t.Errorf("holds(%q, %s) = false, want true", got, id)
				}
			}
			for _, id := range tt.out {
				if tt.table.holds(got, id) {
					t.Errorf("holds(%q, %s) = true, want false", got, id)
				}
			}
		})
	}
}

func TestThePoolsAreNamedOnlyWhereThereAreSeveral(t *testing.T) {
	one := config.Pools{testPools[0]}
	tests := []struct {
		name  string
		table *poolTable
		pool  string
		want  string
	}{
		{name: "several", table: newPoolTable(pooledAccounts(), testPools, "", sameID), pool: acmePool, want: "Acme"},
		{name: "several, one not configured: by its id", table: newPoolTable(pooledAccounts(), testPools, "", sameID), pool: "z5k2", want: "z5k2"},
		{name: "one", table: newPoolTable(pooledAccounts(), one, "", sameID), pool: defaultPool, want: ""},
		{name: "none", table: newPoolTable(pooledAccounts(), nil, "", sameID), pool: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.table.shown(tt.pool); got != tt.want {
				t.Errorf("shown(%q) = %q, want %q", tt.pool, got, tt.want)
			}
		})
	}
}

// readingRecord returns what reads the accounts record given.
func readingRecord(record identity.Record) func() (identity.Record, bool) {
	return func() (identity.Record, bool) { return record, true }
}

// recordOfThePools is the accounts record of a router that followed
// pooledAccounts in testPools once the move to pools noted Default first:
// each pool this Mac's, as written, and each account in its pool.
func recordOfThePools() identity.Record {
	return identity.Record{
		Accounts: []identity.Entry{{ID: "work", Added: start, Pool: defaultPool}, {ID: "personal", Added: start, Pool: acmePool}, {ID: "side", Added: start, Pool: defaultPool}},
		Pools: []identity.PoolEntry{
			{ID: defaultPool, Added: start, First: true, Here: true, Written: identity.PoolWritten{Label: "Default", Primary: "work", PrimaryKnown: true}},
			{ID: acmePool, Added: start, Here: true, Written: identity.PoolWritten{Label: "Acme", Dirs: []string{"/work/acme"}, Primary: "personal", PrimaryKnown: true}},
		},
	}
}

func TestThePoolsAreFollowedAsTheyChangeTheFirstReadAfreshWithThem(t *testing.T) {
	first := acmePool
	m := newMembership(func() (identity.Record, bool) {
		return identity.Record{Pools: []identity.PoolEntry{{ID: first, Added: start, First: true, Here: true}}}, true
	}, never, asGiven, nil)
	accounts := pooledAccounts()

	m.follow(accounts, testPools)
	if got := m.current().of(""); got != acmePool {
		t.Errorf("a request naming none is in %q, want %s, as the record notes it", got, acmePool)
	}
	// The record comes to note Default first, the pools the same: they stand
	// as they were made.
	first = defaultPool
	m.follow(slices.Clone(accounts), slices.Clone(testPools))
	if got := m.current().of(""); got != acmePool {
		t.Errorf("for the same pools, a request naming none is in %q, want %s still, the first not read afresh", got, acmePool)
	}

	relabelled := slices.Clone(testPools)
	relabelled[1].Label = "Acme Corp"
	m.follow(accounts, relabelled)
	if got := m.current(); got.first != defaultPool || got.pools[1].Label != "Acme Corp" {
		t.Errorf("the first is %q and Acme is labelled %q, want the first read afresh, %s, labelled Acme Corp", got.first, got.pools[1].Label, defaultPool)
	}
	first = acmePool
	moved := slices.Clone(accounts)
	moved[2].Pool = acmePool
	m.follow(moved, relabelled)
	if got := m.current(); got.first != acmePool || !got.holds(acmePool, "side") {
		t.Errorf("the first is %q and side in Acme is %v, want the first read afresh, %s, side in Acme", got.first, got.holds(acmePool, "side"), acmePool)
	}

	first = defaultPool
	refenced := slices.Clone(relabelled)
	refenced[1].Dirs, refenced[1].WrittenDirs = []string{"/work/acme", "/work/acme-ops"}, []string{"/work/acme", "/work/acme-ops"}
	m.follow(moved, refenced)
	if got := m.current(); got.first != defaultPool || !slices.Equal(got.pools[1].Dirs, refenced[1].Dirs) {
		t.Errorf("the first is %q and Acme is fenced to %q, want the first read afresh, %s, fenced to %q, its directories alone changed",
			got.first, got.pools[1].Dirs, defaultPool, refenced[1].Dirs)
	}
	first = acmePool
	primaries := slices.Clone(moved)
	primaries[0].Primary, primaries[1].Primary, primaries[2].Primary = false, false, true
	m.follow(primaries, refenced)
	if got := m.current(); got.first != acmePool || !maps.Equal(got.primary, map[string]string{acmePool: "side"}) {
		t.Errorf("the first is %q and the primaries are %v, by pool, want the first read afresh, %s, side Acme's alone", got.first, got.primary, acmePool)
	}
}

func TestAConfigThatLosesItsPoolsKeepsThemOnceTheMoveIsDone(t *testing.T) {
	// The config loses its pools and side with them: work and personal are
	// configured, in no pool.
	lost := config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal", Label: "Personal"}}
	tests := []struct {
		name string
		// recorded has the accounts record keep the pools, as
		// recordOfThePools; pooled has the router follow the pools before
		// they're lost; moved has the move to pools done.
		recorded, pooled, moved bool
		// pool is what each account is in once the pools are lost, by id, ""
		// for none, where a request naming pool goes.
		pool string
		want map[string]bool
		// warned is whether the log says the config names no pools.
		warned bool
	}{
		{
			name: "before the move: every account in the one pool there is, as before pools",
			pool: acmePool, want: map[string]bool{"work": true, "personal": true, "side": true},
		},
		{
			name: "before the move, the record keeping the pools and the router having followed them: Acme stands as told",
			pool: acmePool, recorded: true, pooled: true, want: map[string]bool{"work": false, "personal": true, "side": false}, warned: true,
		},
		{
			name: "before the move, the router having followed pools, the record unread: the first stands as told, but for side, which left",
			pool: defaultPool, pooled: true, want: map[string]bool{"work": true, "personal": false, "side": false}, warned: true,
		},
		{
			name: "once the router has followed pools, the record unread: they stand, but for side, which left",
			pool: defaultPool, pooled: true, moved: true, want: map[string]bool{"work": true, "personal": false, "side": false}, warned: true,
		},
		{
			name: "once the router has followed pools, the record unread: Acme stands",
			pool: acmePool, pooled: true, moved: true, want: map[string]bool{"work": false, "personal": true, "side": false}, warned: true,
		},
		{
			name: "started on it: the first as the record keeps it, but for side, which left",
			pool: defaultPool, recorded: true, moved: true, want: map[string]bool{"work": true, "personal": false, "side": false}, warned: true,
		},
		{
			name: "started on it: Acme as the record keeps it",
			pool: acmePool, recorded: true, moved: true, want: map[string]bool{"work": false, "personal": true, "side": false}, warned: true,
		},
		{
			name: "once the router has followed pools: Acme as the record keeps it",
			pool: acmePool, recorded: true, pooled: true, moved: true, want: map[string]bool{"work": false, "personal": true, "side": false}, warned: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			record := noRecord
			if tt.recorded {
				record = readingRecord(recordOfThePools())
			}
			moved := never
			if tt.moved {
				moved = alwaysMoved
			}
			m := newMembership(record, moved, asGiven, nil)
			if tt.pooled {
				m.follow(pooledAccounts(), testPools)
			}

			m.follow(lost, nil)
			m.follow(lost[:1], nil)
			m.follow(lost, nil)

			for id, want := range tt.want {
				if got := m.current().serves(tt.pool, id); got != want {
					t.Errorf("serves(%q, %s) = %v, want %v", tt.pool, id, got, want)
				}
			}
			want := 0
			if tt.warned {
				want = 1
			}
			if n := timesLogged(log, "level=WARN", "the config names no pools"); n != want {
				t.Errorf("the log says the config names no pools %d times, want %d\n%s", n, want, log)
			}
		})
	}
}

func TestAConfigThatLosesItsPoolsAgainIsWarnedOfAgain(t *testing.T) {
	log := logstest.Capture(t)
	m := newMembership(noRecord, alwaysMoved, asGiven, nil)

	for range 2 {
		m.follow(pooledAccounts(), testPools)
		m.follow(pooledAccounts(), nil)
	}

	if n := timesLogged(log, "level=WARN", "the config names no pools"); n != 2 {
		t.Errorf("the log says the config names no pools %d times, want twice, once each time it lost them\n%s", n, log)
	}
}

func TestAConfigThatLosesItsPoolsKeepsThoseMadeLast(t *testing.T) {
	logstest.Capture(t)
	m := newMembership(noRecord, alwaysMoved, asGiven, nil)
	all := config.Accounts{{ID: "work"}, {ID: "personal"}, {ID: "side"}}
	moved := pooledAccounts()
	moved[2].Pool = acmePool

	m.follow(pooledAccounts(), testPools)
	m.follow(all, nil)
	m.follow(moved, testPools)
	m.follow(all, nil)

	if in, out := m.current().holds(acmePool, "side"), m.current().holds(defaultPool, "side"); !in || out {
		t.Errorf("side is in Acme: %v, and in Default: %v, want in Acme alone, as the pools made last have it", in, out)
	}
}

func TestAConfigThatLosesItsPoolsKnowsItsAccountsByTheirIDsNow(t *testing.T) {
	idNow := func(_ identity.Record, id string) string { return strings.TrimPrefix(id, "old-") }
	tests := []struct {
		name string
		// recorded has the accounts record keep the pools, as
		// recordOfThePools; pooled has the router follow the pools before
		// they're lost.
		recorded, pooled bool
		// lost is the config without pools, by its accounts' ids.
		lost []string
		// want is the pool each account is in once it's followed, "" for
		// none.
		want map[string]string
	}{
		{
			name:   "by the ids they had before switchboard made ids: each in its pool, side, left out, in none",
			pooled: true, lost: []string{"old-work", "old-personal"},
			want: map[string]string{"work": defaultPool, "personal": acmePool, "side": ""},
		},
		{
			name:   "naming an account the pools don't have: that one in none, as those left out are",
			pooled: true, lost: []string{"work", "newcomer"},
			want: map[string]string{"work": defaultPool, "personal": "", "side": "", "newcomer": ""},
		},
		{
			name:     "started on it, by the ids they had: each in its pool, as the record keeps it",
			recorded: true, lost: []string{"old-work", "old-personal", "old-side"},
			want: map[string]string{"work": defaultPool, "personal": acmePool, "side": defaultPool},
		},
		{
			name:     "started on it, naming an account the record doesn't have: that one in the unfenced pool",
			recorded: true, lost: []string{"work", "personal", "newcomer"},
			want: map[string]string{"work": defaultPool, "personal": acmePool, "side": "", "newcomer": defaultPool},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			record := noRecord
			if tt.recorded {
				record = readingRecord(recordOfThePools())
			}
			m := newMembership(record, alwaysMoved, idNow, nil)
			if tt.pooled {
				m.follow(pooledAccounts(), testPools)
			}
			var lost config.Accounts
			for _, id := range tt.lost {
				lost = append(lost, config.Account{ID: id})
			}

			m.follow(lost, nil)

			for id, want := range tt.want {
				for _, pool := range []string{defaultPool, acmePool} {
					if got := m.current().holds(pool, id); got != (pool == want) {
						t.Errorf("holds(%s, %s) = %v, want %v", pool, id, got, pool == want)
					}
				}
			}
		})
	}
}

func TestAConfigWithPoolsKnowsItsAccountsByTheirIDsNow(t *testing.T) {
	m := newMembership(noRecord, never, func(_ identity.Record, id string) string { return strings.TrimPrefix(id, "old-") }, nil)
	old := pooledAccounts()
	for i := range old {
		old[i].ID = "old-" + old[i].ID
	}

	m.follow(old, testPools)

	for id, want := range map[string]string{"work": defaultPool, "personal": acmePool, "side": defaultPool, "old-work": ""} {
		for _, pool := range []string{defaultPool, acmePool} {
			if got := m.current().holds(pool, id); got != (pool == want) {
				t.Errorf("holds(%s, %s) = %v, want %v: each account by the id it has now", pool, id, got, pool == want)
			}
		}
	}
}

func TestAnAccountIsKnownByTheIDTheRouterHasItByWhicheverItsNamedBy(t *testing.T) {
	record := identity.Record{Accounts: []identity.Entry{{ID: "work", Was: "old-work"}, {ID: "personal", Was: "old-personal"}, {ID: "side"}}}
	holding := func(ids ...string) accounts {
		held := make(accounts, len(ids))
		for i, id := range ids {
			held[i] = account{ID: id}
		}
		return held
	}
	tests := []struct {
		name     string
		held     accounts
		id, want string
	}{
		{name: "an id the router has", held: holding("work"), id: "work", want: "work"},
		{name: "the id from before, the router on the new one", held: holding("work"), id: "old-work", want: "work"},
		{name: "the new id, the router on the one from before", held: holding("old-work"), id: "work", want: "old-work"},
		{name: "the new id, the router on neither", held: holding("old-personal"), id: "work", want: "work"},
		{name: "an id with none before it", held: holding("old-work"), id: "side", want: "side"},
		{name: "none", held: holding("work"), id: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.held.idNow(formerlyOld, wasIn(record), tt.id); got != tt.want {
				t.Errorf("idNow(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
	if got := holding("old-work").idNow(nil, nil, "work"); got != "work" {
		t.Errorf("knowing of no ids from before, idNow(work) = %q, want work", got)
	}
}

func TestARouterStartedOnTheIDsFromBeforeRoutesWithinThePoolsOfAConfigNamingThoseMadeSince(t *testing.T) {
	logstest.Capture(t)
	stateDir := t.TempDir()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("old-work", "old-personal", "old-side"))
	cfg := testConfig(at(start), &stubProber{}, tokenstest.Files{"old-work": workToken, "old-personal": personalToken, "old-side": sideToken}.Read)
	cfg.Accounts = config.Accounts{{ID: "old-work", Label: "old-work"}, {ID: "old-personal", Label: "old-personal"}, {ID: "old-side", Label: "old-side"}}
	cfg.ConfigFile, cfg.StateDir = Watch(path), stateDir
	r := newTestRouterOf(t, cfg)
	// Side's quota needs using first, then personal's, then work's.
	readWeeks(r, map[string]quota.Window{"old-side": soonWeek, "old-personal": laterWeek, "old-work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	// Both moves are made as it runs, and the config comes to name the ids
	// made, in pools, side in Acme.
	movedBothWays(t, stateDir)
	writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
	r.upkeep.look()
	defaults := routeIn(t, r, workToken, "one", defaultPool, nil)
	acmes := routeIn(t, r, personalToken, "two", acmePool, nil)

	if defaults.Code != http.StatusOK || acmes.Code != http.StatusOK {
		t.Errorf("Default's request was answered %d and Acme's %d, want 200 each", defaults.Code, acmes.Code)
	}
	if sent, want := upstream.sent(), []string{"work", "side"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: each account, by the id the router has, in the pool the config names", sent, want)
	}
}

// alwaysMoved has the move to pools done.
func alwaysMoved() bool { return true }

func TestAConfigThatLosesItsPoolsOnceMovedWithNothingToTellThemByRoutesNothingSayingWhy(t *testing.T) {
	lost := config.Accounts{{ID: "work"}, {ID: "personal"}, {ID: "side"}}
	tests := []struct {
		name   string
		record func() (identity.Record, bool)
		// why is what the pools are closed for.
		why string
	}{
		{name: "the record unread", record: noRecord, why: "the accounts record can't be read"},
		{
			name:   "the record keeping no pool of this Mac's",
			record: readingRecord(identity.Record{Pools: []identity.PoolEntry{{ID: defaultPool, Added: start, First: true}}}),
			why:    "the accounts record keeps no pool as this Mac's config's",
		},
		{
			name: "the record keeping a pool fenced in a home that can't be found",
			record: readingRecord(identity.Record{Pools: []identity.PoolEntry{
				{ID: defaultPool, Added: start, First: true, Here: true, Written: identity.PoolWritten{Dirs: []string{"~/work/own"}}},
			}}),
			why: "the pools the accounts record keeps can't be read as the config's: find the home directory the pools' dirs are in",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			m := newMembership(tt.record, alwaysMoved, asGiven, nil)

			m.follow(lost, nil)
			m.follow(lost, nil)

			got := m.current()
			if !strings.Contains(got.closed, "the config names no pools and "+tt.why) {
				t.Errorf("the pools are closed for %q, want it said that the config names no pools and %s", got.closed, tt.why)
			}
			for _, id := range lost.IDs() {
				for _, pool := range []string{"", defaultPool, acmePool} {
					if got.serves(pool, id) {
						t.Errorf("serves(%q, %s) = true, want nothing routed", pool, id)
					}
				}
			}
			if n := timesLogged(log, "level=WARN", "nothing is routed"); n != 1 {
				t.Errorf("the log says nothing is routed %d times, want once\n%s", n, log)
			}
		})
	}
}

func TestAConfigPutBackWithAnAccountTheRouterLacksWakesNoReaderForNothingAndChangesSafely(t *testing.T) {
	logstest.Capture(t)
	m := newMembership(noRecord, alwaysMoved, asGiven, nil)
	m.follow(pooledAccounts(), testPools)

	// The config is put back from before pools without side, still listing
	// an account removed since, which the pools as last told don't have: it
	// and side are in none.
	putBack := config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal", Label: "Personal"}, {ID: "lapsed", Label: "Lapsed"}}
	m.follow(putBack, nil)
	kept := m.current()
	relabelled := slices.Clone(putBack)
	relabelled[0].Label = "Work Corp"
	m.follow(relabelled, nil)
	m.follow(relabelled[:2], nil)

	if isClosed(kept.replaced) || m.current() != kept {
		t.Fatal("the pools were replaced where nothing in them changed, waking their readers")
	}
	if !kept.serves(defaultPool, "work") || !kept.serves(acmePool, "personal") || kept.serves(defaultPool, "side") ||
		kept.serves(defaultPool, "lapsed") || kept.serves(acmePool, "lapsed") {
		t.Error("the pools don't stand as last told: work in Default, personal in Acme, side and lapsed in none")
	}

	m.follow(relabelled[:1], nil)
	alone := m.current()
	m.follow(putBack[:1], nil)
	if !isClosed(kept.replaced) || isClosed(alone.replaced) || m.current() != alone {
		t.Fatal("the pools weren't replaced once, as personal left them, and left standing after")
	}
	if alone.serves(acmePool, "personal") || !alone.serves(defaultPool, "work") {
		t.Error("the pools don't stand as the config leaves them: work alone, in Default")
	}
}

func TestTheWholePoolsPutBackAfterTheyWereLostAreStoredAfreshAndReplacedSafely(t *testing.T) {
	logstest.Capture(t)
	m := newMembership(noRecord, alwaysMoved, asGiven, nil)
	m.follow(pooledAccounts(), testPools)
	pooled := m.current()

	// The config loses its pools and side with them, then has them back
	// whole.
	m.follow(config.Accounts{{ID: "work"}, {ID: "personal"}}, nil)
	m.follow(pooledAccounts(), testPools)
	whole := m.current()
	if whole == pooled || !isClosed(pooled.replaced) || isClosed(whole.replaced) || !whole.serves(defaultPool, "side") {
		t.Fatalf("the whole pools were stored as %p, replaced: %v, side in Default: %v; want afresh, open, side back in Default, not %p",
			whole, isClosed(whole.replaced), whole.serves(defaultPool, "side"), pooled)
	}

	// Lost again, still listing an account removed since: it and side in
	// none, the same table followed again left standing.
	lostAgain := config.Accounts{{ID: "work"}, {ID: "personal"}, {ID: "lapsed"}}
	m.follow(lostAgain, nil)
	kept := m.current()
	m.follow(lostAgain, nil)
	if !isClosed(whole.replaced) || m.current() != kept || isClosed(kept.replaced) || kept.serves(defaultPool, "side") || kept.serves(defaultPool, "lapsed") {
		t.Error("the whole pools weren't replaced once, as side left them, by pools left standing after, side and lapsed in none")
	}
}

func TestTablesAreTheSameOnlyWhereEveryFieldRoutingReadsIs(t *testing.T) {
	// made is the table every case changes one field of, which gone has left,
	// and the record keeps lapsed in Acme.
	made := func() *poolTable {
		gone := newPoolTable(config.Accounts{{ID: "gone", Pool: acmePool}}, testPools, defaultPool, sameID)
		return newPoolTable(pooledAccounts(), testPools, defaultPool, sameID).after(gone, map[string]string{"lapsed": acmePool})
	}
	tests := []struct {
		name   string
		change func(t *poolTable)
		want   bool
	}{
		{name: "identical", change: func(*poolTable) {}, want: true},
		{name: "a pool relabelled", change: func(t *poolTable) { t.pools[1].Label = "Acme Corp" }},
		{name: "a pool's directories", change: func(t *poolTable) { t.pools[1].Dirs = []string{"/work/acme-ops"} }},
		{name: "a pool's directories as written", change: func(t *poolTable) { t.pools[1].WrittenDirs = []string{"~/acme"} }},
		{name: "an account in another pool", change: func(t *poolTable) { t.poolOf["side"] = acmePool }},
		{name: "another primary", change: func(t *poolTable) { t.primary[defaultPool] = "side" }},
		{name: "another first", change: func(t *poolTable) { t.first = acmePool }},
		{name: "an account that left in another pool last", change: func(t *poolTable) { t.was["gone"] = defaultPool }},
		{name: "an account the record keeps in another pool", change: func(t *poolTable) { t.recorded["lapsed"] = defaultPool }},
		{name: "closed", change: func(t *poolTable) { t.closed = "the accounts record can't be read" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := made()
			tt.change(changed)
			if got := changed.sameAs(made()); got != tt.want {
				t.Errorf("sameAs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAConfigNamingNoPoolsIsFollowedAfreshTillTheRecordTellsItsPools(t *testing.T) {
	logstest.Capture(t)
	readable := false
	m := newMembership(func() (identity.Record, bool) { return recordOfThePools(), readable }, alwaysMoved, asGiven, nil)
	lost := config.Accounts{{ID: "work"}, {ID: "personal"}, {ID: "side"}}

	m.follow(lost, nil)
	closed := m.current()
	m.follow(lost, nil)
	if m.current() != closed || closed.closed == "" {
		t.Fatalf("followed again, the record still unread, the pools are %+v, want the same closed table, not replaced", m.current())
	}

	readable = true
	m.follow(lost, nil)
	if got := m.current(); got.closed != "" || !got.holds(acmePool, "personal") || !got.holds(defaultPool, "work") {
		t.Errorf("followed again once the record reads, the pools are %+v, want those it keeps, personal in Acme, work in Default", got)
	}
}

// isClosed reports whether ch is closed.
func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}
