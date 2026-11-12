package router

import (
	"bufio"
	"cmp"
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// The configs a reader of the pools is answered in: no pools, as from before
// them; every account in Default alone; Default unfenced beside Acme, fenced,
// as testPools has them; and both fenced.
var (
	onePool        = config.Pools{testPools[0]}
	bothFenced     = config.Pools{{ID: defaultPool, Label: "Default", Dirs: []string{"/work/default"}, WrittenDirs: []string{"/work/default"}}, testPools[1]}
	unconfigured   = "z5k2"
	poolsNamedSaid = defaultPool + " (Default) or " + acmePool + " (Acme)"
)

// inDefault is pooledAccounts with every account in Default, work its
// primary.
func inDefault() config.Accounts {
	accounts := pooledAccounts()
	accounts[1].Pool, accounts[1].Primary = defaultPool, false
	return accounts
}

// pooledRouter builds a router on start's time of accounts in pools, each
// account with a token, serving every request on any account.
func pooledRouter(t *testing.T, accounts config.Accounts, pools config.Pools) *Router {
	t.Helper()
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.Pools = accounts, pools
	r := newTestRouterOf(t, cfg)
	r.proxy.transport = servingAll(served, served, served)
	return r
}

// askControl has r's control API answer a request of method for target, with
// body, and returns its answer. A request for the stream comes with its
// context ended, so the stream answers with the requests in flight alone.
func askControl(t *testing.T, r *Router, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	ctx := t.Context()
	if strings.HasPrefix(target, "/stream") {
		ended, cancel := context.WithCancel(ctx)
		cancel()
		ctx = ended
	}
	rec := httptest.NewRecorder()
	r.Control().ServeHTTP(rec, httptest.NewRequestWithContext(ctx, method, target, strings.NewReader(body)))
	return rec
}

// statusIn returns r's status document of the pool given, as GET /status
// answers asking for it, "" for none.
func statusIn(t *testing.T, r *Router, pool string) status.Document {
	t.Helper()
	var doc status.Document
	decodedOK(t, askControl(t, r, http.MethodGet, withPool("/status", pool), ""), &doc)
	return doc
}

// withPool is target asking for the pool given, as ?pool= names it, where
// it's not "".
func withPool(target, pool string) string {
	if pool == "" {
		return target
	}
	joined := "?"
	if strings.Contains(target, "?") {
		joined = "&"
	}
	return target + joined + PoolQuery + "=" + pool
}

// decodedOK decodes rec's answer into v, failing the test unless it's 200.
func decodedOK(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("answered %d %s, want 200", rec.Code, rec.Body)
	}
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("answered %s: %v", rec.Body, err)
	}
}

// idsOf returns the id of each account given, in order.
func idsOf(accounts []status.Account) []string {
	ids := []string{}
	for _, a := range accounts {
		ids = append(ids, a.ID)
	}
	return ids
}

// routeOneIn has r route a session in each pool given, by its id, "" for
// none: the session's id is the pool's, else "none".
func routeOneIn(t *testing.T, r *Router, pools ...string) {
	t.Helper()
	tokens := map[string]string{"": workToken, defaultPool: workToken, acmePool: personalToken}
	for _, pool := range pools {
		routeIn(t, r, tokens[pool], cmp.Or(pool, "none"), pool, nil)
	}
}

