package router

import (
	"cmp"
	"errors"
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

			if w := routeIn(t, r, workToken, "one", tt.pool, nil); w.Code != http.StatusOK {
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
			wantLog: []string{"level=DEBUG", `msg="pin ignored: account isn't in the request's pool"`, "pin=personal", "pool=" + defaultPool},
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
			name:     "no room in the pool, the client's account another pool's: one of the pool's, for the upstream to say why",
			setUp:    func(r *Router) { readWeeks(r, map[string]quota.Window{"work": spentWeek, "side": spentWeek}) },
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
			name:     "every account of the pool refused it lately, the client's account another pool's: answered by switchboard",
			setUp:    func(r *Router) { forbidOpus(r, "work", "side") },
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
		// read reads the accounts' tokens; out puts Acme's one account out.
		read func(id string) (tokens.Token, error)
		out  func(r *Router)
	}{
		{name: "its one account refused the request lately", read: poolTokens.Read, out: func(r *Router) { forbidOpus(r, "personal") }},
		{name: "its one account without a token", read: testTokens.Read, out: func(*Router) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			cfg := testConfig(at(start), &stubProber{}, tt.read)
			cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
			r := newTestRouterOf(t, cfg)
			readPersonalFirst(r)
			tt.out(r)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			// A session in Acme, holding work's token, which is Default's.
			w := routeIn(t, r, workToken, "one", acmePool, nil)

			if w.Code != http.StatusTooManyRequests {
				t.Errorf("Acme's request was answered %d, want 429", w.Code)
			}
			if body := w.Body.String(); !strings.Contains(body, `"rate_limit_error"`) || !strings.Contains(body, "switchboard: no account in pool Acme can take the request") {
				t.Errorf("Acme's request was answered %s, want switchboard's rate limit error naming the pool", body)
			}
			if got := w.Header().Get("Anthropic-Ratelimit-Unified-Status"); got != "rejected" {
				t.Errorf("Acme's answer's unified status is %q, want rejected, as Claude Code reads a limit off it", got)
			}
			if sent := upstream.sent(); len(sent) > 0 {
				t.Errorf("Acme's request went out on %q, want none: its client's own token is Default's", sent)
			}
			closed := []string{"level=WARN", `msg="no account in the request's pool can take it; answering 429"`, "pool=" + acmePool}
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

			routeIn(t, r, workToken, "one", tt.pool, nil)

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
			if s, err := r.session("one"); err != nil || s.Assignments[0].Auto != tt.want {
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

			routeIn(t, r, workToken, "one", "", nil)

			if sent := upstream.sent(); !slices.Equal(sent, []string{tt.want}) {
				t.Errorf("the request went out on %q, want %s alone", sent, tt.want)
			}
		})
	}
}

