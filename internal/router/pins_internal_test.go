package router

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// extraToken is extra's token, as pairedTokens has it.
const extraToken = "test-token-extra"

// pairedAccounts are two accounts in each of testPools: work, the primary,
// and side in Default; personal, the primary, and extra in Acme.
func pairedAccounts() config.Accounts {
	return config.Accounts{
		{ID: "work", Label: "Work", Pool: defaultPool, Primary: true},
		{ID: "personal", Label: "Personal", Pool: acmePool, Primary: true},
		{ID: "side", Label: "Side", Pool: defaultPool},
		{ID: "extra", Label: "Extra", Pool: acmePool},
	}
}

// pairedTokens has every one of pairedAccounts' tokens.
var pairedTokens = tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken, "extra": extraToken}

// newPairedRouter builds a router of pairedAccounts as newPairedRouterOf
// does: work's quota and personal's need using first, so each pool sends a
// new session to its primary unless pinned.
func newPairedRouter(t *testing.T, now func() time.Time) (*Router, *scriptedUpstream) {
	t.Helper()
	r, upstream := newPairedRouterOf(t, now, pairedAccounts())
	readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": soonWeek, "side": laterWeek, "extra": laterWeek})
	return r, upstream
}

// newPairedRouterOf builds a router on now's time of the accounts given,
// pairedAccounts as configured otherwise, in testPools, each with a token,
// serving every request on any account, nothing read.
func newPairedRouterOf(t *testing.T, now func() time.Time, accounts config.Accounts) (*Router, *scriptedUpstream) {
	t.Helper()
	cfg := testConfig(now, &stubProber{}, pairedTokens.Read)
	cfg.Accounts, cfg.Pools = accounts, testPools
	r := newTestRouterOf(t, cfg)
	upstream := &scriptedUpstream{answers: map[string][]answer{workToken: {served}, personalToken: {served}, sideToken: {served}, extraToken: {served}}}
	r.proxy.transport = upstream
	return r, upstream
}

// moveAccount has r's pools follow a config that puts the account with the
// given id in the pool given, and the rest as r's pools last followed them.
func moveAccount(r *Router, id, pool string) {
	m := r.state.pools
	m.mu.Lock()
	accounts, pools := slices.Clone(m.accounts), m.pools
	m.mu.Unlock()
	for i := range accounts {
		if accounts[i].ID == id {
			accounts[i].Pool = pool
		}
	}
	m.follow(accounts, pools)
}

// askedOK has r's control API answer a request of method for target, with
// body, failing the test unless it's answered 200.
func askedOK(t *testing.T, r *Router, method, target, body string) {
	t.Helper()
	if rec := askControl(t, r, method, target, body); rec.Code != http.StatusOK {
		t.Fatalf("%s %s answered %d %s, want 200", method, target, rec.Code, rec.Body)
	}
}

// newSessionsIn has r route a new session in each pool, Default's then
// Acme's, each named by the pool and n, and returns where each went.
func newSessionsIn(t *testing.T, r *Router, upstream *scriptedUpstream, n string) []string {
	t.Helper()
	was := len(upstream.sent())
	routeIn(t, r, workToken, "default-"+n, defaultPool, nil)
	routeIn(t, r, personalToken, "acme-"+n, acmePool, nil)
	return upstream.sent()[was:]
}

func TestEachPoolsRequestsGoToItsOwnPinAlone(t *testing.T) {
	logstest.Capture(t)
	r, upstream := newPairedRouter(t, at(start))
	lines := keepingLedger(t, r)

	steps := []struct {
		name string
		// ask is what the control API is asked, before a new session in each
		// pool is routed; want is where they go, Default's then Acme's.
		method, target, body string
		want                 []string
	}{
		{name: "unpinned", want: []string{"work", "personal"}},
		{name: "Default pinned", method: http.MethodPost, target: "/pin", body: `{"accounts": ["side"]}`, want: []string{"side", "personal"}},
		{name: "Acme pinned beside it", method: http.MethodPost, target: "/pin", body: `{"accounts": ["extra"]}`, want: []string{"side", "extra"}},
		{name: "Default pinned again", method: http.MethodPost, target: "/pin", body: `{"accounts": ["work"]}`, want: []string{"work", "extra"}},
		{name: "Acme unpinned", method: http.MethodDelete, target: withPool("/pin", acmePool), want: []string{"work", "personal"}},
		{name: "Acme pinned again", method: http.MethodPost, target: "/pin", body: `{"accounts": ["extra"]}`, want: []string{"work", "extra"}},
		{name: "the pool the verbs act on unpinned", method: http.MethodDelete, target: "/pin", want: []string{"work", "extra"}},
	}
	for i, step := range steps {
		if step.method != "" {
			askedOK(t, r, step.method, step.target, step.body)
		}
		if got := newSessionsIn(t, r, upstream, string(rune('a'+i))); !slices.Equal(got, step.want) {
			t.Errorf("%s: Default's new session and Acme's went out on %q, want %q", step.name, got, step.want)
		}
	}
	if got := r.poolPin(defaultPool); !got.IsZero() {
		t.Errorf("Default's pin is %+v once unpinned, want none", got)
	}
	if got := r.poolPin(acmePool); !slices.Equal(got.Accounts, []string{"extra"}) {
		t.Errorf("Acme's pin is %+v once Default's was cleared, want extra's, as it was", got)
	}

	reasons := reasonsOf(lines())
	if got := reasons[2]; got != "pinned (pool)" {
		t.Errorf("Default's session sent to its pin's account gave the reason %q, want %q", got, "pinned (pool)")
	}
	if slices.Contains(reasons, status.ReasonGlobalPin) {
		t.Errorf("the lines give the reasons %q, want none from before pools", reasons)
	}
}