// configuredIn returns the ids of the accounts given in the pool with the
// given id, in order: every one, where it's "".
func configuredIn(accounts config.Accounts, pool string) []string {
	ids := []string{}
	for _, a := range accounts {
		if pool == "" || a.Pool == pool {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// sessionIDs returns the id of each session given, in order.
func sessionIDs(sessions []status.Session) []string {
	ids := []string{}
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	return ids
}

// historyIDs returns the id of each account a history gives, in order.
func historyIDs(h History) []string {
	ids := []string{}
	for _, a := range h.Accounts {
		ids = append(ids, a.ID)
	}
	return ids
}

func TestEachReadIsAnsweredForThePoolItAsksForElseTheOneTheVerbsActOn(t *testing.T) {
	logstest.Capture(t)
	tests := []struct {
		name     string
		accounts config.Accounts
		pools    config.Pools
		// routed are the pools a session is routed in, "" for none.
		routed []string
		asked  string
		// want is the id of the pool answered for, "" for none, and sessions
		// the sessions it lists; refused, how the read is refused, naming the
		// pools, "" where it's answered.
		want     string
		sessions []string
		refused  string
	}{
		{name: "no pools, none asked", accounts: testConfigured, routed: []string{""}, sessions: []string{"none"}},
		{name: "no pools, one asked: as without", accounts: testConfigured, routed: []string{""}, asked: defaultPool, sessions: []string{"none"}},
		{name: "no pools, one not configured asked: as without", accounts: testConfigured, routed: []string{""}, asked: unconfigured, sessions: []string{"none"}},
		{name: "one pool, none asked: the only one", accounts: inDefault(), pools: onePool, routed: []string{defaultPool}, want: defaultPool, sessions: []string{defaultPool}},
		{name: "one pool, it asked", accounts: inDefault(), pools: onePool, routed: []string{defaultPool}, asked: defaultPool, want: defaultPool, sessions: []string{defaultPool}},
		{name: "one pool, one not configured asked", accounts: inDefault(), pools: onePool, asked: acmePool, refused: defaultPool + " (Default)"},
		{name: "one unfenced, none asked: the unfenced", accounts: pooledAccounts(), pools: testPools, routed: []string{defaultPool, acmePool}, want: defaultPool, sessions: []string{defaultPool}},
		{name: "one unfenced, the fenced asked", accounts: pooledAccounts(), pools: testPools, routed: []string{defaultPool, acmePool}, asked: acmePool, want: acmePool, sessions: []string{acmePool}},
		{name: "one unfenced, asked ignoring case", accounts: pooledAccounts(), pools: testPools, routed: []string{defaultPool, acmePool}, asked: "Q7MT", want: acmePool, sessions: []string{acmePool}},
		{name: "one unfenced, one not configured asked", accounts: pooledAccounts(), pools: testPools, asked: unconfigured, refused: poolsNamedSaid},
		{name: "both fenced, none asked", accounts: pooledAccounts(), pools: bothFenced, refused: poolsNamedSaid},
		{name: "both fenced, one asked", accounts: pooledAccounts(), pools: bothFenced, routed: []string{defaultPool, acmePool}, asked: defaultPool, want: defaultPool, sessions: []string{defaultPool}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := pooledRouter(t, tt.accounts, tt.pools)
			routeOneIn(t, r, tt.routed...)
			if tt.refused != "" {
				for _, read := range poolReads {
					rec := askControl(t, r, read.method, withPool(read.target, tt.asked), read.body)
					if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.refused) || !strings.Contains(rec.Body.String(), `"pools":[`) {
						t.Errorf("%s %s answered %d %s, want 400 naming the pools, as %q, and giving them", read.method, read.target, rec.Code, rec.Body, tt.refused)
					}
				}
				return
			}
			accounts := configuredIn(tt.accounts, tt.want)
			for _, target := range []string{"/status", "/refresh", "/pin"} {
				method := map[string]string{"/status": http.MethodGet, "/refresh": http.MethodPost, "/pin": http.MethodDelete}[target]
				var doc status.Document
				decodedOK(t, askControl(t, r, method, withPool(target, tt.asked), `{"max_age": "30m"}`), &doc)
				if doc.PoolID != tt.want || !slices.Equal(idsOf(doc.Accounts), accounts) {
					t.Errorf("%s %s answered for pool %q, of %q; want %q, of %q", method, target, doc.PoolID, idsOf(doc.Accounts), tt.want, accounts)
				}
			}
			var listed []status.Session
			decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions", tt.asked), ""), &listed)
			if got := sessionIDs(listed); !slices.Equal(got, tt.sessions) {
				t.Errorf("GET /sessions listed %q, want %q", got, tt.sessions)
			}
			var h History
			decodedOK(t, askControl(t, r, http.MethodGet, withPool("/history?window=7d&step=1h", tt.asked), ""), &h)
			if got := historyIDs(h); !slices.Equal(got, accounts) {
				t.Errorf("GET /history gave the windows of %q, want %q", got, accounts)
			}
			if rec := askControl(t, r, http.MethodGet, withPool("/stream", tt.asked), ""); rec.Code != http.StatusOK {
				t.Errorf("GET /stream answered %d %s, want 200", rec.Code, rec.Body)
			}
		})
	}
}

// poolReads are the reads of a pool, as askControl asks each.
var poolReads = []struct{ method, target, body string }{
	{http.MethodGet, "/status", ""},
	{http.MethodGet, "/sessions", ""},
	{http.MethodGet, "/history?window=7d&step=1h", ""},
	{http.MethodGet, "/stream", ""},
	{http.MethodPost, "/refresh", `{"max_age": "30m"}`},
	{http.MethodDelete, "/pin", ""},
}

func TestAPinIsAnsweredForThePoolOfTheAccountsItPins(t *testing.T) {
	logstest.Capture(t)
	tests := []struct {
		name, asked, body string
		// want is the pool answered for, and pinned the accounts the pin
		// names after; refused, how it's refused, "" where it isn't.
		want    string
		pinned  []string
		refused string
	}{
		{name: "a fenced pool's account, every pool fenced", body: `{"accounts": ["personal"]}`, want: acmePool, pinned: []string{"personal"}},
		{name: "two of one pool", body: `{"accounts": ["side", "work"]}`, want: defaultPool, pinned: []string{"work", "side"}},
		{name: "its pool asked", asked: acmePool, body: `{"accounts": ["personal"]}`, want: acmePool, pinned: []string{"personal"}},
		{
			name: "accounts of two pools", body: `{"accounts": ["work", "personal"]}`, pinned: []string{"side"},
			refused: "pin accounts of one pool: work is in pool Default, personal is in pool Acme",
		},
		{name: "another pool asked", asked: defaultPool, body: `{"accounts": ["personal"]}`, pinned: []string{"side"}, refused: "are in pool Acme, not Default"},
		{name: "one not configured asked", asked: unconfigured, body: `{"accounts": ["personal"]}`, pinned: []string{"side"}, refused: poolsNamedSaid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := pooledRouter(t, pooledAccounts(), bothFenced)
			r.pinTo(status.Pin{Accounts: []string{"side"}, Since: start}, false)

			rec := askControl(t, r, http.MethodPost, withPool("/pin", tt.asked), tt.body)
			if tt.refused != "" {
				if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.refused) {
					t.Errorf("POST /pin answered %d %s, want 400 saying %q", rec.Code, rec.Body, tt.refused)
				}
			} else {
				var doc status.Document
				decodedOK(t, rec, &doc)
				if doc.PoolID != tt.want || !slices.Equal(doc.Pin.Accounts, tt.pinned) {
					t.Errorf("POST /pin answered for pool %q, its pin %q; want %q, %q", doc.PoolID, doc.Pin.Accounts, tt.want, tt.pinned)
				}
			}
			if got := r.sessions.globalPin(); !slices.Equal(got.Accounts, tt.pinned) {
				t.Errorf("the pin is %+v, want of %q", got, tt.pinned)
			}
		})
	}
}

