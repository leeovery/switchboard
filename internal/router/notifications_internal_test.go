package router

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const day = 24 * time.Hour

func TestALimitIsToldOfOnceWithTheSessionsItMoved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newNotifying(t, config.Notifications{Limits: true})
		h.read("2", h.session(1, 3*time.Hour), h.week(0.5, 3*day))
		h.start()

		h.limit("2", []string{"5h"}, 3*time.Hour)
		for _, session := range []string{"0b5c6f2e", "18bb978f", "7d41a3b9"} {
			h.after(time.Second)
			h.hear(forced(session, "2", "1"))
		}
		h.after(2*time.Second - time.Nanosecond)
		h.expect()
		h.after(time.Nanosecond)
		h.expect("2 · two hit its Session limit, back at Sat 03:00 — 3 sessions moved to 1 · one")
		h.after(time.Hour)
		h.expect("2 · two hit its Session limit, back at Sat 03:00 — 3 sessions moved to 1 · one")

		want := []string{"level=INFO", "msg=notification component=router", "account=2", `news="hit its Session limit, back at Sat 03:00 — 3 sessions moved to 1"`}
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
		if strings.Contains(log.String(), "·") {
			t.Errorf("log reads\n%s\nwant accounts named by their ids alone", log)
		}
	})
}

func TestALimitsNotificationSaysWhereItsSessionsWent(t *testing.T) {
	tests := []struct {
		name string
		// windows are those the limit was reached in, and lifts is how long
		// after the clock began it lifts.
		windows []string
		lifts   time.Duration
		// full leaves 1 and 3 without quota, and refused has their tokens
		// refused.
		full, refused bool
		moves         []Event
		want          string
	}{
		{
			name:    "to one account",
			windows: []string{"5h"}, lifts: 3 * time.Hour,
			moves: []Event{forced("a", "2", "1")},
			want:  "2 · two hit its Session limit, back at Sat 03:00 — 1 session moved to 1 · one",
		},
		{
			name:    "to several, counting a session each once, whatever its models",
			windows: []string{"5h"}, lifts: 3 * time.Hour,
			moves: []Event{forced("a", "2", "1"), forced("b", "2", "3"), forcedModel("a", haiku, "2", "3"), forced("c", "2", "1")},
			want:  "2 · two hit its Session limit, back at Sat 03:00 — 3 sessions moved to 1 · one and 3 · three",
		},
		{
			name:    "to none, with no other account with room",
			windows: []string{"5h"}, lifts: 3 * time.Hour, full: true,
			want: "2 · two hit its Session limit, back at Sat 03:00 — no other account has room",
		},
		{
			name:    "to none, with every other account refused",
			windows: []string{"5h"}, lifts: 3 * time.Hour, refused: true,
			want: "2 · two hit its Session limit, back at Sat 03:00 — no other account has room",
		},
		{
			name:    "to none, with another account with room",
			windows: []string{"5h"}, lifts: 3 * time.Hour,
			want: "2 · two hit its Session limit, back at Sat 03:00",
		},
		{
			name:    "reached in several windows",
			windows: []string{"5h", "7d"}, lifts: 3 * day,
			moves: []Event{forced("a", "2", "1")},
			want:  "2 · two hit its Session and Week limits, back at Tue 00:00 — 1 session moved to 1 · one",
		},
		{
			name:  "reached in no window named",
			lifts: 5 * time.Minute,
			moves: []Event{forced("a", "2", "1")},
			want:  "2 · two hit its limit, back at Sat 00:05 — 1 session moved to 1 · one",
		},
		{
			name:    "reached in a window never read, named by its key",
			windows: []string{"7d_xx"}, lifts: 3 * day,
			moves: []Event{forced("a", "2", "1")},
			want:  "2 · two hit its 7d_xx limit, back at Tue 00:00 — 1 session moved to 1 · one",
		},
		{
			name:    "without when it's back once that has passed",
			windows: []string{"5h"}, lifts: 3 * time.Second,
			moves: []Event{forced("a", "2", "1")},
			want:  "2 · two hit its Session limit — 1 session moved to 1 · one",
		},
		{
			name:    "without when it's back when that doesn't fit",
			windows: []string{"5h", "7d", "7d_oi"}, lifts: 3 * day,
			moves: []Event{forced("a", "2", "1"), forced("b", "2", "3")},
			want:  "2 · two hit its Session, Week and Fable week limits — 2 sessions moved to 1 · one and 3 · three",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, config.Notifications{Limits: true})
				h.read("2", h.session(1, 3*time.Hour), h.week(0.5, 3*day), h.fableWeek(0.5, 4*day))
				used := 0.2
				if tt.full {
					used = 1
				}
				for _, id := range []string{"1", "3"} {
					h.read(id, h.session(used, time.Hour), h.week(0.5, 3*day))
					if tt.refused {
						h.state.refuse(id, http.StatusUnauthorized, someRequest)
					}
				}
				h.start()

				h.limit("2", tt.windows, tt.lifts)
				h.hear(tt.moves...)
				h.after(gatherFor)
				h.expect(tt.want)
			})
		})
	}
}

func TestALimitIsToldOfOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Limits: true})
		h.read("2", h.session(1, 5*time.Hour), h.week(1, 4*day))
		h.start()

		h.limit("2", []string{"5h"}, 3*time.Hour)
		h.after(time.Second)
		h.limit("2", []string{"7d"}, 4*day)
		h.after(gatherFor)
		first := "2 · two hit its Session and Week limits, back at Wed 00:00"
		h.expect(first)

		h.after(10 * time.Minute)
		h.limit("2", []string{"7d"}, 4*day)
		h.after(gatherFor)
		h.expect(first)

		h.after(10 * time.Minute)
		h.limit("2", []string{"5h"}, 5*time.Hour)
		h.after(gatherFor)
		h.expect(first)

		h.after(4 * day)
		h.limit("2", []string{"5h"}, 4*day+5*time.Hour)
		h.after(gatherFor)
		h.expect(first, "2 · two hit its Session limit, back at Wed 05:00")
	})
}

func TestALimitThatDoesntSayWhenItLiftsIsToldOfOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Limits: true})
		h.start()

		for range 4 {
			reached, _ := h.state.limit("2", nil, time.Time{}, h.state.mark())
			h.hear(reached)
			h.after(time.Minute)
		}
		h.expect("2 · two hit its limit, back at Sat 00:05")
	})
}

func TestALimitsNotificationLeavesOutMovesItDidntCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Limits: true})
		h.read("2", h.session(1, 3*time.Hour), h.week(0.5, 3*day))
		h.start()

		h.limit("2", []string{"5h"}, 3*time.Hour)
		h.hear(
			Moved{Session: "idle", Model: opus, From: "2", To: "1", Reason: "rescored after 1h 2m idle"},
			Moved{Session: "pinned", Model: opus, From: "2", To: "1", Reason: "moved by pin"},
			forced("elsewhere", "3", "1"),
			forced("moved", "2", "3"),
		)
		h.after(gatherFor)
		h.expect("2 · two hit its Session limit, back at Sat 03:00 — 1 session moved to 3 · three")
	})
}

