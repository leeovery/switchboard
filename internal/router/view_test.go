package router

import (
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

func TestWindowsCountTheFamiliesTheyveBeenSeenOn(t *testing.T) {
	fableSpent := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 1, ResetsAt: start.Add(48 * time.Hour), Status: quota.StatusRejected}
	windows := []quota.Window{session, week, fableSpent}
	everyFamily := map[string][]string{"5h": {haiku, fable}, "7d": {haiku, fable}, "7d_oi": {fable}}
	tests := []struct {
		name  string
		learn func(s *state)
		// room says, by model, whether work, whose Fable week is spent, has
		// room for a request of it.
		room map[string]bool
	}{
		{
			name:  "a window seen on no family counts every model",
			learn: func(*state) {},
			room:  map[string]bool{fable: false, haiku: false, opus: false},
		},
		{
			name:  "a window seen on a response counts its family's models alone",
			learn: func(s *state) { s.learn(fable, windows) },
			room:  map[string]bool{fable: false, "claude-fable-5": false, haiku: true, opus: true},
		},
		{
			name:  "a window seen on a probe counts its family's models alone",
			learn: func(s *state) { s.recordProbe("work", probed(everyFamily, windows...), nil, s.mark(), fromProbe) },
			room:  map[string]bool{fable: false, "claude-fable-5": false, haiku: true, opus: true},
		},
		{
			name: "a window seen on two families counts both",
			learn: func(s *state) {
				s.learn(fable, windows)
				s.learn(opus, windows)
			},
			room: map[string]bool{fable: false, opus: false, haiku: true},
		},
		{
			name:  "what's seen on one account counts on every account",
			learn: func(s *state) { s.recordProbe("side", probed(everyFamily, windows...), nil, s.mark(), fromProbe) },
			room:  map[string]bool{fable: false, haiku: true},
		},
		{
			name:  "a response to a request of no known model teaches nothing",
			learn: func(s *state) { s.learn("", windows) },
			room:  map[string]bool{fable: false, haiku: false},
		},
		{
			name:  "a request of no known model is counted by windows seen on no family",
			learn: func(s *state) { s.learn(fable, windows) },
			room:  map[string]bool{"": true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", windows, s.mark())

			tt.learn(s)
			for model, want := range tt.room {
				if got := s.view(model, start).room("work"); got != want {
					t.Errorf("work has room for %q: %v, want %v", model, got, want)
				}
			}
		})
	}
}

func TestWindowsEveryModelSharesCountEveryModel(t *testing.T) {
	s := newTestState(&testClock{now: start})
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	s.record("work", []quota.Window{spent, week}, s.mark())

	s.learn(fable, []quota.Window{spent, week})
	if s.view(haiku, start).room("work") {
		t.Error("work has room for Haiku, want none: its session is spent, and every model shares it")
	}
}