func TestARefusedUnpinChangesNothing(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), bothFenced)
	r.pinTo(status.Pin{Accounts: []string{"side"}, Since: start}, false)

	askControl(t, r, http.MethodDelete, "/pin", "")
	if got := r.sessions.globalPin(); !slices.Equal(got.Accounts, []string{"side"}) {
		t.Errorf("the pin is %+v once unpinning was refused, want side's as it was", got)
	}
}

func TestEachPoolsDocumentHoldsItsOwnAccountsBestPrimaryRoomAndSessions(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), testPools)
	// Personal's quota needs using first, but it's in Acme alone: in Default,
	// work's does.
	readPersonalFirst(r)
	routeIn(t, r, workToken, "one", defaultPool, nil)
	routeIn(t, r, personalToken, "two", acmePool, nil)
	routeIn(t, r, personalToken, "three", acmePool, nil)

	tests := []struct {
		pool, label   string
		accounts      []string
		best, primary string
		room          []string
		sessions      map[string]int
		all           int
	}{
		{
			pool: defaultPool, label: "Default", accounts: []string{"work", "side"}, best: "work", primary: "work",
			room: []string{"work", "side"}, sessions: map[string]int{"work": 1, "side": 0}, all: 1,
		},
		// With one account, the pool's room is the account's own.
		{
			pool: acmePool, label: "Acme", accounts: []string{"personal"}, best: "personal", primary: "personal",
			sessions: map[string]int{"personal": 2}, all: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			doc := statusIn(t, r, tt.pool)
			if doc.PoolID != tt.pool || doc.PoolLabel != tt.label {
				t.Errorf("the document names pool %q (%q), want %q (%q)", doc.PoolID, doc.PoolLabel, tt.pool, tt.label)
			}
			if got := idsOf(doc.Accounts); !slices.Equal(got, tt.accounts) {
				t.Errorf("the document holds %q, want %q", got, tt.accounts)
			}
			if doc.Best != tt.best || doc.Primary != tt.primary {
				t.Errorf("the document's best is %q and primary %q, want %q and %q", doc.Best, doc.Primary, tt.best, tt.primary)
			}
			if !slices.Equal(doc.Pool.Accounts, tt.room) {
				t.Errorf("the document's room is of %q, want %q", doc.Pool.Accounts, tt.room)
			}
			if doc.Sessions != tt.all {
				t.Errorf("the document counts %d sessions, want %d", doc.Sessions, tt.all)
			}
			for _, a := range doc.Accounts {
				if a.Sessions != tt.sessions[a.ID] {
					t.Errorf("%s has %d sessions, want %d", a.ID, a.Sessions, tt.sessions[a.ID])
				}
			}
		})
	}
}

