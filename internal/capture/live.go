package capture

import (
	"context"
	"image/color"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// Model is the scenario as Bubble Tea plays it, full screen in a terminal of
// whatever size, in real time from now: its clock reading the scenario's
// start as it starts, and running on from there; its router telling of what
// its cues have it do as each comes, and taking the orders keys give; the
// watch's timers running, so bars ease, pulses travel the cords, sand falls
// and highlights fade; and each key shown as it's pressed, so a recording
// shows what was typed.
func (s Scenario) Model() tea.Model {
	began := time.Now()
	return s.play(func() time.Duration { return time.Since(began) }, waitFor, nil)
}

// play is the scenario as Bubble Tea plays it, elapsed saying how long it has
// been playing, wait waiting for a while to pass as it plays, and after the
// watch's timers: nil runs them on the clock.
func (s Scenario) play(elapsed func() time.Duration, wait func(context.Context, time.Duration) bool, after func(time.Duration, tea.Msg) tea.Cmd) live {
	now := func() time.Time { return s.start.Add(elapsed()) }
	t := &timeline{scenario: s, now: now, wait: wait}
	cfg := watch.Config{
		Source:   t,
		Notifier: quiet{},
		Now:      now,
		After:    after,
		Interval: interval,
		Policy:   claude.Policy,
		Size:     s.Size,
		Ledger:   ledger.NewEmpty(now, ledger.Caps{Shared: claude.SharedWindows}),
		Readings: readings.Empty{},
		Events:   events.Empty{},
	}
	dressed(&cfg, s.theme, s.colourless)
	return live{Model: watch.New(context.Background(), cfg), now: now, after: after, theme: s.theme, coloured: !s.colourless}
}

// waitFor waits for d to pass, reporting false where ctx ends first.
func waitFor(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(max(d, 0))
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// timeline is the router a scenario plays as, in real time: the watch's
// Source, answering each read as the scenario has played up to then, its
// request stream telling of each event as its time comes, and the orders
// keys give played in from when they're given. It's safe for concurrent use.
type timeline struct {
	scenario Scenario
	// now reads the scenario's clock, and wait waits for a while to pass on
	// it, reporting false where the context given ends first.
	now  func() time.Time
	wait func(context.Context, time.Duration) bool

	mu     sync.Mutex
	orders []order
}

// moment is the scenario's clock and the orders given by then, read together
// under mu, as give reads the clock as it adds an order: so a play of the
// scenario as of then has every order given before it, and none after, and
// what the stream tells is never told without an order given before it.
func (t *timeline) moment() (time.Time, []order) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.now(), slices.Clone(t.orders)
}

// at is the router as the scenario has it now.
func (t *timeline) at() source {
	now, orders := t.moment()
	return t.scenario.played(orders, now).world.source(now)
}

// Read reads the router's document as it stands now, whatever the read asks.
func (t *timeline) Read(ctx context.Context, r status.Read) (status.Document, router.Health, error) {
	return t.at().Read(ctx, r)
}

// Sessions lists the sessions as the router has them now.
func (t *timeline) Sessions(ctx context.Context) ([]status.Session, error) {
	return t.at().Sessions(ctx)
}

// History answers as GET /history does, now.
func (t *timeline) History(ctx context.Context, window string, step time.Duration) (router.History, error) {
	return t.at().History(ctx, window, step)
}

// RouterAnswers reports that the router answers, as it always does.
func (t *timeline) RouterAnswers(context.Context) bool {
	return true
}

// Pin has new sessions go to the best of the accounts given from now, which
// a pin that moves running sessions moves them to on their next requests.
func (t *timeline) Pin(_ context.Context, accounts []string, move bool) error {
	t.give(func(w *world, at time.Time) {
		w.pin = status.Pin{Accounts: slices.Clone(accounts), Since: at, Move: move}
	})
	return nil
}

// Unpin has every session routed on its merits from now.
func (t *timeline) Unpin(context.Context) error {
	t.give(func(w *world, _ time.Time) { w.pin = status.Pin{} })
	return nil
}

// PinSession gives the session its own pin to the account from now, which
// moves it there on its next request.
func (t *timeline) PinSession(_ context.Context, session, account string) error {
	t.give(func(w *world, at time.Time) {
		if w.pins == nil {
			w.pins = make(map[string]ownPin)
		}
		w.pins[session] = ownPin{account: account, at: at}
	})
	return nil
}

// UnpinSession clears the session's own pin from now.
func (t *timeline) UnpinSession(_ context.Context, session string) error {
	t.give(func(w *world, _ time.Time) { delete(w.pins, session) })
	return nil
}

// give has the router take an order now, as change says, the clock read with
// mu held, as moment reads it: each play of the scenario from then on plays
// it in as it comes, into a world of its own.
func (t *timeline) give(change func(w *world, at time.Time)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	at := t.now()
	t.orders = append(t.orders, order{at: at, give: func(w *world) { change(w, at) }})
}

// Stream opens the router's request stream, as GET /stream does: as it opens
// it tells of the requests in flight, as the router keeps them, then of each
// event as its time comes, until ctx ends, when it closes.
func (t *timeline) Stream(ctx context.Context) (<-chan router.StreamEvent, error) {
	joined, orders := t.moment()
	inFlight := flying(t.scenario.played(orders, time.Time{}).told, joined)
	events := make(chan router.StreamEvent, len(inFlight))
	for _, e := range inFlight {
		events <- e
	}
	go t.feed(ctx, events, joined)
	return events, nil
}

// feed tells of each event after from on events as its time comes, the
// scenario played afresh with the orders given by then, so a request a pin
// moves goes where the pin sends it; and once ctx ends, closes events.
func (t *timeline) feed(ctx context.Context, events chan<- router.StreamEvent, from time.Time) {
	defer close(events)
	for {
		now, orders := t.moment()
		told := t.scenario.played(orders, time.Time{}).told
		next := slices.IndexFunc(told, func(e router.StreamEvent) bool { return e.At.After(from) })
		if next < 0 {
			<-ctx.Done()
			return
		}
		if !t.wait(ctx, told[next].At.Sub(now)) {
			return
		}
		now, orders = t.moment()
		for _, e := range t.scenario.played(orders, time.Time{}).told {
			if !e.At.After(from) || e.At.After(now) {
				continue
			}
			select {
			case events <- e:
			case <-ctx.Done():
				return
			}
		}
		from = now
	}
}

// live is a watch playing a scenario, as Bubble Tea runs it, and the keys
// pressed lately, drawn over its footer, so a recording shows what was typed.
type live struct {
	watch.Model
	now   func() time.Time
	after func(time.Duration, tea.Msg) tea.Cmd
	// theme is the theme the scenario is drawn in, and coloured whether it's
	// drawn in colour.
	theme    theme.Theme
	coloured bool
	typed    typed
}

// typedFor is how long a key pressed is shown, from when the last was.
const typedFor = 1500 * time.Millisecond

// typed are the keys pressed lately, as they're shown: the last few, while
// each was pressed within typedFor of the next, and when the last was.
type typed struct {
	keys []string
	last time.Time
}

// mostTyped is how many of the keys pressed lately are shown.
const mostTyped = 4

// typedMsg wakes the model to redraw once the keys typed have had their time.
type typedMsg struct{}

// Init starts the watch, as usage -w does, and in colour, answers its
// question of the terminal's background at once, as a tape's terminal would,
// so it's drawn from the start.
func (l live) Init() tea.Cmd {
	cmds := []tea.Cmd{l.Model.Init()}
	if l.coloured {
		cmds = append(cmds, func() tea.Msg { return tea.BackgroundColorMsg{Color: terminalBackground} })
	}
	return tea.Batch(cmds...)
}

// Update takes in a message as the watch does, noting a key pressed to show.
func (l live) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var lapse tea.Cmd
	if key, ok := msg.(tea.KeyPressMsg); ok {
		l.typed = l.typed.pressed(keyName(key), l.now())
		lapse = l.wake(typedFor)
	}
	next, cmd := l.Model.Update(msg)
	l.Model = next.(watch.Model)
	return l, tea.Batch(cmd, lapse)
}

