package router

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// daytime is the day the tests prime over. Work, the first account with a
// token, is primed just after 04:10, and side just after 06:40.
var daytime = config.Prime{Day: config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}}

// local is the tests' local time zone: an hour east of UTC.
var local = time.FixedZone("UTC+1", 60*60)

// onDay is a time on the given day of September 2026, in local time.
func onDay(day, hour, minute int) time.Time {
	return time.Date(2026, time.September, 27+day, hour, minute, 0, 0, local)
}

// afterResets is t moved on by the few seconds a prime waits after each of n
// resets, or slots.
func afterResets(t time.Time, n int) time.Time {
	return t.Add(time.Duration(n) * 5 * time.Second)
}

func TestTheRouterPrimesEachAccountThroughTheDay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 0, 0))
		upstream := newWindowsUpstream(clock)
		r := newPrimingRouter(t, clock.read, upstream, daytime)
		stop := startPriming(r)
		defer stop()

		time.Sleep(29 * time.Hour)
		synctest.Wait()
		// Each prime goes out a few seconds after its slot or the reset
		// before, and starts its window at the ten-minute mark it falls in,
		// as the upstream does, so the resets keep to their marks.
		want := map[string][]time.Time{
			workToken: {afterResets(onDay(1, 4, 10), 1), afterResets(onDay(1, 9, 10), 1), afterResets(onDay(1, 14, 10), 1), afterResets(onDay(1, 19, 10), 1), afterResets(onDay(2, 4, 10), 1)},
			sideToken: {afterResets(onDay(1, 6, 40), 1), afterResets(onDay(1, 11, 40), 1), afterResets(onDay(1, 16, 40), 1), afterResets(onDay(1, 21, 40), 1)},
		}
		if got := upstream.probes(); !reflect.DeepEqual(got, want) {
			t.Errorf("primed at\n%v\nwant each at its slot, just after each reset through the day, and none once it ends till the next day's slot\n%v", got, want)
		}
	})
}

func TestAnAccountWhoseWindowRunsAtItsSlotIsPrimedAsItResets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 1, 0))
		upstream := newWindowsUpstream(clock)
		r := newPrimingRouter(t, clock.read, upstream, daytime)
		// Work's session runs from a late night till 06:00.
		r.state.record("work", []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 6, 0)}, week}, r.state.mark())
		stop := startPriming(r)
		defer stop()

		time.Sleep(6 * time.Hour)
		synctest.Wait()
		want := map[string][]time.Time{workToken: {afterResets(onDay(1, 6, 0), 1)}, sideToken: {afterResets(onDay(1, 6, 40), 1)}}
		if got := upstream.probes(); !reflect.DeepEqual(got, want) {
			t.Errorf("primed at %v, want %v: work's session was running at its slot, 04:10, so it's primed just after that resets", got, want)
		}
	})
}

func TestAPrimeMissedWhileTheMacSleptGoesOutAsItWakes(t *testing.T) {
	tests := []struct {
		name string
		// sleep is how long the Mac sleeps, from 03:00.
		sleep time.Duration
		want  map[string][]time.Time
	}{
		{
			name:  "waking before the day ends",
			sleep: 6 * time.Hour,
			want:  map[string][]time.Time{workToken: {onDay(1, 9, 1)}, sideToken: {onDay(1, 9, 1)}},
		},
		{
			name:  "waking once the day has ended",
			sleep: 20*time.Hour + 30*time.Minute,
			want:  map[string][]time.Time{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clock := newBubbleClock(onDay(1, 3, 0))
				upstream := newWindowsUpstream(clock)
				r := newPrimingRouter(t, clock.read, upstream, daytime)
				stop := startPriming(r)
				defer stop()
				synctest.Wait()

				clock.sleep(tt.sleep)
				time.Sleep(time.Minute)
				synctest.Wait()
				if got := upstream.probes(); !reflect.DeepEqual(got, tt.want) {
					t.Errorf("a minute after waking, primed at %v, want %v", got, tt.want)
				}
			})
		})
	}
}

