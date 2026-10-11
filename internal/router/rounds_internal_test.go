package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// errUnreadable is why a test's probe read nothing.
var errUnreadable = errors.New("HTTP 529 · Overloaded")

func TestTheRoundsProbeAnAccountNothingHasReadInHalfAnHour(t *testing.T) {
	tests := []struct {
		name string
		// before readies side's state, on the clock the test then moves to
		// start.
		before []step
		want   bool
	}{
		{name: "never read", want: true},
		{name: "read by an answer half an hour ago", before: []step{answered(30 * time.Minute)}, want: true},
		{name: "read by an answer 29 minutes ago, as one in use always is", before: []step{answered(29 * time.Minute)}},
		{name: "read by a probe 29 minutes ago", before: []step{probedSide(29*time.Minute, readings.FromProbe)}},
		{name: "read by a prime 29 minutes ago", before: []step{probedSide(29*time.Minute, readings.FromPrime)}},
		{name: "its token refused", before: []step{answered(40 * time.Minute), refusedSide(time.Minute)}},
		{name: "a probe that read nothing a minute ago", before: []step{answered(40 * time.Minute), unread(time.Minute)}},
		{name: "a probe that read nothing two minutes ago", before: []step{answered(40 * time.Minute), unread(2 * time.Minute)}, want: true},
		{
			name:   "three probes in a row that read nothing, the last seven minutes ago",
			before: []step{answered(60 * time.Minute), unread(25 * time.Minute), unread(15 * time.Minute), unread(7 * time.Minute)},
		},
		{
			name:   "three probes in a row that read nothing, the last eight minutes ago",
			before: []step{answered(60 * time.Minute), unread(25 * time.Minute), unread(15 * time.Minute), unread(8 * time.Minute)},
			want:   true,
		},
		{
			name: "a probe that read nothing three minutes ago, the first since a reading",
			before: []step{
				answered(3 * time.Hour), unread(170 * time.Minute), unread(160 * time.Minute), unread(150 * time.Minute),
				answered(35 * time.Minute), unread(3 * time.Minute),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			r := newTestRouter(t, clock.read, &stubProber{})
			for _, s := range tt.before {
				s(r, clock)
			}
			clock.now = start

			if due := r.state.rounding(r.rounds.clears)("side", start); due != tt.want {
				t.Errorf("side due on the router's rounds = %v, want %v", due, tt.want)
			}
		})
	}
}

func TestTheRoundsBackOffWhileAnAccountCantBeRead(t *testing.T) {
	tests := []struct {
		misses int
		want   time.Duration
	}{
		{misses: 0, want: time.Minute},
		{misses: 1, want: 2 * time.Minute},
		{misses: 2, want: 4 * time.Minute},
		{misses: 4, want: 16 * time.Minute},
		{misses: 5, want: 30 * time.Minute},
		{misses: 100, want: 30 * time.Minute},
	}
	for _, tt := range tests {
		if got := retryAfter(tt.misses); got != tt.want {
			t.Errorf("after %d probes in a row that read nothing, the rounds wait %v, want %v", tt.misses, got, tt.want)
		}
	}
}

