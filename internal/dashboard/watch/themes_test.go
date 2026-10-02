package watch

import (
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

// lightBackground and darkBackground are what a terminal says its
// background is.
var (
	lightBackground = color.RGBA{R: 0xFA, G: 0xFA, B: 0xFA, A: 0xff}
	darkBackground  = color.RGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xff}
)

func TestTheScreenWaitsForTheTerminalToSayWhatItsBackgroundIs(t *testing.T) {
	h, _ := themedHarness(t, theme.Choice{})
	h.start()

	if got := h.model.View(); got.Content != "" || got.BackgroundColor != nil {
		t.Errorf("before the terminal says its background, the screen is %q on %v, want it blank, the background left as it is", got.Content, got.BackgroundColor)
	}
	tm, ok := h.pending(unansweredMsg{})
	if !ok || tm.delay != answerWithin {
		t.Fatalf("no wait armed for the terminal to say its background in %v", answerWithin)
	}
	h.deliver(tea.BackgroundColorMsg{Color: darkBackground})
	if got := h.view(); !strings.Contains(got, "work · Work") {
		t.Errorf("once the terminal says its background, the screen is\n%s\nwant the dashboard", got)
	}
}

func TestTheThemeIsChosenByTheTerminalsBackground(t *testing.T) {
	tests := []struct {
		name   string
		choice theme.Choice
		// answer is what the terminal says its background is, or nil for it
		// to say nothing in time.
		answer color.Color
		want   string
	}{
		{name: "the default pair on a dark terminal", answer: darkBackground, want: "nord"},
		{name: "the default pair on a light terminal", answer: lightBackground, want: "tokyo-night-day"},
		{name: "the default pair where the terminal doesn't say: dark", want: "nord"},
		{name: "a pair on a light terminal", choice: theme.Choice{Light: "exchange", Dark: "amber"}, answer: lightBackground, want: "exchange"},
		{name: "one theme, whatever the background", choice: theme.One("amber"), answer: lightBackground, want: "amber"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := themedHarness(t, tt.choice)
			h.start()
			h.answer(tt.answer)

			if got := h.drawnIn(); got != tt.want {
				t.Errorf("drawn in %s, want %s", got, tt.want)
			}
		})
	}
}

func TestALateAnswerChangesNothing(t *testing.T) {
	h, _ := themedHarness(t, theme.Choice{})
	h.start()
	h.answer(nil)
	h.deliver(tea.BackgroundColorMsg{Color: lightBackground})

	if got := h.drawnIn(); got != "nord" {
		t.Errorf("an answer after the dashboard was drawn has it drawn in %s, want nord still: by then the terminal may say the canvas", got)
	}
	if h.model.backdrop.original != nil {
		t.Errorf("the background to put back is %v, want none, an answer after the canvas was set being no telling what it was", h.model.backdrop.original)
	}
}

func TestTheCanvasIsPaintedOnEveryCellAndSetAsTheBackground(t *testing.T) {
	for _, size := range []Size{{Width: 150, Height: 50}, {Width: 120, Height: 4}} {
		h, _ := themedHarness(t, theme.One("nord"))
		h.source.doc = document(three()...)
		h.update(tea.WindowSizeMsg{Width: size.Width, Height: size.Height})
		h.start()
		h.answer(darkBackground)

		view := h.model.View()
		if got := hexOf(view.BackgroundColor); got != "#2E3440" {
			t.Errorf("at %d×%d, the terminal's background is set to %s, want nord's canvas, #2E3440", size.Width, size.Height, got)
		}
		lines := strings.Split(view.Content, "\n")
		if len(lines) != size.Height {
			t.Errorf("at %d×%d, the screen is %d lines, want the terminal's %d", size.Width, size.Height, len(lines), size.Height)
		}
		for i, line := range lines {
			if got := ansi.StringWidth(line); got != size.Width {
				t.Errorf("at %d×%d, line %d is %d cells wide, want the terminal's %d", size.Width, size.Height, i, got, size.Width)
			}
			if !strings.Contains(line, "48;2;46;52;64") {
				t.Errorf("at %d×%d, line %d isn't on the canvas: %q", size.Width, size.Height, i, line)
			}
		}
	}
}

func TestTheTerminalThemeLeavesTheBackgroundAlone(t *testing.T) {
	h, _ := themedHarness(t, theme.One(theme.Terminal))
	h.start()
	h.answer(darkBackground)

	if view := h.model.View(); view.BackgroundColor != nil || strings.Contains(view.Content, "48;") {
		t.Errorf("in the terminal's theme, the background is set to %v, the screen %q, want neither painted", view.BackgroundColor, view.Content)
	}
	if h.model.backdrop.set {
		t.Error("in the terminal's theme, the background is to be put back, want it never set")
	}
}