func TestAPinMovesThePoolsRunningSessionsAlone(t *testing.T) {
	tests := []struct {
		name string
		// pin is what's pinned, with move; want is where Default's running
		// session, on side, and Acme's, on extra, go next, neither on its
		// pool's best account; moved is the session it moves.
		pin   string
		want  []string
		moved string
	}{
		{name: "Acme pinned", pin: `{"accounts": ["personal"], "move": true}`, want: []string{"side", "personal"}, moved: "acme"},
		{name: "Default pinned", pin: `{"accounts": ["work"], "move": true}`, want: []string{"work", "extra"}, moved: "default"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			clock := &testClock{now: start}
			r, upstream := newPairedRouter(t, clock.read)
			readWeeks(r, map[string]quota.Window{"work": laterWeek, "side": soonWeek, "personal": laterWeek, "extra": soonWeek})
			if got, want := newSessionsIn(t, r, upstream, "running"), []string{"side", "extra"}; !slices.Equal(got, want) {
				t.Fatalf("Default's session and Acme's went out on %q, want %q", got, want)
			}
			readWeeks(r, map[string]quota.Window{"work": soonWeek, "side": laterWeek, "personal": soonWeek, "extra": laterWeek})

			clock.now = start.Add(time.Minute)
			askedOK(t, r, http.MethodPost, "/pin", tt.pin)
			was := len(upstream.sent())
			routeIn(t, r, workToken, "default-running", defaultPool, nil)
			routeIn(t, r, personalToken, "acme-running", acmePool, nil)

			if got := upstream.sent()[was:]; !slices.Equal(got, tt.want) {
				t.Errorf("Default's running session and Acme's went out on %q once pinned with move, want %q: the pin's pool's alone moved", got, tt.want)
			}
			for _, session := range []string{"default", "acme"} {
				want := reasonNew
				if session == tt.moved {
					want = reasonMovedByPin
				}
				if got := r.sessions.lookup(key{session: session + "-running", model: opus}).current.Reason; got != want {
					t.Errorf("%s's session gives the reason %q, want %q", session, got, want)
				}
			}
		})
	}
}

func TestForceClearsTheOwnPinsOfThePoolsSessionsAlone(t *testing.T) {
	tests := []struct {
		name, method, target, body string
	}{
		{name: "pinned with force", method: http.MethodPost, target: "/pin", body: `{"accounts": ["personal"], "force": true}`},
		{name: "unpinned with force", method: http.MethodDelete, target: withPool("/pin?force=true", acmePool)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r, _ := newPairedRouter(t, at(start))
			routeIn(t, r, workToken, "default", defaultPool, nil)
			routeIn(t, r, personalToken, "acme", acmePool, nil)
			askedOK(t, r, http.MethodPost, "/sessions/default/pin", `{"account": "side"}`)
			askedOK(t, r, http.MethodPost, "/sessions/acme/pin", `{"account": "extra"}`)

			askedOK(t, r, tt.method, tt.target, tt.body)
			if got := r.sessions.lookup(key{session: "acme", model: opus}); !got.given || got.own.Account != "" {
				t.Errorf("Acme's session's own pin is %+v, want it cleared", got.own)
			}
			if got := r.sessions.lookup(key{session: "default", model: opus}); got.own.Account != "side" {
				t.Errorf("Default's session's own pin is %+v, want side's, as it was", got.own)
			}
		})
	}
}