func TestTheRoundsProbeALapsedAccountOnlyWhereTheWindowItStartsResetsBeforeItsPrime(t *testing.T) {
	far := onDay(4, 0, 0)
	spentWeek := week
	spentWeek.Utilization, spentWeek.Status = 1, quota.StatusRejected
	limited := func(windows ...string) func(s *state) {
		return func(s *state) { s.limit("side", windows, far, s.mark()) }
	}
	// Side's slot is 06:40. Its session, as last read at 00:20, ran till
	// 00:30, and has lapsed since.
	tests := []struct {
		name    string
		prime   config.Prime
		at      time.Time
		running bool
		// holdBack holds side back from 00:20 on.
		holdBack func(s *state)
		want     bool
	}{
		{name: "more than five hours before its slot", prime: daytime, at: onDay(1, 1, 30), want: true},
		{name: "within five hours of its slot", prime: daytime, at: onDay(1, 2, 0)},
		{name: "without priming", at: onDay(1, 2, 0), want: true},
		{name: "as its day of priming runs, when the primer primes it", prime: daytime, at: onDay(1, 12, 0)},
		{name: "its session running, within five hours of its slot", prime: daytime, at: onDay(1, 2, 0), running: true, want: true},
		{name: "a limit holding back its every request, within five hours of its slot", prime: daytime, at: onDay(1, 2, 0), holdBack: limited(), want: true},
		{name: "a limit holding back its every request, as its day of priming runs", prime: daytime, at: onDay(1, 12, 0), holdBack: limited(), want: true},
		{
			name:     "its week spent, within five hours of its slot",
			prime:    daytime,
			at:       onDay(1, 2, 0),
			holdBack: func(s *state) { s.record("side", []quota.Window{spentWeek}, s.mark()) },
			want:     true,
		},
		{name: "a limit on its Fable week alone, within five hours of its slot", prime: daytime, at: onDay(1, 2, 0), holdBack: limited("7d_oi")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: onDay(1, 0, 20)}
			r := newPrimingRouter(t, clock.read, &stubProber{}, tt.prime)
			sideSession := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 0, 30).UTC()}
			if tt.running {
				sideSession.ResetsAt = onDay(1, 4, 0).UTC()
			}
			r.state.record("side", []quota.Window{sideSession, week}, r.state.mark())
			if tt.holdBack != nil {
				tt.holdBack(r.state)
			}
			clock.now = tt.at

			if due := r.state.rounding(r.rounds.clears)("side", tt.at); due != tt.want {
				t.Errorf("side due on the router's rounds = %v, want %v", due, tt.want)
			}
		})
	}
}