func TestEachPoolsDocumentHoldsTheGlobalPinAsItHoldsInThePool(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), testPools)
	readPersonalFirst(r)
	r.pinTo(status.Pin{Accounts: []string{"personal", "side"}, Since: start, Move: true}, false)

	tests := []struct {
		pool string
		want status.Pin
		best string
	}{
		{pool: defaultPool, want: status.Pin{Accounts: []string{"side"}, Since: start, Move: true}, best: "side"},
		{pool: acmePool, want: status.Pin{Accounts: []string{"personal"}, Since: start, Move: true}, best: "personal"},
	}
	for _, tt := range tests {
		if doc := statusIn(t, r, tt.pool); !reflect.DeepEqual(doc.Pin, tt.want) || doc.Best != tt.best {
			t.Errorf("pool %s's document holds pin %+v, best %q, want %+v, best %q", tt.pool, doc.Pin, doc.Best, tt.want, tt.best)
		}
	}

	// A pin naming none of a pool's accounts is no pin there.
	r.pinTo(status.Pin{Accounts: []string{"personal"}, Since: start}, false)
	if doc := statusIn(t, r, defaultPool); !doc.Pin.IsZero() || doc.Pool.Pinned || doc.Best != "work" {
		t.Errorf("Default's document holds pin %+v, its room pinned %v, best %q, with Acme's account pinned alone; want no pin, best work",
			doc.Pin, doc.Pool.Pinned, doc.Best)
	}
}

func TestEachPoolsDocumentShowsTheEventsOfItsPoolItsAccountsAndNone(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), testPools)
	happened := []status.Event{
		{Kind: status.EventRestart, Reason: "the config changed"},
		{Kind: status.EventPin, Accounts: []string{"work"}, Pool: defaultPool},
		{Kind: status.EventLimit, Account: "personal"},
		{Kind: status.EventPin, Account: "personal", Accounts: []string{"personal"}, Pool: acmePool},
		{Kind: status.EventPressure, Account: "side"},
		{Kind: status.EventAuto, Accounts: []string{"personal"}},
		// A move with no pool goes by the account it moved to.
		{Kind: status.EventMoved, Session: "one", From: "personal", To: "work"},
		{Kind: status.EventMoved, Session: "two", To: "personal"},
		// An account that has left the config, held till the restart, is in
		// no pool, and so is a pool that has left it: an event naming either
		// goes by what else it names.
		{Kind: status.EventCap, Account: "lapsed"},
		{Kind: status.EventAuto, Pool: unconfigured},
		{Kind: status.EventPin, Accounts: []string{"personal"}, Pool: unconfigured},
	}
	r.recent.change(func() {
		for _, e := range happened {
			r.recent.add(e, start)
		}
	})

	tests := []struct {
		pool string
		want []string
	}{
		{pool: defaultPool, want: []string{status.EventAuto, status.EventCap, status.EventMoved, status.EventPressure, status.EventPin, status.EventRestart}},
		{pool: acmePool, want: []string{status.EventPin, status.EventAuto, status.EventCap, status.EventMoved, status.EventAuto, status.EventPin, status.EventLimit, status.EventRestart}},
	}
	for _, tt := range tests {
		var kinds []string
		for _, e := range statusIn(t, r, tt.pool).Events {
			kinds = append(kinds, e.Kind)
		}
		if !slices.Equal(kinds, tt.want) {
			t.Errorf("pool %s's document shows the events %q, want %q", tt.pool, kinds, tt.want)
		}
	}
}

