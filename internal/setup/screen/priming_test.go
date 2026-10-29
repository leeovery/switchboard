package screen

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/setup"
)

// The rows of the Priming page at 160×37: its field's, and the one under it.
const (
	fieldRow = 9
	underRow = 10
)

// workingDay is priming's day as a test sets it: 08:00-23:00.
var workingDay = config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}

// errUnwritable is why SetDay fails, where a test has it.
var errUnwritable = errors.New("save the config: permission denied")

// notSet is the fact of a Priming step that failed as err says.
func notSet(err error) setup.Said {
	return setup.Said{Mark: setup.Failed, Label: "Not set", Detail: err.Error()}
}

// primingHarness is a harness opening on Priming, its day as given, started.
func primingHarness(t *testing.T, day config.Day) *harness {
	t.Helper()
	h := newHarness(t, dashboard.SetupPriming)
	h.steps.day = day
	h.start()
	return h
}

// typedDay is what's typed in the Priming page's field.
func (h *harness) typedDay() string {
	h.t.Helper()
	p := h.model.page().Priming
	if p == nil {
		h.t.Fatal("the Priming page holds no field")
	}
	return p.Day.Value
}

// clear takes out what's typed in the field, from its end, a backspace a
// character.
func (h *harness) clear() {
	h.t.Helper()
	h.pressAndDeliver("end")
	for range []rune(h.typedDay()) {
		h.pressAndDeliver("backspace")
	}
}

func TestReachingPrimingReadsTheDayIntoItsFieldTypedIn(t *testing.T) {
	tests := []struct {
		name  string
		day   config.Day
		field string
	}{
		{name: "a day set: the field holds it", day: workingDay, field: "  Your day  08:00-23:00               start and end, as HH:MM-HH:MM"},
		{name: "priming off: the field empty", field: "  Your day                            start and end, as HH:MM-HH:MM"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := primingHarness(t, tt.day)
			if h.steps.primings != 1 {
				t.Errorf("Priming ran %d times, want once, as the step was reached", h.steps.primings)
			}
			if got := h.rows()[fieldRow]; got != tt.field {
				t.Errorf("the field's row reads\n%q\nwant\n%q", got, tt.field)
			}
			if p := h.model.page().Priming; p == nil || !p.Day.Typing {
				t.Errorf("the page's field is %+v, want it typed in", p)
			}
			if got := strings.TrimLeft(h.rows()[keyRow], " "); got != "enter next: Service   q quit" {
				t.Errorf("the key line reads %q, want enter next: Service, then q quit", got)
			}
		})
	}
}

func TestTheWalkReachingPrimingReadsItsDay(t *testing.T) {
	h := newHarness(t, dashboard.SetupAccounts)
	h.steps.day = workingDay
	h.start()
	if h.steps.primings != 0 {
		t.Fatalf("Priming ran %d times before its step, want none", h.steps.primings)
	}
	h.pressAndDeliver("enter")
	if h.steps.primings != 1 || h.typedDay() != "08:00-23:00" {
		t.Errorf("reaching Priming ran it %d times, the field holding %q; want once, holding 08:00-23:00", h.steps.primings, h.typedDay())
	}
}

