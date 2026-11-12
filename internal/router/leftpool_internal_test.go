package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// The words switchboard refuses a request with whose session's token has left
// its pool, as they end.
const (
	leftAcme    = "switchboard: this session's account has left pool Acme — exit and resume it to carry on in the pool its directory is in now"
	leftDefault = "switchboard: this session's account has left pool Default — exit and resume it to carry on in the pool its directory is in now"
	leftItsPool = "switchboard: this session's account has left its pool — exit and resume it to carry on in the pool its directory is in now"
	poolRemoved = "switchboard: this session's pool has been removed — exit and resume it to carry on in the pool its directory is in now"
)

// The log's lines of a refusal, of an account that has left its pool and of
// a pool no longer configured.
const (
	leftLogged = `msg="session's account has left its pool; refusing the request"`
	goneLogged = `msg="session's pool is no longer configured; refusing the request"`
)

// strangerToken is a token no account has.
const strangerToken = "test-token-stranger"

// checkRefused checks that w is switchboard's refusal of a request whose
// session's token has left its pool, saying want: shaped as the API shapes
// its errors, with a status Claude Code neither retries, as it does a 408,
// 409, 429 or 5xx, nor takes for a login to renew, as it does a 401 or 403.
func checkRefused(t *testing.T, w *httptest.ResponseRecorder, want string) {
	t.Helper()
	switch w.Code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusRequestTimeout, http.StatusConflict, http.StatusTooManyRequests:
		t.Errorf("answered %d, which Claude Code retries or takes for a login to renew", w.Code)
	}
	if w.Code < 400 || w.Code >= 500 {
		t.Errorf("answered %d, want a 4xx: Claude Code retries a 5xx", w.Code)
	}
	var body apiError
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("answered %s, want an error shaped as the API shapes them: %v", w.Body, err)
	}
	if w.Code != leftPool.status || body.Type != "error" || body.Error.Type != leftPool.kind || body.Error.Message != want {
		t.Errorf("answered %d %+v, want %d, an %s saying %q", w.Code, body, leftPool.status, leftPool.kind, want)
	}
}

// passIn has r's proxy pass a request to path through, on token, of session
// one, naming the pool given in its header, "" for none, and returns its
// answer.
func passIn(t *testing.T, r *Router, path, token, pool string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader("{}"))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set(claude.SessionHeader, "one")
	if pool != "" {
		req.Header.Set(PoolHeader, pool)
	}
	w := httptest.NewRecorder()
	r.Proxy().ServeHTTP(w, req)
	return w
}

// movedPools are pooledAccounts with personal, Acme's primary, moved to
// Default, and side moved to Acme.
func movedPools() config.Accounts {
	moved := pooledAccounts()
	moved[1].Pool, moved[2].Pool = defaultPool, acmePool
	return moved
}

func TestASessionWhoseTokensAccountLeavesItsPoolIsRefusedWhileOneInThePoolItJoinedCarriesOn(t *testing.T) {
	logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	lines := keepingLedger(t, r)
	r.proxy.pools.follow(movedPools(), testPools)

	// Acme's session started on personal's token, as Acme's primary's, and
	// Default's has it since personal joined Default; Acme has side, with
	// room, all the same.
	acmes := routeIn(t, r, personalToken, "one", acmePool, nil)
	defaults := routeIn(t, r, personalToken, "two", defaultPool, nil)

	checkRefused(t, acmes, leftAcme)
	if defaults.Code != http.StatusOK {
		t.Errorf("Default's request was answered %d, want 200: personal is in its pool", defaults.Code)
	}
	if sent := upstream.sent(); !slices.Equal(sent, []string{"personal"}) {
		t.Errorf("the requests went out on %q, want Default's on personal alone, and Acme's on none", sent)
	}
	got := lines()
	if len(got) != 2 {
		t.Fatalf("the ledger holds %d lines, want Acme's and Default's", len(got))
	}
	if l := got[0]; l.Pool != acmePool || l.Status != leftPool.status || l.Account != "" || l.Session != "one" {
		t.Errorf("Acme's line is of pool %q, session %q, answered %d on %q, want of %s, session one, answered %d on no account",
			l.Pool, l.Session, l.Status, l.Account, acmePool, leftPool.status)
	}
	if a := r.sessions.lookup(key{session: "one", model: opus}); a.assigned {
		t.Errorf("Acme's session is assigned to %s, want it remembered nowhere: nothing was chosen for it", a.current.Account)
	}
}