func TestAPrimeThatFailsIsSentAgainFiveMinutesOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		clock := newBubbleClock(onDay(1, 6, 0))
		upstream := newWindowsUpstream(clock)
		upstream.refuse(sideToken)
		r := newPrimingRouter(t, clock.read, upstream, daytime)
		stop := startPriming(r)
		defer stop()

		time.Sleep(time.Hour)
		synctest.Wait()
		want := []time.Time{afterResets(onDay(1, 6, 40), 1), afterResets(onDay(1, 6, 45), 1), afterResets(onDay(1, 6, 50), 1), afterResets(onDay(1, 6, 55), 1)}
		if got := upstream.probes()[sideToken]; !reflect.DeepEqual(got, want) {
			t.Errorf("side, whose token is refused, was primed at %v, want %v: once at its slot, then every five minutes", got, want)
		}
		if !log.Has("level=WARN", `msg="prime failed"`, "account=side", `error="HTTP 401 · Invalid bearer token"`) {
			t.Errorf("log reads\n%s\nwant the prime's failure", log)
		}
	})
}

func TestAPrimeThatDoesntStartTheWindowIsSentAgainFiveMinutesOn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		clock := newBubbleClock(onDay(1, 6, 0))
		upstream := newWindowsUpstream(clock)
		upstream.leaveIdle(sideToken, onDay(1, 1, 0))
		r := newPrimingRouter(t, clock.read, upstream, daytime)
		stop := startPriming(r)
		defer stop()

		time.Sleep(time.Hour)
		synctest.Wait()
		want := []time.Time{afterResets(onDay(1, 6, 40), 1), afterResets(onDay(1, 6, 45), 1), afterResets(onDay(1, 6, 50), 1), afterResets(onDay(1, 6, 55), 1)}
		if got := upstream.probes()[sideToken]; !reflect.DeepEqual(got, want) {
			t.Errorf("side, whose session a prime doesn't start, was primed at %v, want %v: once at its slot, then every five minutes", got, want)
		}
		if !log.Has("level=WARN", `msg="prime didn't start the window"`, "account=side", "window=5h") {
			t.Errorf("log reads\n%s\nwant side's primes noted as not starting its session", log)
		}
		if log.Has("msg=primed", "account=side") {
			t.Errorf("log reads\n%s\nwant no prime of side noted as done", log)
		}
	})
}

func TestEachPrimeIsLoggedWithTheResetItRead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		clock := newBubbleClock(onDay(1, 4, 0))
		r := newPrimingRouter(t, clock.read, newWindowsUpstream(clock), daytime)
		stop := startPriming(r)
		defer stop()

		time.Sleep(20 * time.Minute)
		synctest.Wait()
		resets := onDay(1, 9, 10).Local().Format("2006-01-02T15:04:05.000-07:00")
		if !log.Has("level=INFO", "msg=primed", "account=work", "resets="+resets) {
			t.Errorf("log reads\n%s\nwant work's prime, with its session's reset", log)
		}
	})
}

func TestAnAccountThatCantStartAWindowIsntPrimed(t *testing.T) {
	far := onDay(4, 0, 0)
	spentWeek := week
	spentWeek.Utilization, spentWeek.Status = 1, quota.StatusRejected
	tests := []struct {
		name string
		// holdBack holds side back as its slot comes.
		holdBack func(s *state)
		wantDue  bool
	}{
		{name: "held back by nothing", holdBack: func(*state) {}, wantDue: true},
		{name: "a limit holding back every request", holdBack: func(s *state) { s.limit("side", []string{"7d"}, far, s.mark()) }},
		{name: "a limit in no window named", holdBack: func(s *state) { s.limit("side", nil, far, s.mark()) }},
		{name: "its week spent", holdBack: func(s *state) { s.record("side", []quota.Window{spentWeek}, s.mark()) }},
		{name: "its token refused", holdBack: func(s *state) { s.refuse("side", http.StatusUnauthorized, someRequest) }},
		{
			name:     "a limit on its Fable week alone, which other models' requests go out beside",
			holdBack: func(s *state) { s.limit("side", []string{"7d_oi"}, far, s.mark()) },
			wantDue:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: onDay(1, 1, 0)}
			r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			// Side's session ran till 01:00, and has lapsed since.
			lapsed := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 1, 0)}
			r.state.record("side", []quota.Window{lapsed, week}, r.state.mark())
			clock.now = afterResets(onDay(1, 6, 40), 1)
			tt.holdBack(r.state)

			if due := r.primer.due("side", clock.now); due != tt.wantDue {
				t.Errorf("side due a prime at its slot = %v, want %v", due, tt.wantDue)
			}
		})
	}
}

