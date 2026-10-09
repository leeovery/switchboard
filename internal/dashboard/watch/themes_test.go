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

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/readings"
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
	if !ok || tm.delay != theme.AnswerWithin {
		t.Fatalf("no wait armed for the terminal to say its background in %v", theme.AnswerWithin)
	}
	h.deliver(tea.BackgroundColorMsg{Color: darkBackground})
	if got := h.view(); !strings.Contains(got, "╭─ 1 Work ") {
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

func TestALateAnswerIsTakenAsTheBackgroundFound(t *testing.T) {
	tests := []struct {
		name   string
		answer color.Color
		want   string
	}{
		{name: "the other half: the screen drawn in it", answer: lightBackground, want: "tokyo-night-day"},
		{name: "the same half", answer: darkBackground, want: "nord"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := themedHarness(t, theme.Choice{})
			h.start()
			h.answer(nil)
			h.deliver(tea.BackgroundColorMsg{Color: tt.answer})

			if got := h.drawnIn(); got != tt.want {
				t.Errorf("an answer after the dashboard was drawn has it drawn in %s, want %s", got, tt.want)
			}
			if got := hexOf(h.model.View().BackgroundColor); got != hexOf(h.model.showing.Colour(theme.Canvas)) {
				t.Errorf("the terminal's background is set to %s, want the canvas of the theme it's drawn in", got)
			}
			var out strings.Builder
			h.model.backdrop.putBack(&out)
			if want := ansi.SetBackgroundColor(rgbHex(tt.answer)); out.String() != want {
				t.Errorf("putting the background back writes %q, want %q, the one the terminal said, asked before the canvas was set", out.String(), want)
			}
		})
	}
}

func TestOnlyTheAnswerToTheOneQuestionIsTaken(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.Choice{})
	h.deliver(tea.BackgroundColorMsg{Color: lightBackground})

	if got := h.drawnIn(); got != "nord" {
		t.Errorf("a second report of the background has the screen drawn in %s, want nord still", got)
	}
	if got := hexOf(h.model.backdrop.original); got != hexOf(darkBackground) {
		t.Errorf("the background to put back is %s, want the first answer's, %s", got, hexOf(darkBackground))
	}
}

func TestACanvasOfTheDashboardsOwnIsNeverSetBack(t *testing.T) {
	nord, _ := theme.Builtin("nord")
	day, _ := theme.Builtin("tokyo-night-day")
	lake := lakeTheme(t)
	tests := []struct {
		name   string
		choice theme.Choice
		answer color.Color
		want   string
	}{
		{name: "the dark default's", answer: nord.Colour(theme.Canvas), want: "nord"},
		{name: "the light default's", answer: day.Colour(theme.Canvas), want: "tokyo-night-day"},
		{name: "a built-in's not chosen", choice: theme.One("amber"), answer: day.Colour(theme.Canvas), want: "amber"},
		{name: "the theme in force's", choice: theme.One("lake"), answer: lake.Colour(theme.Canvas), want: "lake"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, themes := themedHarness(t, tt.choice)
			themes.listing.Entries = append(themes.listing.Entries, theme.Entry{Name: "lake", Slug: "lake", Theme: lake})
			h.model.pair = themes.listing.Pair(tt.choice)
			h.start()
			h.answer(tt.answer)

			if got := h.drawnIn(); got != tt.want {
				t.Errorf("drawn in %s, want %s, the half of the pair as dark as the colour said", got, tt.want)
			}
			if h.model.backdrop.original != nil {
				t.Errorf("the background to set back is %s, want none, the canvas maybe an exit's the dashboard couldn't catch", hexOf(h.model.backdrop.original))
			}
			var out strings.Builder
			h.model.backdrop.putBack(&out)
			if out.String() != ansi.ResetBackgroundColor {
				t.Errorf("putting the background back writes %q, want it reset, %q", out.String(), ansi.ResetBackgroundColor)
			}
		})
	}
}

func TestALightProfileLikeTokyoNightDaysCanvasIsDrawnLight(t *testing.T) {
	day, _ := theme.Builtin("tokyo-night-day")
	h, _ := themedHarness(t, theme.Choice{})
	h.start()
	h.answer(day.Colour(theme.Canvas))

	if got := h.drawnIn(); got != "tokyo-night-day" {
		t.Errorf("a light terminal whose background is tokyo-night-day's canvas is drawn in %s, want the light half, tokyo-night-day", got)
	}
}