func TestTheRoundsProbeAnAccountFiveMinutesBeforeEachWeekOfItsResets(t *testing.T) {
	// Side's slot is 06:40, and its session has lapsed.
	lapsed, weekEnding, fableEnding := lapsedAtOne, endingAtFive(week), endingAtFive(fableWeek)
	tests := []struct {
		name string
		// windows are side's as read at 04:45, and before what else befalls
		// it.
		windows []quota.Window
		before  []step
		at      time.Time
		want    bool
	}{
		{name: "five minutes before its week resets, within its slot's five hours", windows: []quota.Window{lapsed, weekEnding, fableWeek}, at: onDay(1, 4, 55), want: true},
		{name: "six minutes before its week resets", windows: []quota.Window{lapsed, weekEnding, fableWeek}, at: onDay(1, 4, 54)},
		{
			name:    "its week read within those five minutes",
			windows: []quota.Window{lapsed, weekEnding, fableWeek},
			before:  []step{sideReadAt(onDay(1, 4, 56), lapsed, weekEnding)},
			at:      onDay(1, 4, 58),
		},
		{
			name:    "a probe that read nothing within those five minutes, its week's one attempt",
			windows: []quota.Window{lapsed, weekEnding, fableWeek},
			before:  []step{unreadAt(onDay(1, 4, 56))},
			at:      onDay(1, 4, 58),
		},
		{
			name:    "a probe within those five minutes that read the rest, not its Fable week, its one attempt",
			windows: []quota.Window{lapsed, week, fableEnding},
			before:  []step{probeReadAt(onDay(1, 4, 56), lapsed, week)},
			at:      onDay(1, 4, 58),
		},
		{
			name:    "a probe that read it just before those five minutes, ended half a minute ago",
			windows: []quota.Window{lapsed, weekEnding, fableWeek},
			before:  []step{probeReadAt(onDay(1, 4, 54).Add(30*time.Second), lapsed, weekEnding, fableWeek)},
			at:      onDay(1, 4, 55),
		},
		{
			name:    "probes that read nothing since it was read, backing off still",
			windows: []quota.Window{lapsed, weekEnding, fableWeek},
			before:  []step{unreadAt(onDay(1, 4, 46)), unreadAt(onDay(1, 4, 50)), unreadAt(onDay(1, 4, 53))},
			at:      onDay(1, 4, 56),
		},
		{
			name:    "probes that read nothing since it was read, their backoff passed",
			windows: []quota.Window{lapsed, weekEnding, fableWeek},
			before:  []step{unreadAt(onDay(1, 4, 46)), unreadAt(onDay(1, 4, 47))},
			at:      onDay(1, 4, 55),
			want:    true,
		},
		{
			name:    "its token refused",
			windows: []quota.Window{lapsed, weekEnding, fableWeek},
			before:  []step{refusedAt(onDay(1, 4, 50))},
			at:      onDay(1, 4, 56),
		},
		{name: "its week's reset passed an hour ago, unread before it", windows: []quota.Window{lapsed, weekEnding, fableWeek}, at: onDay(1, 6, 0)},
		{name: "five minutes before its week next resets, a week on from one passed unread", windows: []quota.Window{lapsed, weekEnding, fableWeek}, at: onDay(8, 4, 55), want: true},
		{name: "five minutes before its Fable week resets", windows: []quota.Window{lapsed, week, fableEnding}, at: onDay(1, 4, 55), want: true},
		{
			name:    "its account read within those five minutes by an answer that doesn't count its Fable week",
			windows: []quota.Window{lapsed, week, fableEnding},
			before:  []step{sideReadAt(onDay(1, 4, 56), lapsed, week)},
			at:      onDay(1, 4, 58),
			want:    true,
		},
		{
			name:    "its Fable week read within those five minutes",
			windows: []quota.Window{lapsed, week, fableEnding},
			before:  []step{sideReadAt(onDay(1, 4, 56), fableEnding)},
			at:      onDay(1, 4, 58),
		},
		{
			name:    "five minutes before its session resets, a window of no more than a day",
			windows: []quota.Window{endingAtFive(lapsed), week, fableWeek},
			at:      onDay(1, 4, 55),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{}
			r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			sideReadAt(onDay(1, 4, 45), tt.windows...)(r, clock)
			for _, s := range tt.before {
				s(r, clock)
			}
			clock.now = tt.at

			if due := r.state.rounding(r.rounds.clears)("side", tt.at); due != tt.want {
				t.Errorf("side due on the router's rounds = %v, want %v", due, tt.want)
			}
		})
	}
}

func TestTheRoundsProbeAnIdleAccountEveryHalfHourAndOneInUseNever(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 10, 0))
		upstream := newWindowsUpstream(clock)
		r := newPrimingRouter(t, clock.read, upstream, config.Prime{})
		running := quota.Window{Key: "5h", Label: "Session", Utilization: 0.2, ResetsAt: onDay(1, 15, 0).UTC()}
		for _, a := range []struct{ id, token string }{{"work", workToken}, {"side", sideToken}} {
			upstream.startElsewhere(a.token, running.ResetsAt)
			r.state.record(a.id, []quota.Window{running, week}, r.state.mark())
		}
		var answering sync.WaitGroup
		answering.Go(func() {
			// Work's sessions read it every five minutes, by their answers.
			for range 24 {
				time.Sleep(5 * time.Minute)
				r.state.record("work", []quota.Window{running, week}, r.state.mark())
			}
		})
		stop := startRounds(r)
		defer stop()

		time.Sleep(2*time.Hour + 30*time.Second)
		synctest.Wait()
		want := map[string][]time.Time{sideToken: {onDay(1, 10, 30), onDay(1, 11, 0), onDay(1, 11, 30), onDay(1, 12, 0)}}
		if got := upstream.probes(); !reflect.DeepEqual(got, want) {
			t.Errorf("probed at %v, want %v: side every half hour unread, work, in use, never", got, want)
		}
		answering.Wait()
	})
}