func TestView(t *testing.T) {
	s := newTestState(&testClock{now: start})
	s.record("side", []quota.Window{week, session}, s.mark())

	v := s.view(opus, start)
	want := []score.Candidate{{ID: "work"}, {ID: "side", Windows: []quota.Window{session, week}}}
	if !reflect.DeepEqual(v.candidates, want) {
		t.Errorf("candidates =\n%+v\nwant every account with a token, in order, as last read:\n%+v", v.candidates, want)
	}
	for id, want := range map[string]bool{"work": true, "side": true, "personal": false, "nope": false} {
		if got := v.room(id); got != want {
			t.Errorf("room(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestARefusedAccountHasNoRoomForTenMinutes(t *testing.T) {
	tests := []struct {
		name   string
		refuse func(s *state)
		// room says, by model, whether work has room for a request of it
		// while the refusal holds.
		room map[string]bool
	}{
		{
			name:   "its token refused, for any request",
			refuse: func(s *state) { s.refuse("work", http.StatusUnauthorized, someRequest) },
			room:   map[string]bool{opus: false, "claude-opus-4-1-20250805": false, haiku: false},
		},
		{
			name:   "a request refused, for its model's family alone",
			refuse: func(s *state) { s.forbid("work", "opus", http.StatusForbidden, someRequest) },
			room:   map[string]bool{opus: false, "claude-opus-4-1-20250805": false, haiku: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", []quota.Window{session, week}, s.mark())
			tt.refuse(s)

			for model, room := range tt.room {
				for after, lifted := range map[time.Duration]bool{0: false, 10*time.Minute - time.Nanosecond: false, 10 * time.Minute: true} {
					v := s.view(model, start.Add(after))
					want := room || lifted
					if got := v.room("work"); got != want {
						t.Errorf("%v after the refusal, room(work) for %q = %v, want %v", after, model, got, want)
					}
					if picked, _ := v.pick(""); (picked.ID == "work") != want {
						t.Errorf("%v after the refusal, pick() for %q = %q, want work: %v", after, model, picked.ID, want)
					}
				}
			}
		})
	}
}

func TestALimitHoldsBackTheRequestsItsWindowsCount(t *testing.T) {
	tests := []struct {
		name string
		// windows are those the limit was reached in.
		windows []string
		// room says, by model, whether work has room for a request of it
		// while the limit holds.
		room map[string]bool
	}{
		{
			name:    "reached in a window every model shares, every request",
			windows: []string{"5h"},
			room:    map[string]bool{opus: false, haiku: false, fable: false},
		},
		{
			name:    "reached in a model's own window, that model's alone",
			windows: []string{"7d_oi"},
			room:    map[string]bool{opus: true, haiku: true, fable: false},
		},
		{
			name: "reached in no window named, every request",
			room: map[string]bool{opus: false, haiku: false, fable: false},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestState(&testClock{now: start})
			s.record("work", []quota.Window{session, week, fableWeek}, s.mark())
			s.learn(fable, []quota.Window{session, week, fableWeek})

			until := s.limit("work", tt.windows, start.Add(time.Hour)).Until
			for model, want := range tt.room {
				if got := s.view(model, start).room("work"); got != want {
					t.Errorf("under the limit, work has room for %q: %v, want %v", model, got, want)
				}
				if !s.view(model, until).room("work") {
					t.Errorf("once the limit lifts, work has no room for %q, want room", model)
				}
			}
		})
	}
}

func TestALimitLiftsOnAReadingShowingItsWindowsWithRoom(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	fresh := session
	fresh.Utilization, fresh.ResetsAt = 0.01, session.ResetsAt.Add(5*time.Hour)
	tests := []struct {
		name string
		// windows are those the limit was reached in.
		windows []string
		reading []quota.Window
		want    bool
	}{
		{name: "its window read again with room", windows: []string{"5h"}, reading: []quota.Window{fresh, week}, want: true},
		{name: "its window read again, still spent", windows: []string{"5h"}, reading: []quota.Window{spent, week}, want: false},
		{name: "a reading without its window", windows: []string{"5h"}, reading: []quota.Window{week}, want: false},
		{name: "one of its windows read with room, the other unread", windows: []string{"5h", "7d_oi"}, reading: []quota.Window{fresh, week}, want: false},
		{name: "a reading of a limit reached in no window named", reading: []quota.Window{fresh, week}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{now: start}
			s := newTestState(clock)
			s.record("work", []quota.Window{spent, week}, s.mark())
			s.limit("work", tt.windows, start.Add(time.Hour))
			clock.now = start.Add(time.Minute)

			s.recordProbe("work", quota.Probe{Windows: tt.reading}, nil, s.mark(), fromProbe)
			if lifted := s.usage["work"].limited.until.IsZero(); lifted != tt.want {
				t.Errorf("the limit lifted: %v, want %v", lifted, tt.want)
			}
		})
	}
}

func TestALimitThatsAlreadyDueHoldsFiveMinutes(t *testing.T) {
	for _, until := range []time.Time{{}, start.Add(-time.Minute), start} {
		s := newTestState(&testClock{now: start})
		if got := s.limit("work", nil, until).Until; got != start.Add(5*time.Minute) {
			t.Errorf("limit() until %v holds until %v, want five minutes on", until, got)
		}
	}
}

func TestALimitReachedAgainWhileItsInForceIsTheSameLimit(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week, fableWeek}, s.mark())
	steps := []struct {
		name string
		// after is how long after start the limit is reached.
		after   time.Duration
		windows []string
		until   time.Time
		want    LimitReached
	}{
		{
			name:    "reached",
			windows: []string{"7d_oi"}, until: start.Add(time.Hour),
			want: LimitReached{Account: "work", Windows: []string{"7d_oi"}, Until: start.Add(time.Hour)},
		},
		{
			name:  "reached again in another window, until sooner",
			after: 10 * time.Minute, windows: []string{"5h"}, until: start.Add(30 * time.Minute),
			want: LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(30 * time.Minute), Again: true},
		},
		{
			name:  "reached again in no window named, until it doesn't say",
			after: 20 * time.Minute,
			want:  LimitReached{Account: "work", Until: start.Add(25 * time.Minute), Again: true},
		},
		{
			name:  "reached again, until it still doesn't say, which extends it",
			after: 24 * time.Minute,
			want:  LimitReached{Account: "work", Until: start.Add(29 * time.Minute), Again: true},
		},
		{
			name:  "reached once it has lifted",
			after: 30 * time.Minute, windows: []string{"5h"}, until: start.Add(3 * time.Hour),
			want: LimitReached{Account: "work", Windows: []string{"5h"}, Until: start.Add(3 * time.Hour)},
		},
	}
	for _, step := range steps {
		clock.now = start.Add(step.after)
		if got := s.limit("work", step.windows, step.until); !reflect.DeepEqual(got, step.want) {
			t.Errorf("%s: limit() = %+v, want %+v", step.name, got, step.want)
		}
	}
}

