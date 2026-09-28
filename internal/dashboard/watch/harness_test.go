package watch

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// start is when every test's clock starts: a Monday, 13:12 an hour east of UTC.
var start = time.Date(2026, 9, 28, 13, 12, 0, 0, time.FixedZone("UTC+1", 60*60))

// policy judges room as Claude's windows are judged: the session and the week
// apply to every model.
var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d"}

// interval is how often the tests' model reads when nothing brings a read on.
const interval = 30 * time.Minute

const day = 24 * time.Hour

// harness drives a model as Bubble Tea would, on a clock the test moves. It
// runs the commands the model returns, and holds each timer the model arms
// until the test fires it.
type harness struct {
	t        *testing.T
	model    Model
	clock    *fakeClock
	source   *fakeSource
	notifier *fakeNotifier
	timers   []*timer
}

// timer is one the model armed: msg is due after delay, at due.
type timer struct {
	delay time.Duration
	due   time.Time
	msg   tea.Msg
	fired bool
}

// newHarness is a model of a source that reads doc, in a terminal 150 cells
// wide and 50 lines tall. It hasn't started.
func newHarness(t *testing.T, doc status.Document) *harness {
	t.Helper()
	h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: doc}, notifier: &fakeNotifier{}}
	h.model = New(t.Context(), Config{Source: h.source, Notifier: h.notifier, Now: h.clock.Now, Interval: interval, Policy: policy})
	h.model.after = h.arm
	h.update(tea.WindowSizeMsg{Width: 150, Height: 50})
	return h
}

// arm records a timer instead of starting one.
func (h *harness) arm(d time.Duration, msg tea.Msg) tea.Cmd {
	h.timers = append(h.timers, &timer{delay: d, due: h.clock.now.Add(d), msg: msg})
	return nil
}

// init runs the model's first commands, returning what they send back
// undelivered: the first read.
func (h *harness) init() []tea.Msg {
	return h.run(h.model.Init())
}

// start starts the model and delivers its first read.
func (h *harness) start() {
	h.t.Helper()
	h.deliver(h.init()...)
}

// update gives the model msg, and returns what the commands it returns send
// back, undelivered.
func (h *harness) update(msg tea.Msg) []tea.Msg {
	h.t.Helper()
	next, cmd := h.model.Update(msg)
	h.model = next.(Model)
	return h.run(cmd)
}

// deliver gives the model msgs, and whatever they send back, until they're
// all delivered.
func (h *harness) deliver(msgs ...tea.Msg) {
	h.t.Helper()
	for len(msgs) > 0 {
		msgs = append(msgs[1:], h.update(msgs[0])...)
	}
}

// run runs cmd and every command it batches, returning what they send back.
func (h *harness) run(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case nil:
		return nil
	case tea.BatchMsg:
		var msgs []tea.Msg
		for _, c := range msg {
			msgs = append(msgs, h.run(c)...)
		}
		return msgs
	default:
		return []tea.Msg{msg}
	}
}

// press presses key, returning what the model sends back, undelivered.
func (h *harness) press(key string) []tea.Msg {
	h.t.Helper()
	if key == "ctrl+c" {
		return h.update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	}
	return h.update(tea.KeyPressMsg{Code: []rune(key)[0], Text: key})
}

// read has the source read doc from now on, and reads it with r.
func (h *harness) read(doc status.Document) {
	h.t.Helper()
	h.source.doc = doc
	h.deliver(h.press("r")...)
}

// fire moves the clock to when tm is due, and delivers it.
func (h *harness) fire(tm *timer) {
	h.t.Helper()
	h.clock.now = tm.due
	tm.fired = true
	h.deliver(tm.msg)
}

// lastTick is the tick armed last, which is the live chain's.
func (h *harness) lastTick() *timer {
	h.t.Helper()
	for _, tm := range slices.Backward(h.timers) {
		if _, ok := tm.msg.(tickMsg); ok {
			return tm
		}
	}
	h.t.Fatal("no tick armed")
	return nil
}

