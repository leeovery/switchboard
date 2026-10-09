package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/status"
)

func TestStartsByReading(t *testing.T) {
	h := newHarness(t, calm())

	if got, want := h.footer(), unreadKeys+" · reading usage…"; got != want {
		t.Errorf("before the first read, the footer is %q, want %q", got, want)
	}
	if view := h.view(); !strings.Contains(view, "SWITCHBOARD") || strings.Contains(view, "╭─ 1 Work ") {
		t.Errorf("before the first read, the screen is\n%s\nwant the title row, and no account", view)
	}
	read := h.init()
	if h.source.reads != 1 {
		t.Fatalf("starting read the source %d times, want once", h.source.reads)
	}
	h.deliver(read...)
	if got, want := h.footer(), probingKeys+" · read 0s ago · next 13:42"; got != want {
		t.Errorf("after the first read, the footer is %q, want %q", got, want)
	}
	if !strings.Contains(h.view(), "╭─ 1 Work ") {
		t.Errorf("after the first read, the screen is\n%s\nwant the account's card", h.view())
	}
}

func TestTimersAreRealWhenTheConfigGivesNone(t *testing.T) {
	m := New(t.Context(), Config{})

	if got := m.after(time.Millisecond, frameMsg{})(); got != (frameMsg{}) {
		t.Errorf("a timer of a model given no After delivered %#v, want the message it was armed with", got)
	}
}

func TestReadsAgainWhenDue(t *testing.T) {
	tests := []struct {
		name     string
		doc      status.Document
		wantNext string
		// wantRead is when the tick that reads again comes.
		wantRead time.Time
	}{
		{
			name:     "once the interval has passed",
			doc:      calm(),
			wantNext: "13:42",
			wantRead: at(13, 42, 0),
		},
		{
			name:     "a minute after a window resets",
			doc:      document(account("work", "Work", session(0.4, 10*time.Minute), week(0.5))),
			wantNext: "13:23",
			wantRead: at(13, 23, 0),
		},
		{
			name:     "soon after an account couldn't be read",
			doc:      document(account("work", "Work", session(0.25, 3*time.Hour), week(0.5)), unreadable("side", "Side")),
			wantNext: "13:14",
			wantRead: at(13, 14, 0),
		},
		{
			name:     "soon after a window couldn't be read",
			doc:      document(partlyRead("work", "Work")),
			wantNext: "13:14",
			wantRead: at(13, 14, 0),
		},
		{
			name:     "at the soonest of them",
			doc:      document(account("work", "Work", session(0.4, 30*time.Second), week(0.5)), unreadable("side", "Side")),
			wantNext: "13:13",
			wantRead: at(13, 13, 30),
		},
		{
			name:     "not for a reset already past",
			doc:      document(account("work", "Work", refused(session(1, -5*time.Minute)), week(0.5))),
			wantNext: "13:42",
			wantRead: at(13, 42, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.doc)
			h.start()

			if got, want := h.footer(), probingKeys+" · read 0s ago · next "+tt.wantNext; got != want {
				t.Errorf("footer = %q, want %q", got, want)
			}
			if got, want := h.tickUntilRead(), tt.wantRead.Add(tickSlack); !got.Equal(want) {
				t.Errorf("read again at %s, want %s", got.Format(time.StampMilli), want.Format(time.StampMilli))
			}
		})
	}
}

func TestBacksOffWhileReadsFail(t *testing.T) {
	// side's token is refused at every read, and its windows reset long after.
	refusing := document(account("work", "Work", session(0.25, 5*time.Hour), week(0.5)), unreadable("side", "Side"))
	h := newHarness(t, refusing)
	h.start() // The first failure, at 13:12.

	tests := []struct {
		name string
		// then sets what this read finds.
		then     func()
		wantRead time.Time
		wantNext string
	}{
		{name: "a second failure, two minutes on", then: func() {}, wantRead: at(13, 14, 0), wantNext: "13:18"},
		{name: "a third, four minutes on", then: func() {}, wantRead: at(13, 18, 0), wantNext: "13:26"},
		{name: "a fourth, the source's own, eight minutes on", then: func() { h.source.err = errors.New("connection refused") }, wantRead: at(13, 26, 0), wantNext: "13:42"},
		{name: "a fifth, sixteen minutes on", then: func() { h.source.err = nil }, wantRead: at(13, 42, 0), wantNext: "14:12"},
		{name: "a sixth, the interval on rather than 32 minutes", then: func() {}, wantRead: at(14, 12, 0), wantNext: "14:42"},
		{name: "a seventh, the interval on", then: func() {}, wantRead: at(14, 42, 0), wantNext: "15:12"},
		{name: "a complete read, the interval on", then: func() { h.source.doc = calm() }, wantRead: at(15, 12, 0), wantNext: "15:42"},
		{name: "a new first failure, the interval on", then: func() { h.source.doc = refusing }, wantRead: at(15, 42, 0), wantNext: "15:44"},
	}
	for _, tt := range tests {
		tt.then()
		if got, want := h.tickUntilRead(), tt.wantRead.Add(tickSlack); !got.Equal(want) {
			t.Fatalf("%s: read at %s, want %s", tt.name, got.Format(time.StampMilli), want.Format(time.StampMilli))
		}
		if got := h.footer(); !strings.HasSuffix(got, " · next "+tt.wantNext) {
			t.Errorf("%s: footer = %q, want the next read at %s", tt.name, got, tt.wantNext)
		}
	}
}