func TestARefusalSaysWhyAndNamesThePoolWhereThereAreSeveral(t *testing.T) {
	tests := []struct {
		name string
		// follow are the accounts and pools the router takes up, as the
		// config makes them, where it's given; token and pool are what the
		// request carries.
		follow      config.Accounts
		followPools config.Pools
		token, pool string
		// want is what the refusal says, and wantLog how the log says it.
		want, wantLog string
	}{
		{
			name:   "its account moved to another pool",
			follow: movedPools(), followPools: testPools,
			token: personalToken, pool: acmePool,
			want: leftAcme, wantLog: leftLogged,
		},
		{
			name:   "naming none, from a session started before pools, its account outside the first",
			follow: pooledAccounts(), followPools: testPools,
			token: personalToken,
			want:  leftDefault, wantLog: leftLogged,
		},
		{
			name:   "its account out of the config, the one pool left: no pool named",
			follow: config.Accounts{{ID: "work", Pool: defaultPool, Primary: true}, {ID: "side", Pool: defaultPool}}, followPools: testPools[:1],
			token: personalToken, pool: defaultPool,
			want: leftItsPool, wantLog: leftLogged,
		},
		{
			name:   "its pool removed",
			follow: movedPools()[:2], followPools: testPools[:1],
			token: personalToken, pool: acmePool,
			want: poolRemoved, wantLog: goneLogged,
		},
		{
			name:   "naming a pool never configured",
			follow: pooledAccounts(), followPools: testPools,
			token: workToken, pool: "z5k2",
			want: poolRemoved, wantLog: goneLogged,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r := newPoolsRouter(t, at(start), true)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			r.proxy.pools.follow(tt.follow, tt.followPools)

			checkRefused(t, routeIn(t, r, tt.token, "one", tt.pool, nil), tt.want)

			if sent := upstream.sent(); len(sent) > 0 {
				t.Errorf("the request went out on %q, want none", sent)
			}
			if want := []string{"level=WARN", tt.wantLog, "session=one"}; !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

func TestARequestNamingNoPoolIsRefusedOnlyWhereItsTokensAccountIsOutsideTheFirst(t *testing.T) {
	tests := []struct {
		name string
		// first is the pool the accounts record notes as the first, "" for
		// none; token is the one the request carries.
		first, token string
		// wantSent is where it went out, none where it's refused, saying
		// wantRefusal.
		wantSent    []string
		wantRefusal string
	}{
		{name: "work's, in the first", token: workToken, wantSent: []string{"work"}},
		{name: "side's, in the first", token: sideToken, wantSent: []string{"work"}},
		{name: "personal's, in Acme", token: personalToken, wantRefusal: leftDefault},
		{name: "personal's, Acme the first the record notes", first: acmePool, token: personalToken, wantSent: []string{"personal"}},
		{name: "work's, Acme the first the record notes", first: acmePool, token: workToken, wantRefusal: leftAcme},
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
			readWeeks(r, map[string]quota.Window{"work": soonWeek, "personal": laterWeek, "side": latestWeek})
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream

			w := routeIn(t, r, tt.token, "one", "", nil)

			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request went out on %q, want %q", sent, tt.wantSent)
			}
			if tt.wantRefusal != "" {
				checkRefused(t, w, tt.wantRefusal)
			} else if w.Code != http.StatusOK {
				t.Errorf("answered %d, want 200", w.Code)
			}
		})
	}
}