// noteFirstPool has the accounts record in stateDir note the pool with the
// given id as the first.
func noteFirstPool(t *testing.T, stateDir, id string) {
	t.Helper()
	err := identity.NewFile(stateDir).Update(func(r *identity.Record) error {
		r.Pools = append(r.Pools, identity.PoolEntry{ID: id, Added: start, First: true})
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

	routeIn(t, r, workToken, "one", acmePool, nil)

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

	routeIn(t, r, workToken, "one", acmePool, nil)
	unread := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/messages", iotest.ErrReader(errors.New("connection reset")))
	unread.Header.Set("Authorization", "Bearer "+workToken)
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
// returns the path of, keeping its accounts record in stateDir, "" for none.
func newWatchingPoolsRouter(t *testing.T, stateDir string) (*Router, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.Pools, cfg.ConfigFile, cfg.StateDir = pooledAccounts(), testPools, Watch(path), stateDir
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
	routeIn(t, r, workToken, "two", acmePool, nil)

	// Side leaves the config: it's in no pool at once.
	writeConfigFile(t, path, poolsTOML("", defaultTOML, acmeTOML))
	r.upkeep.look()
	routeIn(t, r, workToken, "three", acmePool, nil)

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

// unpooledTOML is a config file of work and personal in no pools, side left
// out, as one put back from before pools.
const unpooledTOML = "[[account]]\nid = \"work\"\nlabel = \"Work\"\n\n[[account]]\nid = \"personal\"\nlabel = \"Personal\"\n"

func TestAConfigThatLosesItsPoolsWhileTheRouterRunsKeepsEachPoolsWorkInIt(t *testing.T) {
	log := logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	// Personal's quota needs using first, then side's, then work's.
	readWeeks(r, map[string]quota.Window{"personal": soonWeek, "side": laterWeek, "work": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	writeConfigFile(t, path, unpooledTOML)
	r.upkeep.look()
	routeIn(t, r, workToken, "one", defaultPool, nil)
	routeIn(t, r, workToken, "two", "", nil)
	// Personal refuses Acme's next request, work's quota still waiting.
	forbidOpus(r, "personal")
	w := routeIn(t, r, workToken, "three", acmePool, nil)

	if sent, want := upstream.sent(), []string{"work", "work"}; !slices.Equal(sent, want) {
		t.Errorf("the requests went out on %q, want %q: Default's on work, side having left the config, and Acme's on none", sent, want)
	}
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Acme's request was answered %d, want switchboard's 429, nothing in Acme to take it", w.Code)
	}
	if n := timesLogged(log, "level=WARN", "the config names no pools"); n != 1 {
		t.Errorf("the log says the config names no pools %d times, want once\n%s", n, log)
	}
}

func TestAThrottledRequestGoesOutAgainOnItsAccountOnlyWhileTheAccountIsInItsPool(t *testing.T) {
	// The request is Default's, and work's quota needs using first. Work
	// throttles it, and moves to Acme in the wait.
	tests := []struct {
		name string
		// side is side's answer.
		side answer
		// wantSent is where the request went out, and wantCode what it was
		// answered.
		wantSent []string
		wantCode int
	}{
		{name: "on side, the pool's next", side: served, wantSent: []string{"work", "side"}, wantCode: http.StatusOK},
		{
			name: "side refusing it, as every account that judged it did: its refusal taken back",
			side: forbidden, wantSent: []string{"work", "side"}, wantCode: http.StatusBadGateway,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				r := newPoolsRouter(t, at(start), true)
				readWeeks(r, map[string]quota.Window{"work": soonWeek, "side": laterWeek, "personal": latestWeek})
				moved := pooledAccounts()
				moved[0].Pool = acmePool
				throttledThenMoved := func(req *http.Request) *http.Response {
					time.AfterFunc(throttleWait/2, func() { r.proxy.pools.follow(moved, testPools) })
					return throttled("")(req)
				}
				upstream := servingAll(throttledThenMoved, served, tt.side)
				r.proxy.transport = upstream
				lines := keepingLedger(t, r)

				w := routeIn(t, r, workToken, "one", defaultPool, nil)

				if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
					t.Errorf("the request went out on %q, want %q: never again on work, Acme's now", sent, tt.wantSent)
				}
				if w.Code != tt.wantCode {
					t.Errorf("answered %d, want %d", w.Code, tt.wantCode)
				}
				if want := []string{"level=INFO", "msg=replaying", "from=work", "to=side", `why="left the pool"`}; !log.Has(want...) {
					t.Errorf("log reads\n%s\nwant a line with %q", log, want)
				}
				if got, want := reasonsOf(lines()), status.ReasonMovedOff+"work "+whyLeftPool; !slices.Equal(got, []string{want}) {
					t.Errorf("the ledger's lines give the reasons %q, want one line, its reason %q", got, want)
				}
				if side, _ := r.Status().Account("side"); side.Refused != (status.Refusal{}) {
					t.Errorf("side's refusal is %+v, want none: work, left for leaving the pool, judged nothing", side.Refused)
				}
			})
		})
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
	pools := newPoolTable(pooledAccounts(), testPools, "")
	none := newPoolTable(config.Accounts(testConfigured), nil, "")
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
		{name: "several", table: newPoolTable(pooledAccounts(), testPools, ""), pool: acmePool, want: "Acme"},
		{name: "several, one not configured: by its id", table: newPoolTable(pooledAccounts(), testPools, ""), pool: "z5k2", want: "z5k2"},
		{name: "one", table: newPoolTable(pooledAccounts(), one, ""), pool: defaultPool, want: ""},
		{name: "none", table: newPoolTable(pooledAccounts(), nil, ""), pool: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.table.shown(tt.pool); got != tt.want {
				t.Errorf("shown(%q) = %q, want %q", tt.pool, got, tt.want)
			}
		})
	}
}