func TestResetOnScreenBringsOnAReadWhileReadsFail(t *testing.T) {
	// The session is back at 13:25. Reads fail from 13:15, so the third
	// failure, at 13:21, would wait eight minutes but for the reset.
	h := newHarness(t, document(account("work", "Work", refused(session(1, 13*time.Minute)), week(0.5))))
	h.start()
	h.source.err = errors.New("connection refused")

	for _, failedAt := range []time.Time{at(13, 15, 0), at(13, 17, 0), at(13, 21, 0)} {
		h.clock.now = failedAt
		h.read(calm())
	}
	if got, want := h.footer(), probingKeys+" · couldn't read usage: connection refused · next 13:26"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
}

func TestTicksReadOnlyOnceDue(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	h.clock.now = at(13, 41, 59)
	h.deliver(h.lastTick().msg)
	if h.source.reads != 1 {
		t.Errorf("a tick before the read is due read the source; reads = %d, want 1", h.source.reads)
	}
	h.clock.now = at(13, 42, 0)
	h.deliver(h.lastTick().msg)
	if h.source.reads != 2 {
		t.Errorf("a tick once the read is due didn't read the source; reads = %d, want 2", h.source.reads)
	}
}

func TestReadsOnTheFirstTickAfterSleep(t *testing.T) {
	h := newHarness(t, calm())
	h.start()
	pending := h.lastTick()

	// The lid closes before the pending tick fires, and opens eight hours on.
	h.clock.now = start.Add(8 * time.Hour)
	read := h.update(pending.msg)

	if h.source.reads != 2 {
		t.Errorf("the first tick after the clock jumped past the read read the source %d times, want once more", h.source.reads-1)
	}
	if got, want := h.footer(), probingKeys+" · refreshing…"; got != want {
		t.Errorf("while reading, footer = %q, want %q", got, want)
	}
	h.deliver(read...)
	if got, want := h.footer(), probingKeys+" · read 0s ago · next 21:42"; got != want {
		t.Errorf("after reading, footer = %q, want %q", got, want)
	}
}

func TestRefreshKey(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	pending := h.press("r")
	if h.source.reads != 2 {
		t.Fatalf("r read the source %d times, want once", h.source.reads-1)
	}
	if got, want := h.footer(), probingKeys+" · refreshing…"; got != want {
		t.Errorf("while reading, footer = %q, want %q", got, want)
	}
	h.deliver(h.press("r")...)
	if h.source.reads != 2 {
		t.Errorf("r while a read was under way read the source again")
	}
	h.clock.now = at(13, 20, 0)
	h.deliver(pending...)
	if got, want := h.footer(), probingKeys+" · read 0s ago · next 13:50"; got != want {
		t.Errorf("after the read, footer = %q, want %q", got, want)
	}
	h.deliver(h.press("R")...)
	if h.source.reads != 3 {
		t.Errorf("R read the source %d times in all, want 3", h.source.reads)
	}
}

func TestQuitKeys(t *testing.T) {
	tests := []struct {
		key      string
		wantQuit bool
	}{
		{key: "q", wantQuit: true},
		{key: "Q", wantQuit: true},
		{key: "ctrl+c", wantQuit: true},
		{key: "x", wantQuit: false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			h := newHarness(t, calm())
			h.start()

			quit := slices.Contains(h.press(tt.key), tea.Msg(tea.QuitMsg{}))
			if quit != tt.wantQuit {
				t.Errorf("%s quit = %v, want %v", tt.key, quit, tt.wantQuit)
			}
			if h.source.reads != 1 {
				t.Errorf("%s read the source", tt.key)
			}
		})
	}
}