func TestAProbeMissedOnTheRoundsWhileTheMacSleptGoesOutAsItWakes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 10, 0))
		upstream := newWindowsUpstream(clock)
		r := newPrimingRouter(t, clock.read, upstream, config.Prime{})
		running := quota.Window{Key: "5h", Label: "Session", Utilization: 0.2, ResetsAt: onDay(1, 15, 0).UTC()}
		upstream.startElsewhere(sideToken, running.ResetsAt)
		r.state.record("side", []quota.Window{running, week}, r.state.mark())
		r.state.record("work", []quota.Window{running, week}, r.state.mark())
		upstream.startElsewhere(workToken, running.ResetsAt)
		stop := startRounds(r)
		defer stop()

		// The Mac sleeps from 10:10:30 till 12:10:30, past side's probe due
		// at 10:30.
		time.Sleep(10*time.Minute + 30*time.Second)
		clock.sleep(2 * time.Hour)
		time.Sleep(time.Minute)
		synctest.Wait()
		woken := onDay(1, 12, 11)
		want := map[string][]time.Time{workToken: {woken}, sideToken: {woken}}
		if got := upstream.probes(); !reflect.DeepEqual(got, want) {
			t.Errorf("a minute after waking, probed at %v, want %v: once each, within a minute of waking", got, want)
		}
	})
}

func TestTheRoundsShareAProbeUnderWay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		prober := &stubProber{gate: make(chan struct{}), readings: map[string]quota.Probe{
			workToken: probed(nil, session, week),
			sideToken: probed(nil, session, week),
		}}
		r := newTestRouter(t, at(start), prober)
		defer r.probes.stop()

		// A choice's probe of side is under way as the rounds find it due,
		// never having been read.
		r.probes.start(r.accounts.only([]string{"side"}), func(string, time.Time) bool { return true })
		time.AfterFunc(time.Second, func() { close(prober.gate) })
		r.rounds.look(t.Context())
		synctest.Wait()
		if n := prober.counts()[sideToken]; n != 1 {
			t.Errorf("side was probed %d times, want once, the rounds sharing the choice's probe", n)
		}
	})
}

func TestOneProbeOnTheRoundsServesEveryReasonDueAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 10, 0))
		upstream := newWindowsUpstream(clock)
		r := newPrimingRouter(t, clock.read, upstream, config.Prime{})
		running := quota.Window{Key: "5h", Label: "Session", Utilization: 0.2, ResetsAt: onDay(1, 15, 0).UTC()}
		// Side's week resets at 10:35, so it falls due five minutes before,
		// as it's half an hour unread.
		weekEnding := week
		weekEnding.ResetsAt = onDay(1, 10, 35).UTC()
		upstream.startElsewhere(sideToken, running.ResetsAt)
		upstream.readWeek(weekEnding)
		r.state.record("side", []quota.Window{running, weekEnding}, r.state.mark())
		stop := startRounds(r)
		defer stop()

		time.Sleep(50 * time.Minute)
		synctest.Wait()
		if got, want := upstream.probes()[sideToken], []time.Time{onDay(1, 10, 30)}; !reflect.DeepEqual(got, want) {
			t.Errorf("side probed at %v, want %v: one probe for both reasons", got, want)
		}
	})
}