func TestEachPoolsDocumentShowsItsOwnAccountsPrimes(t *testing.T) {
	clock := &testClock{now: onDay(1, 5, 0)}
	cfg := testConfig(clock.read, &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.Pools, cfg.Prime, cfg.Policy = pooledAccounts(), testPools, daytime, testPolicy
	r := newTestRouterOf(t, cfg)

	tests := []struct {
		pool string
		want []string
	}{
		{pool: defaultPool, want: []string{"work", "side"}},
		{pool: acmePool, want: []string{"personal"}},
	}
	for _, tt := range tests {
		var primed []string
		for _, slot := range statusIn(t, r, tt.pool).Prime.Slots {
			primed = append(primed, slot.Account)
		}
		if !slices.Equal(primed, tt.want) {
			t.Errorf("pool %s's document primes %q, want %q", tt.pool, primed, tt.want)
		}
	}
}

func TestAPoolWithNoAccountsHasADocumentOfNone(t *testing.T) {
	logstest.Capture(t)
	spare := config.Pool{ID: unconfigured, Label: "Spare", Dirs: []string{"/work/spare"}, WrittenDirs: []string{"/work/spare"}}
	r := pooledRouter(t, pooledAccounts(), append(slices.Clone(testPools), spare))
	readPersonalFirst(r)
	routeOneIn(t, r, defaultPool, acmePool)

	rec := askControl(t, r, http.MethodGet, withPool("/status", unconfigured), "")
	var doc status.Document
	decodedOK(t, rec, &doc)
	if doc.PoolID != unconfigured || doc.PoolLabel != "Spare" || len(doc.Accounts) != 0 || doc.Best != "" || doc.Primary != "" ||
		doc.Sessions != 0 || len(doc.Pool.Accounts) != 0 || len(doc.ComingUp) != 0 {
		t.Errorf("the empty pool's document is %+v, want Spare's, of nothing", doc)
	}
	if !strings.Contains(rec.Body.String(), `"accounts":[]`) {
		t.Errorf("the empty pool's document is %s, want its accounts given, as none", rec.Body)
	}
}

// primaryTOML is a config file of pooledAccounts in testPools, but side in
// the pool given, its primary, and an account added beside, extra, in
// Default, with a spare pool after them.
func primaryTOML(sidePool string) string {
	return "[[account]]\nid = \"work\"\nlabel = \"Work\"\npool = \"" + defaultPool + "\"\n" +
		"\n[[account]]\nid = \"personal\"\nlabel = \"Personal\"\npool = \"" + acmePool + "\"\n" +
		"\n[[account]]\nid = \"side\"\nlabel = \"Side\"\npool = \"" + sidePool + "\"\nprimary = true\n" +
		"\n[[account]]\nid = \"extra\"\nlabel = \"Extra\"\npool = \"" + defaultPool + "\"\n" +
		"\n" + defaultTOML + "\n" + acmeTOML + "\n" + sparePool("Spare")
}

// healthOf returns r's health answer.
func healthOf(t *testing.T, r *Router) Health {
	t.Helper()
	var h Health
	decodedOK(t, askControl(t, r, http.MethodGet, "/health", ""), &h)
	return h
}

func TestTheHealthAnswerReportsThePoolsAsTheRouterRoutesForThemNow(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	defaultRouted := RoutedPool{ID: defaultPool, Label: "Default", Accounts: []string{"work", "side"}, Primary: "work"}
	acmeRouted := RoutedPool{ID: acmePool, Label: "Acme", Dirs: []string{"/work/acme"}, WrittenDirs: []string{"/work/acme"}, Accounts: []string{"personal"}, Primary: "personal"}
	if h := healthOf(t, r); !h.ReportsPools || !reflect.DeepEqual(h.Pools, []RoutedPool{defaultRouted, acmeRouted}) {
		t.Errorf("GET /health reports pools %+v (reports them: %v), want %+v", h.Pools, h.ReportsPools, []RoutedPool{defaultRouted, acmeRouted})
	}

	// Side moves to Acme as its primary, a pool is added, and an account
	// added to Default waits for the router's restart, so isn't among its
	// accounts.
	writeConfigFile(t, path, primaryTOML(acmePool))
	r.upkeep.look()
	want := []RoutedPool{
		{ID: defaultPool, Label: "Default", Accounts: []string{"work"}, Primary: "work"},
		{ID: acmePool, Label: "Acme", Dirs: []string{"/work/acme"}, WrittenDirs: []string{"/work/acme"}, Accounts: []string{"personal", "side"}, Primary: "side"},
		{ID: unconfigured, Label: "Spare", Dirs: []string{"/work/spare"}, WrittenDirs: []string{"/work/spare"}, Accounts: []string{}},
	}
	if h := healthOf(t, r); !reflect.DeepEqual(h.Pools, want) {
		t.Errorf("once the config changed, GET /health reports pools %+v, want %+v", h.Pools, want)
	}
	if doc := statusIn(t, r, acmePool); doc.Primary != "side" || !slices.Equal(primariesOf(doc), []string{"side"}) {
		t.Errorf("once the config changed, Acme's document has primary %q, accounts marked primary %q, want side", doc.Primary, primariesOf(doc))
	}
}

// primariesOf returns the ids of the document's accounts marked primary.
func primariesOf(doc status.Document) []string {
	var ids []string
	for _, a := range doc.Accounts {
		if a.Primary {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

func TestAPoolsPrimaryTheRouterDoesntHaveIsItsFirstItDoes(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	// Extra, waiting for the restart, is made Default's primary in side's
	// place.
	unmarked := strings.Replace(primaryTOML(defaultPool), "primary = true\n", "", 1)
	writeConfigFile(t, path, strings.Replace(unmarked, "label = \"Extra\"\n", "label = \"Extra\"\nprimary = true\n", 1))
	r.upkeep.look()

	h := healthOf(t, r)
	if i := slices.IndexFunc(h.Pools, func(p RoutedPool) bool { return p.ID == defaultPool }); i < 0 || h.Pools[i].Primary != "work" {
		t.Errorf("GET /health reports pools %+v, want Default's primary work, its first the router has", h.Pools)
	}
}

func TestAConfigWithNoPoolsHasTheHealthAnswerReportNone(t *testing.T) {
	r := pooledRouter(t, testConfigured, nil)
	rec := askControl(t, r, http.MethodGet, "/health", "")
	if body := rec.Body.String(); !strings.Contains(body, `"reports_pools":true`) || strings.Contains(body, `"pools"`) {
		t.Errorf("GET /health answered %s, want it to say it reports pools, and none", body)
	}
}

func TestWorkIsReadByThePoolRecordedAndAccountsByThePoolTheyreInNow(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
	routeIn(t, r, workToken, "one", defaultPool, nil)

	// Side, which Default's session went to, moves to Acme partway through
	// the day.
	writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
	r.upkeep.look()

	var listed []status.Session
	decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions", defaultPool), ""), &listed)
	if len(listed) != 1 || listed[0].ID != "one" || listed[0].Assignments[0].Account != "side" {
		t.Errorf("Default lists the sessions %+v, want one, on side, as it was routed in Default", listed)
	}
	decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions", acmePool), ""), &listed)
	if len(listed) != 0 {
		t.Errorf("Acme lists the sessions %+v, want none", listed)
	}

	tests := []struct {
		pool     string
		accounts []string
		sessions int
	}{
		{pool: defaultPool, accounts: []string{"work"}, sessions: 1},
		{pool: acmePool, accounts: []string{"personal", "side"}, sessions: 0},
	}
	for _, tt := range tests {
		var h History
		decodedOK(t, askControl(t, r, http.MethodGet, withPool("/history?window=7d&step=1h", tt.pool), ""), &h)
		var windowed []string
		for _, a := range h.Accounts {
			windowed = append(windowed, a.ID)
		}
		if !slices.Equal(windowed, tt.accounts) {
			t.Errorf("pool %s's history of the week is of %q, want %q", tt.pool, windowed, tt.accounts)
		}
		doc := statusIn(t, r, tt.pool)
		if got := idsOf(doc.Accounts); !slices.Equal(got, tt.accounts) || doc.Sessions != tt.sessions {
			t.Errorf("pool %s's document holds %q and %d sessions, want %q and %d", tt.pool, got, doc.Sessions, tt.accounts, tt.sessions)
		}
		for _, a := range doc.Accounts {
			if a.Sessions != 0 {
				t.Errorf("pool %s's %s has %d sessions, want none: the session on it was routed in Default", tt.pool, a.ID, a.Sessions)
			}
		}
	}
}

func TestASessionIsAnsweredForInItsOwnPoolUnlessAnotherIsAskedFor(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), bothFenced)
	routeIn(t, r, personalToken, "two", acmePool, nil)

	tests := []struct {
		asked string
		want  int
	}{
		{asked: "", want: http.StatusOK},
		{asked: acmePool, want: http.StatusOK},
		{asked: defaultPool, want: http.StatusNotFound},
		{asked: unconfigured, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		if rec := askControl(t, r, http.MethodGet, withPool("/sessions/two", tt.asked), ""); rec.Code != tt.want {
			t.Errorf("GET /sessions/two asking for pool %q answered %d %s, want %d", tt.asked, rec.Code, rec.Body, tt.want)
		}
	}
}

func TestTheStreamTellsOfTheRequestsRoutedWithinThePoolAsked(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), testPools)
	srv := httptest.NewServer(r.Control())
	t.Cleanup(srv.Close)
	acme := openStream(t, srv, acmePool)

	routeIn(t, r, workToken, "one", defaultPool, nil)
	routeIn(t, r, personalToken, "two", acmePool, nil)
	for _, e := range streamedTillDone(t, acme) {
		if e.Pool != acmePool || e.Session != "two" {
			t.Errorf("Acme's stream told of %+v, want Acme's request alone", e)
		}
	}
}

// streamedTillDone reads the events of the stream from body until one is
// done, and returns them.
func streamedTillDone(t *testing.T, body io.Reader) []StreamEvent {
	t.Helper()
	var events []StreamEvent
	lines := bufio.NewScanner(body)
	for lines.Scan() {
		var e StreamEvent
		if err := json.Unmarshal(lines.Bytes(), &e); err != nil {
			t.Fatalf("the stream held %s: %v", lines.Bytes(), err)
		}
		if events = append(events, e); e.Kind == StreamDone {
			return events
		}
	}
	t.Fatalf("the stream ended after %+v, before a request was done", events)
	return nil
}

// The bodies in testdata/nopools were recorded from the router as it was
// before it answered for a pool, from this test's history.
func TestAConfigWithNoPoolsIsAnsweredAsBeforeWhateverPoolIsAsked(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, testConfigured, nil)
	readPersonalFirst(r)
	routeIn(t, r, workToken, "one", "", nil)

	recorded := map[string]string{
		"/status": "status.json", "/sessions": "sessions.json", "/sessions/one": "session.json", "/history?window=7d&step=1h": "history.json",
	}
	for target, file := range recorded {
		want, err := os.ReadFile(filepath.Join("testdata", "nopools", file))
		if err != nil {
			t.Fatal(err)
		}
		for _, asked := range []string{"", defaultPool} {
			if got := askControl(t, r, http.MethodGet, withPool(target, asked), "").Body.String(); got != string(want) {
				t.Errorf("GET %s asking for pool %q answered\n%s\nwant as before pools\n%s", target, asked, got, want)
			}
		}
	}
}

