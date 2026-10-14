package capture

import (
	"context"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// tape is a scenario played in a test as a tape plays it: on its clock, the
// watch's timers firing as the clock passes them, keys pressed at their
// times, and its request stream held, as the watch's own tests hold it.
type tape struct {
	t      *testing.T
	base   *base
	clock  *testClock
	l      live
	timers []timer
	queued int
}

// timer is a watch's timer, as a tape keeps it: when it's due, and what it
// delivers then; seq orders those due together as they were set.
type timer struct {
	due time.Time
	seq int
	msg tea.Msg
}

// played is the scenario given played as a tape plays it, from its start.
func played(t *testing.T, s Scenario) *tape {
	t.Helper()
	b := s.based()
	p := &tape{t: t, base: b, clock: &testClock{now: b.start}}
	p.l = s.play(t.Context(), b, p.clock.since(b.start), stillWaiting, p.after, heldStream)
	p.l = drive(p.l, p.l.Init())
	return p
}

// stillWaiting waits on a stream's next event till ctx ends, as a tape's
// stream is held.
func stillWaiting(ctx context.Context, _ time.Duration) bool {
	<-ctx.Done()
	return false
}

// after is the watch's timers as a tape keeps them: msg delivered once d
// has passed on its clock.
func (p *tape) after(d time.Duration, msg tea.Msg) tea.Cmd {
	return func() tea.Msg {
		p.queued++
		p.timers = append(p.timers, timer{due: p.clock.read().Add(d), seq: p.queued, msg: msg})
		return nil
	}
}

// until plays the tape to the given seconds into the scenario, each timer
// firing in turn as its time comes.
func (p *tape) until(seconds float64) {
	p.t.Helper()
	end := p.base.start.Add(offset(seconds))
	for fired := 0; ; fired++ {
		if fired > 100_000 {
			p.t.Fatal("the watch's timers fire without end")
		}
		next := slices.MinFunc(append([]timer{{due: end.Add(time.Nanosecond)}}, p.timers...), func(a, b timer) int {
			if c := a.due.Compare(b.due); c != 0 {
				return c
			}
			return a.seq - b.seq
		})
		if next.due.After(end) {
			break
		}
		p.timers = slices.DeleteFunc(p.timers, func(tm timer) bool { return tm.seq == next.seq })
		p.clock.set(next.due)
		p.l = drive(p.l, nil, next.msg)
	}
	p.clock.set(end)
}

// press plays the tape to the given seconds, and presses the keys there.
func (p *tape) press(seconds float64, keys ...tea.KeyPressMsg) {
	p.t.Helper()
	p.until(seconds)
	for _, k := range keys {
		p.l = drive(p.l, nil, k)
	}
}

// screen is what the tape shows, as text alone.
func (p *tape) screen() string {
	return ansi.Strip(p.l.View().Content)
}