func TestAProbeBeforeAWeekResetsThatStartsALapsedWindowShiftsItsSlot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		clock := newBubbleClock(onDay(1, 3, 0))
		upstream := newWindowsUpstream(clock)
		// Work's week resets at 04:05, five minutes before its slot, 04:10;
		// its session, read at 03:00, lapsed at 01:00.
		weekEnding := week
		weekEnding.ResetsAt = onDay(1, 4, 5).UTC()
		upstream.readWeek(weekEnding)
		r := newPrimingRouter(t, clock.read, upstream, daytime)
		lapsed := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 1, 0).UTC()}
		r.state.record("work", []quota.Window{lapsed, weekEnding}, r.state.mark())
		r.state.record("side", []quota.Window{lapsed, week}, r.state.mark())
		ctx, cancel := context.WithCancel(context.Background())
		var looking sync.WaitGroup
		looking.Go(func() { r.primer.run(ctx) })
		looking.Go(func() { r.rounds.run(ctx) })
		defer func() {
			cancel()
			looking.Wait()
			r.probes.stop()
		}()

		time.Sleep(80 * time.Minute)
		synctest.Wait()
		if got, want := upstream.probes()[workToken], []time.Time{onDay(1, 4, 0)}; !reflect.DeepEqual(got, want) {
			t.Errorf("work probed at %v, want %v: five minutes before its week resets, its session lapsed, and not primed at its slot, its session running", got, want)
		}
		if got, want := r.Status().Prime.Slots[0].Next, afterResets(onDay(1, 9, 0), 1).UTC(); !got.Equal(want) {
			t.Errorf("work next primed at %v, want %v: as the session that probe started resets", got, want)
		}
	})
}

func TestTheRoundsAskAfterThePrimeOnlyWhereItMatters(t *testing.T) {
	running := quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 6, 0).UTC()}
	tests := []struct {
		name string
		// windows are side's as read at read, and at when it's asked of.
		windows  []quota.Window
		read, at time.Time
		want     bool
	}{
		{name: "read lately", windows: []quota.Window{lapsedAtOne, week}, read: onDay(1, 1, 50), at: onDay(1, 2, 0)},
		{name: "idle, its session running", windows: []quota.Window{running, week}, read: onDay(1, 1, 0), at: onDay(1, 2, 0)},
		{name: "idle, its week about to reset", windows: []quota.Window{lapsedAtOne, endingAtFive(week)}, read: onDay(1, 1, 0), at: onDay(1, 4, 56)},
		{name: "idle, its session lapsed", windows: []quota.Window{lapsedAtOne, week}, read: onDay(1, 1, 0), at: onDay(1, 2, 0), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{}
			r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			sideReadAt(tt.read, tt.windows...)(r, clock)
			clock.now = tt.at
			asked := false
			r.state.rounding(func(string, time.Time) bool { asked = true; return true })("side", tt.at)
			if asked != tt.want {
				t.Errorf("asked after side's prime = %v, want %v: only for an idle account a probe would start a window on", asked, tt.want)
			}
		})
	}
}

// takeUp is a way side comes to go out on another token, or its own again,
// and the token it goes out on then.
type takeUp struct {
	name  string
	token string
	do    func(r *Router, files *changingFiles)
}

// takeUps are the ways side comes to go out on another token, or its own
// again.
var takeUps = []takeUp{
	{
		name:  "replaced, as the router looks at the token files",
		token: renewedToken,
		do: func(r *Router, files *changingFiles) {
			files.set(tokenstest.Files{"work": workToken, "side": renewedToken})
			r.upkeep.tokens.look()
		},
	},
	{
		name:  "replaced, as the upstream refuses the token a request went out on",
		token: renewedToken,
		do: func(r *Router, files *changingFiles) {
			files.set(tokenstest.Files{"work": workToken, "side": renewedToken})
			side, _ := r.accounts.byID("side")
			(&replay{p: r.proxy, ex: &exchange{account: side}, sent: side.token()}).renewed()
		},
	},
	{
		name:  "its own back, once its file held none",
		token: sideToken,
		do: func(r *Router, files *changingFiles) {
			files.set(tokenstest.Files{"work": workToken})
			r.upkeep.tokens.look()
			r.upkeep.tokens.look()
			files.set(testTokens)
			r.upkeep.tokens.look()
		},
	},
}