func TestRoomAgain(t *testing.T) {
	withRoom := func(h *notifying) { h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day)) }
	tests := []struct {
		name string
		// before sets 2 up before notifications start, and bar takes its
		// quota away after. lift gives it back where time alone doesn't, and
		// wait is how long it takes to come back.
		before func(h *notifying)
		bar    func(h *notifying)
		lift   func(h *notifying)
		wait   time.Duration
	}{
		{
			name:   "when a limit lifts, with no traffic",
			before: withRoom,
			bar:    func(h *notifying) { h.state.limit("2", []string{"5h"}, h.began.Add(time.Minute), h.state.mark()) },
			wait:   time.Minute,
		},
		{
			name:   "when a limit reached in no window named lifts",
			before: withRoom,
			bar:    func(h *notifying) { h.state.limit("2", nil, h.began.Add(time.Minute), h.state.mark()) },
			wait:   time.Minute,
		},
		{
			name:   "when a spent window resets, with no traffic",
			before: func(h *notifying) { h.read("2", h.session(1, 2*time.Minute), h.week(0.5, 3*day)) },
			bar:    func(*notifying) {},
			wait:   2 * time.Minute,
		},
		{
			name:   "when a reading lifts a limit",
			before: func(h *notifying) { h.read("2", h.session(1, time.Hour), h.week(0.5, 3*day)) },
			bar:    func(h *notifying) { h.state.limit("2", []string{"5h"}, h.began.Add(2*time.Hour), h.state.mark()) },
			lift:   func(h *notifying) { h.read("2", h.session(0.01, 6*time.Hour)) },
			wait:   lookEvery,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, config.Notifications{Room: true})
				tt.before(h)
				h.start()
				tt.bar(h)
				h.after(lookEvery)
				h.expect()

				if tt.lift != nil {
					tt.lift(h)
				}
				h.after(tt.wait)
				h.expect("2 · two has room again")
			})
		})
	}
}

func TestRoomAgainOnceTheWindowAtAnAccountsReserveResets(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		reserving := slices.Clone(numbered)
		reserving[1].Reserve = 0.1
		h := newNotifyingOver(t, config.Notifications{Room: true}, reserving)
		h.read("2", h.session(0.2, 2*time.Minute), h.week(0.5, 3*day))
		h.start()

		h.read("2", h.session(0.93, 2*time.Minute))
		h.after(lookEvery)
		h.expect()
		h.after(2 * time.Minute)
		h.expect("2 · two has room again")
	})
}

func TestRoomAgainOnceQuotaIsBackAndARefusalAlongsideHasLifted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Room: true})
		h.read("2", h.session(1, 2*time.Minute), h.week(0.5, 3*day))
		h.start()

		h.state.refuse("2", http.StatusUnauthorized, someRequest)
		h.hear(Refused{Account: "2", Status: http.StatusUnauthorized})
		h.after(5 * time.Minute)
		h.expect()
		h.after(refusedFor - 5*time.Minute)
		h.expect("2 · two has room again")
	})
}

func TestARevokedTokenIsNeverRoomAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Room: true})
		h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day))
		h.start()

		for range 5 {
			h.state.refuse("2", http.StatusUnauthorized, someRequest)
			h.hear(Refused{Account: "2", Status: http.StatusUnauthorized})
			h.after(refusedFor + lookEvery)
		}
		h.expect()
	})
}

func TestNoRoomAgainUnlessQuotaRanOut(t *testing.T) {
	tests := []struct {
		name  string
		setUp func(h *notifying)
	}{
		{
			name: "at the first look, finding it with room",
			setUp: func(h *notifying) {
				h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day))
				h.start()
			},
		},
		{
			name: "at its first reading",
			setUp: func(h *notifying) {
				h.start()
				h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day))
			},
		},
		{
			name: "under a limit that holds back one model alone",
			setUp: func(h *notifying) {
				h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day))
				h.start()
				h.state.limit("2", []string{"7d_oi"}, h.began.Add(time.Minute), h.state.mark())
			},
		},
		{
			name: "once a refusal lifts, with its quota never out",
			setUp: func(h *notifying) {
				h.read("2", h.session(0.2, 5*time.Hour), h.week(0.5, 3*day))
				h.start()
				h.state.refuse("2", http.StatusUnauthorized, someRequest)
			},
		},
		{
			name: "once a refusal lifts, with nothing read of it",
			setUp: func(h *notifying) {
				h.start()
				h.state.refuse("2", http.StatusUnauthorized, someRequest)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, config.Notifications{Room: true})
				tt.setUp(h)
				h.after(refusedFor + lookEvery)
				h.expect()
			})
		})
	}
}