// pendingFrame is the frame armed and not yet fired, if any.
func (h *harness) pendingFrame() (*timer, bool) {
	for _, tm := range h.timers {
		if _, ok := tm.msg.(frameMsg); ok && !tm.fired {
			return tm, true
		}
	}
	return nil, false
}

// settle fires frames until the bars have eased to their readings.
func (h *harness) settle() {
	h.t.Helper()
	for tm, ok := h.pendingFrame(); ok; tm, ok = h.pendingFrame() {
		h.fire(tm)
	}
}

// tickUntilRead fires the live chain's ticks until one reads the source, and
// returns the time it did.
func (h *harness) tickUntilRead() time.Time {
	h.t.Helper()
	reads := h.source.reads
	for range 1000 {
		h.fire(h.lastTick())
		if h.source.reads > reads {
			return h.clock.now
		}
	}
	h.t.Fatal("no tick read the source")
	return time.Time{}
}

// view is the screen as drawn, without its escapes.
func (h *harness) view() string {
	return ansi.Strip(h.model.View().Content)
}

// footer is the screen's last line, without its margin.
func (h *harness) footer() string {
	lines := strings.Split(h.view(), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// fakeClock tells the time the test sets.
type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}

// fakeSource answers every read with doc, or with err when it's set, and
// counts the reads.
type fakeSource struct {
	doc   status.Document
	err   error
	reads int
}

func (s *fakeSource) Fetch(context.Context) (status.Document, error) {
	s.reads++
	return s.doc, s.err
}

// fakeNotifier records what it's asked to post, and fails when err is set.
type fakeNotifier struct {
	posted []string
	err    error
}

func (n *fakeNotifier) Notify(message string) error {
	n.posted = append(n.posted, message)
	return n.err
}

// window is a window resetting a duration after the clock starts.
func window(key, label string, utilization float64, resetsIn time.Duration) quota.Window {
	return quota.Window{Key: key, Label: label, Utilization: utilization, ResetsAt: start.Add(resetsIn).UTC()}
}

func session(utilization float64, resetsIn time.Duration) quota.Window {
	return window("5h", "Session", utilization, resetsIn)
}

func week(utilization float64) quota.Window {
	return window("7d", "Week", utilization, 3*day)
}

func fableWeek(utilization float64) quota.Window {
	return window("7d_oi", "Fable week", utilization, 4*day)
}

func refused(w quota.Window) quota.Window {
	w.Status = quota.StatusRejected
	return w
}

// account is an account whose windows were read.
func account(id, label string, windows ...quota.Window) status.Account {
	return status.Account{ID: id, Label: label, TokenSet: true, FetchedAt: start.UTC(), Windows: windows}
}

// unreadable is an account whose token the API refused.
func unreadable(id, label string) status.Account {
	return status.Account{ID: id, Label: label, TokenSet: true, Error: "HTTP 401 · Invalid bearer token"}
}

// partlyRead is an account whose Fable window couldn't be read.
func partlyRead(id, label string) status.Account {
	return status.Account{
		ID: id, Label: label, TokenSet: true, FetchedAt: start.UTC(),
		Windows:  []quota.Window{session(0.25, 3*time.Hour), week(0.5)},
		Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
	}
}

func document(accounts ...status.Account) status.Document {
	return status.Document{GeneratedAt: start.UTC(), Source: status.SourceProbe, Accounts: accounts}
}

// calm is one account with room, whose windows reset long after the interval.
func calm() status.Document {
	return document(account("work", "Work", session(0.25, 3*time.Hour), week(0.5)))
}

// at is a time on the day the clock starts.
func at(hour, minute, second int) time.Time {
	return time.Date(start.Year(), start.Month(), start.Day(), hour, minute, second, 0, start.Location())
}
