package router

import (
	"iter"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// The tests of a session resumed in another pool: session one starts in
// Default, on its primary, work, as pairedWeeks have it, and is resumed in
// Acme's directories while the process it started as runs on in Default, so
// its requests come from both pools in turn, each on its pool's primary's
// token. Each pool's work is its own: the session is new to Acme, and stays
// where it is in Default.

// fromBothPools are the pools session one's requests come from, in turn, once
// it's resumed in Acme, with the token each carries: Default's process first.
var fromBothPools = []struct{ pool, token string }{
	{defaultPool, workToken}, {acmePool, personalToken},
	{defaultPool, workToken}, {acmePool, personalToken},
	{defaultPool, workToken}, {acmePool, personalToken},
}

// resumeInAcme has r route session one's requests from each pool in turn, as
// fromBothPools has them, a minute apart from start on clock.
func resumeInAcme(t *testing.T, r *Router, clock *setClock) {
	t.Helper()
	for i, from := range fromBothPools {
		clock.set(start.Add(time.Duration(i) * time.Minute))
		if w := routeIn(t, r, from.token, "one", from.pool, nil); w.Code != http.StatusOK {
			t.Fatalf("request %d, in pool %s, answered %d, want 200", i+1, from.pool, w.Code)
		}
	}
}

func TestASessionResumedInAnotherPoolIsNewThereAndStaysPutInTheFirst(t *testing.T) {
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, upstream := newPairedRouter(t, clock.read)
	lines := keepingLedger(t, r)

	resumeInAcme(t, r, clock)

	if got, want := upstream.sent(), []string{"work", "personal", "work", "personal", "work", "personal"}; !slices.Equal(got, want) {
		t.Errorf("the session's requests went out on %q, want %q: each pool's on its own account, neither flipping", got, want)
	}
	got := lines()
	if want := []string{reasonNew, reasonNew, reasonSticky, reasonSticky, reasonSticky, reasonSticky}; !slices.Equal(reasonsOf(got), want) {
		t.Errorf("the ledger's lines give the reasons %q, want %q: new in each pool, then sticky in each", reasonsOf(got), want)
	}
	if from := ofEachLine(got, func(l ledger.Line) string { return l.From }); slices.ContainsFunc(from, func(id string) bool { return id != "" }) {
		t.Errorf("the ledger's lines name the accounts moved from %q, want none: nothing moved", from)
	}
	if moves := movesOf(r); len(moves) > 0 {
		t.Errorf("the router tells of the moves %+v, want none", moves)
	}
}

func TestEachPoolsDayOfASessionResumedInAnotherPoolHasItsOwnAccountsAlone(t *testing.T) {
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, _ := newPairedRouter(t, clock.read)
	lines := keepingLedger(t, r)
	resumeInAcme(t, r, clock)

	day, err := ledger.Summarise(dayfile.DateOf(start), slices.Values(lines()), noReadings, nil, ledger.Caps{}, clock.read())
	if err != nil {
		t.Fatal(err)
	}
	for pool, want := range map[string][]string{defaultPool: {"work"}, acmePool: {"personal"}} {
		var worked []string
		for _, a := range day.InPool(pool, defaultPool).Accounts {
			if !a.Idle() {
				worked = append(worked, a.Account)
			}
		}
		if !slices.Equal(worked, want) {
			t.Errorf("pool %s's day has work of %q, want %q alone", pool, worked, want)
		}
	}
}

// noReadings is a readings history that holds none.
func noReadings(time.Time, time.Time) iter.Seq[readings.Reading] {
	return func(func(readings.Reading) bool) {}
}

func TestASessionResumedInAnotherPoolIsListedInEachOnItsOwnAccount(t *testing.T) {
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, _ := newPairedRouter(t, clock.read)
	resumeInAcme(t, r, clock)

	for pool, want := range map[string][]string{defaultPool: {"work"}, acmePool: {"personal"}} {
		var listed []status.Session
		decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions", pool), ""), &listed)
		if len(listed) != 1 || listed[0].ID != "one" || !slices.Equal(assignedTo(listed[0]), want) {
			t.Errorf("pool %s lists the sessions %+v, want session one alone, on %q", pool, listed, want)
		}
		var named status.Session
		decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions/one", pool), ""), &named)
		if got := assignedTo(named); !slices.Equal(got, want) {
			t.Errorf("session one, asked after in pool %s, is on %q, want %q", pool, got, want)
		}
	}
	var named status.Session
	decodedOK(t, askControl(t, r, http.MethodGet, "/sessions/one", ""), &named)
	if got := assignedTo(named); !slices.Equal(got, []string{"personal"}) {
		t.Errorf("session one, asked after by its id alone, is on %q, want personal: Acme's, the pool it was routed in last", got)
	}
}

// assignedTo returns the account each of the session's assignments is on.
func assignedTo(s status.Session) []string {
	accounts := make([]string, len(s.Assignments))
	for i, a := range s.Assignments {
		accounts[i] = a.Account
	}
	return accounts
}

func TestASessionResumedInAnotherPoolLeavesAnAccountThatLeftOnePoolInThatPoolAlone(t *testing.T) {
	// Default's session is on side, Acme's on personal; then an account moves
	// from Default to Acme, and each pool's process sends its next request.
	tests := []struct {
		name string
		// moved is the account that moves to Acme.
		moved string
		// wantSent is where those requests went, and wantCode and wantReason
		// what Default's was answered and why its line says it went where it
		// did, as Acme's stays on personal.
		wantSent   []string
		wantCode   int
		wantReason string
	}{
		{
			name: "Default's account for the session: it moves off it in Default", moved: "side",
			wantSent: []string{"work", "personal"}, wantCode: http.StatusOK, wantReason: status.ReasonMovedOff + "side " + status.WhyLeftPool,
		},
		{
			name: "Default's primary, whose token its process holds: the process refused", moved: "work",
			wantSent: []string{"personal"}, wantCode: leftPool.status, wantReason: status.ReasonTokenLeftPool,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logstest.Capture(t)
			clock := &setClock{now: start}
			r, upstream := newPairedRouter(t, clock.read)
			readWeeks(r, map[string]quota.Window{"side": soonWeek, "work": laterWeek})
			resumeInAcme(t, r, clock)
			if got := upstream.sent()[:2]; !slices.Equal(got, []string{"side", "personal"}) {
				t.Fatalf("the session started on %q, want side in Default and personal in Acme", got)
			}
			was := len(upstream.sent())
			moveAccount(r, tt.moved, acmePool)
			lines := keepingLedger(t, r)

			clock.set(start.Add(10 * time.Minute))
			if w := routeIn(t, r, workToken, "one", defaultPool, nil); w.Code != tt.wantCode {
				t.Errorf("Default's request answered %d, want %d", w.Code, tt.wantCode)
			}
			routeIn(t, r, personalToken, "one", acmePool, nil)

			if got := upstream.sent()[was:]; !slices.Equal(got, tt.wantSent) {
				t.Errorf("the requests went out on %q, want %q", got, tt.wantSent)
			}
			if got, want := reasonsOf(lines()), []string{tt.wantReason, reasonSticky}; !slices.Equal(got, want) {
				t.Errorf("the ledger's lines give the reasons %q, want %q: Acme's session stays where it is", got, want)
			}
		})
	}
}