func TestWarnings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Warning: 0.9})
		h.read("2", h.session(0.2, 5*time.Hour), h.week(0.85, 3*day))
		h.read("3", h.session(0.2, 5*time.Hour), h.week(0.95, 3*day))
		h.start()
		h.after(lookEvery)
		h.expect()

		h.read("2", h.week(0.91, 3*day))
		h.after(lookEvery)
		h.expect("2 · two: Week at 91%")

		h.read("2", h.week(0.95, 3*day))
		h.after(time.Minute)
		h.expect("2 · two: Week at 91%")

		h.read("2", h.week(0.1, 7*day))
		h.after(lookEvery)
		h.read("2", h.week(0.92, 7*day))
		h.after(lookEvery)
		h.expect("2 · two: Week at 91%", "2 · two: Week at 92%")
	})
}

func TestWarningsAtTheShareGiven(t *testing.T) {
	tests := []struct {
		name    string
		warning float64
		want    []string
	}{
		{name: "passing it", warning: 0.8, want: []string{"2 · two: Session at 85%"}},
		{name: "reaching it", warning: 0.85, want: []string{"2 · two: Session at 85%"}},
		{name: "short of it", warning: 0.86},
		{name: "none at 0", warning: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, config.Notifications{Warning: tt.warning})
				h.read("2", h.session(0.5, 5*time.Hour), h.week(0.5, 3*day))
				h.start()

				h.read("2", h.session(0.85, 5*time.Hour))
				h.after(lookEvery)
				h.expect(tt.want...)
			})
		})
	}
}

func TestAWindowIsWarnedOfOnceAReset(t *testing.T) {
	reset := start.Add(3 * day)
	shown := func(used float64, resets time.Time) standings {
		w := quota.Window{Key: "7d", Label: "Week", Utilization: used, ResetsAt: resets}
		two := status.Account{ID: "2", Label: "two", Windows: []quota.Window{w}}
		return standings{{Account: two, quota: true, known: true}}
	}
	l := newLookout(false, 0.9)
	var warned []string
	for _, s := range []standings{
		shown(0.5, reset), shown(0.91, reset), shown(0.85, reset), shown(0.93, reset),
		shown(0.2, reset.Add(7*day)), shown(0.94, reset.Add(7*day)),
	} {
		for _, n := range l.look(s) {
			warned = append(warned, n.Message)
		}
	}
	if want := []string{"2 · two: Week at 91%", "2 · two: Week at 94%"}; !slices.Equal(warned, want) {
		t.Errorf("warned %q, want %q: once a reset, however its readings come and go", warned, want)
	}
}

func TestMoves(t *testing.T) {
	tests := []struct {
		name     string
		settings config.Notifications
		want     []string
	}{
		{
			name:     "none by default",
			settings: config.Notifications{Limits: true, Room: true, Warning: 0.9},
			want:     []string{"2 · two hit its Session limit, back at Sat 03:00 — 1 session moved to 1 · one"},
		},
		{
			name:     "those a limit's notification doesn't tell of",
			settings: config.Notifications{Limits: true, Room: true, Warning: 0.9, Moves: true},
			want: []string{
				"session 18bb978f moved from 3 · three to 1 · one (rescored after 1h 2m idle)",
				"2 · two hit its Session limit, back at Sat 03:00 — 1 session moved to 1 · one",
			},
		},
		{
			name:     "every one, without limits",
			settings: config.Notifications{Moves: true},
			want: []string{
				"session 18bb978f moved from 3 · three to 1 · one (rescored after 1h 2m idle)",
				"session 0b5c6f2e moved from 2 · two to 1 · one (moved: 2 hit its limit)",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := newNotifying(t, tt.settings)
				h.read("2", h.session(1, 3*time.Hour), h.week(0.5, 3*day))
				h.start()

				h.hear(Moved{Session: "18bb978f-5e0c-4c1b-9a3f-2d6e8b1c4a7f", Model: opus, From: "3", To: "1", Reason: "rescored after 1h 2m idle"})
				h.limit("2", []string{"5h"}, 3*time.Hour)
				h.hear(forced("0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", "2", "1"))
				h.after(gatherFor)
				h.expect(tt.want...)
			})
		})
	}
}

