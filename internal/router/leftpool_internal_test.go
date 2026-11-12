package router

import (
	"encoding/json"
	"maps"
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
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// The words switchboard refuses a request with whose session's token has left
// its pool.
const (
	leftAcme         = "switchboard: this session's account has left pool Acme — exit and resume it to carry on in the pool its directory is in now"
	leftDefault      = "switchboard: this session's account has left pool Default — exit and resume it to carry on in the pool its directory is in now"
	leftItsPool      = "switchboard: this session's account has left its pool — exit and resume it to carry on in the pool its directory is in now"
	poolUnconfigured = "switchboard: this session's pool isn't configured — exit and resume it to carry on in the pool its directory is in now"
)

// The log's lines of a refusal, of an account that has left its pool and of
// a pool that isn't configured.
const (
	leftLogged         = `msg="session's account has left its pool; refusing the request"`
	unconfiguredLogged = `msg="session's pool isn't configured; refusing the request"`
)

// strangerToken is a token no account has, and goneToken one of gone, an
// account the config doesn't configure.
const (
	strangerToken = "test-token-stranger"
	goneToken     = "test-token-gone"
)

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

// accountTOML is an account as a config file writes it, labelled by its id,
// in the pool given, and marked its pool's primary where primary is set.
func accountTOML(id, pool string, primary bool) string {
	account := "[[account]]\nid = \"" + id + "\"\nlabel = \"" + id + "\"\npool = \"" + pool + "\"\n"
	if primary {
		account += "primary = true\n"
	}
	return account + "\n"
}

func TestASessionWhoseTokensAccountLeavesItsPoolIsRefusedWhileOneInThePoolItJoinedCarriesOn(t *testing.T) {
	log := logstest.Capture(t)
	r := newPoolsRouter(t, at(start), true)
	readPersonalFirst(r)
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	lines := keepingLedger(t, r)
	r.proxy.pools.follow(movedPools(), testPools)

	// Acme's session started on personal's token, as Acme's primary's,
	// launched pinned to work, Default's; Default's has personal's token
	// since personal joined Default. Acme has side, with room, all the same.
	acmes := routeIn(t, r, personalToken, "one", acmePool, map[string]string{PinHeader: "work"})
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
	if log.Has("pin ignored") {
		t.Errorf("log reads\n%s\nwant no pin read of a request refused", log)
	}
}

func TestARefusalSaysWhyAndNamesThePoolWhereThereAreSeveral(t *testing.T) {
	tests := []struct {
		name string
		// follow are the accounts and pools the router takes up, as the
		// config makes them; token and pool are what the request carries.
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
			name:   "its account moved to the one pool left, which it names no more",
			follow: movedPools()[:2], followPools: testPools[:1],
			token: personalToken, pool: acmePool,
			want: poolUnconfigured, wantLog: unconfiguredLogged,
		},
		{
			name:   "naming a pool the config doesn't have",
			follow: pooledAccounts(), followPools: testPools,
			token: workToken, pool: "z5k2",
			want: poolUnconfigured, wantLog: unconfiguredLogged,
		},
		{
			name:   "its account, of Acme, out of the config, the one pool left its session's: no pool named",
			follow: config.Accounts{{ID: "work", Pool: defaultPool, Primary: true}, {ID: "side", Pool: defaultPool}}, followPools: testPools[:1],
			token: personalToken, pool: defaultPool,
			want: leftItsPool, wantLog: leftLogged,
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

func TestAConfigWithoutPoolsAnswersAsBeforePools(t *testing.T) {
	// Personal's quota needs using first, so every routed request goes out
	// on personal, whatever its token or the pool it names; one passed
	// through goes out as it came, on its own token.
	tests := []struct {
		name string
		// routed has the request sent to /v1/messages, which is routed, else
		// to /v1/files, which is passed through; token and pool are what it
		// carries.
		routed      bool
		token, pool string
		wantSent    []string
	}{
		{name: "routed, naming a pool", routed: true, token: workToken, pool: acmePool, wantSent: []string{"personal"}},
		{name: "routed on personal's token, naming another", routed: true, token: personalToken, pool: defaultPool, wantSent: []string{"personal"}},
		{name: "routed, naming none", routed: true, token: sideToken, wantSent: []string{"personal"}},
		{name: "passed through, naming a pool", token: workToken, pool: acmePool, wantSent: []string{"work"}},
		{name: "passed through, naming none", token: sideToken, wantSent: []string{"side"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r := newPoolsRouter(t, at(start), false)
			readPersonalFirst(r)
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream
			lines := keepingLedger(t, r)

			var w *httptest.ResponseRecorder
			if tt.routed {
				w = routeIn(t, r, tt.token, "one", tt.pool, nil)
			} else {
				w = passIn(t, r, "/v1/files", tt.token, tt.pool)
			}

			if got := w.Body.String(); w.Code != http.StatusOK || got != `{"type":"message"}` {
				t.Errorf("answered %d %q, want the upstream's 200 and its body", w.Code, got)
			}
			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request went out on %q, want %q", sent, tt.wantSent)
			}
			got := lines()
			switch {
			case !tt.routed && len(got) > 0:
				t.Errorf("the ledger holds %+v, want no line of a request passed through", got)
			case tt.routed && (len(got) != 1 || got[0].Account != "personal" || got[0].Pool != "" || got[0].Status != http.StatusOK):
				t.Errorf("the ledger holds %+v, want one line, on personal, of no pool, answered 200", got)
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

func TestARequestPassedThroughIsRefusedOnlyWhereItNamesAPoolItsTokensAccountIsOutside(t *testing.T) {
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
		{name: "naming a pool the config doesn't have", pooled: true, token: workToken, pool: "z5k2", wantRefusal: poolUnconfigured},
		{name: "naming none, its token's account in the first", pooled: true, token: workToken, wantSent: []string{"work"}},
		{name: "naming none, its token's account outside the first", pooled: true, token: personalToken, wantSent: []string{"personal"}},
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

// heirTestAccounts are the accounts the heir tests configure: Default has
// work, its primary, and side; Acme has extra, then personal, its primary.
func heirTestAccounts() config.Accounts {
	return config.Accounts{
		{ID: "work", Pool: defaultPool, Primary: true},
		{ID: "extra", Pool: acmePool},
		{ID: "personal", Pool: acmePool, Primary: true},
		{ID: "side", Pool: defaultPool},
	}
}

// held returns the accounts a router holds with the given ids, each with the
// token poolTokens gives it, if any.
func held(ids ...string) accounts {
	configured := make(config.Accounts, len(ids))
	for i, id := range ids {
		configured[i] = config.Account{ID: id}
	}
	return resolve(configured, poolTokens.Read, unrecorded)
}

func TestTheHeirOfAnAccountThatLeavesTheConfigIsTheFirstOfItsCandidatesTheRouterHolds(t *testing.T) {
	pooled := heirTestAccounts()
	withSpare := append(slices.Clone(testPools), config.Pool{ID: "z5k2", Label: "Spare", Dirs: []string{"/work/spare"}})
	everyone := []string{"work", "extra", "personal", "side"}
	tests := []struct {
		name     string
		accounts config.Accounts
		pools    config.Pools
		// first is the first pool the accounts record notes, and was the
		// pool the account that leaves was in, "" for none known; held are
		// the accounts the router holds, every one where it's nil.
		first, was string
		held       []string
		want       string
		wantOK     bool
	}{
		{name: "in Acme: Acme's primary", accounts: pooled, pools: testPools, was: acmePool, want: "personal", wantOK: true},
		{name: "in Acme, its id in another case", accounts: pooled, pools: testPools, was: "Q7MT", want: "personal", wantOK: true},
		{name: "in Acme, none marked its primary: its first", accounts: unmarked(pooled), pools: testPools, was: acmePool, want: "extra", wantOK: true},
		{
			name: "in Acme, its primary added since the router started: the first's", accounts: pooled, pools: testPools, was: acmePool,
			held: []string{"work", "extra", "side"}, want: "work", wantOK: true,
		},
		{name: "its pool not known: the first's", accounts: pooled, pools: testPools, want: "work", wantOK: true},
		{name: "its pool not known, the record noting Acme the first: Acme's", accounts: pooled, pools: testPools, first: acmePool, want: "personal", wantOK: true},
		{name: "its pool gone: the first's", accounts: pooled, pools: testPools, was: "z5k2", want: "work", wantOK: true},
		{name: "its pool without accounts: the first's", accounts: pooled, pools: withSpare, was: "z5k2", want: "work", wantOK: true},
		{
			name: "its pool and the first without accounts: the primary", accounts: pooled[1:3], pools: testPools, was: "z5k2",
			want: "personal", wantOK: true,
		},
		{name: "no candidate held", accounts: pooled, pools: testPools, was: acmePool, held: []string{"side"}},
		{name: "a config from before pools: the primary", accounts: testConfigured, was: acmePool, want: "work", wantOK: true},
		{name: "no accounts configured", pools: testPools, was: acmePool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := heirs{accounts: tt.accounts, pools: tt.pools, first: func() string { return tt.first }, poolWas: func(string) string { return tt.was }}
			ids := tt.held
			if ids == nil {
				ids = everyone
			}
			got, ok := held(ids...).heir(h, "gone")
			if got.ID != tt.want || ok != tt.wantOK {
				t.Errorf("heir(gone) = %q, %v, want %q, %v", got.ID, ok, tt.want, tt.wantOK)
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

func TestTheFirstPoolIsReadForAnHeirOnlyWhereNeededOnceALook(t *testing.T) {
	reads := 0
	m := newMembership(func() string {
		reads++
		return ""
	}, sameID, noPoolKnown)
	m.follow(heirTestAccounts(), testPools)
	as := held("work", "extra", "personal", "side")

	h := m.heirs(heirTestAccounts(), testPools)
	if heir, _ := as.heir(h, "personal"); heir.ID != "personal" || reads != 1 {
		t.Fatalf("personal's heir is %q, the first pool read %d times, want personal, read once, as the pools were followed", heir.ID, reads)
	}
	for range 2 {
		if heir, _ := as.heir(h, "gone"); heir.ID != "work" {
			t.Errorf("gone's heir is %q, want work, the first pool's primary, gone's pool not known", heir.ID)
		}
	}
	if reads != 2 {
		t.Errorf("the first pool was read %d times, want twice: as the pools were followed, then once for the heirs of a look", reads)
	}
}

func TestAnHeirIsOfThePoolTheAccountsRecordKeepsWhereThePoolsCantTell(t *testing.T) {
	// Gone left the config before the router started; personal is in Acme.
	m := newMembership(noFirst, sameID, func(id string) string {
		if id == "gone" {
			return acmePool
		}
		return ""
	})
	h := m.heirs(heirTestAccounts(), testPools)
	m.follow(heirTestAccounts(), testPools)
	as := held("work", "extra", "personal", "side")

	if heir, _ := as.heir(h, "gone"); heir.ID != "personal" {
		t.Errorf("gone's heir is %q, want personal, Acme's primary, as the record keeps gone in Acme", heir.ID)
	}
	if heir, _ := as.heir(h, "side"); heir.ID != "work" {
		t.Errorf("side's heir is %q, want work, Default's primary, as the pools followed since the heirs were made have side in Default", heir.ID)
	}
}

func TestWhetherARequestsTokenHasLeftItsPool(t *testing.T) {
	// The pools as pooledAccounts make them, but for personal, of Acme,
	// which has left the config; gone left it before the router started.
	withoutPersonal := slices.DeleteFunc(pooledAccounts(), func(a config.Account) bool { return a.ID == "personal" })
	of := func(id, removed string) sender {
		return sender{ID: id, removed: removed}
	}
	tests := []struct {
		name string
		// unpooled has the config make no pools; named is the pool the
		// request names, s who sent it, and recorded the pool the accounts
		// record keeps gone in.
		unpooled        bool
		named, recorded string
		s               sender
		wantPool        string
		wantLeft        bool
	}{
		{name: "work's, in its pool", named: defaultPool, s: of("work", ""), wantPool: defaultPool},
		{name: "work's, naming Acme", named: acmePool, s: of("work", ""), wantPool: acmePool, wantLeft: true},
		{name: "side's, naming none: the first", s: of("side", ""), wantPool: defaultPool},
		{name: "naming a pool the config doesn't have", named: "z5k2", s: of("work", ""), wantPool: "z5k2", wantLeft: true},
		{name: "personal's own, it having left the config, naming Acme, where it was", named: acmePool, s: of("personal", ""), wantPool: acmePool},
		{name: "personal's own, naming Default", named: defaultPool, s: of("personal", ""), wantPool: defaultPool, wantLeft: true},
		{name: "taken up from personal, naming Acme", named: acmePool, s: of("work", "personal"), wantPool: acmePool},
		{name: "taken up from personal, naming Default", named: defaultPool, s: of("work", "personal"), wantPool: defaultPool, wantLeft: true},
		{name: "taken up from gone, its pool not known: in the pool it names", named: acmePool, s: of("work", "gone"), wantPool: acmePool},
		{name: "taken up from gone, recorded in Acme, naming Acme", named: acmePool, recorded: acmePool, s: of("work", "gone"), wantPool: acmePool},
		{name: "taken up from gone, recorded in Default, naming Acme", named: acmePool, recorded: defaultPool, s: of("work", "gone"), wantPool: acmePool, wantLeft: true},
		{name: "a config from before pools", unpooled: true, named: acmePool, s: of("work", "personal")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newMembership(noFirst, sameID, func(id string) string {
				if id == "gone" {
					return tt.recorded
				}
				return ""
			})
			if tt.unpooled {
				m.follow(testConfigured, nil)
			} else {
				m.follow(pooledAccounts(), testPools)
				m.follow(withoutPersonal, testPools)
			}
			p := &proxy{pools: m}

			pool, left := p.strays(m.current(), tt.named, tt.s)
			if pool != tt.wantPool || left != tt.wantLeft {
				t.Errorf("strays(%q, %+v) = %q, %v, want %q, %v", tt.named, tt.s, pool, left, tt.wantPool, tt.wantLeft)
			}
		})
	}
}

func TestTheSessionsOfAnAccountThatLeavesTheConfigStayInItsPool(t *testing.T) {
	// Personal, Acme's primary, leaves the config; its file still holds its
	// token, which its sessions send.
	pools := defaultTOML + "\n" + acmeTOML
	tests := []struct {
		name string
		// config is what the config file comes to hold, and pool the pool
		// the request names.
		config, pool string
		// wantHeir is the account personal's tokens count as; wantSent is
		// where the request went, and wantCode what it was answered, or
		// wantRefusal what switchboard refused it saying.
		wantHeir    string
		wantSent    []string
		wantCode    int
		wantRefusal string
	}{
		{
			name:   "side moved to Acme, its primary now: routed in Acme",
			config: accountTOML("work", defaultPool, false) + accountTOML("side", acmePool, false) + pools, pool: acmePool,
			wantHeir: "side", wantSent: []string{"side"}, wantCode: http.StatusOK,
		},
		{
			name: "Acme's primary one the config added since the router started, side moved to it: routed in Acme, the first's primary the heir",
			config: accountTOML("work", defaultPool, false) + accountTOML("side", acmePool, false) + accountTOML("fresh", acmePool, true) +
				pools,
			pool: acmePool, wantHeir: "work", wantSent: []string{"side"}, wantCode: http.StatusOK,
		},
		{
			name:   "Acme left with no accounts: answered in Acme, as no account in it takes the request",
			config: accountTOML("work", defaultPool, false) + accountTOML("side", defaultPool, false) + pools, pool: acmePool,
			wantHeir: "work", wantCode: http.StatusTooManyRequests,
		},
		{
			name:   "Acme gone with it: refused",
			config: accountTOML("work", defaultPool, false) + accountTOML("side", defaultPool, false) + defaultTOML, pool: acmePool,
			wantHeir: "work", wantRefusal: poolUnconfigured,
		},
		{
			name:   "a session of Default on its token, side moved to Acme: refused, personal having been in Acme",
			config: accountTOML("work", defaultPool, false) + accountTOML("side", acmePool, false) + pools, pool: defaultPool,
			wantHeir: "side", wantRefusal: leftDefault,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			r, path := newWatchingPoolsRouter(t, "")
			upstream := servingAll(served, served, served)
			r.proxy.transport = upstream

			writeConfigFile(t, path, tt.config)
			r.upkeep.look()
			w := routeIn(t, r, personalToken, "one", tt.pool, nil)

			adopted := []string{"level=INFO", `msg="account no longer configured; its tokens count as the primary's for a week"`, "account=personal", "primary=" + tt.wantHeir}
			if !log.Has(adopted...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, adopted)
			}
			if a, ok := r.accounts.byToken(personalToken, start); !ok || a.ID != tt.wantHeir {
				t.Errorf("a request on personal's token is %q's (%v), want %s's, though personal's file still holds it", a.ID, ok, tt.wantHeir)
			}
			if sent := upstream.sent(); !slices.Equal(sent, tt.wantSent) {
				t.Errorf("the request on personal's token went out on %q, want %q", sent, tt.wantSent)
			}
			if tt.wantRefusal != "" {
				checkRefused(t, w, tt.wantRefusal)
			} else if w.Code != tt.wantCode {
				t.Errorf("answered %d %s, want %d", w.Code, w.Body, tt.wantCode)
			}
		})
	}
}

func TestTokensTakenUpFromAnAccountStayItsAsTheAccountThatTookThemLeavesInTurn(t *testing.T) {
	// Personal, of Acme, leaves the config, Acme left with no accounts, so
	// work, the first's primary, takes its tokens up; then work leaves,
	// side moved to Acme, and side, the one account left, takes work's up,
	// personal's among them; then personal is configured again.
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	pools := defaultTOML + "\n" + acmeTOML

	writeConfigFile(t, path, accountTOML("work", defaultPool, false)+accountTOML("side", defaultPool, false)+pools)
	r.upkeep.look()
	writeConfigFile(t, path, accountTOML("side", acmePool, false)+pools)
	r.upkeep.look()

	if s, ok := r.accounts.sender(personalToken, start); !ok || s.ID != "side" || s.removed != "personal" {
		t.Errorf("personal's token is sent as %q's, taken up from %q (%v), want as side's, taken up from personal", s.ID, s.removed, ok)
	}
	acmes := routeIn(t, r, personalToken, "one", acmePool, nil)
	defaults := routeIn(t, r, personalToken, "two", defaultPool, nil)
	if acmes.Code != http.StatusOK {
		t.Errorf("Acme's request on personal's token was answered %d %s, want 200: personal was in Acme", acmes.Code, acmes.Body)
	}
	checkRefused(t, defaults, leftDefault)
	if sent := upstream.sent(); !slices.Equal(sent, []string{"side"}) {
		t.Errorf("the requests went out on %q, want Acme's alone, on side", sent)
	}

	writeConfigFile(t, path, accountTOML("personal", acmePool, false)+accountTOML("side", acmePool, false)+pools)
	r.upkeep.look()

	if side, _ := r.accounts.byID("side"); side.secret.was(hash(personalToken), start) {
		t.Error("side keeps personal's token as its own once personal is configured again, want it given back")
	}
}

func TestTheSessionsOfAnAccountThatLeftTheConfigStayInItsPoolAsTheConfigChangesAgain(t *testing.T) {
	// Personal, Acme's primary, leaves the config, side moved to Acme in its
	// place; then Acme's directory changes, and nothing else.
	logstest.Capture(t)
	r, path := newWatchingPoolsRouter(t, "")
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream
	accounts := accountTOML("work", defaultPool, false) + accountTOML("side", acmePool, false)

	writeConfigFile(t, path, accounts+defaultTOML+"\n"+acmeTOML)
	r.upkeep.look()
	writeConfigFile(t, path, accounts+defaultTOML+"\n"+strings.Replace(acmeTOML, "/work/acme", "/work/acme-labs", 1))
	r.upkeep.look()

	if acme, _ := r.proxy.pools.current().pools.ByID(acmePool); !slices.Equal(acme.Dirs, []string{"/work/acme-labs"}) {
		t.Fatalf("Acme's directories are %q, want the second config's followed", acme.Dirs)
	}
	acmes := routeIn(t, r, personalToken, "one", acmePool, nil)
	defaults := routeIn(t, r, personalToken, "two", defaultPool, nil)
	if acmes.Code != http.StatusOK {
		t.Errorf("Acme's request on personal's token was answered %d %s, want 200: personal was in Acme", acmes.Code, acmes.Body)
	}
	checkRefused(t, defaults, leftDefault)
	if sent := upstream.sent(); !slices.Equal(sent, []string{"side"}) {
		t.Errorf("the requests went out on %q, want Acme's alone, on side", sent)
	}
}

func TestATableKnowsThePoolEachAccountThatLeftTheTablesBeforeItWasInLast(t *testing.T) {
	without := func(ids ...string) *poolTable {
		accounts := slices.DeleteFunc(pooledAccounts(), func(a config.Account) bool { return slices.Contains(ids, a.ID) })
		return newPoolTable(accounts, testPools, "")
	}
	every := without()
	noPersonal := without("personal").after(every)
	noSide := without("personal", "side").after(noPersonal)
	tests := []struct {
		name string
		got  *poolTable
		want map[string]string
	}{
		{name: "personal leaves", got: noPersonal, want: map[string]string{"personal": acmePool}},
		{name: "side leaves after: personal's carried across", got: noSide, want: map[string]string{"personal": acmePool, "side": defaultPool}},
		{name: "personal configured again: side's alone", got: without("side").after(noSide), want: map[string]string{"side": defaultPool}},
		{name: "none leaves: personal's still carried", got: without("personal").after(noPersonal), want: map[string]string{"personal": acmePool}},
		{name: "side leaves a config from before pools: nothing known", got: without("side").after(newPoolTable(testConfigured, nil, "")), want: map[string]string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !maps.Equal(tt.got.was, tt.want) {
				t.Errorf("the table knows the accounts that left in %q, want %q", tt.got.was, tt.want)
			}
		})
	}
}

func TestARemovedAccountsSessionsAreRoutedInTheirPoolAfterARestartWhereItsPoolIsntKnown(t *testing.T) {
	// The router starts without personal, Acme's primary, side in Acme in
	// its place; the state file keeps personal's token.
	logstest.Capture(t)
	path := filepath.Join(t.TempDir(), "state.json")
	putState(t, path, savedState{Version: stateVersion, Tokens: map[string]savedTokens{"personal": {SHA256: hash(personalToken)}}})
	cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
	cfg.Accounts = config.Accounts{{ID: "work", Label: "Work", Pool: defaultPool}, {ID: "side", Label: "Side", Pool: acmePool}}
	cfg.Pools = testPools
	r := newTestRouterOf(t, cfg)
	r.file.load(path)
	upstream := servingAll(served, served, served)
	r.proxy.transport = upstream

	acmes := routeIn(t, r, personalToken, "one", acmePool, nil)
	defaults := routeIn(t, r, personalToken, "two", defaultPool, nil)

	if acmes.Code != http.StatusOK || defaults.Code != http.StatusOK {
		t.Errorf("Acme's request was answered %d and Default's %d, want 200 each: nothing says personal was in another pool", acmes.Code, defaults.Code)
	}
	if sent := upstream.sent(); !slices.Equal(sent, []string{"side", "work"}) {
		t.Errorf("the requests went out on %q, want Acme's on side and Default's on work, each in its own pool", sent)
	}
}

func TestTheTokensOfAnAccountRemovedWhileTheRouterWasAwayCountAsItsPoolsPrimarys(t *testing.T) {
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

func TestARouterStartedWithoutAnAccountGivesItsTokensToTheFirstPoolsPrimaryAsTheRecordNotesIt(t *testing.T) {
	// The state file keeps gone's token, which the config no longer has,
	// nor the accounts record its pool.
	tests := []struct {
		name, first string
		want        string
	}{
		{name: "the record noting Acme the first pool: personal's, Acme's primary", first: acmePool, want: "personal"},
		{name: "the record noting none: work's, the config's first's primary", want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			stateDir := t.TempDir()
			if tt.first != "" {
				noteFirstPool(t, stateDir, tt.first)
			}
			path := filepath.Join(stateDir, stateFileName)
			putState(t, path, savedState{Version: stateVersion, Tokens: map[string]savedTokens{"gone": {SHA256: hash(goneToken)}}})
			cfg := testConfig(at(start), &stubProber{}, poolTokens.Read)
			cfg.Accounts, cfg.Pools, cfg.StateDir = pooledAccounts(), testPools, stateDir
			r := newTestRouterOf(t, cfg)

			r.file.load(path)

			if a, ok := r.accounts.byToken(goneToken, start); !ok || a.ID != tt.want {
				t.Errorf("a request on gone's token is %q's (%v), want %s's", a.ID, ok, tt.want)
			}
		})
	}
}

func TestARemovedAccountsOwnTokenIsRoutedWithoutPools(t *testing.T) {
	// Side leaves a config without pools, its file still holding its token,
	// which a session of it sends; work, the primary, takes its tokens up.
	tests := []struct {
		name string
		// files are what the token files hold as the router starts; after
		// readies the router once side has left.
		files tokenstest.Files
		after func(r *Router, files *changingFiles, clock *testClock)
	}{
		{
			name:  "work without a usable token",
			files: tokenstest.Files{"personal": personalToken, "side": sideToken},
			after: func(*Router, *changingFiles, *testClock) {},
		},
		{
			name:  "a week on, its token no longer work's",
			files: poolTokens,
			after: func(r *Router, _ *changingFiles, clock *testClock) {
				clock.now = start.Add(formerFor)
				r.accounts.forget(clock.now)
			},
		},
		{
			name:  "its file taking up a new token, work without a usable token",
			files: tokenstest.Files{"personal": personalToken, "side": sideToken},
			after: func(r *Router, files *changingFiles, _ *testClock) {
				files.set(tokenstest.Files{"personal": personalToken, "side": renewedToken})
				r.upkeep.look()
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			clock := &testClock{now: start}
			files := &changingFiles{files: tt.files}
			path := filepath.Join(t.TempDir(), "config.toml")
			writeConfigFile(t, path, unpooledTOML("work", "personal", "side"))
			cfg := testConfig(clock.read, &stubProber{}, files.read)
			cfg.ConfigFile = Watch(path)
			r := newTestRouterOf(t, cfg)
			upstream := servingAll(served, served, served)
			upstream.answers[renewedToken] = []answer{served}
			r.proxy.transport = upstream

			writeConfigFile(t, path, unpooledTOML("work", "personal"))
			r.upkeep.look()
			tt.after(r, files, clock)
			w := routeIn(t, r, sideToken, "one", "", nil)

			if a, ok := r.accounts.byToken(sideToken, clock.now); !ok || a.ID != "side" {
				t.Errorf("a request on side's token is %q's (%v), want side's own", a.ID, ok)
			}
			if w.Code != http.StatusOK || !log.Has("msg=routed", "status=200") {
				t.Errorf("answered %d, log reads\n%s\nwant it routed and answered 200, never passed through unrouted", w.Code, log)
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