func TestNoColour(t *testing.T) {
	h := newHarness(t, calm())
	h.start()

	if _, asked := h.pending(unansweredMsg{}); asked {
		t.Error("without colour, the terminal is asked its background, want nothing asked")
	}
	view := h.model.View()
	if view.BackgroundColor != nil {
		t.Errorf("without colour, the terminal's background is set to %v, want it left as it is", view.BackgroundColor)
	}
	if strings.Contains(view.Content, "38;") || strings.Contains(view.Content, "48;") {
		t.Errorf("without colour, the screen is in colour:\n%q", view.Content)
	}
	if !strings.Contains(view.Content, "\x1b[1m") {
		t.Error("without colour, nothing is bold, want state told by bold where colour told it")
	}
	h.press("t")
	if h.model.picker.open {
		t.Error("without colour, t opened the theme picker, want it to do nothing")
	}
	if strings.Contains(h.footer(), "t themes") {
		t.Errorf("without colour, the footer reads %q, want no t", h.footer())
	}
}

func TestTOpensThePickerOverTheView(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})

	if !strings.Contains(h.footer(), "t themes") {
		t.Errorf("the footer reads %q, want it to say t opens the themes", h.footer())
	}
	h.press("t")

	view := h.view()
	for _, want := range []string{"│ Themes", "│ ▌ nord", "● dark", "● light", "│ ⏎    set theme", "broken"} {
		if !strings.Contains(view, want) {
			t.Errorf("with the picker open, the screen reads\n%s\nwant %q", view, want)
		}
	}
	if !strings.Contains(view, "work · Work") {
		t.Errorf("with the picker open, the screen reads\n%s\nwant the view beside it", view)
	}
	h.press("esc")
	h.press("t")
	if themes.lists != 2 {
		t.Errorf("the themes were listed %d times, want afresh each time the picker opens", themes.lists)
	}
}

func TestThePickerShowsEachThemeAsTheCursorReachesIt(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})
	h.press("t")

	var reached []string
	for range 7 {
		h.press("down")
		reached = append(reached, h.drawnIn())
	}
	if want := []string{"terminal", "tokyo-night", "tokyo-night-day", "tokyo-night-day", "tokyo-night-day", "tokyo-night-day", "tokyo-night-day"}; !slices.Equal(reached, want) {
		t.Errorf("moving down from nord, the screen is drawn in %q, want %q, stopping at the last", reached, want)
	}
	for range 7 {
		h.press("up")
	}
	if got := h.drawnIn(); got != "amber" {
		t.Errorf("moving up to the top, the screen is drawn in %s, want amber, past the broken theme", got)
	}
	if len(themes.kept) != 0 {
		t.Errorf("moving the cursor kept %+v, want nothing kept", themes.kept)
	}
}

func TestEnterSetsOneTheme(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})
	h.press("t")
	h.press("up")
	h.press("up")
	h.press("enter")

	if want := []theme.Choice{theme.One("amber")}; !slices.Equal(themes.kept, want) {
		t.Errorf("kept %+v, want %+v", themes.kept, want)
	}
	if !h.model.picker.open || h.drawnIn() != "amber" {
		t.Errorf("after enter, the picker is open: %v, the screen in %s; want it open, on amber", h.model.picker.open, h.drawnIn())
	}
	if view := h.view(); !strings.Contains(view, "▌ amber") || !strings.Contains(view, "●") || strings.Contains(view, "● dark") {
		t.Errorf("the screen reads\n%s\nwant amber badged as the one theme, the pair's badges gone", view)
	}
	h.press("esc")
	if got := h.drawnIn(); got != "amber" {
		t.Errorf("closed, the screen is drawn in %s, want amber, now in force", got)
	}
}

func TestDAndLSetTheHalvesOfThePair(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})
	h.press("t")
	h.press("down")
	h.press("d")
	h.press("down")
	h.press("l")

	want := []theme.Choice{{Dark: "terminal"}, {Light: "tokyo-night", Dark: "terminal"}}
	if !slices.Equal(themes.kept, want) {
		t.Errorf("kept %+v, want %+v", themes.kept, want)
	}
	if view := h.view(); !strings.Contains(view, "terminal") || !strings.Contains(view, "● dark") || !strings.Contains(view, "● light") {
		t.Errorf("the screen reads\n%s\nwant the halves badged", view)
	}
	h.press("esc")
	if got := h.drawnIn(); got != "terminal" {
		t.Errorf("closed on a dark terminal, the screen is drawn in %s, want the dark half, terminal", got)
	}
}