func TestALimitReachedAgainInAnotherWindowHoldsAsItsLatestAnswerSays(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week, fableWeek}, s.mark())
	s.learn(fable, []quota.Window{session, week, fableWeek})
	// A Fable request reaches the Fable week's limit, for three days; then a
	// Haiku request, which that doesn't hold back, reaches the session's,
	// which resets in two hours.
	s.limit("work", []string{"7d_oi"}, start.Add(3*24*time.Hour))
	clock.now = start.Add(time.Hour)
	s.limit("work", []string{"5h"}, start.Add(3*time.Hour))

	if !s.view(haiku, start.Add(3*time.Hour)).room("work") {
		t.Error("once the session resets, work has no room for Haiku, want room: the Fable week never held Haiku back")
	}
}

func TestALimitLiftedEarlyIsReachedAfresh(t *testing.T) {
	clock := &testClock{now: start}
	s := newTestState(clock)
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	fresh := session
	fresh.Utilization, fresh.ResetsAt = 0.01, session.ResetsAt.Add(5*time.Hour)
	s.record("work", []quota.Window{spent, week}, s.mark())
	s.limit("work", []string{"5h"}, start.Add(24*time.Hour))
	clock.now = start.Add(time.Minute)
	s.record("work", []quota.Window{fresh, week}, s.mark())

	if got := s.limit("work", []string{"7d"}, week.ResetsAt); got.Again {
		t.Errorf("limit() = %+v, want a limit reached afresh: the last lifted early", got)
	}
}

func TestAViewWithoutAccounts(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	s := newTestState(&testClock{now: start})
	s.record("work", []quota.Window{spent, week}, s.mark())
	s.record("side", []quota.Window{session, week}, s.mark())

	v := s.view(opus, start)
	if got := v.full(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("full() = %q, want work, whose session is spent", got)
	}
	without := v.without([]string{"side"})
	if without.room("side") || !without.has("side") {
		t.Error("without side, side has room, or can't be gone out on, want neither")
	}
	if c, ok := without.pick(""); ok {
		t.Errorf("without side, pick() = %q, want none: work has no room", c.ID)
	}
	if got := without.without([]string{"work"}).full(); len(got) > 0 {
		t.Errorf("without either, full() = %q, want none: a probe finding room on either would be no use", got)
	}
	if !v.room("side") {
		t.Error("the view side was taken from has lost it, want it as it was")
	}
}

func TestDueAgain(t *testing.T) {
	tests := []struct {
		name string
		// read and probed are how long before start the account was last
		// read and probed, or never when zero.
		read, probed time.Duration
		want         bool
	}{
		{name: "never read or probed", want: true},
		{name: "read a minute ago", read: time.Minute, want: true},
		{name: "read under a minute ago", read: 59 * time.Second, want: false},
		{name: "probed a minute ago", read: time.Hour, probed: time.Minute, want: true},
		{name: "probed under a minute ago", read: time.Hour, probed: 59 * time.Second, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{}
			s := newTestState(clock)
			if tt.read > 0 {
				clock.now = start.Add(-tt.read)
				s.record("work", []quota.Window{session, week}, s.mark())
			}
			if tt.probed > 0 {
				clock.now = start.Add(-tt.probed)
				s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"), s.mark(), fromProbe)
			}

			if got := s.dueAgain("work", start); got != tt.want {
				t.Errorf("dueAgain() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDue(t *testing.T) {
	tests := []struct {
		name string
		// read and probed are how long before start the account was last
		// read and probed, or never when zero.
		read, probed time.Duration
		// failed says whether that probe failed.
		failed bool
		want   bool
	}{
		{name: "never read", want: true},
		{name: "read 15 minutes ago", read: 15 * time.Minute, want: false},
		{name: "read over 15 minutes ago", read: 15*time.Minute + time.Second, want: true},
		{name: "probed and read over 15 minutes ago", read: 16 * time.Minute, probed: 16 * time.Minute, want: true},
		{name: "never read, and a probe failed under a minute ago", probed: 59 * time.Second, failed: true, want: false},
		{name: "never read, and a probe failed a minute ago", probed: time.Minute, failed: true, want: true},
		{name: "read an hour ago, and a probe failed under a minute ago", read: time.Hour, probed: 30 * time.Second, failed: true, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{}
			s := newTestState(clock)
			if tt.read > 0 {
				clock.now = start.Add(-tt.read)
				s.record("work", []quota.Window{session, week}, s.mark())
			}
			if tt.probed > 0 {
				clock.now = start.Add(-tt.probed)
				var err error
				if tt.failed {
					err = errors.New("HTTP 529 · Overloaded")
				}
				s.recordProbe("work", quota.Probe{}, err, s.mark(), fromProbe)
			}

			if got := s.due("work", start); got != tt.want {
				t.Errorf("due() = %v, want %v", got, tt.want)
			}
		})
	}
}
