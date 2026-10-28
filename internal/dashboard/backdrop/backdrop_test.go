package backdrop_test

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/backdrop"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/theme"
)

// lightBackground and darkBackground are what a terminal says its
// background is.
var (
	lightBackground = color.RGBA{R: 0xFA, G: 0xFA, B: 0xFA, A: 0xff}
	darkBackground  = color.RGBA{R: 0x1A, G: 0x1B, B: 0x26, A: 0xff}
)

func TestAskAsksTheTerminalAndGivesItItsTime(t *testing.T) {
	type armed struct {
		d   time.Duration
		msg tea.Msg
	}
	var timers []armed
	after := func(d time.Duration, msg tea.Msg) tea.Cmd {
		timers = append(timers, armed{d: d, msg: msg})
		return func() tea.Msg { return msg }
	}

	batch, ok := backdrop.Ask(after)().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("Ask() sends %v, want a batch of the question and the wait", batch)
	}
	if got, want := batch[0](), tea.RequestBackgroundColor(); got != want {
		t.Errorf("Ask() first sends %#v, want the question of the terminal's background, %#v", got, want)
	}
	if len(timers) != 1 || timers[0] != (armed{d: theme.AnswerWithin, msg: backdrop.Unanswered{}}) {
		t.Errorf("Ask() arms %+v, want Unanswered once %v has passed", timers, theme.AnswerWithin)
	}
	if got := batch[1](); got != (backdrop.Unanswered{}) {
		t.Errorf("Ask()'s wait sends %#v, want Unanswered", got)
	}
}