func TestEnterAtPriming(t *testing.T) {
	tests := []struct {
		name string
		day  config.Day
		// edit is what's done in the field before enter; set the days SetDay
		// is given.
		edit func(h *harness)
		set  []string
	}{
		{name: "the day as it was read: nothing written", day: workingDay, edit: func(*harness) {}},
		{name: "left empty, priming off: nothing written, priming left off", edit: func(*harness) {}},
		{name: "blanks alone, priming off: left empty, nothing written", edit: func(h *harness) { h.typeIn("  ") }},
		{name: "a day typed, priming off: the day set", edit: func(h *harness) { h.typeIn("09:00-18:00") }, set: []string{"09:00-18:00"}},
		{name: "the day changed: the day set", day: workingDay, edit: func(h *harness) { h.clear(); h.typeIn("07:30-22:00") }, set: []string{"07:30-22:00"}},
		{name: "the day changed and changed back: nothing written", day: workingDay, edit: func(h *harness) { h.pressAndDeliver("backspace"); h.typeIn("0") }},
		{name: "the day cleared: priming turned off", day: workingDay, edit: func(h *harness) { h.clear() }, set: []string{""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := primingHarness(t, tt.day)
			tt.edit(h)
			h.pressAndDeliver("enter")
			if !slices.Equal(h.steps.set, tt.set) {
				t.Errorf("SetDay was given %q, want %q", h.steps.set, tt.set)
			}
			if got := h.model.page(); got.Step != dashboard.SetupService || !slices.Contains(got.Passed, dashboard.SetupPriming) {
				t.Errorf("enter shows %s, %v passed; want the Service, Priming passed", got.Step.Title(), got.Passed)
			}
		})
	}
}

func TestADayRefusedIsSaidAndSetupStaysOnThePage(t *testing.T) {
	h := primingHarness(t, workingDay)
	h.clear()
	h.typeIn("8-9")
	h.pressAndDeliver("enter")

	if len(h.steps.set) != 0 {
		t.Errorf("SetDay was given %q, want nothing: the day isn't one", h.steps.set)
	}
	if got := h.model.page(); got.Step != dashboard.SetupPriming || got.Failed {
		t.Fatalf("enter on a day refused shows %s, failed %v; want Priming, not failed", got.Step.Title(), got.Failed)
	}
	// Why alone, as config says it, the day it quotes typed above it.
	if got, want := h.rows()[underRow], strings.Repeat(" ", 12)+"must be two times of day, HH:MM, joined by -, such as 08:00-23:00"; got != want {
		t.Errorf("the row under the field reads\n%q\nwant\n%q", got, want)
	}
	if got := strings.TrimLeft(h.rows()[keyRow], " "); got != "enter next: Service   q quit" {
		t.Errorf("the key line reads %q, want enter next: Service, then q quit, as the step isn't under way", got)
	}

	said := h.rows()[underRow]
	h.pressAndDeliver("left")
	if got := h.rows()[underRow]; got != said {
		t.Errorf("once ← has moved the cursor, the row under the field reads %q, want it still said, as the day's unchanged", got)
	}
	h.pressAndDeliver("backspace")
	if got := h.rows()[underRow]; got != "" {
		t.Errorf("once the day's changed, the row under the field reads %q, want it unsaid", got)
	}
	h.clear()
	h.typeIn("08:00-20:00")
	h.pressAndDeliver("enter")
	if !slices.Equal(h.steps.set, []string{"08:00-20:00"}) || h.model.page().Step != dashboard.SetupService {
		t.Errorf("a day put right: SetDay given %q, showing %s; want 08:00-20:00, then the Service", h.steps.set, h.model.page().Step.Title())
	}
}

func TestADaySetUnderWayTakesNoKeysTillItsSet(t *testing.T) {
	h := primingHarness(t, workingDay)
	h.clear()
	h.typeIn("07:00-22:00")
	set := h.press("enter")

	if got := strings.TrimLeft(h.rows()[keyRow], " "); got != "q quit" {
		t.Errorf("while the day's set, the key line reads %q, want q quit alone", got)
	}
	h.typeIn("x")
	h.pressAndDeliver("enter")
	if got := h.typedDay(); got != "07:00-22:00" {
		t.Errorf("a key while the day's set leaves the field holding %q, want 07:00-22:00", got)
	}
	h.deliver(tea.PasteMsg{Content: "09:00"})
	if got := h.typedDay(); got != "07:00-22:00" {
		t.Errorf("a paste while the day's set leaves the field holding %q, want 07:00-22:00", got)
	}
	h.deliver(set...)
	if len(h.steps.set) != 1 || h.model.page().Step != dashboard.SetupService {
		t.Errorf("SetDay ran %d times, showing %s; want once, then the Service", len(h.steps.set), h.model.page().Step.Title())
	}
}

