package capture

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// interval is how often a watch reads in full when usage -w isn't told
// otherwise.
const interval = 30 * time.Minute

// Frame draws the fixture as a terminal of the given size shows it: a line
// for each of its rows, styled as the dashboard draws them, each ending in a
// newline.
func (f Fixture) Frame(size watch.Size) string {
	lines := strings.Split(f.settle(size).View().Content, "\n")
	lines = lines[:min(len(lines), size.Height)]
	for len(lines) < size.Height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n") + "\n"
}

// Model is the fixture as Bubble Tea runs it, full screen in a terminal of
// whatever size: brought to the fixture's moment, its first read taken, so
// starting it reads nothing more, and from there it takes keys as the
// dashboard does.
func (f Fixture) Model() tea.Model {
	return settled{f.settle(f.Size)}
}

// settle builds the dashboard's watch model of the fixture, as watch.New
// builds it for usage -w, on a terminal of the given size, and brings it to
// the fixture's moment: its first read lands readAgo before, then the clock
// reads the moment, and the fixture's keys are pressed. Its timers never
// fire, so nothing moves from there but what a key does.
func (f Fixture) settle(size watch.Size) watch.Model {
	clock := &clock{now: ago(f.now, readAgo)}
	m := watch.New(context.Background(), watch.Config{
		Source:   f.router,
		Notifier: quiet{},
		Now:      clock.read,
		After:    held,
		Interval: interval,
		Policy:   claude.Policy,
		Size:     size,
	})
	m = deliver(m, run(m.Init())...)
	clock.now = f.now
	for _, key := range f.keys {
		m = deliver(m, key)
	}
	return m
}

// deliver gives the model each message, and each that the commands it
// returns send back, until none is left, as Bubble Tea would, but at once.
func deliver(m watch.Model, msgs ...tea.Msg) watch.Model {
	for len(msgs) > 0 {
		next, cmd := m.Update(msgs[0])
		m = next.(watch.Model)
		msgs = append(msgs[1:], run(cmd)...)
	}
	return m
}

// run runs cmd, and every command it batches, returning what they send back.
// With the model's timers held, none of them waits.
func run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, run(c)...)
		}
		return msgs
	default:
		return []tea.Msg{msg}
	}
}

// held is the watch's timers as a capture keeps them: none fires, so the
// clock never moves on from the moment drawn, and nothing redraws itself.
func held(time.Duration, tea.Msg) tea.Cmd {
	return nil
}

// clock is a fixture's clock, which stands still where it's set.
type clock struct {
	now time.Time
}

// read reads the clock.
func (c *clock) read() time.Time {
	return c.now
}

// quiet posts nothing: a capture never notifies.
type quiet struct{}

// Notify posts nothing.
func (quiet) Notify(string) error {
	return nil
}

// settled is a watch model brought to its fixture's moment, as Bubble Tea
// runs it: starting it reads nothing, its first read taken already.
type settled struct {
	watch.Model
}

// Init starts nothing.
func (settled) Init() tea.Cmd {
	return nil
}

// Update takes in a message as the watch model does.
func (s settled) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := s.Model.Update(msg)
	return settled{next.(watch.Model)}, cmd
}