func TestTheHalfOfThePairInForceGoesByTheTerminalsBackground(t *testing.T) {
	pair := defaultPair(t)
	tests := []struct {
		name string
		// answer is what the terminal says its background is, or nil for it
		// to say nothing in time.
		answer color.Color
		want   string
	}{
		{name: "a dark terminal", answer: darkBackground, want: pair.Dark.Slug},
		{name: "a light terminal", answer: lightBackground, want: pair.Light.Slug},
		{name: "a terminal that doesn't say: dark", want: pair.Dark.Slug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b backdrop.Backdrop
			if b.Settled() {
				t.Fatal("asked nothing yet, the backdrop is settled, want the screen blank")
			}
			b, afresh := settle(b, tt.answer, pair)

			if !b.Settled() || !afresh {
				t.Errorf("settled %v, to be drawn afresh %v, want both", b.Settled(), afresh)
			}
			if got := b.InForce(pair).Slug; got != tt.want {
				t.Errorf("InForce() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestALateAnswerIsStillHeard(t *testing.T) {
	pair := defaultPair(t)
	tests := []struct {
		name       string
		answer     color.Color
		wantAfresh bool
		want       string
	}{
		{name: "the other half: drawn in it afresh", answer: lightBackground, wantAfresh: true, want: pair.Light.Slug},
		{name: "the same half: nothing to draw afresh", answer: darkBackground, want: pair.Dark.Slug},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := backdrop.Backdrop{}.NoAnswer()
			b = b.DrawIn(b.InForce(pair))
			b, afresh := b.Answered(tt.answer, pair)

			if afresh != tt.wantAfresh {
				t.Errorf("a late answer has the screen drawn afresh %v, want %v", afresh, tt.wantAfresh)
			}
			if got := b.InForce(pair).Slug; got != tt.want {
				t.Errorf("InForce() = %s, want %s", got, tt.want)
			}
			if got, want := putBack(b), ansi.SetBackgroundColor(hex(tt.answer)); got != want {
				t.Errorf("PutBack() writes %q, want %q, the background the late answer said", got, want)
			}
		})
	}
}

func TestOnlyTheFirstAnswerCounts(t *testing.T) {
	pair := defaultPair(t)
	b, _ := backdrop.Backdrop{}.Answered(darkBackground, pair)
	b = b.DrawIn(b.InForce(pair))

	b, afresh := b.Answered(lightBackground, pair)
	if afresh || b.InForce(pair).Slug != pair.Dark.Slug {
		t.Errorf("a second answer has the screen drawn afresh %v in %s, want neither, %s still", afresh, b.InForce(pair).Slug, pair.Dark.Slug)
	}
	if got, want := putBack(b), ansi.SetBackgroundColor(hex(darkBackground)); got != want {
		t.Errorf("PutBack() writes %q, want the first answer's, %q", got, want)
	}
	if _, afresh := b.NoAnswer(); afresh {
		t.Error("the wait running out after an answer has the screen drawn afresh, want it heard as nothing")
	}
}

func TestTheLogSaysHowTheTerminalAnswered(t *testing.T) {
	log := logstest.Capture(t)
	b, _ := backdrop.Backdrop{}.NoAnswer()
	b.Answered(defaultPair(t).Dark.Colour(theme.Canvas), defaultPair(t))

	if !log.Has(`msg="terminal background"`, "answered=true", "painted=true", "late=true") {
		t.Errorf("the log reads\n%s\nwant the terminal's background, answered, painted and late", log)
	}
}

func TestTheBackgroundSetIsTheCanvasOfAThemeThatPaints(t *testing.T) {
	pair := defaultPair(t)
	terminal, _ := theme.Builtin(theme.Terminal)
	tests := []struct {
		name   string
		answer color.Color
		// drawn are the themes the screen is drawn in, in turn.
		drawn []theme.Theme
		want  color.Color
	}{
		{name: "a theme that paints: its canvas", answer: darkBackground, drawn: []theme.Theme{pair.Dark}, want: pair.Dark.Colour(theme.Canvas)},
		{name: "one that doesn't, nothing set: left alone", answer: darkBackground, drawn: []theme.Theme{terminal}},
		{name: "one that doesn't, after one that does: the background found", answer: darkBackground, drawn: []theme.Theme{pair.Dark, terminal}, want: darkBackground},
		{name: "one that doesn't, after one that does, the terminal not saying: reset", drawn: []theme.Theme{pair.Dark, terminal}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := settle(backdrop.Backdrop{}, tt.answer, pair)
			for _, drawn := range tt.drawn {
				b = b.DrawIn(drawn)
			}
			last := tt.drawn[len(tt.drawn)-1]

			if got := b.Background(dashboard.Screen(last)); hexOrNone(got) != hexOrNone(tt.want) {
				t.Errorf("Background() = %s, want %s", hexOrNone(got), hexOrNone(tt.want))
			}
		})
	}
}

func TestTheBackgroundIsPutBackAsItWas(t *testing.T) {
	lake := lakeTheme(t)
	pair := theme.Pair{Light: defaultPair(t).Light, Dark: lake}
	terminal, _ := theme.Builtin(theme.Terminal)
	tests := []struct {
		name   string
		answer color.Color
		// drawn are the themes the screen is drawn in, in turn, the terminal
		// asked first: none, as under NO_COLOR, asks it nothing.
		drawn []theme.Theme
		want  string
	}{
		{name: "set, and the terminal said what it was", answer: darkBackground, drawn: []theme.Theme{lake}, want: ansi.SetBackgroundColor("#1a1b26")},
		{name: "set, and the terminal didn't say", drawn: []theme.Theme{lake}, want: ansi.ResetBackgroundColor},
		{name: "set, the terminal saying a built-in's canvas", answer: pair.Light.Colour(theme.Canvas), drawn: []theme.Theme{pair.Light}, want: ansi.ResetBackgroundColor},
		{name: "set, the terminal saying the pair's canvas", answer: lake.Colour(theme.Canvas), drawn: []theme.Theme{lake}, want: ansi.ResetBackgroundColor},
		{name: "never set, drawn in the terminal's own colours", answer: darkBackground, drawn: []theme.Theme{terminal}},
		{name: "never set, nothing drawn in colour, as under NO_COLOR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b backdrop.Backdrop
			if tt.drawn != nil {
				b, _ = settle(b, tt.answer, pair)
			}
			for _, drawn := range tt.drawn {
				b = b.DrawIn(drawn)
			}

			if got := putBack(b); got != tt.want {
				t.Errorf("PutBack() writes %q, want %q", got, tt.want)
			}
		})
	}
}

// settle has the terminal say its background is answer, in pair, or, with
// nil, say nothing in time.
func settle(b backdrop.Backdrop, answer color.Color, pair theme.Pair) (backdrop.Backdrop, bool) {
	if answer == nil {
		return b.NoAnswer()
	}
	return b.Answered(answer, pair)
}

// defaultPair is the pair drawn in where the user chose none.
func defaultPair(t *testing.T) theme.Pair {
	t.Helper()
	return theme.Listing{}.Pair(theme.Choice{})
}

// lakeTheme is a theme of the user's: Nord's colours, but its canvas,
// #102030.
func lakeTheme(t *testing.T) theme.Theme {
	t.Helper()
	nord, _ := theme.Builtin("nord")
	var file strings.Builder
	for tok := theme.TextPrimary; tok <= theme.TextOnAttention; tok++ {
		colour := hexOrNone(nord.Colour(tok))
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

// putBack is what b writes putting the terminal's background back.
func putBack(b backdrop.Backdrop) string {
	var out strings.Builder
	b.PutBack(&out)
	return out.String()
}

// hex is c written #rrggbb, as the terminal's background is set.
func hex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// hexOrNone is c written #rrggbb, or "none".
func hexOrNone(c color.Color) string {
	if c == nil {
		return "none"
	}
	return hex(c)
}