func TestQQuitsWhileTheDayIsSet(t *testing.T) {
	h := primingHarness(t, workingDay)
	h.clear()
	h.typeIn("07:00-22:00")
	h.press("enter")

	if !quits(h.press("q")) {
		t.Fatal("q while the day's set didn't quit, as the key line says it does")
	}
	if got := h.typedDay(); got != "07:00-22:00" {
		t.Errorf("q while the day's set leaves the field holding %q, want 07:00-22:00", got)
	}
	if got := h.model.walk.left(); got != (Left{Ending: Paused}) {
		t.Errorf("setup stands to leave %+v, want paused", got)
	}
}

func TestBlanksRoundADayAreLeftOutOfTheDaySet(t *testing.T) {
	h := primingHarness(t, config.Day{})
	h.typeIn(" 09:00-18:00 ")
	h.pressAndDeliver("enter")
	if !slices.Equal(h.steps.set, []string{"09:00-18:00"}) {
		t.Errorf("SetDay was given %q, want 09:00-18:00", h.steps.set)
	}
}

// A refusal of 200 words, as only a test has, makes the Priming page taller
// than it shows at 95×25, for it to scroll with its field typed in.
func TestThePageScrollsWithItsFieldTypedInByPgDnPgUpAndTheWheel(t *testing.T) {
	tests := []struct {
		name string
		do   func(h *harness)
		// down is whether it's scrolled down after.
		down bool
	}{
		{name: "pgdown", do: func(h *harness) { h.pressAndDeliver("pgdown") }, down: true},
		{name: "pgdown, then pgup", do: func(h *harness) { h.pressAndDeliver("pgdown"); h.pressAndDeliver("pgup") }},
		{name: "the wheel down", do: func(h *harness) { h.deliver(tea.MouseWheelMsg{Button: tea.MouseWheelDown}) }, down: true},
		{name: "j types, scrolling nothing", do: func(h *harness) { h.pressAndDeliver("j") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := primingHarness(t, workingDay)
			h.update(tea.WindowSizeMsg{Width: 95, Height: 25})
			f := *h.model.priming
			f.refused = strings.Repeat("wrong ", 200)
			h.model.priming = &f
			if !h.model.typing() || h.model.page().Scrolling(95, 25).Most == 0 {
				t.Fatal("the page isn't typed in and taller than it shows, as the test needs")
			}
			tt.do(h)
			if got := h.model.scroll > 0; got != tt.down {
				t.Errorf("scrolled %d rows, want scrolled down: %v", h.model.scroll, tt.down)
			}
		})
	}
}

func TestAPastedEscapeNeverReachesTheTerminal(t *testing.T) {
	h := primingHarness(t, config.Day{})
	h.deliver(tea.PasteMsg{Content: "\x1b[2J\x1b]0;title\x0708:00"})
	if got := h.typedDay(); strings.ContainsRune(got, '\x1b') {
		t.Errorf("the field holds %q, an escape among it", got)
	}
	if view := h.model.View().Content; strings.Contains(view, "\x1b[2J") || strings.Contains(view, "\x1b]0;") {
		t.Errorf("the screen draws the pasted escapes: %q", view)
	}
}