func TestAnAccountOnAnotherTokenIsProbedAfresh(t *testing.T) {
	for _, tt := range takeUps {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			files := &changingFiles{files: testTokens}
			r := newTestRouterReading(t, clock.read, &stubProber{}, files.read)
			// Read 30 minutes ago by a probe that couldn't read its Fable
			// week, side's probes have read nothing since, three in a row,
			// the last three minutes ago: it waits eight.
			for _, s := range []step{partlyRead(30 * time.Minute), unread(20 * time.Minute), unread(10 * time.Minute), unread(3 * time.Minute)} {
				s(r, clock)
			}
			clock.now = start
			due := r.state.rounding(r.rounds.clears)
			if due("side", start) {
				t.Fatal("side due on the rounds before its token changed, want it backing off")
			}
			if side, _ := r.Status().Account("side"); len(side.Failures) == 0 {
				t.Fatal("side's Fable week reads as read before its token changed, want it unread")
			}

			tt.do(r, files)
			if !due("side", start) {
				t.Error("side not due on the rounds, want it probed afresh on the token it goes out on now")
			}
			side, _ := r.Status().Account("side")
			if side.Error != "" {
				t.Errorf("side's error reads %q, want none: it was the token before's", side.Error)
			}
			if len(side.Failures) > 0 {
				t.Errorf("side's windows unread are %+v, want none: they were the token before's", side.Failures)
			}
		})
	}
}

func TestAProbeOnTheTokenBeforeTellsOfTheAccountsUsageAlone(t *testing.T) {
	higher := week
	higher.Utilization = 0.97
	outcomes := []struct {
		name string
		// reading and err are what the probe on the token before finds.
		reading quota.Probe
		err     error
		// wantWeek is what side's week reads once the probe has ended, and
		// wantLog what the log says of the probe.
		wantWeek float64
		wantLog  []string
		// wantDue is whether side is due on the next round, a minute on.
		wantDue bool
	}{
		{
			name:     "refused, which says nothing of the token side goes out on now",
			err:      errors.New("HTTP 401 · Invalid bearer token"),
			wantWeek: week.Utilization,
			wantLog:  []string{"level=DEBUG", `msg="probe on the token before failed; its failure isn't kept"`, "account=side", `error="HTTP 401 · Invalid bearer token"`},
			wantDue:  true,
		},
		{
			name:     "reading its windows, which read the same on either token",
			reading:  probed(nil, session, higher),
			wantWeek: higher.Utilization,
			wantLog:  []string{"level=DEBUG", `msg="probed account"`, "account=side", "windows=2"},
			// The probe read side a minute before, as the rounds count it.
			wantDue: false,
		},
	}
	for _, o := range outcomes {
		for _, tt := range takeUps {
			t.Run(o.name+", "+tt.name, func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					log := logstest.Capture(t)
					clock := &testClock{now: start}
					files := &changingFiles{files: testTokens}
					prober := &stubProber{
						gate:     make(chan struct{}),
						readings: map[string]quota.Probe{sideToken: o.reading},
						errs:     map[string]error{sideToken: o.err},
					}
					r := newTestRouterReading(t, clock.read, prober, files.read)
					defer r.probes.stop()
					answered(40*time.Minute)(r, clock)
					clock.now = start
					due := r.state.rounding(r.rounds.clears)

					// Side's rounds probe it, unread for 40 minutes, and the
					// probe is still out on its token as side takes one up.
					underway := r.probes.start(r.accounts.only([]string{"side"}), due)
					synctest.Wait()
					if n := prober.counts()[sideToken]; n != 1 {
						t.Fatalf("side's token probed %d times, want once before side takes a token up", n)
					}
					tt.do(r, files)
					close(prober.gate)
					<-underway[0].done

					if got := probesOf(r, "side"); got != (probesWent{}) {
						t.Errorf("side's probes went %+v, want as they start afresh on the token it goes out on now", got)
					}
					side, _ := r.Status().Account("side")
					if side.Error != "" {
						t.Errorf("side's error reads %q, want none: it was the token before's", side.Error)
					}
					if got := side.Windows[1].Utilization; got != o.wantWeek {
						t.Errorf("side's week reads %v, want %v", got, o.wantWeek)
					}
					if !log.Has(o.wantLog...) || log.Has(`msg="probe failed"`) {
						t.Errorf("log reads\n%s\nwant a line with %q, and no probe failed", log, o.wantLog)
					}

					clock.now = start.Add(roundEvery)
					if got := due("side", clock.now); got != o.wantDue {
						t.Fatalf("side due on the next round = %v, want %v", got, o.wantDue)
					}
					if !o.wantDue {
						return
					}
					before := prober.counts()[tt.token]
					r.rounds.look(t.Context())
					if n := prober.counts()[tt.token] - before; n != 1 {
						t.Errorf("the next round probed side's token %d times, want once", n)
					}
					if got := probesOf(r, "side").ended; !got.Equal(clock.now) {
						t.Errorf("side's last probe ended %v, want %v: the next round's, on the token it goes out on now, counts", got, clock.now)
					}
				})
			})
		}
	}
}