func TestATerminalThemeShownMidwayPutsBackTheBackgroundFound(t *testing.T) {
	tests := []struct {
		name   string
		answer color.Color
		want   string
	}{
		{name: "the terminal said its background: that, set again", answer: darkBackground, want: hexOf(darkBackground)},
		{name: "the terminal didn't say: reset", want: "none"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := themedHarness(t, theme.Choice{})
			h.start()
			h.answer(tt.answer)
			h.typed("t")
			h.typed("down")

			if h.drawnIn() != theme.Terminal {
				t.Fatalf("the cursor moved down from nord to %s, want terminal", h.drawnIn())
			}
			if got := hexOf(h.model.View().BackgroundColor); got != tt.want {
				t.Errorf("shown the terminal's theme, the terminal's background is set to %s, want %s", got, tt.want)
			}
		})
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
	h.typed("t")
	if h.model.picker.open {
		t.Error("without colour, t opened the theme picker, want it to do nothing")
	}
}

func TestTOpensThePickerOverTheView(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})

	if strings.Contains(h.footer(), "t themes") {
		t.Errorf("the footer reads %q, want t left for the help to list", h.footer())
	}
	h.typed("t")

	view := h.view()
	for _, want := range []string{"│ Themes", "│ ▌ nord", "● dark", "● light", "│ ⏎    set theme", "broken"} {
		if !strings.Contains(view, want) {
			t.Errorf("with the picker open, the screen reads\n%s\nwant %q", view, want)
		}
	}
	if !strings.Contains(view, "╭─ 1 Work ") {
		t.Errorf("with the picker open, the screen reads\n%s\nwant the view beside it", view)
	}
	h.typed("esc")
	h.typed("t")
	if themes.lists != 2 {
		t.Errorf("the themes were listed %d times, want afresh each time the picker opens", themes.lists)
	}
}

func TestThePickerShowsEachThemeAsTheCursorReachesIt(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})
	h.typed("t")

	var reached []string
	for range 7 {
		h.typed("down")
		reached = append(reached, h.drawnIn())
	}
	if want := []string{"terminal", "tokyo-night", "tokyo-night-day", "tokyo-night-day", "tokyo-night-day", "tokyo-night-day", "tokyo-night-day"}; !slices.Equal(reached, want) {
		t.Errorf("moving down from nord, the screen is drawn in %q, want %q, stopping at the last", reached, want)
	}
	for range 7 {
		h.typed("up")
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
	h.typed("t")
	h.typed("up")
	h.typed("up")
	h.typed("enter")

	if want := []theme.Choice{theme.One("amber")}; !slices.Equal(themes.kept, want) {
		t.Errorf("kept %+v, want %+v", themes.kept, want)
	}
	if !h.model.picker.open || h.drawnIn() != "amber" {
		t.Errorf("after enter, the picker is open: %v, the screen in %s; want it open, on amber", h.model.picker.open, h.drawnIn())
	}
	if view := h.view(); !strings.Contains(view, "▌ amber") || !strings.Contains(view, "●") || strings.Contains(view, "● dark") {
		t.Errorf("the screen reads\n%s\nwant amber badged as the one theme, the pair's badges gone", view)
	}
	h.typed("esc")
	if got := h.drawnIn(); got != "amber" {
		t.Errorf("closed, the screen is drawn in %s, want amber, now in force", got)
	}
}