func TestAHalfOverOneThemeAsksFirst(t *testing.T) {
	tests := []struct {
		name    string
		answers []string
		want    []theme.Choice
	}{
		{name: "yes", answers: []string{"y"}, want: []theme.Choice{{Light: "nord"}}},
		{name: "no", answers: []string{"n"}},
		{name: "esc", answers: []string{"esc"}},
		{name: "anything else, then yes", answers: []string{"r", "down", "Y"}, want: []theme.Choice{{Light: "nord"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, themes := settledThemedHarness(t, theme.One("amber"))
			h.press("t")
			h.press("down")
			h.press("down")
			h.press("l")

			if view := h.view(); !strings.Contains(view, "clear amber?  y / n") || !strings.Contains(view, "y    confirm") {
				t.Errorf("setting a half over one theme, the screen reads\n%s\nwant it to ask first", view)
			}
			for _, key := range tt.answers {
				h.press(key)
			}
			if !slices.Equal(themes.kept, tt.want) {
				t.Errorf("kept %+v, want %+v", themes.kept, tt.want)
			}
			if strings.Contains(h.view(), "y / n") {
				t.Errorf("answered, the screen reads\n%s\nwant the question gone", h.view())
			}
			if !h.model.picker.open {
				t.Error("answering closed the picker, want it open")
			}
		})
	}
}

func TestEscPutsBackTheThemeInForce(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{Dark: "exchange"})
	h.press("t")
	h.press("up")
	h.press("up")
	if got := h.drawnIn(); got == "exchange" {
		t.Fatalf("moving the cursor left the screen in %s, want another theme previewed", got)
	}
	h.press("esc")

	if h.model.picker.open {
		t.Error("esc left the picker open")
	}
	if got := h.drawnIn(); got != "exchange" {
		t.Errorf("closed, the screen is drawn in %s, want exchange, the theme in force", got)
	}
	if len(themes.kept) != 0 {
		t.Errorf("esc kept %+v, want nothing", themes.kept)
	}
}

func TestAChoiceThatIsntKeptStandsAsItWas(t *testing.T) {
	log := logstest.Capture(t)
	h, themes := settledThemedHarness(t, theme.Choice{})
	themes.err = errors.New("read-only file system")
	h.press("t")
	h.press("down")
	h.press("enter")

	if view := h.view(); !strings.Contains(view, "⚠ not kept: see the log") {
		t.Errorf("the screen reads\n%s\nwant the picker to say the choice wasn't kept", view)
	}
	if got := h.model.choice; got != (theme.Choice{}) {
		t.Errorf("the choice is %+v, want it as it was", got)
	}
	if !log.Has("level=WARN", `msg="couldn't keep the theme chosen"`, `error="read-only file system"`) {
		t.Errorf("the log reads\n%s\nwant it to say why the choice wasn't kept", log)
	}
	h.press("down")
	if strings.Contains(h.view(), "not kept") {
		t.Error("the note outlasted the next key")
	}
}

func TestThePickerTakesEveryKeyButQuit(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.Choice{})
	h.press("t")
	reads := len(h.source.asked)
	for _, key := range []string{"r", "1", "a", "m", "t"} {
		h.press(key)
	}
	if len(h.source.asked) != reads || len(h.source.orders) != 0 || !h.model.picker.open {
		t.Errorf("with the picker open, keys read %d more times and gave orders %q, the picker open: %v; want them taken by the picker", len(h.source.asked)-reads, h.source.orders, h.model.picker.open)
	}
	if !quits(h.press("q")) {
		t.Error("with the picker open, q didn't quit")
	}
}

func TestThePickerNeedsRoom(t *testing.T) {
	const tooSmall = "the terminal's too small for the theme picker"
	for _, size := range []tea.WindowSizeMsg{{Width: 20, Height: 50}, {Width: 150, Height: 8}} {
		h, _ := settledThemedHarness(t, theme.Choice{})
		h.update(size)
		h.press("t")
		if h.model.picker.open || h.model.note != tooSmall {
			t.Errorf("at %d×%d, t opened the picker: %v, the footer saying %q; want it closed, saying %q", size.Width, size.Height, h.model.picker.open, h.model.note, tooSmall)
		}
	}

	h, _ := settledThemedHarness(t, theme.Choice{})
	h.press("t")
	h.press("down")
	h.update(tea.WindowSizeMsg{Width: 150, Height: 8})
	if h.model.picker.open || h.drawnIn() != "nord" || !strings.Contains(h.footer(), tooSmall) {
		t.Errorf("shrunk too short, the picker is open: %v, the screen in %s, the footer reading %q; want it closed, the theme in force put back, saying why",
			h.model.picker.open, h.drawnIn(), h.footer())
	}
}