func TestAConfigWithoutPoolsRefusesNothingAnsweringAsBefore(t *testing.T) {
	// Each request is sent to a router whose config has no pools twice: as
	// a session started before pools sends it, naming none, and naming the
	// pool given. Personal's quota needs using first.
	tests := []struct {
		name, path, token, pool string
	}{
		{name: "routed, naming a pool the config has none of", path: "/v1/messages", token: workToken, pool: acmePool},
		{name: "routed on personal's token, naming another", path: "/v1/messages", token: personalToken, pool: defaultPool},
		{name: "passed through, naming a pool", path: "/v1/files", token: personalToken, pool: acmePool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			type answered struct {
				code int
				body string
				sent []string
			}
			send := func(pool string) answered {
				r := newPoolsRouter(t, at(start), false)
				readPersonalFirst(r)
				upstream := servingAll(served, served, served)
				r.proxy.transport = upstream
				var w *httptest.ResponseRecorder
				if tt.path == "/v1/messages" {
					w = routeIn(t, r, tt.token, "one", pool, nil)
				} else {
					w = passIn(t, r, tt.path, tt.token, pool)
				}
				return answered{code: w.Code, body: w.Body.String(), sent: upstream.sent()}
			}
			log := logstest.Capture(t)

			before, named := send(""), send(tt.pool)

			if named.code != http.StatusOK || named.code != before.code || named.body != before.body || !slices.Equal(named.sent, before.sent) {
				t.Errorf("naming %s, answered %d %q, sent on %q, want as naming none: %d %q, sent on %q",
					tt.pool, named.code, named.body, named.sent, before.code, before.body, before.sent)
			}
			if strings.Contains(log.String(), "refusing the request") {
				t.Errorf("log reads\n%s\nwant no refusal", log)
			}
		})
	}
}

func TestARefusalIsWarnedOfOnceForEachSession(t *testing.T) {
	log := logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	r.proxy.pools.follow(movedPools(), testPools)

	for _, session := range []string{"one", "one", "two", "one"} {
		routeIn(t, r, personalToken, session, acmePool, nil)
	}
	passIn(t, r, "/v1/files", personalToken, acmePool)

	for level, want := range map[string]int{"WARN": 2, "DEBUG": 3} {
		if got := timesLogged(log, "level="+level, leftLogged); got != want {
			t.Errorf("log reads\n%s\nwant the refusal noted at %s %d times, not %d: warned of once for each session, at debug after, passed through or not",
				log, level, want, got)
		}
	}
}

func TestARequestPassedThroughIsRefusedWhereItsTokensAccountIsOutsideItsPool(t *testing.T) {
	tests := []struct {
		name string
		// pooled has the config put the accounts in pools; token and pool
		// are what the request carries.
		pooled      bool
		token, pool string
		// wantSent is the account it went out on as it came, none where it's
		// refused, saying wantRefusal.
		wantSent    []string
		wantRefusal string
	}{
		{name: "its token's account in its pool", pooled: true, token: personalToken, pool: acmePool, wantSent: []string{"personal"}},
		{name: "its token's account in another", pooled: true, token: workToken, pool: acmePool, wantRefusal: leftAcme},
		{name: "naming none, its token's account in the first", pooled: true, token: workToken, wantSent: []string{"work"}},
		{name: "naming none, its token's account outside the first", pooled: true, token: personalToken, wantRefusal: leftDefault},
		{name: "naming a pool never configured", pooled: true, token: workToken, pool: "z5k2", wantRefusal: poolRemoved},
		{name: "a token no account has", pooled: true, token: strangerToken, pool: acmePool, wantSent: []string{""}},
		{name: "a config without pools", token: workToken, pool: acmePool, wantSent: []string{"work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			r := newPoolsRouter(t, at(start), tt.pooled)
			upstream := servingAll(served, served, served)
			upstream.answers[strangerToken] = []answer{served}
			r.proxy.transport = upstream

			w := passIn(t, r, "/v1/files", tt.token, tt.pool)

			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request went out on %q, want %q", sent, tt.wantSent)
			}
			if tt.wantRefusal != "" {
				checkRefused(t, w, tt.wantRefusal)
			} else if w.Code != http.StatusOK {
				t.Errorf("answered %d, want 200, passed through", w.Code)
			}
		})
	}
}

func TestASessionOnAnAccountThatLeftItsPoolMovesOffItSayingSo(t *testing.T) {
	// The session, of Default, is on side, which moves to Acme.
	tests := []struct {
		name string
		// idle is how long ago the session's last request was.
		idle time.Duration
	}{
		{name: "its cache warm", idle: 5 * time.Minute},
		{name: "its cache cold", idle: 2 * time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r := newPoolsRouter(t, at(start), true)
			readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek, "personal": latestWeek})
			assignFor(r.sessions, Request{Session: "one", Model: opus, Pool: defaultPool}, decision{account: "side", reason: reasonNew}, start.Add(-tt.idle))
			moved := pooledAccounts()
			moved[2].Pool = acmePool
			r.proxy.pools.follow(moved, testPools)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			if w := routeIn(t, r, workToken, "one", defaultPool, nil); w.Code != http.StatusOK {
				t.Errorf("answered %d, want 200", w.Code)
			}

			want := status.ReasonMovedOff + "side " + status.WhyLeftPool
			if sent := upstream.sent(); !slices.Equal(sent, []string{"work"}) {
				t.Errorf("the request went out on %q, want work alone, Default's one account left", sent)
			}
			if got := reasonsOf(lines()); !slices.Equal(got, []string{want}) {
				t.Errorf("the ledger's lines give the reasons %q, want one line, its reason %q", got, want)
			}
			if routed := []string{"msg=routed", "account=work", `reason="` + want + `"`}; !log.Has(routed...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, routed)
			}
			if a := r.sessions.lookup(key{session: "one", model: opus}).current; a.Account != "work" {
				t.Errorf("the session is on %q, want work", a.Account)
			}
		})
	}
}

