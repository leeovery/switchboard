package router

import (
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

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
	r.state.refuse("side", http.StatusUnauthorized)
	r.state.limit("work", nil, time.Time{})

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
// clock, which stops in sleep, doesn't.
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