func TestThePickerWaitsForTheTerminalsBackground(t *testing.T) {
	h, _ := themedHarness(t, theme.Choice{})
	h.start()
	h.press("t")

	if h.model.picker.open {
		t.Error("t opened the picker before the screen was drawn")
	}
}

func TestTheBackgroundIsPutBackAsItWas(t *testing.T) {
	tests := []struct {
		name string
		b    backdrop
		want string
	}{
		{name: "set, and the terminal said what it was", b: backdrop{set: true, original: darkBackground}, want: ansi.SetBackgroundColor("#1a1b26")},
		{name: "set, and the terminal didn't say", b: backdrop{set: true}, want: ansi.ResetBackgroundColor},
		{name: "never set", b: backdrop{original: darkBackground}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			tt.b.putBack(&out)
			if out.String() != tt.want {
				t.Errorf("putBack() wrote %q, want %q", out.String(), tt.want)
			}
		})
	}
}

// themedHarness is a model drawn in colour, in the themes choice draws in,
// of the built-ins and a broken theme, in a terminal 150 cells wide and 50
// lines tall, reading calm. It hasn't started.
func themedHarness(t *testing.T, choice theme.Choice) (*harness, *fakeThemes) {
	t.Helper()
	themes := &fakeThemes{listing: listing()}
	h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: calm()}, notifier: &fakeNotifier{}}
	h.model = New(t.Context(), Config{
		Source: h.source, Notifier: h.notifier, Notifications: notifications, Now: h.clock.Now, After: h.arm, Interval: interval, Policy: policy,
		Size: Size{Width: 150, Height: 50}, Choice: choice, Pair: themes.listing.Pair(choice), Themes: themes,
	})
	return h, themes
}

// settledThemedHarness is a themedHarness started, its terminal's background
// dark.
func settledThemedHarness(t *testing.T, choice theme.Choice) (*harness, *fakeThemes) {
	t.Helper()
	h, themes := themedHarness(t, choice)
	h.start()
	h.answer(darkBackground)
	return h, themes
}

// answer has the terminal say its background is background, or with nil,
// say nothing in time.
func (h *harness) answer(background color.Color) {
	h.t.Helper()
	if background != nil {
		h.deliver(tea.BackgroundColorMsg{Color: background})
		return
	}
	tm, ok := h.pending(unansweredMsg{})
	if !ok {
		h.t.Fatal("no wait armed for the terminal's background")
	}
	h.fire(tm)
}

// drawnIn is the slug of the theme the screen is drawn in.
func (h *harness) drawnIn() string {
	return h.model.showing.Slug
}

// listing lists the built-ins and a broken theme, as the picker finds them.
func listing() theme.Listing {
	var l theme.Listing
	for _, b := range theme.Builtins() {
		l.Entries = append(l.Entries, theme.Entry{Name: b.Slug, Slug: b.Slug, Theme: b})
	}
	broken := theme.Entry{Name: "broken", Slug: "broken", Problem: &theme.Problem{Reason: "bad colour", Detail: "canvas = slate"}}
	l.Entries = append(l.Entries[:1], append([]theme.Entry{broken}, l.Entries[1:]...)...)
	return l
}

// fakeThemes lists its listing, counting each time, and keeps each choice
// it's given, or fails to with err.
type fakeThemes struct {
	listing theme.Listing
	lists   int
	kept    []theme.Choice
	err     error
}

func (f *fakeThemes) List() theme.Listing {
	f.lists++
	return f.listing
}

func (f *fakeThemes) Keep(c theme.Choice) error {
	if f.err != nil {
		return f.err
	}
	f.kept = append(f.kept, c)
	return nil
}

// quits reports whether msgs hold Bubble Tea's quit.
func quits(msgs []tea.Msg) bool {
	return slices.Contains(msgs, tea.Msg(tea.QuitMsg{}))
}

// hexOf is c written #RRGGBB, or "none".
func hexOf(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02X%02X%02X", r>>8, g>>8, b>>8)
}