func TestTheStreamOpensWithThePoolsRequestsInFlightAlone(t *testing.T) {
	r := pooledRouter(t, pooledAccounts(), testPools)
	r.stream.publishOf(StreamEvent{Kind: StreamSent, Request: "1", Session: "one", Pool: defaultPool}, key{session: "one"})
	r.stream.publishOf(StreamEvent{Kind: StreamSent, Request: "2", Session: "two", Pool: acmePool}, key{session: "two"})

	body := askControl(t, r, http.MethodGet, withPool("/stream", acmePool), "").Body
	var told []string
	for line := range strings.Lines(body.String()) {
		var e StreamEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		told = append(told, e.Kind+" "+e.Request)
	}
	if want := []string{StreamInFlight + " 2"}; !slices.Equal(told, want) {
		t.Errorf("Acme's stream opened telling of %q, want %q", told, want)
	}
}

// streamWait is how long openStream keeps a stream open at most.
const streamWait = 30 * time.Second

// openStream opens r's request stream, served by srv, asking for the pool
// given, "" for none, and returns its body, open till the test ends, or for
// streamWait at most, so a stream that goes on where it should have ended
// fails the test rather than holding it till the run's timeout.
func openStream(t *testing.T, srv *httptest.Server, pool string) io.ReadCloser {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), streamWait)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+withPool("/stream", pool), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp.Body
}