func TestOneNotificationAnAccountAMinute(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newNotifying(t, config.Notifications{Room: true, Warning: 0.9, Moves: true})
		h.read("2", h.session(1, 3*time.Minute), h.week(0.5, 3*day))
		h.start()
		h.after(3 * time.Minute)
		h.expect("2 · two has room again")

		h.read("2", h.session(0.2, 5*time.Hour), h.week(0.91, 3*day))
		h.hear(Moved{Session: "18bb978f", Model: opus, From: "1", To: "3", Reason: "moved by pin"})
		h.expect("2 · two has room again", "session 18bb978f moved from 1 · one to 3 · three (moved by pin)")
		want := []string{"level=DEBUG", `msg="notification dropped: too soon after the last about the account"`, "account=2", `news="Week at 91%"`}
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}

		h.after(time.Minute)
		h.read("2", h.week(0.1, 7*day))
		h.after(lookEvery)
		h.read("2", h.week(0.92, 7*day))
		h.after(lookEvery)
		h.expect("2 · two has room again", "session 18bb978f moved from 1 · one to 3 · three (moved by pin)", "2 · two: Week at 92%")
	})
}

func TestALimitIsToldOfWhateverWentOutJustBefore(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newNotifying(t, config.Notifications{Limits: true, Warning: 0.9})
		h.read("2", h.session(0.2, 5*time.Hour), h.week(0.85, 3*day))
		h.start()
		h.read("2", h.week(0.91, 3*day))
		h.after(lookEvery)
		h.expect("2 · two: Week at 91%")

		h.after(30 * time.Second)
		h.limit("2", []string{"5h"}, 3*time.Hour)
		h.after(gatherFor)
		h.expect("2 · two: Week at 91%", "2 · two hit its Session limit, back at Sat 03:00")

		h.read("2", h.session(0.95, 5*time.Hour))
		h.after(lookEvery)
		h.expect("2 · two: Week at 91%", "2 · two hit its Session limit, back at Sat 03:00")
		want := []string{"level=DEBUG", `msg="notification dropped: too soon after the last about the account"`, "account=2", `news="Session at 95%"`}
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant the warning after the limit dropped: a line with %q", log, want)
		}
	})
}

func TestNothingWithEverythingOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{})
		h.read("2", h.session(1, time.Minute), h.week(0.85, 3*day))
		h.start()

		h.limit("2", []string{"5h"}, time.Hour)
		h.hear(forced("a", "2", "1"), Moved{Session: "b", Model: opus, From: "3", To: "1", Reason: "moved by pin"})
		h.read("2", h.session(0.2, 5*time.Hour), h.week(0.95, 3*day))
		h.after(2 * time.Hour)
		h.expect()
	})
}

func TestARouterPostsNotificationsOnlyWhenAskedTo(t *testing.T) {
	tests := []struct {
		name     string
		notifier Notifier
		settings config.Notifications
		want     bool
	}{
		{name: "with a notifier and some asked for", notifier: &noting{}, settings: config.Notifications{Moves: true}, want: true},
		{name: "with none asked for", notifier: &noting{}},
		{name: "without a notifier", settings: config.Notifications{Limits: true, Room: true, Warning: 0.9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(Config{
				Accounts:      testConfigured,
				Token:         testTokens.Read,
				Upstream:      "http://127.0.0.1:1",
				Provider:      claude.Provider{},
				Prober:        &stubProber{},
				Policy:        testPolicy,
				Now:           at(start),
				Notifier:      tt.notifier,
				Notifications: tt.settings,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if got := r.notifications != nil; got != tt.want {
				t.Errorf("posts notifications: %v, want %v", got, tt.want)
			}
		})
	}
}

func TestANotifierThatHangsNeverHoldsUpTheListener(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newNotifying(t, config.Notifications{Moves: true})
		h.notifier.hold = make(chan struct{})
		h.start()

		began := time.Now()
		for i := range queueSize + 10 {
			h.n.hear(Moved{Session: "s", Model: opus, From: numbered[i%len(numbered)].ID, To: "1", Reason: "moved by pin"})
		}
		if waited := time.Since(began); waited != 0 {
			t.Errorf("the listener waited %v with the notifier hanging, want no wait", waited)
		}
		synctest.Wait()
		if !log.Has("level=WARN", `msg="notifications fell behind; event dropped"`, "event=router.Moved") {
			t.Errorf("log reads\n%s\nwant the events there was no room for dropped", log)
		}
		close(h.notifier.hold)
	})
}