func TestAPrimingStepThatFailsStopsSetupThere(t *testing.T) {
	tests := []struct {
		name string
		// fail has the step fail, and edit is done before enter.
		fail func(*fakeSteps)
		edit func(*harness)
		want error
	}{
		{name: "the day couldn't be read", fail: func(s *fakeSteps) { s.primingErr = errNoConfig }, edit: func(*harness) {}, want: errNoConfig},
		{name: "the day couldn't be set", fail: func(s *fakeSteps) { s.setErr = errUnwritable }, edit: func(h *harness) { h.clear() }, want: errUnwritable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, dashboard.SetupPriming)
			h.steps.day = workingDay
			tt.fail(h.steps)
			h.start()
			tt.edit(h)
			h.pressAndDeliver("enter")

			if got := h.model.walk.left(); got != (Left{Ending: Failed, Err: tt.want}) {
				t.Errorf("setup stands to leave %+v, want failing with %v", got, tt.want)
			}
			if got, want := h.rows()[fieldRow], factRowOf("✗", notSet(tt.want)); got != want {
				t.Errorf("the page's row reads\n%q\nwant\n%q", got, want)
			}
			if got := strings.TrimLeft(h.rows()[keyRow], " "); got != "q quit" {
				t.Errorf("the key line reads %q, want q quit alone", got)
			}
			if !quits(h.press("q")) {
				t.Error("q at a Priming step that failed didn't quit")
			}
		})
	}
}

func TestTheKeysWithTheFieldTypedIn(t *testing.T) {
	t.Run("j, k and q type into it, quitting and scrolling nothing", func(t *testing.T) {
		h := primingHarness(t, config.Day{})
		for _, key := range []string{"j", "k", "q"} {
			if msgs := h.press(key); quits(msgs) {
				t.Fatalf("%s quit, want it typed", key)
			}
		}
		if got := h.typedDay(); got != "jkq" {
			t.Errorf("the field holds %q, want jkq", got)
		}
	})
	t.Run("pgdown and pgup type nothing", func(t *testing.T) {
		h := primingHarness(t, workingDay)
		h.pressAndDeliver("pgdown")
		h.pressAndDeliver("pgup")
		if got := h.typedDay(); got != "08:00-23:00" {
			t.Errorf("the field holds %q, want 08:00-23:00", got)
		}
	})
	t.Run("ctrl+c leaves", func(t *testing.T) {
		h := primingHarness(t, workingDay)
		if !quits(h.press("ctrl+c")) {
			t.Error("ctrl+c with the field typed in didn't quit")
		}
	})
	t.Run("esc gives the field up, q then leaving, and tab takes it again", func(t *testing.T) {
		h := primingHarness(t, workingDay)
		h.pressAndDeliver("esc")
		if p := h.model.page().Priming; p.Day.Typing {
			t.Fatal("esc left the field typed in")
		}
		h.pressAndDeliver("tab")
		h.pressAndDeliver("q")
		if got := h.typedDay(); got != "08:00-23:00q" {
			t.Errorf("q once tab took the field again: the field holds %q, want 08:00-23:00q", got)
		}
		h.pressAndDeliver("esc")
		if !quits(h.press("q")) {
			t.Error("q with the field given up didn't quit")
		}
	})
	t.Run("enter with the field given up moves on as with it typed in", func(t *testing.T) {
		h := primingHarness(t, config.Day{})
		h.typeIn("09:00-18:00")
		h.pressAndDeliver("esc")
		h.pressAndDeliver("enter")
		if !slices.Equal(h.steps.set, []string{"09:00-18:00"}) || h.model.page().Step != dashboard.SetupService {
			t.Errorf("SetDay was given %q, showing %s; want 09:00-18:00, then the Service", h.steps.set, h.model.page().Step.Title())
		}
	})
	t.Run("a paste goes in at the cursor, its line breaks left out", func(t *testing.T) {
		h := primingHarness(t, config.Day{})
		h.deliver(tea.PasteMsg{Content: "08:00-23:00\n"})
		if got := h.typedDay(); got != "08:00-23:00" {
			t.Errorf("the field holds %q, want 08:00-23:00", got)
		}
	})
	t.Run("a paste with the field given up goes nowhere", func(t *testing.T) {
		h := primingHarness(t, config.Day{})
		h.pressAndDeliver("esc")
		h.deliver(tea.PasteMsg{Content: "08:00-23:00"})
		if got := h.typedDay(); got != "" {
			t.Errorf("the field holds %q, want nothing", got)
		}
	})
}