func TestTheHeirOfAnAccountThatLeavesTheConfig(t *testing.T) {
	// Default has work, its primary, and side; Acme has extra, then
	// personal, its primary; Spare has none.
	pooled := config.Accounts{
		{ID: "work", Pool: defaultPool, Primary: true},
		{ID: "extra", Pool: acmePool},
		{ID: "personal", Pool: acmePool, Primary: true},
		{ID: "side", Pool: defaultPool},
	}
	withSpare := append(slices.Clone(testPools), config.Pool{ID: "z5k2", Label: "Spare", Dirs: []string{"/work/spare"}})
	tests := []struct {
		name     string
		accounts config.Accounts
		pools    config.Pools
		// first is the first pool the accounts record notes, and was the
		// pool the account that leaves was in, "" for none known.
		first, was string
		want       string
		wantOK     bool
	}{
		{name: "in Acme: Acme's primary", accounts: pooled, pools: testPools, was: acmePool, want: "personal", wantOK: true},
		{name: "in Acme, its id in another case", accounts: pooled, pools: testPools, was: "Q7MT", want: "personal", wantOK: true},
		{name: "in Acme, none marked its primary: its first", accounts: unmarked(pooled), pools: testPools, was: acmePool, want: "extra", wantOK: true},
		{name: "its pool not known: the first's", accounts: pooled, pools: testPools, want: "work", wantOK: true},
		{name: "its pool not known, the record noting Acme the first: Acme's", accounts: pooled, pools: testPools, first: acmePool, want: "personal", wantOK: true},
		{name: "its pool gone: the first's", accounts: pooled, pools: testPools, was: "z5k2", want: "work", wantOK: true},
		{name: "its pool without accounts: the first's", accounts: pooled, pools: withSpare, was: "z5k2", want: "work", wantOK: true},
		{
			name: "its pool and the first without accounts: the primary", accounts: pooled[1:3], pools: testPools, was: "z5k2",
			want: "personal", wantOK: true,
		},
		{name: "a config from before pools: the primary", accounts: testConfigured, was: acmePool, want: "work", wantOK: true},
		{name: "no accounts configured", pools: testPools, was: acmePool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := heirs{accounts: tt.accounts, pools: tt.pools, first: func() string { return tt.first }, poolWas: func(string) string { return tt.was }}
			got, ok := h.of("gone")
			if got.ID != tt.want || ok != tt.wantOK {
				t.Errorf("of(gone) = %q, %v, want %q, %v", got.ID, ok, tt.want, tt.wantOK)
			}
		})
	}
}

// unmarked returns accounts with none marked primary.
func unmarked(accounts config.Accounts) config.Accounts {
	accounts = slices.Clone(accounts)
	for i := range accounts {
		accounts[i].Primary = false
	}
	return accounts
}

