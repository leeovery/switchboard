package router

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
)

func TestTheRoutersClockKeepsTheWallsTimeThroughASleep(t *testing.T) {
	clock := newSleepingClock(t)
	r := newTestRouter(t, clock.read, &stubProber{})
	awake := clock.read().Round(0)
	roomy := []quota.Window{
		{Key: "5h", Utilization: 0.1, ResetsAt: awake.Add(4 * time.Hour)},
		{Key: "7d", Utilization: 0.5, ResetsAt: awake.Add(3 * 24 * time.Hour)},
	}
	r.state.record("work", roomy, r.state.mark())
	r.state.record("side", roomy, r.state.mark())
	req := Request{Session: "one", Model: opus, Client: "work"}
	choose(t.Context(), r, req)
	for range minFailures {
		r.health.record(awake, true)
	}
	r.state.refuse("side", http.StatusUnauthorized, someRequest)
	r.state.limit("work", nil, time.Time{}, r.state.mark())

	clock.sleep(2 * time.Hour)
	if got := r.health.report(); !got.Healthy || got.Requests > 0 {
		t.Errorf("after two hours asleep, health = %+v, want healthy, the failures before it forgotten", got)
	}
	if got := r.Status().Sessions; got != 0 {
		t.Errorf("after two hours asleep, %d sessions are active, want none", got)
	}
	for _, id := range []string{"work", "side"} {
		if !r.state.view(opus, clock.read()).room(id) {
			t.Errorf("after two hours asleep, %s is still barred, want its bar lifted", id)
		}
	}
	if got := choose(t.Context(), r, req); !strings.HasPrefix(got.Reason, "rescored after 2h") {
		t.Errorf("after two hours asleep, the session's choice = %+v, want it rescored, its cache cold", got)
	}
}

func TestTheRouterStartsItsConnectionsAfreshOnceItNoticesTheMacWake(t *testing.T) {
	tests := []struct {
		name string
		// between moves the clock on between two looks.
		between  func(c *sleepingClock)
		wantWake bool
	}{
		{name: "an hour awake", between: func(c *sleepingClock) { c.pass(time.Hour) }},
		{name: "the clock set on a moment", between: func(c *sleepingClock) { c.sleep(minSleep - time.Second) }},
		{name: "the clock set back an hour", between: func(c *sleepingClock) { c.sleep(-time.Hour) }},
		{name: "asleep as long as a sleep takes to count", between: func(c *sleepingClock) { c.sleep(minSleep) }, wantWake: true},
		{name: "an hour asleep", between: func(c *sleepingClock) { c.sleep(time.Hour) }, wantWake: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			clock := newSleepingClock(t)
			r := newTestRouter(t, clock.read, &stubProber{})
			before := r.proxy.transport.(*pool).current()

			r.upkeep.wakes.look()
			tt.between(clock)
			r.upkeep.wakes.look()
			if renewed := r.proxy.transport.(*pool).current() != before; renewed != tt.wantWake {
				t.Errorf("the upstream transport renewed = %v, want %v", renewed, tt.wantWake)
			}
			if woke := log.Has("level=INFO", `msg="woke from sleep"`); woke != tt.wantWake {
				t.Errorf("log reads\n%s\nwant a wake logged: %v", log, tt.wantWake)
			}
		})
	}
}

func TestFailuresOfRequestsThatArrivedBeforeAWakeWasNoticedDontCount(t *testing.T) {
	clock := newSleepingClock(t)
	r := newTestRouter(t, clock.read, &stubProber{})
	r.upkeep.wakes.look()
	// One request arrives before the Mac sleeps, and another as it wakes,
	// before the router notices.
	beforeSleep := clock.read().Round(0)
	clock.sleep(time.Hour)
	onWaking := clock.read().Round(0)
	clock.pass(2 * time.Second)
	r.upkeep.wakes.look()

	for range minFailures {
		r.health.record(beforeSleep, true)
		r.health.record(onWaking, true)
	}
	if got := r.health.report(); !got.Healthy || got.Requests > 0 {
		t.Errorf("health = %+v, want healthy, counting none of the failures of requests that arrived before the wake was noticed", got)
	}
	since := clock.read().Round(0)
	for range minFailures {
		r.health.record(since, true)
	}
	if got := r.health.report(); got.Healthy {
		t.Errorf("health = %+v, want unhealthy: the failures of requests that arrived since count", got)
	}
}

