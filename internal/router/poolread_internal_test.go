package router

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
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

func TestEachReadIsAnsweredForThePoolItAsksForElseTheOneTheVerbsActOn(t *testing.T) {
	logstest.Capture(t)
	reads := []struct{ method, target, body string }{
		{http.MethodGet, "/status", ""},
		{http.MethodGet, "/sessions", ""},
		{http.MethodGet, "/history?window=7d&step=1h", ""},
		{http.MethodGet, "/stream", ""},
		{http.MethodPost, "/refresh", `{"max_age": "30m"}`},
		{http.MethodPost, "/pin", `{"accounts": ["work"]}`},
		{http.MethodDelete, "/pin", ""},
	}
	tests := []struct {
		name     string
		accounts config.Accounts
		pools    config.Pools
		asked    string
		// want is the id of the pool the document answers for, "" for
		// none; refused, how the read is refused, naming the pools, "" where
		// it's answered.
		want    string
		refused string
	}{
		{name: "no pools, none asked", accounts: testConfigured},
		{name: "no pools, one asked: as without", accounts: testConfigured, asked: defaultPool},
		{name: "no pools, one not configured asked: as without", accounts: testConfigured, asked: unconfigured},
		{name: "one pool, none asked: the only one", accounts: inDefault(), pools: onePool, want: defaultPool},
		{name: "one pool, it asked", accounts: inDefault(), pools: onePool, asked: defaultPool, want: defaultPool},
		{name: "one pool, one not configured asked", accounts: inDefault(), pools: onePool, asked: acmePool, refused: defaultPool + " (Default)"},
		{name: "one unfenced, none asked: the unfenced", accounts: pooledAccounts(), pools: testPools, want: defaultPool},
		{name: "one unfenced, the fenced asked", accounts: pooledAccounts(), pools: testPools, asked: acmePool, want: acmePool},
		{name: "one unfenced, asked ignoring case", accounts: pooledAccounts(), pools: testPools, asked: "Q7MT", want: acmePool},
		{name: "one unfenced, one not configured asked", accounts: pooledAccounts(), pools: testPools, asked: unconfigured, refused: poolsNamedSaid},
		{name: "both fenced, none asked", accounts: pooledAccounts(), pools: bothFenced, refused: poolsNamedSaid},
		{name: "both fenced, one asked", accounts: pooledAccounts(), pools: bothFenced, asked: defaultPool, want: defaultPool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := pooledRouter(t, tt.accounts, tt.pools)
			for _, read := range reads {
				rec := askControl(t, r, read.method, withPool(read.target, tt.asked), read.body)
				switch {
				case tt.refused != "" && (rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.refused)):
					t.Errorf("%s %s answered %d %s, want 400 naming the pools, as %q", read.method, read.target, rec.Code, rec.Body, tt.refused)
				case tt.refused == "" && rec.Code != http.StatusOK:
					t.Errorf("%s %s answered %d %s, want 200", read.method, read.target, rec.Code, rec.Body)
				}
			}
			if tt.refused != "" {
				return
			}
			if doc := statusIn(t, r, tt.asked); doc.PoolID != tt.want {
				t.Errorf("GET /status answered for pool %q, want %q", doc.PoolID, tt.want)
			}
		})
	}
}

func TestARefusedPinOrUnpinChangesNothing(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, pooledAccounts(), bothFenced)
	r.pinTo(status.Pin{Accounts: []string{"side"}, Since: start}, false)

	askControl(t, r, http.MethodPost, "/pin", `{"accounts": ["work"]}`)
	askControl(t, r, http.MethodDelete, "/pin", "")
	if got := r.sessions.globalPin(); !slices.Equal(got.Accounts, []string{"side"}) {
		t.Errorf("the pin is %+v once pinning and unpinning were refused, want side's as it was", got)
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

func TestEachPoolsDocumentSharesTheEventsThatNameNoPool(t *testing.T) {
	r := pooledRouter(t, pooledAccounts(), testPools)
	read := poolRead{pools: r.state.pools.current(), pool: defaultPool}
	all := []status.Event{
		{ID: 3, Kind: status.EventPin, Accounts: []string{"personal"}, Pool: acmePool},
		{ID: 2, Kind: status.EventLimit, Account: "personal"},
		{ID: 1, Kind: status.EventPin, Accounts: []string{"work"}, Pool: defaultPool},
	}
	if got, want := read.events(all), []status.Event{all[1], all[2]}; !reflect.DeepEqual(got, want) {
		t.Errorf("Default shows the events %+v, want %+v", got, want)
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
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+withPool("/stream", acmePool), nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	routeIn(t, r, workToken, "one", defaultPool, nil)
	routeIn(t, r, personalToken, "two", acmePool, nil)
	for _, e := range streamedTillDone(t, resp.Body) {
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

func TestAConfigWithNoPoolsIsAnsweredAsBeforeWhateverPoolIsAsked(t *testing.T) {
	logstest.Capture(t)
	r := pooledRouter(t, testConfigured, nil)
	readPersonalFirst(r)
	routeIn(t, r, workToken, "one", "", nil)

	whole, err := json.Marshal(r.assembled(r.state.everyAccount()).WorkedOut(r.cfg.Policy))
	if err != nil {
		t.Fatal(err)
	}
	if got := askControl(t, r, http.MethodGet, "/status", "").Body.String(); got != string(whole)+"\n" {
		t.Errorf("GET /status answered\n%s\nwant every account's document, as before pools\n%s", got, whole)
	}
	for _, target := range []string{"/status", "/sessions", "/sessions/one", "/history?window=7d&step=1h"} {
		without := askControl(t, r, http.MethodGet, target, "").Body.String()
		with := askControl(t, r, http.MethodGet, withPool(target, defaultPool), "").Body.String()
		if with != without {
			t.Errorf("GET %s asking for a pool answered\n%s\nwant as without\n%s", target, with, without)
		}
		if strings.Contains(without, `"pool_id"`) || strings.Contains(without, `"pool_label"`) {
			t.Errorf("GET %s answered %s, naming a pool where there are none", target, without)
		}
	}
}