func TestTheSessionsOfAnAccountThatLeavesTheConfigStayInItsPoolAsItsPrimarys(t *testing.T) {
	// Personal, Acme's primary, leaves the config, side moving to Acme as it
	// does, so Acme's primary now; personal's file still holds its token.
	// was gives the pool the router knows personal was in.
	const withoutPersonal = "[[account]]\nid = \"work\"\nlabel = \"Work\"\npool = \"" + defaultPool + "\"\n\n" +
		"[[account]]\nid = \"side\"\nlabel = \"Side\"\npool = \"" + acmePool + "\"\n\n" + defaultTOML + "\n" + acmeTOML
	tests := []struct {
		name, was string
		// wantHeir is the account personal's tokens count as, and wantSent
		// where Acme's request on personal's token went, none where it's
		// refused.
		wantHeir string
		wantSent []string
	}{
		{name: "known to have been in Acme: side's, and routed in Acme", was: acmePool, wantHeir: "side", wantSent: []string{"side"}},
		{name: "its pool gone: work's, the first's primary, and refused", was: "z5k2", wantHeir: "work"},
		{name: "its pool not known: work's, the first's primary, and refused", wantHeir: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r, path := newWatchingPoolsRouter(t, "")
			r.proxy.pools.poolWas = func(id string) string {
				if id == "personal" {
					return tt.was
				}
				return ""
			}
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream

			writeConfigFile(t, path, withoutPersonal)
			r.upkeep.look()
			w := routeIn(t, r, personalToken, "one", acmePool, nil)

			adopted := []string{"level=INFO", `msg="account no longer configured; its tokens count as the primary's for a week"`, "account=personal", "primary=" + tt.wantHeir}
			if !log.Has(adopted...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, adopted)
			}
			if a, ok := r.accounts.byToken(personalToken, start); !ok || a.ID != tt.wantHeir {
				t.Errorf("a request on personal's token is %q's (%v), want %s's, though personal's file still holds it", a.ID, ok, tt.wantHeir)
			}
			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("Acme's request on personal's token went out on %q, want %q", sent, tt.wantSent)
			}
			if tt.wantSent == nil {
				checkRefused(t, w, leftAcme)
			}
		})
	}
}

func TestTheTokensOfAnAccountRemovedWhileTheRouterWasAwayCountAsItsPoolsPrimarys(t *testing.T) {
	const goneToken = "test-token-gone"
	tests := []struct {
		name, was string
		want      string
	}{
		{name: "known to have been in Acme: personal's", was: acmePool, want: "personal"},
		{name: "its pool not known: work's, the first's primary", want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			path := filepath.Join(t.TempDir(), "state.json")
			putState(t, path, savedState{Version: stateVersion, Tokens: map[string]savedTokens{"gone": {SHA256: hash(goneToken)}}})
			as := resolve(pooledAccounts(), poolTokens.Read, unrecorded)
			changes := newChanges()
			usage := newState(as, unpooled(), testPolicy, claude.Provider{}.Family, at(start), changes.note, changes.routine)
			h := heirs{accounts: pooledAccounts(), pools: testPools, first: noFirst, poolWas: func(id string) string {
				if id == "gone" {
					return tt.was
				}
				return ""
			}}
			f := newStateFile(at(start), changes, newSessions(at(start), changes.note, changes.routine), as, usage, newYearsTold(changes.note), h)

			f.load(path)

			if a, ok := as.byToken(goneToken, start); !ok || a.ID != tt.want {
				t.Errorf("a request on gone's token is %q's (%v), want %s's", a.ID, ok, tt.want)
			}
		})
	}
}

// The detector is what holds this test too: a request reads the pools whole
// as it's checked, and again as it's chosen, however they change under it,
// and never goes out on another pool's account.
func TestARequestRacesAChangeToItsTokensPool(t *testing.T) {
	logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	inDefault := pooledAccounts()
	inDefault[1].Pool = defaultPool
	var mu sync.Mutex
	var codes []int
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
					w := routeIn(t, r, personalToken, "", acmePool, nil)
					mu.Lock()
					codes = append(codes, w.Code)
					mu.Unlock()
				}
			}
		})
	}

	answered := func() int {
		mu.Lock()
		defer mu.Unlock()
		return len(codes)
	}
	for i := 0; i < 200 || answered() < 100; i++ {
		if i%2 == 0 {
			r.proxy.pools.follow(inDefault, testPools)
		} else {
			r.proxy.pools.follow(pooledAccounts(), testPools)
		}
	}
	close(stop)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	for _, code := range codes {
		if code != http.StatusOK && code != leftPool.status && code != http.StatusTooManyRequests {
			t.Errorf("an Acme request was answered %d, want 200 on personal, switchboard's refusal, or its 429", code)
			break
		}
	}
	if i := slices.IndexFunc(upstream.sent(), func(id string) bool { return id != "personal" }); i >= 0 {
		t.Errorf("Acme's request %d went out on %s, want personal alone, Acme's one account", i, upstream.sent()[i])
	}
	r.proxy.pools.follow(inDefault, testPools)
	checkRefused(t, routeIn(t, r, personalToken, "", acmePool, nil), leftAcme)
}