func TestAnAccountThatCantStartAWindowIsPrimedOnceItCan(t *testing.T) {
	// Past side's slot, 06:40, it can take no request until half a minute
	// past 06:45, between the primer's looks a minute apart.
	freed := onDay(1, 6, 45).Add(30 * time.Second)
	spent := quota.Window{Key: "5h", Label: "Session", Utilization: 1, ResetsAt: freed.UTC(), Status: quota.StatusRejected}
	tests := []struct {
		name string
		// holdBack holds side back till freed, from ten minutes before.
		holdBack func(s *state)
		want     time.Time
	}{
		{
			name:     "its session spent till its reset",
			holdBack: func(s *state) { s.record("side", []quota.Window{spent, week}, s.mark()) },
			want:     afterResets(freed, 1),
		},
		{
			name:     "a limit holding back its every request",
			holdBack: func(s *state) { s.limit("side", nil, freed, s.mark()) },
			want:     freed,
		},
		{
			name:     "its token refused",
			holdBack: func(s *state) { s.refuse("side", http.StatusUnauthorized, someRequest) },
			want:     freed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				clock := newBubbleClock(onDay(1, 6, 0))
				upstream := newWindowsUpstream(clock)
				r := newPrimingRouter(t, clock.read, upstream, daytime)
				// Side's session ran till 01:00, and has lapsed since.
				lapsed := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 1, 0).UTC()}
				r.state.record("side", []quota.Window{lapsed, week}, r.state.mark())
				stop := startPriming(r)
				defer stop()

				time.Sleep(freed.Sub(clock.read()) - refusedFor)
				tt.holdBack(r.state)
				time.Sleep(90 * time.Minute)
				synctest.Wait()
				if got, want := upstream.probes()[sideToken], []time.Time{tt.want}; !reflect.DeepEqual(got, want) {
					t.Errorf("side was primed at %v, want %v: as soon as it can start a window", got, want)
				}
			})
		})
	}
}

func TestAPrimeThatSharesAProbeUnderWayIsLoggedAsAPrime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		prober := &stubProber{gate: make(chan struct{}), readings: map[string]quota.Probe{workToken: probed(nil, session, week)}}
		r := newPrimingRouter(t, at(start), prober, daytime)
		defer r.probes.stop()
		always := func(string, time.Time) bool { return true }
		work := r.accounts.only([]string{"work"})

		// A choice's probe of work is under way as work falls due a prime.
		r.probes.start(work, always)
		r.probes.prime(work, always)
		close(prober.gate)
		synctest.Wait()
		if n := prober.counts()[workToken]; n != 1 {
			t.Errorf("work was probed %d times, want once, the prime sharing the probe", n)
		}
		if !log.Has("level=INFO", "msg=primed", "account=work") || log.Has(`msg="probed account"`) {
			t.Errorf("log reads\n%s\nwant the probe logged as a prime", log)
		}
	})
}

func TestTheRoutersDocumentGivesTheScheduleAndWhenEachAccountIsNextPrimed(t *testing.T) {
	clock := &testClock{now: onDay(1, 5, 0)}
	r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
	running := quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: onDay(1, 9, 10)}
	r.state.record("work", []quota.Window{running, week}, r.state.mark())

	want := status.Prime{
		Day:    "08:00-23:00",
		Window: "5h",
		Slots: []status.Slot{
			{Account: "work", At: "04:10", Next: afterResets(onDay(1, 9, 10), 1).UTC()},
			{Account: "side", At: "06:40", Next: afterResets(onDay(1, 6, 40), 1).UTC()},
		},
	}
	if got := r.Status().Prime; !reflect.DeepEqual(got, want) {
		t.Errorf("the document's schedule is\n%+v\nwant\n%+v", got, want)
	}
	if got := newTestRouter(t, clock.read, &stubProber{}).Status().Prime; !reflect.DeepEqual(got, status.Prime{}) {
		t.Errorf("without a day, the document's schedule is %+v, want none", got)
	}
}