func TestDAndLSetTheHalvesOfThePair(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})
	h.typed("t")
	h.typed("down")
	h.typed("d")
	h.typed("down")
	h.typed("l")

	want := []theme.Choice{{Dark: "terminal"}, {Light: "tokyo-night", Dark: "terminal"}}
	if !slices.Equal(themes.kept, want) {
		t.Errorf("kept %+v, want %+v", themes.kept, want)
	}
	if view := h.view(); !strings.Contains(view, "terminal") || !strings.Contains(view, "● dark") || !strings.Contains(view, "● light") {
		t.Errorf("the screen reads\n%s\nwant the halves badged", view)
	}
	h.typed("esc")
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
			h.typed("t")
			h.typed("down")
			h.typed("down")
			h.typed("l")

			if view := h.view(); !strings.Contains(view, "clear amber?  y / n") || !strings.Contains(view, "y    confirm") {
				t.Errorf("setting a half over one theme, the screen reads\n%s\nwant it to ask first", view)
			}
			for _, key := range tt.answers {
				h.typed(key)
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
	h.typed("t")
	h.typed("up")
	h.typed("up")
	if got := h.drawnIn(); got == "exchange" {
		t.Fatalf("moving the cursor left the screen in %s, want another theme previewed", got)
	}
	h.typed("esc")

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
	h.typed("t")
	h.typed("down")
	h.typed("enter")

	if view := h.view(); !strings.Contains(view, "⚠ not kept: see the log") {
		t.Errorf("the screen reads\n%s\nwant the picker to say the choice wasn't kept", view)
	}
	if got := h.model.choice; got != (theme.Choice{}) {
		t.Errorf("the choice is %+v, want it as it was", got)
	}
	if !log.Has("level=WARN", `msg="couldn't keep the theme chosen"`, `error="read-only file system"`) {
		t.Errorf("the log reads\n%s\nwant it to say why the choice wasn't kept", log)
	}
	h.typed("down")
	if strings.Contains(h.view(), "not kept") {
		t.Error("the note outlasted the next key")
	}
}

func TestAPickIsSetInTheChoiceAsItStandsNow(t *testing.T) {
	tests := []struct {
		name string
		// was is the choice the dashboard started with, and now what another
		// dashboard kept since.
		was, now theme.Choice
		keys     []string
		// asks is set where setting the half asks first to clear one theme.
		asks bool
		want []theme.Choice
	}{
		{
			name: "a half, beside the other half kept since",
			now:  theme.Choice{Light: "exchange"},
			keys: []string{"t", "down", "d"},
			want: []theme.Choice{{Light: "exchange", Dark: "terminal"}},
		},
		{
			name: "a half, over one theme kept since, asking first",
			now:  theme.One("amber"),
			keys: []string{"t", "down", "l", "y"},
			asks: true,
			want: []theme.Choice{{Light: "terminal"}},
		},
		{
			name: "a half, over one theme cleared since, asking nothing",
			was:  theme.One("amber"),
			now:  theme.Choice{Dark: "nord"},
			keys: []string{"t", "down", "l"},
			want: []theme.Choice{{Light: "exchange", Dark: "nord"}},
		},
		{
			name: "one theme, the pair kept since cleared",
			now:  theme.Choice{Light: "exchange", Dark: "amber"},
			keys: []string{"t", "down", "enter"},
			want: []theme.Choice{theme.One("terminal")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, themes := settledThemedHarness(t, tt.was)
			themes.choice = tt.now
			var asked bool
			for _, key := range tt.keys {
				h.typed(key)
				asked = asked || strings.Contains(h.view(), "y / n")
			}

			if !slices.Equal(themes.kept, tt.want) {
				t.Errorf("kept %+v, want %+v", themes.kept, tt.want)
			}
			if asked != tt.asks {
				t.Errorf("asked to clear one theme: %v, want %v, as the choice kept stands now", asked, tt.asks)
			}
			if got, want := h.model.choice, themes.choice; got != want {
				t.Errorf("drawn by %+v, want %+v, the choice as kept", got, want)
			}
		})
	}
}

func TestThePickerReadsAndKeepsOutsideUpdate(t *testing.T) {
	h, themes := settledThemedHarness(t, theme.Choice{})
	for _, key := range []tea.KeyPressMsg{{Code: 't', Text: "t"}, {Code: tea.KeyDown}, {Code: tea.KeyEnter}} {
		lists, kept := themes.lists, len(themes.kept)
		next, cmd := h.model.Update(key)
		if themes.lists != lists || len(themes.kept) != kept {
			t.Errorf("%s listed the themes or kept a choice in Update, which blocks every key and frame till it's done, want it done in a command", key)
		}
		h.model = next.(Model)
		h.deliver(h.run(cmd)...)
	}
	if want := []theme.Choice{theme.One("terminal")}; themes.lists != 1 || !slices.Equal(themes.kept, want) {
		t.Errorf("listed %d times, kept %+v; want the themes listed once, and %+v kept, once each command ran", themes.lists, themes.kept, want)
	}
}

