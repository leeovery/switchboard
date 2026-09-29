package router

import (
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

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
	r.state.record("work", roomy, fromResponse)
	r.state.record("side", roomy, fromResponse)
	req := Request{Session: "one", Model: opus, Client: "work"}
	choose(t.Context(), r, req)
	for range minFailures {
		r.health.record(true)
	}
	r.state.refuse("side")
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