// probesWent is how an account's probes went, as the state has it: when one
// last ended, why it read nothing, and how many in a row did.
type probesWent struct {
	ended  time.Time
	err    string
	misses int
}

// probesOf returns how the probes of the account with the given id went.
func probesOf(r *Router, id string) probesWent {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	u := r.state.usage[id]
	return probesWent{ended: u.probed, err: u.probeErr, misses: u.misses}
}

func TestAWeekTheLastAnswerDidntCarryIsProbedBeforeItResetsAfterARestart(t *testing.T) {
	tests := []struct {
		name string
		// windows are side's as read at 04:45, and answered those an answer
		// read at 04:56.
		windows, answered []quota.Window
		// unkept has the state file keep no window's read time, as a router
		// from before kept them.
		unkept bool
		want   bool
	}{
		{
			name:     "its Fable week, which an Opus answer doesn't carry",
			windows:  []quota.Window{lapsedAtOne, week, endingAtFive(fableWeek)},
			answered: []quota.Window{lapsedAtOne, week},
			want:     true,
		},
		{
			name:     "its week, which the answer carried",
			windows:  []quota.Window{lapsedAtOne, endingAtFive(week), fableWeek},
			answered: []quota.Window{lapsedAtOne, endingAtFive(week)},
		},
		{
			name:     "its week, which the answer carried, as a router from before kept it",
			windows:  []quota.Window{lapsedAtOne, endingAtFive(week), fableWeek},
			answered: []quota.Window{lapsedAtOne, endingAtFive(week)},
			unkept:   true,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{}
			before := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			sideReadAt(onDay(1, 4, 45), tt.windows...)(before, clock)
			sideReadAt(onDay(1, 4, 56), tt.answered...)(before, clock)
			data, err := json.Marshal(before.state.saved())
			if err != nil {
				t.Fatal(err)
			}
			var kept savedUsage
			if err := json.Unmarshal(data, &kept); err != nil {
				t.Fatal(err)
			}
			if tt.unkept {
				for id, reading := range kept.Readings {
					reading.WindowsReadAt = nil
					kept.Readings[id] = reading
				}
			}

			// The router restarts at 04:57, the week resetting at 05:00.
			clock.now = onDay(1, 4, 57)
			after := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
			after.state.recall(kept)
			if due := after.state.rounding(after.rounds.clears)("side", clock.now); due != tt.want {
				t.Errorf("side due on the router's rounds after the restart = %v, want %v", due, tt.want)
			}
		})
	}
}

func TestAReadingThatDoesntCountIsNoReadOfItsWeek(t *testing.T) {
	clock := &testClock{now: onDay(1, 4, 40)}
	r := newPrimingRouter(t, clock.read, &stubProber{}, daytime)
	// A request goes out at 04:40, its answer slow; one sent after it is
	// answered at 04:45.
	slow := r.state.mark()
	weekEnding := endingAtFive(week)
	sideReadAt(onDay(1, 4, 45), lapsedAtOne, weekEnding)(r, clock)
	// At 04:56 the slow answer comes, reading the week as it was before.
	clock.now = onDay(1, 4, 56)
	earlier := weekEnding
	earlier.Utilization -= 0.1
	r.state.record("side", []quota.Window{earlier}, slow)
	clock.now = onDay(1, 4, 58)

	if got, _ := r.Status().Account("side"); got.Windows[1].Utilization != weekEnding.Utilization {
		t.Fatalf("side's week reads %v, want %v: the slow answer's reading outweighed", got.Windows[1].Utilization, weekEnding.Utilization)
	}
	if !r.state.rounding(r.rounds.clears)("side", clock.now) {
		t.Error("side not due on the router's rounds, want it probed before its week resets: the slow answer's reading of it didn't count")
	}
}