func TestAStreamThatNamedNoPoolEndsOnceThePoolTheVerbsActOnIsAnother(t *testing.T) {
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.ConfigFile = Watch(path)
	r := newTestRouterOf(t, cfg)
	r.proxy.transport = servingAll(served, served, served)
	srv := httptest.NewServer(r.Control())
	t.Cleanup(srv.Close)
	unnamed := openStream(t, srv, "")

	// The config comes to have pools: the stream that named none, of every
	// account, ends, to be joined again for Default's.
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	if rest, err := io.ReadAll(unnamed); err != nil || len(rest) != 0 {
		t.Errorf("the stream that named no pool told of %q, %v, once the config came to have pools, want it ended", rest, err)
	}

	// Default and Acme swap their fences: one named stays open, one that
	// named none ends.
	named, unnamed := openStream(t, srv, defaultPool), openStream(t, srv, "")
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML+"dirs = [\"/work/default\"]\n", strings.Replace(acmeTOML, "dirs = [\"/work/acme\"]\n", "", 1)))
	r.upkeep.look()
	if rest, err := io.ReadAll(unnamed); err != nil || len(rest) != 0 {
		t.Errorf("the stream that named no pool told of %q, %v, once Acme was the unfenced pool, want it ended", rest, err)
	}
	routeIn(t, r, workToken, "one", defaultPool, nil)
	if told := streamedTillDone(t, named); told[0].Pool != defaultPool {
		t.Errorf("Default's stream told of %+v, want Default's request", told)
	}
}

func TestARefreshOfAPoolProbesItsAccountsAlone(t *testing.T) {
	logstest.Capture(t)
	prober := &stubProber{}
	cfg := testConfig(at(start), prober, poolTokens.Read)
	cfg.Accounts, cfg.Pools = pooledAccounts(), testPools
	r := newTestRouterOf(t, cfg)

	askControl(t, r, http.MethodPost, withPool("/refresh", acmePool), `{"max_age": "0s"}`)
	prober.mu.Lock()
	defer prober.mu.Unlock()
	if want := map[string]int{personalToken: 1}; !maps.Equal(prober.probes, want) {
		t.Errorf("a refresh of Acme probed %v, by token, want Acme's account alone", prober.probes)
	}
}

func TestASessionsAccountIsMarkedPrimaryAsItsPoolsPrimaryIsNow(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
	routeIn(t, r, workToken, "one", defaultPool, nil)

	// Side, which the session is on, is made Default's primary in work's
	// place.
	writeConfigFile(t, path, primaryTOML(defaultPool))
	r.upkeep.look()
	var s status.Session
	decodedOK(t, askControl(t, r, http.MethodGet, "/sessions/one", ""), &s)
	if s.Account.ID != "side" || !s.Account.Primary {
		t.Errorf("the session's account is %s, marked primary: %v; want side, Default's primary now", s.Account.ID, s.Account.Primary)
	}
}