func TestASessionsOwnPinToAnotherPoolsAccountIsRefused(t *testing.T) {
	tests := []struct {
		name, session, pool, account string
		refused                      string
	}{
		{name: "an Acme session to its own pool's account", session: "acme", pool: acmePool, account: "extra"},
		{
			name: "an Acme session to Default's account", session: "acme", pool: acmePool, account: "side",
			refused: "side is in pool Default; this session is in pool Acme",
		},
		{
			name: "a session from before pools, in the first, to Acme's account", session: "before", account: "extra",
			refused: "extra is in pool Acme; this session is in pool Default",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r, _ := newPairedRouter(t, at(start))
			routeIn(t, r, tokenIn(tt.pool), tt.session, tt.pool, nil)

			rec := askControl(t, r, http.MethodPost, "/sessions/"+tt.session+"/pin", `{"account": "`+tt.account+`"}`)
			got := r.sessions.lookup(key{session: tt.session, model: opus})
			if tt.refused == "" {
				if rec.Code != http.StatusOK || got.own.Account != tt.account {
					t.Errorf("answered %d %s, the session's own pin %+v; want 200, pinned to %s", rec.Code, rec.Body, got.own, tt.account)
				}
				return
			}
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), tt.refused) {
				t.Errorf("answered %d %s, want 400 saying %q", rec.Code, rec.Body, tt.refused)
			}
			if got.given {
				t.Errorf("the session's own pin is %+v once refused, want none given", got.own)
			}
			if events := r.recent.events(); slices.ContainsFunc(events, func(e status.Event) bool { return e.Kind == status.EventPin }) {
				t.Errorf("the events are %+v once refused, want no pin told", events)
			}
		})
	}
}

func TestEveryPinEventNamesItsPool(t *testing.T) {
	logstest.Capture(t)
	r, _ := newPairedRouter(t, at(start))
	routeIn(t, r, personalToken, "acme", acmePool, nil)
	routeIn(t, r, workToken, "before", "", nil)

	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["extra"]}`)
	askedOK(t, r, http.MethodDelete, withPool("/pin", acmePool), "")
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"]}`)
	askedOK(t, r, http.MethodDelete, "/pin", "")
	askedOK(t, r, http.MethodPost, "/sessions/acme/pin", `{"account": "extra"}`)
	askedOK(t, r, http.MethodDelete, "/sessions/acme/pin", "")
	askedOK(t, r, http.MethodPost, "/sessions/before/pin", `{"account": "side"}`)
	askedOK(t, r, http.MethodDelete, "/sessions/before/pin", "")

	want := []status.Event{
		{Kind: status.EventPin, Accounts: []string{"extra"}, Account: "extra", Pool: acmePool},
		{Kind: status.EventAuto, Accounts: []string{"extra"}, Pool: acmePool},
		{Kind: status.EventPin, Accounts: []string{"side"}, Account: "side", Pool: defaultPool},
		{Kind: status.EventAuto, Accounts: []string{"side"}, Pool: defaultPool},
		{Kind: status.EventPin, Account: "extra", Session: "acme", Pool: acmePool},
		{Kind: status.EventAuto, Account: "extra", Session: "acme", Pool: acmePool},
		{Kind: status.EventPin, Account: "side", Session: "before", Pool: defaultPool},
		{Kind: status.EventAuto, Account: "side", Session: "before", Pool: defaultPool},
	}
	var got []status.Event
	for _, e := range slices.Backward(r.recent.events()) {
		if e.Kind == status.EventPin || e.Kind == status.EventAuto {
			got = append(got, status.Event{Kind: e.Kind, Accounts: e.Accounts, Account: e.Account, Session: e.Session, Pool: e.Pool})
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the pin events are\n%+v\nwant\n%+v", got, want)
	}
}

func TestAConfigWithoutPoolsPinsAsBeforeItsEventsNamingNoPool(t *testing.T) {
	logstest.Capture(t)
	clock := &testClock{now: start}
	r := newPoolsRouter(t, clock.read, false)
	readPersonalFirst(r)
	routeIn(t, r, workToken, "one", "", nil)

	clock.now = start.Add(time.Minute)
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"], "move": true}`)
	routeIn(t, r, workToken, "one", "", nil)
	routeIn(t, r, workToken, "two", "", nil)
	askedOK(t, r, http.MethodDelete, "/pin", "")

	for _, id := range []string{"one", "two"} {
		if got := r.sessions.lookup(key{session: id, model: opus}).current.Account; got != "side" {
			t.Errorf("session %s is on %q, want side, which the pin named", id, got)
		}
	}
	for _, e := range r.recent.events() {
		if (e.Kind == status.EventPin || e.Kind == status.EventAuto) && e.Pool != "" {
			t.Errorf("the event %+v names pool %q, want none, as the config has no pools", e, e.Pool)
		}
	}
	if got := r.sessions.pinsNow(); len(got) != 0 {
		t.Errorf("the pins are %+v once unpinned, want none", got)
	}
}

func TestEachPoolsPinIsKeptAcrossARestart(t *testing.T) {
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	before, _ := newPairedRouter(t, at(start))
	before.file.load(path)
	askedOK(t, before, http.MethodPost, "/pin", `{"accounts": ["side"], "move": true}`)
	askedOK(t, before, http.MethodPost, "/pin", `{"accounts": ["extra"]}`)
	before.file.save()

	var saved map[string]json.RawMessage
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if _, ok := saved["pin"]; ok {
		t.Errorf("the state file holds\n%s\nwant no pin from before pools", data)
	}

	after, _ := newPairedRouter(t, at(start))
	after.file.load(path)
	if got, want := after.sessions.pinsNow(), before.sessions.pinsNow(); !reflect.DeepEqual(got, want) {
		t.Errorf("after the restart, the pins are %+v, want %+v, as kept", got, want)
	}
}

