package router

import (
	"errors"
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
			learn: func(s *state) { s.recordProbe("work", probed(everyFamily, windows...), nil) },
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
			learn: func(s *state) { s.recordProbe("side", probed(everyFamily, windows...), nil) },
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
			s.record("work", windows, fromResponse)

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
	s.record("work", []quota.Window{spent, week}, fromResponse)

	s.learn(fable, []quota.Window{spent, week})
	if s.view(haiku, start).room("work") {
		t.Error("work has room for Haiku, want none: its session is spent, and every model shares it")
	}
}

func TestView(t *testing.T) {
	s := newTestState(&testClock{now: start})
	s.record("side", []quota.Window{week, session}, fromResponse)

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
	clock := &testClock{now: start}
	s := newTestState(clock)
	s.record("work", []quota.Window{session, week}, fromResponse)
	s.refuse("work")

	for after, want := range map[time.Duration]bool{0: false, 10*time.Minute - time.Nanosecond: false, 10 * time.Minute: true} {
		v := s.view(opus, start.Add(after))
		if got := v.room("work"); got != want {
			t.Errorf("%v after its refusal, room(work) = %v, want %v", after, got, want)
		}
		if picked, _ := v.pick(""); (picked == "work") != want {
			t.Errorf("%v after its refusal, pick() = %q, want work: %v", after, picked, want)
		}
	}
}

func TestAViewWithoutAccounts(t *testing.T) {
	spent := session
	spent.Utilization, spent.Status = 1, quota.StatusRejected
	s := newTestState(&testClock{now: start})
	s.record("work", []quota.Window{spent, week}, fromResponse)
	s.record("side", []quota.Window{session, week}, fromResponse)

	v := s.view(opus, start)
	if got := v.full(); !slices.Equal(got, []string{"work"}) {
		t.Errorf("full() = %q, want work, whose session is spent", got)
	}
	without := v.without([]string{"side"})
	if without.room("side") || !without.has("side") {
		t.Error("without side, side has room, or can't be gone out on, want neither")
	}
	if id, ok := without.pick(""); ok {
		t.Errorf("without side, pick() = %q, want none: work has no room", id)
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
				s.record("work", []quota.Window{session, week}, fromResponse)
			}
			if tt.probed > 0 {
				clock.now = start.Add(-tt.probed)
				s.recordProbe("work", quota.Probe{}, errors.New("HTTP 529 · Overloaded"))
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
				s.record("work", []quota.Window{session, week}, fromResponse)
			}
			if tt.probed > 0 {
				clock.now = start.Add(-tt.probed)
				var err error
				if tt.failed {
					err = errors.New("HTTP 529 · Overloaded")
				}
				s.recordProbe("work", quota.Probe{}, err)
			}

			if got := s.due("work", start); got != tt.want {
				t.Errorf("due() = %v, want %v", got, tt.want)
			}
		})
	}
}
