package capture

import (
	"context"
	"image/color"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// interval is how often a watch reads in full when dashboard isn't told
// otherwise.
const interval = 30 * time.Minute

// framesTraceScale is the requests a minute that fill a trace's height in the
// frames, which pin it, where the live page scales as its rule says.
const framesTraceScale = 16

// framesWeekStarts is the day the frames' calendar weeks start on, as their
// worlds' config sets it.
const framesWeekStarts = time.Monday

// Frame draws the fixture as a terminal of the given size shows it: a line
// for each of its rows, styled as the dashboard draws them, each ending in a
// newline. A printout is drawn as usage prints it instead.
func (f Fixture) Frame(size watch.Size) string {
	if f.Printout {
		return f.printout(size.Width)
	}
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
// builds it for dashboard, on a terminal of the given size, and brings it to
// the fixture's moment: its first read lands readAgo before, the terminal
// says what its background is, then the clock reads the moment, which the
// screen is drawn at, and the fixture's keys are pressed. A router that
// stops answering before the moment is read last readAgo before it goes,
// and found gone as it goes, by R. A fixture with keys pressed before its
// moment is read first readAgo before those, which are pressed then; then
// at its moment, read again, by R, so what its ledger has gained since is
// read. Its timers never fire, so nothing moves from there but what a key
// does.
func (f Fixture) settle(size watch.Size) watch.Model {
	gone := f.router.gone
	read := !gone.IsZero() && gone.Before(f.now)
	clock := &clock{now: ago(f.now, readAgo+f.before.by)}
	if read {
		clock.now = ago(gone, readAgo)
	}
	m := watch.New(context.Background(), f.config(size, clock.read))
	m = deliver(m, run(m.Init())...)
	if !f.colourless {
		m = deliver(m, tea.BackgroundColorMsg{Color: terminalBackground})
	}
	if read {
		clock.now = gone
		m = deliver(m, keysOf("R")[0])
	}
	if len(f.before.keys) > 0 {
		clock.now = ago(f.now, f.before.by)
		m = pressed(deliver(m, reached{}), f.before.keys)
		clock.now = f.now
		m = deliver(m, keysOf("R")[0])
	}
	clock.now = f.now
	return pressed(deliver(m, reached{}), f.keys)
}

// pressed is the model with the keys given pressed, in turn.
func pressed(m watch.Model, keys []tea.KeyPressMsg) watch.Model {
	for _, key := range keys {
		m = deliver(m, key)
	}
	return m
}

// config configures the watch of the fixture on a terminal of the given size,
// by now's clock, as dashboard configures it, its seams the fixture's.
func (f Fixture) config(size watch.Size, now func() time.Time) watch.Config {
	source := f.router.by(now)
	cfg := watch.Config{
		Source:     source,
		Notifier:   quiet{},
		Now:        now,
		After:      held,
		Await:      source.await,
		Interval:   interval,
		Policy:     claude.Policy,
		WindowName: status.NameOfKey,
		Accounts:   f.accounts,
		WeekStarts: framesWeekStarts,
		WeekWindow: claude.WeekWindow,
		Family:     claude.Provider{}.Family,
		Size:       size,
		View:       f.view,
		Prices:     drawnPrices,
		TraceScale: framesTraceScale,
	}
	cfg.Ledger, cfg.Readings, cfg.Events = f.readers(now)
	dressed(&cfg, f.theme, f.colourless)
	return cfg
}

// readers are what the watch of the fixture reads the request ledger, the
// readings history and the router's events through, by now's clock: its
// world's records, or, of a fixture over none, as milestone 5's are, none.
func (f Fixture) readers(now func() time.Time) (views.Ledger, views.Readings, views.Events) {
	if f.records == nil {
		return ledger.NewEmpty(now, ledger.Caps{Shared: claude.SharedWindows}), readings.Empty{}, events.Empty{}
	}
	return f.records.readers(now)
}

// dressed has the watch cfg configures drawn in t, as the one theme chosen,
// its themes a capture's; or, colourless, without colour, as NO_COLOR asks.
func dressed(cfg *watch.Config, t theme.Theme, colourless bool) {
	if colourless {
		return
	}
	cfg.Choice, cfg.Pair = theme.One(t.Slug), theme.Pair{Light: t, Dark: t}
	cfg.Themes = &themes{listing: listing(t), choice: cfg.Choice}
}

// terminalBackground is what the terminal a tape runs says its background
// is: the frames' canvas, Nord's.
var terminalBackground = color.RGBA{R: 0x2E, G: 0x34, B: 0x40, A: 0xff}

// themes are a capture's themes, as the theme picker takes them: listed,
// never looked for in the themes directory, and a choice made among them
// kept for the capture alone, never written.
type themes struct {
	listing theme.Listing
	mu      sync.Mutex
	choice  theme.Choice
}

// List lists the capture's themes.
func (t *themes) List() theme.Listing {
	return t.listing
}

// Chosen is the choice kept for the capture.
func (t *themes) Chosen() theme.Choice {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.choice
}

// Keep changes the choice kept for the capture, as a capture writes nothing.
func (t *themes) Keep(change func(theme.Choice) theme.Choice) (theme.Choice, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.choice = change(t.choice)
	return t.choice, nil
}

// listing lists the built-ins, and t where it isn't one, as when the
// capture tool is given a theme's file, as the themes directory lists a
// file: taking a built-in's slug, it's never to be picked.
func listing(t theme.Theme) theme.Listing {
	var l theme.Listing
	for _, b := range theme.Builtins() {
		l.Entries = append(l.Entries, theme.Entry{Name: b.Slug, Slug: b.Slug, Theme: b})
	}
	if b, ok := theme.Builtin(t.Slug); !ok || b != t {
		l.Entries = append(l.Entries, theme.FileEntry(t))
	}
	return l
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

// reached is the clock reaching a fixture's moment: the watch takes it in as
// any message, doing nothing with it but drawing its screen there afresh,
// as a tick would.
type reached struct{}

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