func TestAPinKeptFromBeforePoolsIsItsAccountsPools(t *testing.T) {
	tests := []struct {
		name     string
		accounts string
		// want are the pins taken in, by pool.
		want map[string][]string
	}{
		{name: "Acme's accounts", accounts: `["extra", "personal"]`, want: map[string][]string{acmePool: {"personal", "extra"}}},
		{name: "Default's accounts", accounts: `["side"]`, want: map[string][]string{defaultPool: {"side"}}},
		{name: "accounts of both pools: the first's, naming its own alone", accounts: `["side", "extra"]`, want: map[string][]string{defaultPool: {"side"}}},
		{name: "an account no longer configured beside Acme's", accounts: `["gone", "extra"]`, want: map[string][]string{acmePool: {"extra"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			path := filepath.Join(t.TempDir(), "state.json")
			writeFile(t, path, `{"version": 1, "pin": {"accounts": `+tt.accounts+`, "since": "2026-09-28T12:12:00Z"}, "sessions": []}`)
			r, _ := newPairedRouter(t, at(start))

			r.file.load(path)
			got := make(map[string][]string)
			for pool, pin := range r.sessions.pinsNow() {
				got[pool] = pin.Accounts
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the pins taken in are %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPinKeptForAPoolNoLongerConfiguredIsDropped(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	writeFile(t, path, `{"version": 1, "pins": {"`+unconfigured+`": {"accounts": ["side"]}, "`+acmePool+`": {"accounts": ["extra"]}}, "sessions": []}`)
	r, _ := newPairedRouter(t, at(start))

	r.file.load(path)
	if got := r.sessions.pinsNow(); len(got) != 1 || !slices.Equal(got[acmePool].Accounts, []string{"extra"}) {
		t.Errorf("the pins taken in are %+v, want Acme's alone", got)
	}
	if want := []string{"level=WARN", `msg="pin dropped: its pool is no longer configured"`, "pool=" + unconfigured, "accounts=side"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	if !r.file.changes.unsaved.Load() {
		t.Error("the pin dropped is still in the state file, want it due to be saved")
	}
}

func TestAPinFromBeforePoolsHoldsInItsPoolOnceThePoolsAreTakenUp(t *testing.T) {
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.Accounts, cfg.ConfigFile = config.Accounts{{ID: "work", Label: "work"}, {ID: "personal", Label: "personal"}, {ID: "side", Label: "side"}}, Watch(path)
	r := newTestRouterOf(t, cfg)
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": laterWeek, "side": laterWeek})
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["personal"]}`)

	// The pools come in, personal in Acme: the pin is Acme's.
	writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
	r.upkeep.look()
	routeIn(t, r, personalToken, "acme", acmePool, nil)
	routeIn(t, r, workToken, "default", defaultPool, nil)

	if got, want := upstream.sent(), []string{"personal", "work"}; !slices.Equal(got, want) {
		t.Errorf("Acme's new session and Default's went out on %q, want %q: the pin Acme's alone", got, want)
	}
	if got := r.sessions.lookup(key{session: "acme", model: opus}).current.Reason; got != reasonPoolPin {
		t.Errorf("Acme's session gives the reason %q, want %q", got, reasonPoolPin)
	}
	if doc := statusIn(t, r, defaultPool); !doc.Pin.IsZero() {
		t.Errorf("Default's document holds pin %+v, want none", doc.Pin)
	}
	if doc := statusIn(t, r, acmePool); !slices.Equal(doc.Pin.Accounts, []string{"personal"}) {
		t.Errorf("Acme's document holds pin %+v, want personal's", doc.Pin)
	}

	// Unpinning Default leaves Acme's; unpinning Acme clears it.
	askedOK(t, r, http.MethodDelete, withPool("/pin", defaultPool), "")
	if got := r.poolPin(acmePool); !slices.Equal(got.Accounts, []string{"personal"}) {
		t.Errorf("Acme's pin is %+v once Default was unpinned, want personal's", got)
	}
	askedOK(t, r, http.MethodDelete, withPool("/pin", acmePool), "")
	if got := r.sessions.pinsNow(); len(got) != 0 {
		t.Errorf("the pins are %+v once Acme was unpinned, want none", got)
	}
}

func TestAPinFromBeforePoolsIsThePoolsItsAccountsAreIn(t *testing.T) {
	table := newPoolTable(pairedAccounts(), testPools, acmePool)
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{name: "all in one pool", ids: []string{"side", "work"}, want: defaultPool},
		{name: "all in another", ids: []string{"extra"}, want: acmePool},
		{name: "in two pools: the first, as the accounts record notes it", ids: []string{"side", "extra"}, want: acmePool},
		{name: "in none: the first", ids: []string{"gone"}, want: acmePool},
		{name: "one in none beside one pool's", ids: []string{"gone", "side"}, want: defaultPool},
	}
	for _, tt := range tests {
		if got := table.home(tt.ids); got != tt.want {
			t.Errorf("%s: home(%q) = %q, want %q", tt.name, tt.ids, got, tt.want)
		}
	}
}

func TestEachPoolsPinStandsAsAnotherIsSetOrCleared(t *testing.T) {
	side := status.Pin{Accounts: []string{"side"}, Since: start}
	tests := []struct {
		name string
		do   func(ps pins) pins
		want pins
	}{
		{
			name: "Default pinned beside Acme's",
			do:   func(ps pins) pins { return ps.with(defaultPool, side) },
			want: pins{defaultPool: side, acmePool: {Accounts: []string{"extra"}}},
		},
		{
			name: "Acme pinned over its own",
			do:   func(ps pins) pins { return ps.with(acmePool, status.Pin{Accounts: []string{"personal"}}) },
			want: pins{acmePool: {Accounts: []string{"personal"}}},
		},
		{
			name: "Acme unpinned",
			do: func(ps pins) pins {
				next, _ := ps.without(acmePool)
				return next
			},
			want: pins{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ps := pins{acmePool: {Accounts: []string{"extra"}}}
			was := maps.Clone(ps)
			if got := tt.do(ps); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(ps, was) {
				t.Errorf("the pins changed under it, to %+v, want them as they were, %+v: they're replaced whole", ps, was)
			}
		})
	}
}

func TestEachPoolsPinIsFittedToThePools(t *testing.T) {
	pooled := newPoolTable(pairedAccounts(), testPools, acmePool)
	unpooledTable := newPoolTable(config.Accounts{{ID: "work"}, {ID: "side"}}, nil, "")
	pin := func(since time.Time, ids ...string) status.Pin {
		return status.Pin{Accounts: ids, Since: since, Move: true}
	}
	tests := []struct {
		name  string
		table *poolTable
		ps    pins
		want  pins
		// gone are the pins dropped whole, by the pool each was the pin of;
		// logged are what the log says of what's left out, each in order.
		gone   []gonePin
		logged []string
	}{
		{
			name:  "each pin naming its own pool's accounts",
			table: pooled,
			ps:    pins{defaultPool: pin(start, "work", "side"), acmePool: pin(start, "extra")},
			want:  pins{defaultPool: pin(start, "work", "side"), acmePool: pin(start, "extra")},
		},
		{
			name:  "a pin from before pools, its accounts in one pool: that pool's",
			table: pooled,
			ps:    pins{"": pin(start, "side")},
			want:  pins{defaultPool: pin(start, "side")},
		},
		{
			name:   "a pin from before pools, its accounts in two: the first's, naming its own alone",
			table:  pooled,
			ps:     pins{"": pin(start, "side", "extra")},
			want:   pins{acmePool: pin(start, "extra")},
			logged: []string{"dropped from its pool's pin: the account is no longer in the pool"},
		},
		{
			name:   "a pin from before pools beside a later one of its pool's own: the pool's stands",
			table:  pooled,
			ps:     pins{"": pin(start, "side"), defaultPool: pin(start.Add(time.Minute), "work")},
			want:   pins{defaultPool: pin(start.Add(time.Minute), "work")},
			gone:   []gonePin{{pool: defaultPool, pin: pin(start, "side")}},
			logged: []string{"pin dropped: a later one holds in its pool"},
		},
		{
			name:   "a pin from before pools beside one of its pool's own as late: the pool's stands",
			table:  pooled,
			ps:     pins{"": pin(start, "side"), defaultPool: pin(start, "work")},
			want:   pins{defaultPool: pin(start, "work")},
			gone:   []gonePin{{pool: defaultPool, pin: pin(start, "side")}},
			logged: []string{"pin dropped: a later one holds in its pool"},
		},
		{
			name:   "a pin from before pools later than its pool's own: it stands",
			table:  pooled,
			ps:     pins{"": pin(start.Add(time.Minute), "side"), defaultPool: pin(start, "work")},
			want:   pins{defaultPool: pin(start.Add(time.Minute), "side")},
			gone:   []gonePin{{pool: defaultPool, pin: pin(start, "work")}},
			logged: []string{"pin dropped: a later one holds in its pool"},
		},
		{
			name:   "a pool's pin naming an account another pool holds: left out of it",
			table:  pooled,
			ps:     pins{defaultPool: pin(start, "work", "extra")},
			want:   pins{defaultPool: pin(start, "work")},
			logged: []string{"dropped from its pool's pin: the account is no longer in the pool"},
		},
		{
			name:   "a pool's pin naming none of its accounts: gone",
			table:  pooled,
			ps:     pins{defaultPool: pin(start, "extra"), acmePool: pin(start, "personal")},
			want:   pins{acmePool: pin(start, "personal")},
			gone:   []gonePin{{pool: defaultPool, pin: pin(start, "extra")}},
			logged: []string{"dropped from its pool's pin: the account is no longer in the pool"},
		},
		{
			name:   "the pin of a pool no longer configured: gone",
			table:  pooled,
			ps:     pins{"gone": pin(start, "work"), acmePool: pin(start, "personal")},
			want:   pins{acmePool: pin(start, "personal")},
			gone:   []gonePin{{pool: "gone", pin: pin(start, "work")}},
			logged: []string{"pin dropped: its pool is no longer configured"},
		},
		{
			name:  "a config from before pools: its pin as it is",
			table: unpooledTable,
			ps:    pins{"": pin(start, "work", "side")},
			want:  pins{"": pin(start, "work", "side")},
		},
		{
			name:   "a config from before pools: a pool's pin gone",
			table:  unpooledTable,
			ps:     pins{defaultPool: pin(start, "work")},
			want:   pins{},
			gone:   []gonePin{{pool: defaultPool, pin: pin(start, "work")}},
			logged: []string{"pin dropped: its pool is no longer configured"},
		},
		{
			name:  "none",
			table: pooled,
			want:  pins{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logged := logstest.Capture(t)
			was := maps.Clone(tt.ps)
			got, gone, changed := tt.ps.fitted(tt.table)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("fitted() = %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(gone, tt.gone) {
				t.Errorf("fitted() dropped %+v whole, want %+v", gone, tt.gone)
			}
			if want := !maps.EqualFunc(tt.ps, tt.want, samePin); changed != want {
				t.Errorf("fitted() reported changed %t, want %t", changed, want)
			}
			if !reflect.DeepEqual(tt.ps, was) {
				t.Errorf("the pins changed under it, to %+v, want them as they were, %+v: they're replaced whole", tt.ps, was)
			}
			warned := slices.DeleteFunc(logged.Lines(), func(line string) bool { return !strings.Contains(line, "level=WARN") })
			matched := len(warned) == len(tt.logged)
			for i := range min(len(warned), len(tt.logged)) {
				matched = matched && strings.Contains(warned[i], `msg="`+tt.logged[i]+`"`)
			}
			if !matched {
				t.Errorf("the log warns %q, want a line of each of %q", warned, tt.logged)
			}
		})
	}
}

// The detector is what holds this test: a pin set or cleared, a session's
// own pin given, and requests routed, as the pools change under them.
func TestAPinSetRacesAChangeToThePools(t *testing.T) {
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	routeIn(t, r, personalToken, "acme", acmePool, nil)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var once sync.Once
	halt := func() {
		once.Do(func() { close(stop) })
		wg.Wait()
	}
	t.Cleanup(halt)
	repeat := func(do func()) {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					do()
				}
			}
		})
	}
	repeat(func() {
		askControl(t, r, http.MethodPost, "/pin", `{"accounts": ["side"], "move": true, "force": true}`)
	})
	repeat(func() { askControl(t, r, http.MethodDelete, withPool("/pin?force=true", acmePool), "") })
	repeat(func() { askControl(t, r, http.MethodPost, "/sessions/acme/pin", `{"account": "side"}`) })
	repeat(func() { askControl(t, r, http.MethodGet, withPool("/status", acmePool), "") })
	repeat(func() { routeIn(t, r, workToken, "", defaultPool, nil) })
	repeat(func() { routeIn(t, r, personalToken, "acme", acmePool, nil) })

	for _, sidePool := range []string{acmePool, defaultPool, acmePool} {
		was := len(upstream.sent())
		waitFor(t, "requests routed", func() bool { return len(upstream.sent()) >= was+10 })
		writeConfigFile(t, path, poolsTOML(sidePool, defaultTOML, acmeTOML))
		r.upkeep.look()
	}
	halt()
	raced := len(upstream.sent())
	for range 3 {
		routeIn(t, r, workToken, "", defaultPool, nil)
	}

	if after := upstream.sent()[raced:]; !slices.Equal(after, []string{"work", "work", "work"}) {
		t.Errorf("once the pools were taken up, Default's requests went out on %q, want work alone, side having moved to Acme", after)
	}
	if got := r.sessions.lookup(key{session: "acme", model: opus}).current.Account; got == "work" {
		t.Errorf("Acme's session is on work, Default's account")
	}
}

// keptPinOf returns the pin of the pool with the given id the state's copy
// holds: zero where it has none.
func keptPinOf(r *Router, pool string) status.Pin {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return r.state.pins[pool]
}

func TestTheStateHoldsThePinsTheSessionsDo(t *testing.T) {
	logstest.Capture(t)
	r, _ := newPairedRouter(t, at(start))
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"]}`)
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["extra"]}`)
	askedOK(t, r, http.MethodDelete, withPool("/pin", defaultPool), "")

	for _, pool := range []string{defaultPool, acmePool} {
		if got, want := keptPinOf(r, pool), r.poolPin(pool); !reflect.DeepEqual(got, want) {
			t.Errorf("the state holds pool %s's pin as %+v, want the sessions', %+v", pool, got, want)
		}
	}
}

// autosOf returns the events r keeps of routing given back to it, as pin auto
// files them.
func autosOf(r *Router) []status.Event {
	return slices.DeleteFunc(r.recent.events(), func(e status.Event) bool { return e.Kind != status.EventAuto })
}

func TestAnAccountLeavingItsPoolLeavesThePoolsPin(t *testing.T) {
	log := logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	r, upstream := newPairedRouter(t, at(start))
	r.file.load(path)
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"]}`)

	// Side moves to Acme: it leaves Default's pin, which goes with it, and
	// isn't Acme's pin either.
	moveAccount(r, "side", acmePool)
	for _, pool := range []string{defaultPool, acmePool} {
		if got := r.poolPin(pool); !got.IsZero() {
			t.Errorf("pool %s's pin is %+v once side moved to Acme, want none", pool, got)
		}
		if got := keptPinOf(r, pool); !got.IsZero() {
			t.Errorf("the state holds pool %s's pin as %+v once side moved to Acme, want none", pool, got)
		}
	}
	if want := []string{"level=WARN", `msg="dropped from its pool's pin: the account is no longer in the pool"`, "pool=" + defaultPool, "account=side"}; !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	if got, want := newSessionsIn(t, r, upstream, "moved"), []string{"work", "personal"}; !slices.Equal(got, want) {
		t.Errorf("once side moved to Acme, new sessions in Default and Acme went out on %q, want %q: neither pinned", got, want)
	}

	// Side comes back: Default's pin doesn't come back with it, nor does a
	// restart bring it back.
	moveAccount(r, "side", defaultPool)
	if got := r.poolPin(defaultPool); !got.IsZero() {
		t.Errorf("Default's pin is %+v once side came back, want none", got)
	}
	if got, want := newSessionsIn(t, r, upstream, "back"), []string{"work", "personal"}; !slices.Equal(got, want) {
		t.Errorf("once side came back, new sessions in Default and Acme went out on %q, want %q: neither pinned", got, want)
	}
	r.file.save()
	if data, err := os.ReadFile(path); err != nil || strings.Contains(string(data), `"pin`) {
		t.Errorf("the state file holds\n%s\n%v, want no pin", data, err)
	}
}

func TestPinAutoForAPoolWhosePinsAccountsHaveLeftItFilesNothing(t *testing.T) {
	logstest.Capture(t)
	r, _ := newPairedRouter(t, at(start))
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"]}`)
	moveAccount(r, "side", acmePool)
	was := autosOf(r)

	askedOK(t, r, http.MethodDelete, withPool("/pin", defaultPool), "")
	if got := autosOf(r); len(got) != len(was) {
		t.Errorf("pin auto for Default filed %+v, want nothing more than %+v: its pin went as side left it", got, was)
	}
}

// givenBack returns what r's events of routing given back to it tell, oldest
// first: the accounts, the pool and who gave it back.
func givenBack(r *Router) []status.Event {
	var told []status.Event
	for _, e := range slices.Backward(autosOf(r)) {
		told = append(told, status.Event{Kind: e.Kind, Accounts: e.Accounts, Pool: e.Pool, By: e.By})
	}
	return told
}

func TestEachPinThePoolsNoLongerLeaveStandingIsToldAsGivenBackByTheRouter(t *testing.T) {
	byRouter := func(pool string, ids ...string) status.Event {
		return status.Event{Kind: status.EventAuto, Accounts: ids, Pool: pool, By: string(ByRouter)}
	}
	loading := func(state string) func(t *testing.T, r *Router) {
		return func(t *testing.T, r *Router) {
			path := filepath.Join(t.TempDir(), "state.json")
			writeFile(t, path, state)
			r.file.load(path)
		}
	}
	tests := []struct {
		name string
		do   func(t *testing.T, r *Router)
		want []status.Event
	}{
		{
			name: "a pool's pin whose one account leaves the pool",
			do: func(t *testing.T, r *Router) {
				askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"]}`)
				moveAccount(r, "side", acmePool)
			},
			want: []status.Event{byRouter(defaultPool, "side")},
		},
		{
			name: "a pool's pin one of whose two accounts leaves the pool: it stands, narrowed",
			do: func(t *testing.T, r *Router) {
				askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["work", "side"]}`)
				moveAccount(r, "side", acmePool)
			},
		},
		{
			name: "a pin kept for a pool no longer configured",
			do:   loading(`{"version": 1, "pins": {"` + unconfigured + `": {"accounts": ["side"]}}, "sessions": []}`),
			want: []status.Event{byRouter(unconfigured, "side")},
		},
		{
			name: "a pin kept from before pools giving way to a later one of its pool's own",
			do: loading(`{"version": 1, "pin": {"accounts": ["side"], "since": "2026-09-28T12:00:00Z"},
				"pins": {"` + defaultPool + `": {"accounts": ["work"], "since": "2026-09-28T12:10:00Z"}}, "sessions": []}`),
			want: []status.Event{byRouter(defaultPool, "side")},
		},
		{
			name: "a pin kept from before pools naming accounts of both pools: it stands, narrowed to the first's",
			do:   loading(`{"version": 1, "pin": {"accounts": ["side", "extra"]}, "sessions": []}`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r, _ := newPairedRouter(t, at(start))
			tt.do(t, r)
			if got := givenBack(r); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the events of routing given back are\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestAPinWhoseAccountsThePoolsMovedSinceItWasReadIsRefused(t *testing.T) {
	tests := []struct {
		name string
		// ids are the accounts pinned, read as Default's before side moves
		// to Acme; refused is what the refusal says.
		ids     []string
		refused string
	}{
		{name: "its one account moved", ids: []string{"side"}, refused: "the accounts to pin have just moved pool"},
		{name: "one of its two accounts moved", ids: []string{"work", "side"}, refused: "pin accounts of one pool"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r, _ := newPairedRouter(t, at(start))
			rd, err := r.pinRead(httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/pin", nil), tt.ids)
			if err != nil || rd.pool != defaultPool {
				t.Fatalf("pinRead() = %+v, %v, want Default's read", rd, err)
			}

			moveAccount(r, "side", acmePool)
			if err := r.pin(rd, PinRequest{Accounts: tt.ids, By: ByCLI}); err == nil || !strings.Contains(err.Error(), tt.refused) {
				t.Errorf("pin() = %v, want it refused, saying %q", err, tt.refused)
			}
			if got := r.sessions.pinsNow(); len(got) != 0 {
				t.Errorf("the pins are %+v, want none: a refused pin changes nothing", got)
			}
			for _, e := range r.recent.events() {
				if e.Kind == status.EventPin {
					t.Errorf("the router told %+v, want no pin: it was refused", e)
				}
			}
		})
	}
}

func TestAPinFromBeforePoolsStaysInItsPoolAsItsAccountMoves(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(fmt.Sprintf("restarted %t", restart), func(t *testing.T) {
			log := logstest.Capture(t)
			state := filepath.Join(t.TempDir(), "state.json")
			path := filepath.Join(t.TempDir(), "config.toml")
			writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
			cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
			cfg.Accounts, cfg.ConfigFile = config.Accounts{{ID: "work", Label: "work"}, {ID: "personal", Label: "personal"}, {ID: "side", Label: "side"}}, Watch(path)
			r := newTestRouterOf(t, cfg)
			r.file.load(state)
			askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["side"]}`)

			// The pools come in, side in Default: the pin is Default's, once
			// and for all.
			writeConfigFile(t, path, poolsTOML(defaultPool, defaultTOML, acmeTOML))
			r.upkeep.look()
			if got := r.sessions.pinsNow(); len(got) != 1 || !slices.Equal(got[defaultPool].Accounts, []string{"side"}) {
				t.Fatalf("once the pools came in, the pins are %+v, want Default's alone, naming side", got)
			}
			if restart {
				r.file.save()
				r, path = newWatchingPoolsRouter(t, "")
				r.file.load(state)
			}

			// Side moves to Acme: the pin stays Default's, and goes as side
			// leaves it.
			writeConfigFile(t, path, poolsTOML(acmePool, defaultTOML, acmeTOML))
			r.upkeep.look()
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": soonWeek, "side": laterWeek})
			routeIn(t, r, personalToken, "acme", acmePool, nil)
			routeIn(t, r, workToken, "default", defaultPool, nil)

			if got, want := upstream.sent(), []string{"personal", "work"}; !slices.Equal(got, want) {
				t.Errorf("Acme's new session and Default's went out on %q, want %q: neither pinned", got, want)
			}
			for _, pool := range []string{defaultPool, acmePool} {
				if doc := statusIn(t, r, pool); !doc.Pin.IsZero() {
					t.Errorf("pool %s's document holds pin %+v, want none", pool, doc.Pin)
				}
			}
			if want := []string{"level=WARN", `msg="dropped from its pool's pin: the account is no longer in the pool"`, "pool=" + defaultPool, "account=side"}; !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

func TestAPoolsPinSpendsTheReserveOfItsOwnAccountsAloneAsTheCapsAreTold(t *testing.T) {
	logstest.Capture(t)
	reserving := pairedAccounts()
	for i := range reserving {
		reserving[i].Reserve = 0.1
	}
	r, _ := newPairedRouterOf(t, at(start), reserving)
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["extra"]}`)
	reads := func(used float64) {
		for _, id := range []string{"side", "extra"} {
			r.state.record(id, []quota.Window{{Key: "7d", Label: "Week", Utilization: used, ResetsAt: weekEnds, Status: quota.StatusAllowed}}, r.state.mark())
		}
		r.recent.look()
	}

	// Side, Default's, and extra, Acme's, reach their reserves: Acme's pin
	// spends extra's, and Default's side is capped.
	reads(0.5)
	reads(0.95)
	var capped []string
	for _, e := range r.recent.events() {
		if e.Kind == status.EventCap {
			capped = append(capped, e.Account)
		}
	}
	if !slices.Equal(capped, []string{"side"}) {
		t.Errorf("the caps told are of %q, want side's alone: Acme's pin spends extra's reserve, and no pin side's", capped)
	}
}