func TestThePoolsAreFollowedAsTheyChangeTheFirstReadAfreshWithThem(t *testing.T) {
	reads := 0
	m := newMembership(func() string {
		reads++
		return acmePool
	})
	accounts := pooledAccounts()

	m.follow(accounts, testPools)
	m.follow(slices.Clone(accounts), slices.Clone(testPools))
	if reads != 1 {
		t.Errorf("the record was read %d times for the same pools, want once", reads)
	}
	if got := m.current().of(""); got != acmePool {
		t.Errorf("a request naming none is in %q, want %s, as the record notes it", got, acmePool)
	}

	relabelled := slices.Clone(testPools)
	relabelled[1].Label = "Acme Corp"
	m.follow(accounts, relabelled)
	if reads != 2 {
		t.Errorf("the record was read %d times, want again for pools relabelled", reads)
	}
	moved := slices.Clone(accounts)
	moved[2].Pool = acmePool
	m.follow(moved, relabelled)
	if reads != 3 || !m.current().holds(acmePool, "side") {
		t.Errorf("the record was read %d times and side in Acme is %v, want read again, side in Acme", reads, m.current().holds(acmePool, "side"))
	}

	refenced := slices.Clone(relabelled)
	refenced[1].Dirs, refenced[1].WrittenDirs = []string{"/work/acme", "/work/acme-ops"}, []string{"/work/acme", "/work/acme-ops"}
	m.follow(moved, refenced)
	if reads != 4 || !slices.Equal(m.current().pools[1].Dirs, refenced[1].Dirs) {
		t.Errorf("the record was read %d times and Acme is fenced to %q, want read again, fenced to %q, its directories alone changed",
			reads, m.current().pools[1].Dirs, refenced[1].Dirs)
	}
	primaries := slices.Clone(moved)
	primaries[0].Primary, primaries[2].Primary = false, true
	m.follow(primaries, refenced)
	if reads != 5 {
		t.Errorf("the record was read %d times, want again for primaries alone changed", reads)
	}
}

func TestAConfigThatLosesItsPoolsKeepsThemOnceTheMoveIsDone(t *testing.T) {
	// The config loses its pools and side with them: work and personal are
	// configured, in no pool.
	lost := config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal", Label: "Personal"}}
	tests := []struct {
		name string
		// first is the first pool the accounts record notes, "" for none;
		// pooled has the router follow the pools before they're lost.
		first  string
		pooled bool
		// pool is what each account is in once the pools are lost, by id, ""
		// for none, where a request naming pool goes.
		pool string
		want map[string]bool
		// warned is whether the log says the config names no pools.
		warned bool
	}{
		{
			name: "before the move: every account in the one pool there is, as today",
			pool: acmePool, want: map[string]bool{"work": true, "personal": true, "side": true},
		},
		{
			name: "once the router has followed pools: they stand, but for side, which left",
			pool: defaultPool, pooled: true, want: map[string]bool{"work": true, "personal": false, "side": false}, warned: true,
		},
		{
			name: "once the router has followed pools: Acme stands",
			pool: acmePool, pooled: true, want: map[string]bool{"work": false, "personal": true, "side": false}, warned: true,
		},
		{
			name: "started on it once the record notes a first: every account in that pool, as the restart heals the config",
			pool: defaultPool, first: defaultPool, want: map[string]bool{"work": true, "personal": true, "side": false}, warned: true,
		},
		{
			name: "started on it once the record notes a first: no account in another",
			pool: acmePool, first: defaultPool, want: map[string]bool{"work": false, "personal": false, "side": false}, warned: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			m := newMembership(func() string { return tt.first })
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
	m := newMembership(noFirst)

	for range 2 {
		m.follow(pooledAccounts(), testPools)
		m.follow(pooledAccounts(), nil)
	}

	if n := timesLogged(log, "level=WARN", "the config names no pools"); n != 2 {
		t.Errorf("the log says the config names no pools %d times, want twice, once each time it lost them\n%s", n, log)
	}
}