func TestANotificationThatFailsIsLogged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newNotifying(t, config.Notifications{Room: true})
		h.notifier.fail(errors.New("post notification: exit status 1"))
		h.read("2", h.session(1, time.Minute), h.week(0.5, 3*day))
		h.start()

		h.after(time.Minute)
		h.expect("2 · two has room again")
		want := []string{"level=WARN", `msg="notification failed" component=router`, "account=2", `news="room again"`, `error="post notification: exit status 1"`}
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
		if log.Has("msg=notification ") {
			t.Errorf("log reads\n%s\nwant no word of the notification going", log)
		}
	})
}

func TestANotificationThatFailsDoesntQuietItsAccount(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Room: true, Warning: 0.9})
		h.notifier.fail(errors.New("post notification: exit status 1"))
		h.read("2", h.session(1, 3*time.Minute), h.week(0.5, 3*day))
		h.start()
		h.after(3 * time.Minute)
		h.expect("2 · two has room again")

		h.notifier.fail(nil)
		h.read("2", h.session(0.2, 5*time.Hour), h.week(0.91, 3*day))
		h.after(lookEvery)
		h.expect("2 · two has room again", "2 · two: Week at 91%")
	})
}

func TestStoppingTellsOfTheLimitsStillGatheringAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Limits: true, Room: true, Warning: 0.9, Moves: true})
		h.read("2", h.session(1, time.Hour), h.week(0.5, 3*day))
		h.start()
		h.limit("2", []string{"5h"}, time.Hour)
		h.hear(forced("a", "2", "1"))

		began := time.Now()
		h.stop()
		if waited := time.Since(began); waited != 0 {
			t.Errorf("stopping took %v, want no wait, even with a limit gathering", waited)
		}
		h.expect("2 · two hit its Session limit, back at Sat 01:00 — 1 session moved to 1 · one")
	})
}

func TestStoppingWaitsForANotifierThatHangsAShortWhileAtMost(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		h := newNotifying(t, config.Notifications{Limits: true})
		h.notifier.hold = make(chan struct{})
		h.start()
		h.limit("2", []string{"5h"}, time.Hour)

		began := time.Now()
		h.stop()
		if waited := time.Since(began); waited != 3*time.Second {
			t.Errorf("stopping took %v with the notifier hanging, want 3s", waited)
		}
		if !log.Has("level=WARN", `msg="stopped waiting for notifications to post"`, "after=3s") {
			t.Errorf("log reads\n%s\nwant the wait cut short", log)
		}
		close(h.notifier.hold)
	})
}

func TestStoppingTellsOfALimitStillQueued(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := newNotifying(t, config.Notifications{Limits: true})
		h.read("2", h.session(1, time.Hour), h.week(0.5, 3*day))

		reached, _ := h.state.limit("2", []string{"5h"}, h.began.Add(time.Hour), h.state.mark())
		h.n.hear(reached)
		h.n.finish()
		h.expect("2 · two hit its Session limit, back at Sat 01:00")
	})
}

// numbered are accounts named as the design's examples name them: 1 · one,
// 2 · two and 3 · three, each with a token.
var numbered = []config.Account{
	{ID: "1", Label: "one"},
	{ID: "2", Label: "two"},
	{ID: "3", Label: "three"},
}

// numberedTokens are the numbered accounts' tokens.
var numberedTokens = tokenstest.Files{"1": "test-token-1", "2": "test-token-2", "3": "test-token-3"}

// notifying is notifications at work in a synctest bubble, over the state of
// the numbered accounts, on the bubble's clock read in UTC, which begins on a
// Saturday at midnight.
type notifying struct {
	t        *testing.T
	state    *state
	n        *notifications
	notifier *noting
	// began is when the clock began.
	began time.Time
	// stop stops notifications, and waits for them to end.
	stop func()
}