func TestAPoolNamedWithoutAValueNamesNone(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), bothFenced)
	routeIn(t, r, personalToken, "two", acmePool, nil)
	if rec := askControl(t, r, http.MethodGet, "/sessions/two?pool=", ""); rec.Code != http.StatusOK {
		t.Errorf("GET /sessions/two?pool= answered %d %s, want 200, the session in its own pool", rec.Code, rec.Body)
	}
}

func TestThePoolsKeptOfAConfigNamingNoneKeepTheirLabelsFromTheAccountsRecord(t *testing.T) {
	logstest.Capture(t)
	first := identity.PoolEntry{ID: defaultPool, Added: start, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}}
	m := newMembership(readingRecord(identity.Record{Pools: []identity.PoolEntry{first}}), alwaysMoved, sameID)
	m.follow(config.Accounts(testConfigured), nil)
	if got := m.current().reported(nil); len(got) != 1 || got[0].ID != defaultPool || got[0].Label != "Default" {
		t.Errorf("the kept pools report %+v, want Default by its label", got)
	}
}

func TestTheHealthAnswerSaysHowTheMoveStands(t *testing.T) {
	r := pooledRouter(t, pooledAccounts(), testPools)
	if h, doc := healthOf(t, r), r.Status(); !reflect.DeepEqual(h.Moving, doc.Moving) {
		t.Errorf("GET /health says the move stands %+v, want as the document says, %+v", h.Moving, doc.Moving)
	}
}

func TestASessionsAccountCountsEveryOneOnItWithPools(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), bothFenced)
	routeIn(t, r, personalToken, "two", acmePool, nil)
	routeIn(t, r, personalToken, "three", acmePool, nil)

	var s status.Session
	decodedOK(t, askControl(t, r, http.MethodGet, "/sessions/two", ""), &s)
	if s.Account.ID != "personal" || s.Account.Sessions != 2 {
		t.Errorf("the session's account is %s with %d sessions, want personal with 2", s.Account.ID, s.Account.Sessions)
	}
}

func TestAStreamThatNamedAPoolEndsOnceThePoolsAnswerForItWithAnother(t *testing.T) {
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.ConfigFile = Watch(path)
	r := newTestRouterOf(t, cfg)
	r.proxy.transport = servingAll(served, served, served)
	srv := httptest.NewServer(r.Control())
	t.Cleanup(srv.Close)

	// Acme, named before there were pools, was answered as the one pool there
	// was, every account's.
	beforePools := openStream(t, srv, acmePool)
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML, sparePool("Spare")))
	r.upkeep.look()
	if rest, err := io.ReadAll(beforePools); err != nil || len(rest) != 0 {
		t.Errorf("the stream that named Acme before pools told of %q, %v, once the config came to have them, want it ended", rest, err)
	}

	spare := openStream(t, srv, unconfigured)
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	if rest, err := io.ReadAll(spare); err != nil || len(rest) != 0 {
		t.Errorf("the stream that named Spare told of %q, %v, once Spare was gone, want it ended", rest, err)
	}
}

func TestASessionsStartAndMoveShowInThePoolItsRequestsWereRoutedWithin(t *testing.T) {
	tests := []struct {
		name string
		// config is the config file side's session moves off it under; after,
		// where it's not "", the one that follows the move.
		config, after string
	}{
		{name: "side moved to Acme", config: poolsTOML(acmePool, defaultTOML, acmeTOML)},
		{name: "side dropped from the config", config: poolsTOML("", defaultTOML, acmeTOML)},
		{
			name: "work, which it moved to, moved to Acme since", config: poolsTOML(acmePool, defaultTOML, acmeTOML),
			after: strings.Replace(poolsTOML(acmePool, defaultTOML, acmeTOML), "pool = \""+defaultPool+"\"", "pool = \""+acmePool+"\"", 1),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r, path := newWatchingPoolsRouter(t, "")
			readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
			routeIn(t, r, sideToken, "one", defaultPool, nil)

			writeConfigFile(t, path, tt.config)
			r.upkeep.look()
			routeIn(t, r, workToken, "one", defaultPool, nil)
			if tt.after != "" {
				writeConfigFile(t, path, tt.after)
				r.upkeep.look()
			}

			for pool, want := range map[string][]string{defaultPool: {"one"}, acmePool: nil} {
				events := statusIn(t, r, pool).Events
				for _, kind := range []string{status.EventStarted, status.EventMoved} {
					if got := sessionsOf(events, kind); !slices.Equal(got, want) {
						t.Errorf("pool %s's document shows the %s events of the sessions %q, want %q", pool, kind, got, want)
					}
				}
			}
		})
	}
}

// sessionsOf returns the session of each event of the kind given among
// events, in order.
func sessionsOf(events []status.Event, kind string) []string {
	var sessions []string
	for _, e := range events {
		if e.Kind == kind {
			sessions = append(sessions, e.Session)
		}
	}
	return sessions
}
