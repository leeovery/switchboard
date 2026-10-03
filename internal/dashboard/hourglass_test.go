package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// sketch is the glass's grains, a row a line: the outside as ".", its inside
// "o", its caps "=", its sand above "s", its pile "p", and its stream "|".
func sketch(g glass) []string {
	marks := map[grain]rune{outside: '.', empty: 'o', capped: '=', sand: 's', piled: 'p', falling: '|'}
	rows := make([]string, len(g.grains))
	for y, row := range g.grains {
		var b strings.Builder
		for _, gr := range row {
			b.WriteRune(marks[gr])
		}
		rows[y] = b.String()
	}
	return rows
}

func TestAnHourglassHasCapsFromFourRowsAndBulbsThatCurveInToItsNeck(t *testing.T) {
	tests := []struct {
		rows int
		want []string
	}{
		{rows: 2, want: []string{
			".oooooooooo.",
			".....oo.....",
			".....oo.....",
			".oooooooooo.",
		}},
		{rows: 3, want: []string{
			".oooooooooooooo.",
			"...oooooooooo...",
			".......oo.......",
			".......oo.......",
			"...oooooooooo...",
			".oooooooooooooo.",
		}},
		{rows: 4, want: []string{
			"====================",
			".oooooooooooooooooo.",
			"...oooooooooooooo...",
			".........oo.........",
			".........oo.........",
			"...oooooooooooooo...",
			".oooooooooooooooooo.",
			"====================",
		}},
	}
	for _, tt := range tests {
		if got := sketch(newGlass(glassCells(tt.rows), tt.rows)); !slices.Equal(got, tt.want) {
			t.Errorf("%d rows tall, the glass is\n%s\nwant\n%s", tt.rows, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
		}
	}
}

func TestAnHourglassHoldsTheRoomLeftAboveAndTheUsePiledBelow(t *testing.T) {
	g := newGlass(10, 4)
	g.pour(0.58)

	want := []string{
		"====================",
		".oooooooooooooooooo.",
		"...osssssssssssso...",
		".........ss.........",
		".........oo.........",
		"...ooooooppoooooo...",
		".pppppppppppppppppp.",
		"====================",
	}
	if got := sketch(g); !slices.Equal(got, want) {
		t.Errorf("42%% left and 58%% used, the glass is\n%s\nwant\n%s\n: the sand settled from the neck up and the pile from the foot, each the last of it in the middle", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAnHourglassStreamFallsAGrainAStep(t *testing.T) {
	poured := func(phase int) []string {
		g := newGlass(10, 4)
		g.pour(0.1)
		g.stream(4, phase)
		return sketch(g)[3:7]
	}
	tests := []struct {
		phase int
		want  []string
	}{
		{phase: 0, want: []string{
			".........ss.........",
			".........||.........",
			"...ooooo|o|oooooo...",
			".oooooooppp|ooooooo.",
		}},
		{phase: 1, want: []string{
			".........ss.........",
			".........|o.........",
			"...ooooo||||ooooo...",
			".ooooooopppoooooooo.",
		}},
	}
	for _, tt := range tests {
		if got := poured(tt.phase); !slices.Equal(got, tt.want) {
			t.Errorf("at phase %d, the stream is\n%s\nwant\n%s\n: four grains wide, its lines a grain out of step, each gap a grain lower than the phase before", tt.phase, strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
		}
	}
}

func TestAnHourglassHoldsEachGrainOnceWhereItsUseLandsOnAHalf(t *testing.T) {
	whole := newGlass(10, 4)
	g := newGlass(10, 4)
	g.pour(0.25)

	held := 0
	for _, row := range sketch(g) {
		held += strings.Count(row, "s") + strings.Count(row, "p")
	}
	if want := whole.inside(whole.top); held != want {
		t.Errorf("a quarter used, 8½ of its bulb's %d grains, the glass is\n%s\nholding %d grains of sand, want %d", want, strings.Join(sketch(g), "\n"), held, want)
	}
}

func TestAnHourglassStreamIsNeverWiderThanItsGlass(t *testing.T) {
	g := newGlass(2, 4)
	g.pour(0.1)
	g.stream(6, 0)

	if got := strings.Join(sketch(g), "\n"); !strings.Contains(got, "|") {
		t.Errorf("a stream six grains wide in a glass four wide, the glass is\n%s\nwant it falling down all four", got)
	}
}

func TestAnHourglassInANarrowChartDraws(t *testing.T) {
	a := readAccount("work", sessionOf(0.4, 3*time.Hour), weekOf(0.3, 4*day))
	usedLately(5)(&a)
	for width := range 12 {
		charting{style: Hourglass, doc: routerDoc("", 0, a), account: a, key: "5h", condition: status.Open, busy: true, width: width, rows: 4}.draw(t)
	}
}

func TestAnHourglassInALookThatCantBlendDrawsItsCapsOverItsSand(t *testing.T) {
	a := readAccount("work", sessionOf(0, 3*time.Hour), weekOf(0.3, 4*day))
	terminal := Screen(builtin(t, theme.Terminal))
	c := charting{style: Hourglass, doc: routerDoc("", 0, a), account: a, key: "5h", condition: status.Open, look: &terminal, width: 44, rows: 4}.draw(t)

	if got, want := c.rows(Look{})[0], strings.Repeat(" ", 13)+"▜████████▛"; got != want {
		t.Errorf("row 1 = %q, want %q: its cap and the sand under it as blocks, as no faded surface shows in a look that can't blend", got, want)
	}
}

func TestAnHourglassFallsWhereItShowsAlone(t *testing.T) {
	busyOn := func(id string) []status.Session {
		return []status.Session{{ID: "d28c5e17", Assignments: []status.Assignment{{Account: id, LastSeen: now.Add(-20 * time.Second).UTC()}}}}
	}
	help := []Key{{Key: "g", Does: "cycle the chart every card draws"}, {Key: "?", Does: "these keys, and the key to the glyphs"}}
	tests := []struct {
		name     string
		busy     string
		scrolled bool
		help     []Key
		want     bool
	}{
		{name: "a card in view", busy: "a", want: true},
		{name: "a card scrolled out of view", busy: "h"},
		{name: "scrolled into view", busy: "h", scrolled: true, want: true},
		{name: "a card under the help", busy: "b", help: help},
		{name: "a card beside the help", busy: "a", help: help, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 28)
			f.Chart, f.Sessions, f.Help = Hourglass, busyOn(tt.busy), tt.help
			doc := accountsOf(8, false)
			if tt.scrolled {
				f.Scroll = f.Scrolling(doc, now).Most
			}
			if got := f.Motion(doc, now).Falling; got != tt.want {
				t.Errorf("Falling = %v, want %v", got, tt.want)
			}
		})
	}
}

// glassAt is the plot of work's session, 40% used and resetting in 3 hours,
// so lasting to its reset at 20% an hour, its account as edit leaves it,
// where there's an edit, busy or not, at the moment given.
func glassAt(t *testing.T, edit func(*status.Account), busy bool, at time.Time) plot {
	t.Helper()
	a := readAccount("work", sessionOf(0.4, 3*time.Hour), weekOf(0.3, 4*day))
	if edit != nil {
		edit(&a)
	}
	w, _ := a.Window("5h")
	doc := routerDoc("", 0, a)
	f := Frame{Policy: claudeLike}
	fc := face{account: a, state: doc.StateOf(a, at, claudeLike), featured: standingOf(doc, a, w, at, claudeLike), busy: []bool{busy}}
	return f.plotOf(doc, fc, at)
}

// usedLately has the account's session used at rate an hour over the last
// half hour, as the router saw it.
func usedLately(rate float64) func(*status.Account) {
	return func(a *status.Account) {
		a.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: now.Add(-30 * time.Minute).UTC()}}
	}
}

func TestAnHourglassStreamIsAsThickAsItsRecentRate(t *testing.T) {
	tests := []struct {
		name string
		edit func(*status.Account)
		busy bool
		at   time.Time
		want int
	}{
		{name: "used at no rate, nor busy: none", edit: usedLately(0), want: 0},
		{name: "no rate known, and not busy: none", want: 0},
		{name: "no rate known, but busy: two grains", busy: true, want: 2},
		{name: "used at no rate, but busy: two grains", edit: usedLately(0), busy: true, want: 2},
		{name: "used no faster than lasts: two grains, still", edit: usedLately(0.2), want: 2},
		{name: "up to twice as fast: four", edit: usedLately(0.3), want: 4},
		{name: "more: six", edit: usedLately(0.5), want: 6},
		{name: "six at the most", edit: usedLately(5), busy: true, want: 6},
		{name: "held at its limit: none, whatever its rate", busy: true, edit: func(a *status.Account) {
			usedLately(0.5)(a)
			a.Limit = status.Limit{Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC()}
		}, want: 0},
		{name: "at the reserve that holds it back: none, the sand left kept", busy: true, edit: func(a *status.Account) {
			usedLately(0.5)(a)
			a.Reserve = 0.6
		}, want: 0},
		{name: "reset since it was read: none", edit: usedLately(0.5), busy: true, at: now.Add(3*time.Hour + time.Minute), want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := tt.at
			if at.IsZero() {
				at = now
			}
			if got := glassAt(t, tt.edit, tt.busy, at).streamWide(); got != tt.want {
				t.Errorf("streamWide() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAnHourglassStreamFallsWhileItsAccountIsBusyAndIsStillWhileItIsnt(t *testing.T) {
	for _, busy := range []bool{true, false} {
		phases := make(map[int]bool)
		for step := range grainCycle {
			p := glassAt(t, usedLately(0.3), busy, now.Add(time.Duration(step)*FallStep))
			phases[p.phase()] = true
			if p.falls() != busy {
				t.Errorf("busy %v, falls() = %v, want %v", busy, p.falls(), busy)
			}
			if wide := p.streamWide(); wide != 4 {
				t.Errorf("busy %v, the stream is %d grains wide, want 4, falling or still", busy, wide)
			}
		}
		if want := map[bool]int{true: grainCycle, false: 1}[busy]; len(phases) != want {
			t.Errorf("busy %v, a step apart the stream stood at %d phases, want %d", busy, len(phases), want)
		}
	}
}

func TestAnHourglassStandsOverNow(t *testing.T) {
	a, trail := halfway()
	c := charting{style: Hourglass, doc: routerDoc("", 0, a), account: a, key: "5h", history: History{{Account: "work", Window: "5h"}: trail}, condition: status.Open, width: 44, rows: 4}.draw(t)

	rows := c.rows(Look{})
	if got, want := rows[0], strings.Repeat(" ", 18)+"▜████████▛"; got != want {
		t.Errorf("row 1 = %q, want %q: the glass, its cap over its empty top, ten cells wide, its middle at the column for now", got, want)
	}
	if got, want := rows[4], "10:42                now               15:42"; got != want {
		t.Errorf("its axis reads %q, want %q, as a burn-down's", got, want)
	}
	piled := c.at(21, 3)
	if want := (ink{token: theme.StatePositive, fade: projectionFade, on: glassInk.hue()}); piled.glyph != "▀" || piled.ink != want {
		t.Errorf("the pile on the glass's foot is %q in %+v, want ▀ in %+v: the state's colour faded halfway, on the glass", piled.glyph, piled.ink, want)
	}
	if edge := c.at(20, 1); edge.glyph != "▀" || edge.ink.on != (hue{}) {
		t.Errorf("the sand by the bulb's wall is %q on %+v, want ▀ on the canvas, which shows past the glass", edge.glyph, edge.ink.on)
	}
}

func TestAnHourglassTurnsOverAtItsReset(t *testing.T) {
	p := glassAt(t, usedLately(0.3), true, now.Add(3*time.Hour+time.Minute))
	g := p.glass(10, 4)

	for y, row := range sketch(g) {
		if top, bottom := slices.Contains(g.top, y), slices.Contains(g.bottom, y); top && strings.Contains(row, "o") || bottom && strings.ContainsAny(row, "p|") {
			t.Errorf("reset since it was read, the glass is\n%s\nwant it turned over: all its sand above, none piled, nothing falling", strings.Join(sketch(g), "\n"))
			break
		}
	}
	a := readAccount("work", sessionOf(0.4, 3*time.Hour))
	c := charting{style: Hourglass, doc: routerDoc("", 0, a), account: a, key: "5h", condition: status.Open, at: now.Add(3*time.Hour + time.Minute), width: 44, rows: 4}.draw(t)
	if got, want := c.rows(Look{})[0], "▗▄▄▄▄▄▄▄▄▖"; got != want {
		t.Errorf("row 1 = %q, want %q: the glass at the chart's start, as its window starts again, full under its cap", got, want)
	}
}

func TestAnHourglassHeldAtItsLimitHasAllItsSandPiledBelow(t *testing.T) {
	p := glassAt(t, func(a *status.Account) {
		usedLately(0.5)(a)
		a.Limit = status.Limit{Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC()}
	}, true, now)
	g := p.glass(10, 4)

	if got := sketch(g); strings.Contains(strings.Join(got[:4], ""), "s") || strings.Contains(strings.Join(got[4:], ""), "o") {
		t.Errorf("held at its limit, the glass is\n%s\nwant nothing above, and all its sand piled below, with no stream", strings.Join(got, "\n"))
	}
}

func TestALapsedWindowsHourglassStandsFullAtItsStartSayingWhenItStarts(t *testing.T) {
	a := readAccount("work", quota.Window{Key: "5h", Label: "Session"}, weekOf(0.3, 4*day))
	a.Lapsed = []string{"5h"}
	doc := routerDoc("", 0, a)
	doc.Prime = status.Prime{Window: "5h", Slots: []status.Slot{{Account: "work", At: "14:20", Next: now.Add(68 * time.Minute).UTC()}}}
	c := charting{style: Hourglass, doc: doc, account: a, key: "5h", condition: status.Idle, width: 60, rows: 4}.draw(t)

	want := []string{
		"▗▄▄▄▄▄▄▄▄▖",
		" ▝▀▀▜▛▀▀▘",
		" ▗▄▄▟▙▄▄▖      full · window starts at its prime, 14:20",
		"▟████████▙",
		"",
	}
	if got := c.rows(Look{}); !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: the glass at the chart's start, all its sand above, still, saying when it starts, and no axis, the window not running", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := c.at(4, 1).ink; got != faintLevelInk {
		t.Errorf("its sand is in %+v, want dim", got)
	}
}

func TestAnHourglassWithoutColourDrawsItsGlassAsTheTrack(t *testing.T) {
	a := readAccount("work", sessionOf(0.4, 3*time.Hour), weekOf(0.3, 4*day))
	colourless := NoColour()
	c := charting{style: Hourglass, doc: routerDoc("", 0, a), account: a, key: "5h", condition: status.Open, look: &colourless, width: 44, rows: 4}.draw(t)

	want := []string{
		strings.Repeat(" ", 13) + "▀▀▀▀██▀▀▀▀",
		strings.Repeat(" ", 14) + "▝▀▀▜▛▀▀▘",
		strings.Repeat(" ", 15) + "░░░░░░",
		strings.Repeat(" ", 13) + "▄▟██████▙▄",
	}
	if got := c.rows(Look{})[:4]; !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s\n: its caps, its sand and its pile as blocks, and the empty inside of its bottom bulb as the bars' track", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestFallingSaysWhetherAnHourglassFallsOnACard(t *testing.T) {
	busy := []status.Session{{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: now.Add(-20 * time.Second).UTC()}}}}
	idle := []status.Session{{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: now.Add(-9 * time.Minute).UTC()}}}}
	tests := []struct {
		name     string
		view     View
		style    Chart
		sessions []status.Session
		flipped  bool
		want     bool
	}{
		{name: "an hourglass, its account busy", view: Accounts, style: Hourglass, sessions: busy, want: true},
		{name: "an hourglass, its account idle", view: Accounts, style: Hourglass, sessions: idle},
		{name: "an hourglass, its card flipped", view: Accounts, style: Hourglass, sessions: busy, flipped: true},
		{name: "a burn rate", view: Accounts, style: BurnRate, sessions: busy},
		{name: "Sessions shown", view: Sessions, style: Hourglass, sessions: busy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 40)
			f.View, f.Chart, f.Sessions, f.Flipped = tt.view, tt.style, tt.sessions, map[string]bool{"work": tt.flipped}
			doc := routerDoc("", 1, readAccount("work", sessionOf(0.4, 3*time.Hour), weekOf(0.3, 4*day)))
			if got := f.Motion(doc, now).Falling; got != tt.want {
				t.Errorf("Falling() = %v, want %v", got, tt.want)
			}
		})
	}
}