func TestTheRoutersDocumentSaysNothingOfWhenAnAccountThatCantStartAWindowIsNextPrimed(t *testing.T) {
	far := onDay(4, 0, 0)
	spentWeek := week
	spentWeek.Utilization, spentWeek.Status = 1, quota.StatusRejected
	// Side's session ran till 01:00, and has lapsed since.
	lapsed := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 1, 0).UTC()}
	tests := []struct {
		name string
		// holdBack holds side back.
		holdBack func(s *state)
	}{
		{name: "its token refused", holdBack: func(s *state) { s.refuse("side", http.StatusUnauthorized, someRequest) }},
		{name: "a limit holding back its every request", holdBack: func(s *state) { s.limit("side", nil, far, s.mark()) }},
		{name: "its week spent", holdBack: func(s *state) { s.record("side", []quota.Window{lapsed, spentWeek}, s.mark()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: onDay(1, 5, 0)}
			r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			tt.holdBack(r.state)

			want := []status.Slot{
				{Account: "work", At: "04:10", Next: onDay(1, 5, 0).UTC()},
				{Account: "side", At: "06:40"},
			}
			if got := r.Status().Prime.Slots; !reflect.DeepEqual(got, want) {
				t.Errorf("the document's slots are\n%+v\nwant\n%+v: side's without when it's next primed", got, want)
			}
		})
	}
}

func TestTheScheduleIsWorkedOutAgainAsAnAccountGainsOrLosesItsToken(t *testing.T) {
	clock := &testClock{now: onDay(1, 2, 0)}
	files := &changingFiles{files: testTokens}
	r := newPrimingRouterReading(t, clock.read, &stubProber{}, daytime, files.read)
	slots := func() []string {
		var slots []string
		for _, slot := range r.Status().Prime.Slots {
			slots = append(slots, slot.Account+" "+slot.At)
		}
		return slots
	}
	if got, want := slots(), []string{"work 04:10", "side 06:40"}; !slices.Equal(got, want) {
		t.Fatalf("as the router starts, the slots are %q, want %q", got, want)
	}

	files.set(tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken})
	r.upkeep.tokens.look()
	if got, want := slots(), []string{"work 03:50", "personal 05:30", "side 07:10"}; !slices.Equal(got, want) {
		t.Errorf("with personal's token, the slots are %q, want %q", got, want)
	}

	// A token is lost once its file holds none two looks in a row.
	files.set(tokenstest.Files{"personal": personalToken, "side": sideToken})
	r.upkeep.tokens.look()
	r.upkeep.tokens.look()
	if got, want := slots(), []string{"personal 04:10", "side 06:40"}; !slices.Equal(got, want) {
		t.Errorf("without work's token, the slots are %q, want %q", got, want)
	}

	files.set(tokenstest.Files{})
	r.upkeep.tokens.look()
	r.upkeep.tokens.look()
	if got := r.Status().Prime; !reflect.DeepEqual(got, status.Prime{}) {
		t.Errorf("with no account's token, the schedule is %+v, want none", got)
	}
}

func TestAnAccountThatGainsItsTokenIsPrimedAtItsSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 2, 0))
		upstream := newWindowsUpstream(clock)
		files := &changingFiles{files: testTokens}
		r := newPrimingRouterReading(t, clock.read, upstream, daytime, files.read)
		stop := startPriming(r)
		defer stop()

		time.Sleep(30 * time.Minute)
		files.set(tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken})
		r.upkeep.tokens.look()
		time.Sleep(6 * time.Hour)
		synctest.Wait()
		want := map[string][]time.Time{
			workToken:     {afterResets(onDay(1, 3, 50), 1)},
			personalToken: {afterResets(onDay(1, 5, 30), 1)},
			sideToken:     {afterResets(onDay(1, 7, 10), 1)},
		}
		if got := upstream.probes(); !reflect.DeepEqual(got, want) {
			t.Errorf("once personal has its token, primed at %v, want each at its slot in the schedule of three %v", got, want)
		}
	})
}