func TestTicksEverySecond(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if ticks := h.tickUntil(at(13, 14, 0).Add(tickSlack)); ticks != 120 {
		t.Errorf("ticked %d times in the two minutes after the read, want every second, as the clock on screen shows the seconds", ticks)
	}
	if got, want := h.lastTick().delay, time.Second; got != want {
		t.Errorf("a tick armed the next %v later, want %v", got, want)
	}
	if got := h.view(); !strings.Contains(got, "13:14:00") {
		t.Errorf("at %s, the screen is\n%s\nwant the time to the second", h.clock.now.Format(time.StampMilli), got)
	}
}

func TestCountdownShowsSecondsBetweenTicks(t *testing.T) {
	h := newHarness(t, document(account("work", "Work", refused(session(1, 12*time.Minute)), week(0.5))))
	h.start()

	for h.clock.now.Before(at(13, 16, 17)) {
		h.fire(h.lastTick())
	}
	before := between(h.view())
	h.fire(h.lastTick())
	if after := between(h.view()); after == before {
		t.Errorf("at %s, a second on, the screen is\n%s\nwant the session's countdown, its last ten minutes in seconds, moved on", h.clock.now.Format(time.StampMilli), after)
	}
}

// between is the screen between its title row and its footer, which tell
// the time and how long ago the document was read.
func between(screen string) string {
	rows := strings.Split(screen, "\n")
	return strings.Join(rows[1:len(rows)-1], "\n")
}

func TestATickFromBeforeAReadIsDropped(t *testing.T) {
	h := newHarness(t, calm())
	h.start()
	stale := h.lastTick()

	h.clock.now = at(13, 12, 0).Add(500 * time.Millisecond)
	h.read(calm())
	if got, want := h.lastTick().due, at(13, 12, 1).Add(tickSlack); !got.Equal(want) {
		t.Errorf("after a read, the next tick is at %s, want %s", got.Format(time.StampMilli), want.Format(time.StampMilli))
	}

	armed := len(h.timers)
	h.fire(stale)
	if len(h.timers) != armed || h.source.reads != 2 {
		t.Errorf("a tick from before the read armed %d timers and read %d times, want it dropped", len(h.timers)-armed, h.source.reads-2)
	}
}

func TestFooter(t *testing.T) {
	failing := errors.New("connection refused")
	tests := []struct {
		name string
		// then moves the model on from its first read.
		then func(h *harness)
		want string
	}{
		{
			name: "read",
			then: func(*harness) {},
			want: probingKeys + " · read 0s ago · next 13:42",
		},
		{
			name: "read a while ago",
			then: func(h *harness) { h.clock.now = at(13, 12, 59) },
			want: probingKeys + " · read 59s ago · next 13:42",
		},
		{
			name: "read minutes ago",
			then: func(h *harness) { h.clock.now = at(13, 15, 30) },
			want: probingKeys + " · read 3m ago · next 13:42",
		},
		{
			name: "reading again",
			then: func(h *harness) { h.press("r") },
			want: probingKeys + " · refreshing…",
		},
		{
			name: "after a read failed",
			then: func(h *harness) {
				h.source.err = failing
				h.clock.now = at(13, 20, 0)
				h.read(calm())
			},
			want: probingKeys + " · couldn't read usage: connection refused · next 13:22",
		},
		{
			name: "after a read failed, and the next didn't",
			then: func(h *harness) {
				h.source.err = failing
				h.read(calm())
				h.source.err = nil
				h.clock.now = at(13, 20, 0)
				h.read(calm())
			},
			want: probingKeys + " · read 0s ago · next 13:50",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, calm())
			h.start()

			tt.then(h)
			if got := h.footer(); got != tt.want {
				t.Errorf("footer = %q, want %q", got, tt.want)
			}
			if !strings.Contains(h.view(), "╭─ 1 Work ") {
				t.Errorf("screen is\n%s\nwant the account's card still", h.view())
			}
		})
	}
}

func TestFirstReadFails(t *testing.T) {
	h := newHarness(t, calm())
	h.source.err = errors.New("connection refused")
	h.start()

	if got, want := h.footer(), unreadKeys+" · couldn't read usage: connection refused · next 13:14"; got != want {
		t.Errorf("footer = %q, want %q", got, want)
	}
	if strings.Contains(h.view(), "╭─ 1 Work ") {
		t.Errorf("screen is\n%s\nwant no account, nothing having been read", h.view())
	}
	h.source.err = nil
	if got, want := h.tickUntilRead(), at(13, 14, 0).Add(tickSlack); !got.Equal(want) {
		t.Errorf("read again at %s, want %s", got.Format(time.StampMilli), want.Format(time.StampMilli))
	}
}