func TestRequestsUnderWayAsAWakeIsNoticedFailWithoutCountingAgainstTheRouter(t *testing.T) {
	tests := []struct {
		name        string
		wake        bool
		wantHealthy bool
	}{
		{name: "the wake noticed while they're under way", wake: true, wantHealthy: true},
		{name: "no wake", wantHealthy: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The upstream takes each request in, and drops its connection
			// once the test lets it go.
			var arrived sync.WaitGroup
			arrived.Add(minFailures)
			release := make(chan struct{})
			up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				arrived.Done()
				<-release
				conn, _, err := http.NewResponseController(w).Hijack()
				if err != nil {
					t.Errorf("hijack the connection: %v", err)
					return
				}
				_ = conn.Close()
			}))
			t.Cleanup(up.Close)
			clock := newSleepingClock(t)
			r, err := New(Config{Accounts: testConfigured, Token: testTokens.Read, Upstream: up.URL, Provider: claude.Provider{}, Prober: &stubProber{}, Policy: testPolicy, Now: clock.read})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			proxy := httptest.NewServer(r.Proxy())
			t.Cleanup(proxy.Close)
			r.upkeep.wakes.look()

			var answered sync.WaitGroup
			for i := range minFailures {
				answered.Go(func() { askFailing(t, proxy.URL, "session-"+strconv.Itoa(i)) })
			}
			arrived.Wait()
			if tt.wake {
				clock.sleep(time.Hour)
				r.upkeep.wakes.look()
			}
			close(release)
			answered.Wait()
			if got := r.health.report(); got.Healthy != tt.wantHealthy {
				t.Errorf("once %d requests that arrived before the router noticed a wake failed, health = %+v, want healthy %v", minFailures, got, tt.wantHealthy)
			}
		})
	}
}

// askFailing sends a messages request of session to the proxy at url, which
// the router is to answer with a failure of its own: 502.
func askFailing(t *testing.T, url, session string) {
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url+"/v1/messages", strings.NewReader(`{"model":"`+opus+`","max_tokens":1}`))
	if err != nil {
		t.Error(err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+workToken)
	req.Header.Set(claude.SessionHeader, session)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Error(err)
		return
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("the router answered %d, want 502, the upstream having dropped the request", resp.StatusCode)
	}
}

// sleepingClock is a clock whose readings carry a monotonic reading, as
// time.Now's do, which a test moves on as the Mac sleeps. It's safe for
// concurrent use.
type sleepingClock struct {
	t *testing.T

	mu  sync.Mutex
	now time.Time
}

func newSleepingClock(t *testing.T) *sleepingClock {
	return &sleepingClock{t: t, now: time.Now()}
}

func (c *sleepingClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// sleep has the Mac sleep for d: the wall clock moves on, and the monotonic
// clock, which stops in sleep, doesn't. Less than nothing is the wall clock
// set back.
func (c *sleepingClock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = asleep(c.t, c.now, d)
}

// pass has d pass while the Mac is awake: both clocks move on.
func (c *sleepingClock) pass(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// asleep returns what a clock that read before reads after a sleep of d.
func asleep(t *testing.T, before time.Time, d time.Duration) time.Time {
	t.Helper()
	after := before.Add(d)
	// time offers no way to move a reading's wall clock without its
	// monotonic reading, so this moves the monotonic reading back, from the
	// second word of time.Time, where it's held in nanoseconds. The check
	// after proves the layout is as this takes it to be.
	(*[2]int64)(unsafe.Pointer(&after))[1] -= int64(d)
	if after.Sub(before) != 0 || after.Round(0).Sub(before.Round(0)) != d {
		t.Fatal("time.Time isn't laid out as asleep takes it to be, so it can't make a reading after a sleep")
	}
	return after
}