// newPrimingRouter builds a router of testConfigured that primes on the
// schedule prime gives, on now's time, probing with prober.
func newPrimingRouter(t *testing.T, now func() time.Time, prober Prober, prime config.Prime) *Router {
	t.Helper()
	return newPrimingRouterReading(t, now, prober, prime, testTokens.Read)
}

// newPrimingRouterReading builds a router as newPrimingRouter does, reading
// the accounts' tokens with read.
func newPrimingRouterReading(t *testing.T, now func() time.Time, prober Prober, prime config.Prime, read func(id string) (tokens.Token, error)) *Router {
	t.Helper()
	r, err := New(Config{
		Accounts: testConfigured,
		Token:    read,
		Upstream: "http://127.0.0.1:1",
		Provider: claude.Provider{},
		Prober:   prober,
		Policy:   testPolicy,
		Prime:    prime,
		Now:      now,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return r
}

// startPriming runs the router's primer until the stop it returns is called,
// which waits for it, and its probes, to end.
func startPriming(r *Router) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var priming sync.WaitGroup
	priming.Go(func() { r.primer.run(ctx) })
	return func() {
		cancel()
		priming.Wait()
		r.probes.stop()
	}
}

// bubbleClock reads the wall clock in a synctest bubble, from base on: time
// passes on it as the bubble's does, and it jumps on as a Mac sleeps, which
// the bubble's timers don't see. It's safe for concurrent use.
type bubbleClock struct {
	base, began time.Time

	mu    sync.Mutex
	slept time.Duration
}

func newBubbleClock(base time.Time) *bubbleClock {
	return &bubbleClock{base: base, began: time.Now()}
}

func (c *bubbleClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.base.Add(time.Since(c.began) + c.slept)
}

// sleep has the Mac sleep for d: the wall clock moves on, and the bubble's
// timers don't.
func (c *bubbleClock) sleep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.slept += d
}

// windowsUpstream answers probes as the upstream answers a request on an
// account, by clock's time: a request starts the account's session when it
// isn't running, at the ten-minute mark it falls in, and it resets five hours
// after that mark. It notes when each token was
// probed, refuses the tokens it's told to, and starts no session on those it's
// told to leave idle.
type windowsUpstream struct {
	clock *bubbleClock

	mu       sync.Mutex
	sessions map[string]time.Time
	refused  map[string]bool
	idle     map[string]bool
	probed   map[string][]time.Time
}

func newWindowsUpstream(clock *bubbleClock) *windowsUpstream {
	return &windowsUpstream{
		clock:    clock,
		sessions: make(map[string]time.Time),
		refused:  make(map[string]bool),
		idle:     make(map[string]bool),
		probed:   make(map[string][]time.Time),
	}
}

func (u *windowsUpstream) Probe(_ context.Context, token string) (quota.Probe, error) {
	now := u.clock.read()
	u.mu.Lock()
	defer u.mu.Unlock()
	u.probed[token] = append(u.probed[token], now)
	if u.refused[token] {
		return quota.Probe{}, errors.New("HTTP 401 · Invalid bearer token")
	}
	if reset, running := u.sessions[token]; !u.idle[token] && (!running || !reset.After(now)) {
		u.sessions[token] = now.Truncate(10 * time.Minute).Add(5 * time.Hour)
	}
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.01, ResetsAt: u.sessions[token].UTC(), Status: quota.StatusAllowed}
	return probed(nil, session, week), nil
}

// refuse has the upstream refuse token from now on.
func (u *windowsUpstream) refuse(token string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.refused[token] = true
}

// leaveIdle has the upstream start no session on token from now on: its
// probes read the session as it last ran, till it reset at lapsed.
func (u *windowsUpstream) leaveIdle(token string, lapsed time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.idle[token] = true
	u.sessions[token] = lapsed
}

// probes returns when each token was probed, by the token.
func (u *windowsUpstream) probes() map[string][]time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	probes := make(map[string][]time.Time, len(u.probed))
	for token, times := range u.probed {
		for _, t := range times {
			probes[token] = append(probes[token], t.In(local))
		}
	}
	return probes
}