func TestAPinOfASessionResumedInAnotherPoolActsInOnePoolAlone(t *testing.T) {
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, _ := newPairedRouter(t, clock.read)
	resumeInAcme(t, r, clock)
	steps := []struct {
		name, method, target, body string
		// wantOn is the account the session answered with runs on, in the pool
		// it was pinned in, where it's answered 200; else wantRefused is what
		// the refusal says. wantPins are the session's pins in Default and in
		// Acme after.
		wantOn      string
		wantRefused string
		wantPins    [2]string
	}{
		{
			name: "pinned to a Default account: pinned in Default", method: http.MethodPost, target: "/sessions/one/pin", body: `{"account": "side"}`,
			wantOn: "work", wantPins: [2]string{"side", ""},
		},
		{
			name: "pinned to an Acme account: pinned in Acme, its pin in Default standing", method: http.MethodPost, target: "/sessions/one/pin", body: `{"account": "extra"}`,
			wantOn: "personal", wantPins: [2]string{"side", "extra"},
		},
		{
			name: "pinned in Acme, as named, to a Default account: refused", method: http.MethodPost, target: withPool("/sessions/one/pin", acmePool), body: `{"account": "side"}`,
			wantRefused: "side is in pool Default; this session is in pool Acme", wantPins: [2]string{"side", "extra"},
		},
		{
			name: "unpinned in Default, as named", method: http.MethodDelete, target: withPool("/sessions/one/pin", defaultPool),
			wantOn: "work", wantPins: [2]string{"", "extra"},
		},
		{
			name: "unpinned by its id alone: in Acme, the pool it was routed in last", method: http.MethodDelete, target: "/sessions/one/pin",
			wantOn: "personal", wantPins: [2]string{"", ""},
		},
	}
	for _, step := range steps {
		rec := askControl(t, r, step.method, step.target, step.body)
		if step.wantRefused != "" {
			if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), step.wantRefused) {
				t.Errorf("%s: answered %d %s, want 400 saying %q", step.name, rec.Code, rec.Body, step.wantRefused)
			}
		} else {
			var answered status.Session
			decodedOK(t, rec, &answered)
			if got := assignedTo(answered); !slices.Equal(got, []string{step.wantOn}) {
				t.Errorf("%s: answered with the session on %q, want it on %s", step.name, got, step.wantOn)
			}
		}
		if got := pinsOfOne(t, r); got != step.wantPins {
			t.Errorf("%s: the session's pins in Default and Acme are %q, want %q", step.name, got, step.wantPins)
		}
	}
}