// wake has the model woken to redraw once d has passed.
func (l live) wake(d time.Duration) tea.Cmd {
	if l.after != nil {
		return l.after(d, typedMsg{})
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return typedMsg{} })
}

// View draws the watch, and over the right of its last row, the keys pressed
// lately, while they're shown.
func (l live) View() tea.View {
	v := l.Model.View()
	keys := l.typed.shown(l.now())
	if len(keys) == 0 || v.Content == "" {
		return v
	}
	lines := strings.Split(v.Content, "\n")
	last := len(lines) - 1
	lines[last] = l.keycaps(lines[last], keys, v.BackgroundColor)
	v.SetContent(strings.Join(lines, "\n"))
	return v
}

// pressed are the keys typed once key is pressed at now: those pressed
// within typedFor of it before, the last few, then it.
func (t typed) pressed(key string, now time.Time) typed {
	keys := []string{key}
	if now.Sub(t.last) < typedFor {
		keys = append(slices.Clone(t.keys), key)
	}
	return typed{keys: keys[max(len(keys)-mostTyped, 0):], last: now}
}

// shown are the keys typed as they're shown at now: none once they've had
// their time.
func (t typed) shown(now time.Time) []string {
	if now.Sub(t.last) >= typedFor {
		return nil
	}
	return t.keys
}