// lapsedAtOne is side's session as read in the tests of its weeks: it ran
// till 01:00, and has lapsed since. Side's slot is 06:40.
var lapsedAtOne = quota.Window{Key: "5h", Label: "Session", Utilization: 0.4, ResetsAt: onDay(1, 1, 0).UTC()}

// endingAtFive is w resetting at 05:00.
func endingAtFive(w quota.Window) quota.Window {
	w.ResetsAt = onDay(1, 5, 0).UTC()
	return w
}

// sideReadAt has an answer read side's windows at a time.
func sideReadAt(at time.Time, windows ...quota.Window) step {
	return func(r *Router, clock *testClock) {
		clock.now = at
		r.state.record("side", windows, r.state.mark())
	}
}

// probeReadAt has a probe of side read its windows at a time.
func probeReadAt(at time.Time, windows ...quota.Window) step {
	return func(r *Router, clock *testClock) {
		clock.now = at
		r.state.recordProbe("side", probed(nil, windows...), nil, r.state.mark(), readings.FromProbe)
	}
}

// step readies the router's state for a test, on the clock it moves as it
// goes.
type step func(r *Router, clock *testClock)

// answered has an answer read side's session and week ago before start.
func answered(ago time.Duration) step {
	return func(r *Router, clock *testClock) {
		clock.now = start.Add(-ago)
		r.state.record("side", []quota.Window{session, week}, r.state.mark())
	}
}

// probedSide has a probe of side, as from says, read its session and week
// ago before start.
func probedSide(ago time.Duration, from readings.Source) step {
	return func(r *Router, clock *testClock) {
		clock.now = start.Add(-ago)
		r.state.recordProbe("side", probed(nil, session, week), nil, r.state.mark(), from)
	}
}

// partlyRead has a probe of side read its session and week ago before start,
// but not its Fable week.
func partlyRead(ago time.Duration) step {
	return func(r *Router, clock *testClock) {
		clock.now = start.Add(-ago)
		reading := probed(nil, session, week)
		reading.Failures = []quota.Failure{{Label: "Fable", Window: fableWeek.Key, Error: errUnreadable.Error()}}
		r.state.recordProbe("side", reading, nil, r.state.mark(), readings.FromProbe)
	}
}

// unread has a probe of side read nothing ago before start.
func unread(ago time.Duration) step {
	return unreadAt(start.Add(-ago))
}

// unreadAt has a probe of side read nothing at a time.
func unreadAt(at time.Time) step {
	return func(r *Router, clock *testClock) {
		clock.now = at
		r.state.recordProbe("side", quota.Probe{}, errUnreadable, r.state.mark(), readings.FromProbe)
	}
}

// refusedSide has the upstream refuse side's token ago before start.
func refusedSide(ago time.Duration) step {
	return refusedAt(start.Add(-ago))
}

// refusedAt has the upstream refuse side's token at a time.
func refusedAt(at time.Time) step {
	return func(r *Router, clock *testClock) {
		clock.now = at
		r.state.refuse("side", http.StatusUnauthorized, someRequest)
	}
}

// startRounds runs the router's rounds until the stop it returns is called,
// which waits for them, and their probes, to end.
func startRounds(r *Router) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	var rounding sync.WaitGroup
	rounding.Go(func() { r.rounds.run(ctx) })
	return func() {
		cancel()
		rounding.Wait()
		r.probes.stop()
	}
}