func TestForceInOnePoolLeavesThePinOfASessionResumedInAnotherInTheOther(t *testing.T) {
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, _ := newPairedRouter(t, clock.read)
	resumeInAcme(t, r, clock)
	askedOK(t, r, http.MethodPost, "/sessions/one/pin", `{"account": "side"}`)
	askedOK(t, r, http.MethodPost, "/sessions/one/pin", `{"account": "extra"}`)

	askedOK(t, r, http.MethodDelete, withPool("/pin?force=true", acmePool), "")

	if got, want := pinsOfOne(t, r), [2]string{"side", ""}; got != want {
		t.Errorf("the session's pins in Default and Acme are %q, want %q: Acme's alone cleared", got, want)
	}
}

func TestAPinToAnAccountOfAPoolASessionRanInOnceIsRefusedWhereItRunsNow(t *testing.T) {
	// Session one ran in Default, on work, and a day on is resumed in Acme,
	// its process in Default gone: a pin to side, Default's, by its id alone,
	// acts in Acme, where it runs, which side is outside.
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, _ := newPairedRouter(t, clock.read)
	routeIn(t, r, workToken, "one", defaultPool, nil)
	clock.set(start.Add(26 * time.Hour))
	routeIn(t, r, personalToken, "one", acmePool, nil)

	rec := askControl(t, r, http.MethodPost, "/sessions/one/pin", `{"account": "side"}`)
	if want := "side is in pool Default; this session is in pool Acme"; rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), want) {
		t.Errorf("answered %d %s, want 400 saying %q", rec.Code, rec.Body, want)
	}
	if got := pinsOfOne(t, r); got != [2]string{} {
		t.Errorf("the session's pins in Default and Acme are %q, want none", got)
	}
}

func TestASessionResumedInAnotherPoolIsListedInFlightInThePoolItsRequestIsIn(t *testing.T) {
	logstest.Capture(t)
	clock := &setClock{now: start}
	r, upstream := newPairedRouter(t, clock.read)
	resumeInAcme(t, r, clock)
	sent, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	upstream.answers[personalToken] = []answer{func(req *http.Request) *http.Response {
		close(sent)
		<-release
		return served(req)
	}}
	go func() {
		defer close(done)
		routeIn(t, r, personalToken, "one", acmePool, nil)
	}()
	t.Cleanup(func() {
		close(release)
		<-done
	})
	select {
	case <-sent:
	case <-done:
		t.Fatal("Acme's request was answered without going out on personal")
	}

	for pool, want := range map[string]string{defaultPool: "", acmePool: status.Asking} {
		var listed []status.Session
		decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions", pool), ""), &listed)
		if len(listed) != 1 || len(listed[0].Assignments) != 1 || listed[0].Assignments[0].InFlight != want {
			t.Errorf("pool %s lists the sessions %+v, want session one with a request in flight %q", pool, listed, want)
		}
	}
}

// pinsOfOne returns session one's pins in Default and in Acme, as r answers
// for it in each.
func pinsOfOne(t *testing.T, r *Router) [2]string {
	t.Helper()
	var pins [2]string
	for i, pool := range []string{defaultPool, acmePool} {
		var s status.Session
		decodedOK(t, askControl(t, r, http.MethodGet, withPool("/sessions/one", pool), ""), &s)
		pins[i] = s.Pin
	}
	return pins
}
