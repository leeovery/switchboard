package router

import (
	"encoding/json"
	"maps"
	"net/http"
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

// newPairedRouter builds a router on now's time of pairedAccounts in
// testPools, each account with a token, serving every request on any
// account: work's quota and personal's need using first, so each pool sends
// a new session to its primary unless pinned.
func newPairedRouter(t *testing.T, now func() time.Time) (*Router, *scriptedUpstream) {
	t.Helper()
	cfg := testConfig(now, &stubProber{}, pairedTokens.Read)
	cfg.Accounts, cfg.Pools = pairedAccounts(), testPools
	r := newTestRouterOf(t, cfg)
	upstream := &scriptedUpstream{answers: map[string][]answer{workToken: {served}, personalToken: {served}, sideToken: {served}, extraToken: {served}}}
	r.proxy.transport = upstream
	readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": soonWeek, "side": laterWeek, "extra": laterWeek})
	return r, upstream
}

// moveAccount has r's pools put the account with the given id in the pool
// given from now on, as a config that moves it would once followed.
func moveAccount(r *Router, id, pool string) {
	moved := *r.state.pools.current()
	moved.poolOf = maps.Clone(moved.poolOf)
	moved.poolOf[id] = pool
	r.state.pools.table.Store(moved.stored())
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
	logstest.Capture(t)
	clock := &testClock{now: start}
	r, upstream := newPairedRouter(t, clock.read)
	routeIn(t, r, workToken, "default", defaultPool, nil)
	routeIn(t, r, personalToken, "acme", acmePool, nil)

	clock.now = start.Add(time.Minute)
	askedOK(t, r, http.MethodPost, "/pin", `{"accounts": ["extra"], "move": true}`)
	was := len(upstream.sent())
	routeIn(t, r, workToken, "default", defaultPool, nil)
	routeIn(t, r, personalToken, "acme", acmePool, nil)

	if got, want := upstream.sent()[was:], []string{"work", "extra"}; !slices.Equal(got, want) {
		t.Errorf("Default's running session and Acme's went out on %q once Acme was pinned with move, want %q: Acme's moved alone", got, want)
	}
	if got := r.sessions.lookup(key{session: "acme", model: opus}).current.Reason; got != reasonMovedByPin {
		t.Errorf("Acme's session gives the reason %q, want %q", got, reasonMovedByPin)
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
		{name: "accounts of both pools, the first's", accounts: `["side", "extra"]`, want: map[string][]string{defaultPool: {"side", "extra"}}},
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
	table := newPoolTable(pairedAccounts(), testPools, defaultPool)
	before := status.Pin{Accounts: []string{"extra"}, Since: start}
	tests := []struct {
		name string
		ps   pins
		do   func(ps pins) pins
		want pins
	}{
		{
			name: "Default pinned beside a pin from before pools of Acme's",
			ps:   pins{"": before},
			do:   func(ps pins) pins { return ps.with(table, defaultPool, status.Pin{Accounts: []string{"side"}}) },
			want: pins{defaultPool: {Accounts: []string{"side"}}, acmePool: before},
		},
		{
			name: "Acme pinned over a pin from before pools of its own",
			ps:   pins{"": before},
			do:   func(ps pins) pins { return ps.with(table, acmePool, status.Pin{Accounts: []string{"personal"}}) },
			want: pins{acmePool: {Accounts: []string{"personal"}}},
		},
		{
			name: "Acme unpinned, its pin from before pools",
			ps:   pins{"": before},
			do: func(ps pins) pins {
				next, _ := ps.without(table, acmePool)
				return next
			},
			want: pins{},
		},
		{
			name: "Default unpinned beside Acme's",
			ps:   pins{defaultPool: {Accounts: []string{"side"}}, acmePool: before},
			do: func(ps pins) pins {
				next, _ := ps.without(table, defaultPool)
				return next
			},
			want: pins{acmePool: before},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			was := maps.Clone(tt.ps)
			if got := tt.do(tt.ps); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
			if !reflect.DeepEqual(tt.ps, was) {
				t.Errorf("the pins changed under it, to %+v, want them as they were, %+v: they're replaced whole", tt.ps, was)
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
// holds, as the pools stand now.
func keptPinOf(r *Router, pool string) status.Pin {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return r.state.pins.of(r.state.pools.current(), pool)
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