// keyName is a key as the dashboard's footer names it.
func keyName(key tea.KeyPressMsg) string {
	name := key.String()
	switch name {
	case "up":
		return "↑"
	case "down":
		return "↓"
	case "left":
		return "←"
	case "right":
		return "→"
	case "enter":
		return "⏎"
	case "shift+tab":
		return "shift-tab"
	case "pgup":
		return "PgUp"
	case "pgdown":
		return "PgDn"
	}
	return name
}

// keycaps draws keys as keycaps over the right of line, the screen's last
// row, a cell apart, and a cell in from its edge, clearing what of the footer
// they'd cut into back to the two spaces that set its groups apart: in the
// theme the screen is drawn in, as its canvas says, or where it paints none,
// in reverse.
func (l live) keycaps(line string, keys []string, canvas color.Color) string {
	capStyle, gapStyle := lipgloss.NewStyle().Bold(true).Reverse(true), lipgloss.NewStyle()
	if t, ok := l.showing(canvas); ok {
		capStyle = lipgloss.NewStyle().Bold(true).Foreground(t.Colour(theme.Canvas)).Background(t.Colour(theme.AccentKey))
		gapStyle = lipgloss.NewStyle().Background(t.Colour(theme.Canvas))
	}
	var caps strings.Builder
	for _, key := range keys {
		caps.WriteString(capStyle.Render(" " + key + " "))
		caps.WriteString(gapStyle.Render(" "))
	}
	width, capsWidth := ansi.StringWidth(line), ansi.StringWidth(caps.String())
	keep := width - capsWidth
	if keep < 0 {
		return line
	}
	plain := ansi.Strip(line)
	for keep > 1 && ansi.Cut(plain, keep-2, keep) != "  " {
		keep--
	}
	return ansi.Truncate(line, keep, "") + ansi.ResetStyle + gapStyle.Render(strings.Repeat(" ", width-capsWidth-keep)) + caps.String()
}

// showing is the theme the screen is drawn in, as its canvas says: the
// scenario's, or the built-in the theme picker shows, whose canvas it is;
// reporting false for a screen that paints none.
func (l live) showing(canvas color.Color) (theme.Theme, bool) {
	if canvas == nil || !l.coloured {
		return theme.Theme{}, false
	}
	for _, t := range append([]theme.Theme{l.theme}, theme.Builtins()...) {
		if c := t.Colour(theme.Canvas); c != nil && rgbOf(c) == rgbOf(canvas) {
			return t, true
		}
	}
	return theme.Theme{}, false
}

// rgbOf is a colour's red, green and blue, for telling two colours apart.
func rgbOf(c color.Color) [3]uint32 {
	r, g, b, _ := c.RGBA()
	return [3]uint32{r >> 8, g >> 8, b >> 8}
}