func TestWhileThePickerIsOpenTheFooterListsItsKeysAlone(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.One("amber"))
	h.typed("t")
	if got, want := listedKeys(h.model.keys()), []string{"↑↓", "⏎", "d", "l", "esc", "q"}; !slices.Equal(got, want) {
		t.Errorf("with the picker open, the footer lists %q, want the picker's keys alone, %q", got, want)
	}
	h.typed("down", "l")
	if got, want := listedKeys(h.model.keys()), []string{"y", "n", "q"}; !slices.Equal(got, want) {
		t.Errorf("with the picker asking, the footer lists %q, want its answers alone, %q", got, want)
	}
	h.typed("n", "esc")
	if got := listedKeys(h.model.keys()); !slices.Contains(got, "?") {
		t.Errorf("the picker closed, the footer lists %q, want the dashboard's keys back", got)
	}
}

func TestThePickerTakesEveryKeyButQuit(t *testing.T) {
	h, _ := settledThemedHarness(t, theme.Choice{})
	h.typed("t")
	reads := len(h.source.asked)
	for _, key := range []string{"r", "1", "a", "m", "t"} {
		h.typed(key)
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
		h.typed("t")
		if h.model.picker.open || h.model.note != tooSmall {
			t.Errorf("at %d×%d, t opened the picker: %v, the footer saying %q; want it closed, saying %q", size.Width, size.Height, h.model.picker.open, h.model.note, tooSmall)
		}
	}

	h, _ := settledThemedHarness(t, theme.Choice{})
	h.typed("t")
	h.typed("down")
	h.update(tea.WindowSizeMsg{Width: 150, Height: 8})
	if h.model.picker.open || h.drawnIn() != "nord" || !strings.Contains(h.footer(), tooSmall) {
		t.Errorf("shrunk too short, the picker is open: %v, the screen in %s, the footer reading %q; want it closed, the theme in force put back, saying why",
			h.model.picker.open, h.drawnIn(), h.footer())
	}
}

func TestThePickerWaitsForTheTerminalsBackground(t *testing.T) {
	h, _ := themedHarness(t, theme.Choice{})
	h.start()
	h.typed("t")

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
	themes := &fakeThemes{listing: listing(), choice: choice}
	h := &harness{t: t, clock: &fakeClock{now: start}, source: &fakeSource{doc: calm()}, notifier: &fakeNotifier{}}
	h.model = New(t.Context(), Config{
		Source: h.source, Notifier: h.notifier, Notifications: notifications, Now: h.clock.Now, After: h.arm, Interval: interval, Policy: policy,
		Size: Size{Width: 150, Height: 50}, Choice: choice, Pair: themes.listing.Pair(choice), Themes: themes,
		Ledger: h.emptyLedger(), Readings: readings.Empty{}, Events: events.Empty{},
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

// typed presses each key in turn, as press does, delivering what each sends
// back: the themes listed, a question asked, or a choice kept.
func (h *harness) typed(keys ...string) {
	h.t.Helper()
	for _, key := range keys {
		h.deliver(h.press(key)...)
	}
}

// lakeTheme is a theme of the user's: Nord's colours, but its canvas,
// #102030.
func lakeTheme(t *testing.T) theme.Theme {
	t.Helper()
	nord, _ := theme.Builtin("nord")
	var file strings.Builder
	for tok := theme.TextPrimary; tok <= theme.TextOnAttention; tok++ {
		colour := hexOf(nord.Colour(tok))
		if tok == theme.Canvas {
			colour = "#102030"
		}
		fmt.Fprintf(&file, "%s = %s\n", tok, colour)
	}
	lake, err := theme.Parse("lake", []byte(file.String()))
	if err != nil {
		t.Fatal(err)
	}
	return lake
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

// fakeThemes lists its listing, counting each time, and keeps its choice,
// which another dashboard may change, changed as each change it's given
// says, noting what it kept, or fails to with err.
type fakeThemes struct {
	listing theme.Listing
	lists   int
	choice  theme.Choice
	kept    []theme.Choice
	err     error
}

func (f *fakeThemes) List() theme.Listing {
	f.lists++
	return f.listing
}

func (f *fakeThemes) Chosen() theme.Choice {
	return f.choice
}

func (f *fakeThemes) Keep(change func(theme.Choice) theme.Choice) (theme.Choice, error) {
	if f.err != nil {
		return theme.Choice{}, f.err
	}
	f.choice = change(f.choice)
	f.kept = append(f.kept, f.choice)
	return f.choice, nil
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