// newNotifying sets notifications up as settings asks, over the numbered
// accounts, posting to a notifier that notes each message. They run once the
// test starts them.
func newNotifying(t *testing.T, settings config.Notifications) *notifying {
	t.Helper()
	return newNotifyingOver(t, settings, numbered)
}

// newNotifyingOver is newNotifying over the accounts configured, which have
// the numbered accounts' tokens.
func newNotifyingOver(t *testing.T, settings config.Notifications, configured []config.Account) *notifying {
	t.Helper()
	now := func() time.Time { return time.Now().UTC() }
	s := newState(resolve(configured, numberedTokens.Read), testPolicy, claude.Provider{}.Family, now, unkept, unkept)
	notifier := &noting{}
	return &notifying{t: t, state: s, n: newNotifications(settings, notifier, s, now), notifier: notifier, began: now()}
}

// start runs notifications until the test ends or stops them, returning once
// their first look is done.
func (h *notifying) start() {
	ctx, cancel := context.WithCancel(h.t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.n.run(ctx)
	}()
	h.stop = func() {
		cancel()
		<-done
	}
	synctest.Wait()
}

// hear has notifications hear each event, and deal with it.
func (h *notifying) hear(events ...Event) {
	for _, e := range events {
		h.n.hear(e)
	}
	synctest.Wait()
}

// limit has the account with the given id reach its limit in the windows
// given, until lifts after the clock began, and tells of it.
func (h *notifying) limit(id string, windows []string, lifts time.Duration) {
	reached, _ := h.state.limit(id, windows, h.began.Add(lifts), h.state.mark())
	h.hear(reached)
}

// read has the router read windows of the account with the given id.
func (h *notifying) read(id string, windows ...quota.Window) {
	h.state.record(id, windows, h.state.mark())
}

// after lets d pass, and notifications deal with all it brings.
func (h *notifying) after(d time.Duration) {
	time.Sleep(d)
	synctest.Wait()
}

// expect checks what has been posted, in order.
func (h *notifying) expect(want ...string) {
	h.t.Helper()
	if got := h.notifier.posted(); !slices.Equal(got, want) {
		h.t.Errorf("posted %q, want %q", got, want)
	}
}

// session is a five-hour window used as given, which resets left after the
// clock began, and refuses requests once it's spent.
func (h *notifying) session(used float64, left time.Duration) quota.Window {
	return h.window("5h", "Session", used, left)
}

// week is a week as session is a five-hour window.
func (h *notifying) week(used float64, left time.Duration) quota.Window {
	return h.window("7d", "Week", used, left)
}

// fableWeek is Fable's own week as session is a five-hour window.
func (h *notifying) fableWeek(used float64, left time.Duration) quota.Window {
	return h.window("7d_oi", "Fable week", used, left)
}

func (h *notifying) window(key, label string, used float64, left time.Duration) quota.Window {
	verdict := quota.StatusAllowed
	if used >= 1 {
		verdict = quota.StatusRejected
	}
	return quota.Window{Key: key, Label: label, Utilization: used, ResetsAt: h.began.Add(left), Status: verdict}
}

// forced is a session's opus requests leaving an account that couldn't take
// them for another.
func forced(session, from, to string) Moved {
	return forcedModel(session, opus, from, to)
}

// forcedModel is forced, for requests of the model given.
func forcedModel(session, model, from, to string) Moved {
	return Moved{Session: session, Model: model, From: from, To: to, Reason: "moved: " + from + " hit its limit", Forced: true}
}

// noting is a notifier that notes each message it's given, and fails as fail
// has it. While hold is open, each post waits for it to close.
type noting struct {
	hold chan struct{}

	mu       sync.Mutex
	err      error
	messages []string
}

func (n *noting) Notify(message string) error {
	n.mu.Lock()
	n.messages = append(n.messages, message)
	err := n.err
	n.mu.Unlock()
	if n.hold != nil {
		<-n.hold
	}
	return err
}

// fail has each post from now on fail with err, or none when it's nil.
func (n *noting) fail(err error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.err = err
}

// posted returns the messages given so far.
func (n *noting) posted() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.messages)
}