func TestDrawsAtTheTerminalsSize(t *testing.T) {
	h := newHarness(t, calm())
	h.start()
	h.settle()

	for _, size := range []tea.WindowSizeMsg{
		{Width: 150, Height: 50},
		{Width: 60, Height: 50},
		{Width: 150, Height: 6},
	} {
		h.update(size)
		view := h.model.View()
		if want := calmScreen(Size{Width: size.Width, Height: size.Height}, h.clock.now, h.model.trails); view.Content != want {
			t.Errorf("at %dx%d, the screen is\n%s\nwant\n%s", size.Width, size.Height, view.Content, want)
		}
		if !view.AltScreen {
			t.Errorf("at %dx%d, the view isn't full screen", size.Width, size.Height)
		}
	}
}

func TestPassesOnTheSizeItDrawsAt(t *testing.T) {
	given := Size{Width: 120, Height: 40}
	tests := []struct {
		name   string
		before []tea.WindowSizeMsg
		size   tea.WindowSizeMsg
		want   []tea.Msg
	}{
		{name: "for 0×0", size: tea.WindowSizeMsg{}, want: []tea.Msg{tea.WindowSizeMsg{Width: 120, Height: 40}}},
		{name: "for a width alone", size: tea.WindowSizeMsg{Width: 100}, want: []tea.Msg{tea.WindowSizeMsg{Width: 100, Height: 40}}},
		{name: "for 0×0 after a real size", before: []tea.WindowSizeMsg{{Width: 150, Height: 50}}, size: tea.WindowSizeMsg{}, want: []tea.Msg{tea.WindowSizeMsg{Width: 150, Height: 50}}},
		{name: "nothing for a real size", size: tea.WindowSizeMsg{Width: 150, Height: 50}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := unsizedHarness(t, calm(), given)
			for _, size := range tt.before {
				h.deliver(size)
			}

			if got := h.update(tt.size); !slices.Equal(got, tt.want) {
				t.Errorf("given %dx%d, the model passed on %v, want %v", tt.size.Width, tt.size.Height, got, tt.want)
			}
		})
	}
}

func TestDrawsAtTheGivenSizeUntilTheTerminalGivesOne(t *testing.T) {
	given := Size{Width: 120, Height: 40}
	tests := []struct {
		name  string
		sizes []tea.WindowSizeMsg
		want  Size
	}{
		{name: "before the terminal gives a size", want: given},
		{name: "when it gives 0×0", sizes: []tea.WindowSizeMsg{{}}, want: given},
		{name: "when it gives a width alone", sizes: []tea.WindowSizeMsg{{Width: 100}}, want: Size{Width: 100, Height: 40}},
		{name: "once it gives its size after 0×0", sizes: []tea.WindowSizeMsg{{}, {Width: 150, Height: 50}}, want: Size{Width: 150, Height: 50}},
		{name: "keeping the size it gave through a later 0×0", sizes: []tea.WindowSizeMsg{{Width: 150, Height: 50}, {}}, want: Size{Width: 150, Height: 50}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := unsizedHarness(t, calm(), given)
			for _, size := range tt.sizes {
				h.deliver(size)
			}
			h.start()
			h.settle()

			if got, want := h.model.View().Content, calmScreen(tt.want, h.clock.now, h.model.trails); got != want {
				t.Errorf("the screen is\n%s\nwant it drawn at %dx%d:\n%s", got, tt.want.Width, tt.want.Height, want)
			}
		})
	}
}

// calmScreen is the screen of calm read at now, its windows used as trails
// says, as a terminal of the size given shows it without colour, probing,
// with nothing more to say.
func calmScreen(size Size, now time.Time, trails dashboard.History) string {
	return strings.Join(dashboard.Frame{
		Width: size.Width, Height: size.Height, Look: dashboard.NoColour(),
		Views: dashboard.Views(), View: dashboard.Accounts, History: trails,
		Keys: []dashboard.Key{
			{Key: "tab", Does: "views"}, {Key: "w", Does: "window: auto"}, {Key: "←→", Does: "focus"}, {Key: "space", Does: "flip"},
			{Key: "g", Does: "chart"}, {Key: "?", Does: "keys", Always: true}, {Key: "q", Does: "quit", Always: true},
		},
		Status: "read 0s ago · next 13:42",
		Policy: policy,
	}.Draw(calm(), now), "\n")
}
