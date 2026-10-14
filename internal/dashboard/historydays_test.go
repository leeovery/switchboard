package dashboard

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"unique"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// stripOf is a strip's eight weeks' days at the levels the digits give, a
// digit a day from the first, those past them at level 0, and a 5-hour limit
// on each day of those limited.
func stripOf(levels string, limited ...int) []views.StripDay {
	days := make([]views.StripDay, 56)
	for i, level := range levels {
		days[i].Level = int(level - '0')
	}
	for _, i := range limited {
		days[i].Limit = true
	}
	return days
}

// busy are a strip's days but the three to come, each active.
var busy = strings.Repeat("1234", 13) + "1"

// daysWorld is Days as history-days-160 draws it: the eight weeks from 10
// August, Friday to Sunday to come; the four families' shares; and the
// accounts given.
func daysWorld(accounts ...views.DaysStrip) views.DaysPage {
	return daysShared([4]float64{0.59, 0.37, 0.02, 0.02}, accounts...)
}

// daysShared is daysWorld with the four families' shares given, Opus's to
// Haiku's, the accounts given unlabelled, as their frames name them by id.
func daysShared(shares [4]float64, accounts ...views.DaysStrip) views.DaysPage {
	families := make([]views.DaysStrip, len(shares))
	for i, family := range []string{"opus", "sonnet", "fable", "haiku"} {
		families[i] = views.DaysStrip{Of: family, Days: stripOf(busy), Share: &shares[i]}
	}
	weeks := []string{"2026-08-10", "2026-08-17", "2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28"}
	return views.DaysPage{Weeks: weeks, ToCome: 3, ByModel: families, ByAccount: accounts}
}

// The accounts the frames draw, each with its 5-hour limits on the days the
// frames mark them.
var (
	workDays     = views.DaysStrip{Of: "work", Limits: 33, Days: stripOf(busy, 0, 2, 4, 7, 9, 10, 11, 14, 15, 16, 17, 18, 21, 22, 23, 24, 25, 28, 29, 30, 31, 32, 35, 36, 37, 39, 42, 43, 44, 45, 46, 50, 52)}
	personalDays = views.DaysStrip{Of: "personal", Primary: true, Limits: 11, Days: stripOf(busy, 0, 7, 10, 22, 25, 30, 35, 37, 39, 42, 45)}
	sideDays     = views.DaysStrip{Of: "side", Limits: 4, Days: stripOf(busy, 15, 30, 45, 50)}
	tenDays      = []views.DaysStrip{
		workDays, personalDays, sideDays,
		{Of: "client", Limits: 2, Days: stripOf(busy, 25, 45)},
		{Of: "spare", Days: stripOf(busy)},
		{Of: "lab", Limits: 13, Days: stripOf(busy, 0, 7, 10, 15, 25, 29, 30, 32, 35, 37, 42, 45, 50)},
		{Of: "extra", Days: stripOf(busy)}, {Of: "team", Days: stripOf(busy)}, {Of: "ops", Days: stripOf(busy)}, {Of: "test", Days: stripOf(busy)},
	}
)

// daysFrame is a frame of History's Days of the page given, in a terminal
// of the size given, in nord.
func daysFrame(t *testing.T, page views.DaysPage, width, height int) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.View, f.Look = History, Screen(builtin(t, "nord"))
	f.History = HistoryPage{Shown: HistoryDays, Read: true, Days: &page}
	return f
}

// Days draws its frames' text, from the weeks' first days on row 7 to the
// key on row 35. The frames' ▲ after side, where new sessions go, is
// routing, which History never shows, as the design holds: it's passed over.
func TestDaysDrawsItsFramesText(t *testing.T) {
	tests := []struct {
		stem string
		page views.DaysPage
	}{
		{stem: "history-days-160", page: daysWorld(workDays, personalDays, sideDays)},
		{stem: "history-days-ten-160", page: daysShared([4]float64{0.57, 0.39, 0.01, 0.03}, tenDays...)},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := daysFrame(t, tt.page, 160, 37)
			drawn, frame := textOf(historyDrawn(f).rows(f.Look)), referenceRows(t, tt.stem, "txt")
			unmarked := strings.NewReplacer(" ▲", "  ")
			for y := 6; y < 35; y++ {
				want := strings.TrimRight(unmarked.Replace(frame[y]), " ")
				if drawn[y] != want {
					t.Errorf("row %d reads\n%q\nwant\n%q", y+1, drawn[y], want)
				}
			}
		})
	}
}

// Days draws its frames cell for cell, in nord, from row 7 to the key on row
// 35: but the days of each strip, whose levels are the world's, which F's
// fixtures hold; and the two cells of the ▲ after side, where new sessions
// go, which is routing, and History shows no routing, as the design holds.
func TestDaysHoldsToItsFramesCells(t *testing.T) {
	tests := []struct {
		stem string
		page views.DaysPage
	}{
		{stem: "history-days-160", page: daysWorld(workDays, personalDays, sideDays)},
		{stem: "history-days-ten-160", page: daysShared([4]float64{0.57, 0.39, 0.01, 0.03}, tenDays...)},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := daysFrame(t, tt.page, 160, 37)
			passed := []spot{{row: 25, first: 4, last: 5}}
			for _, row := range []int{10, 12, 14, 16, 21, 23, 25, 27, 29, 31} {
				passed = append(passed, spot{row: row, first: daysAt, last: 126})
			}
			drawn := screenOf(t, historyDrawn(f).rows(f.Look))
			heldTo(t, tt.stem, reference(t, tt.stem), drawn, region{top: 6, bottom: 34, left: 0, right: 159}, shadeOf(f.Look.Canvas()), passed, nil)
		})
	}
}

// A family's days take its own colour in four steps, and an account's the
// days' shades, as the frames draw them in nord.
func TestDaysShadesEachStripsDays(t *testing.T) {
	f := daysFrame(t, daysWorld(workDays), 160, 37)
	drawn := screenOf(t, historyDrawn(f).rows(f.Look))
	tests := []struct {
		name string
		y    int
		want []string
	}{
		{name: "Opus's steps", y: 10, want: []string{"#495B76", "#546E91", "#5E81AC", "#B7C6DA"}},
		{name: "work's shades", y: 21, want: []string{"#44587A", "#5E81AC", "#88C0D0", "#D8EEF5"}},
	}
	for _, tt := range tests {
		for level, want := range tt.want {
			if got := seenAt(drawn, daysAt+swatchCells*level+1, tt.y).bg.String(); got != want {
				t.Errorf("%s: level %d is on %s, want %s", tt.name, level+1, got, want)
			}
		}
	}
}

// Each part of Days is in the ink the design gives it.
func TestDaysDrawsEachPartInItsInk(t *testing.T) {
	f := daysFrame(t, daysWorld(workDays, personalDays, sideDays, views.DaysStrip{Of: "spare", Days: stripOf("0")}), 160, 37)
	c := historyDrawn(f)
	opus := familyHue("opus")
	limitOn := func(level int) ink { return ink{token: theme.StateDestructive, on: dayInk(level).hue()} }
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "a week's first day", x: 14, y: 6, want: faintInk},
		{name: "this week's", x: 119, y: 6, want: strongInk},
		{name: "this week's days' initials", x: 121, y: 7, want: faintInk},
		{name: "BY MODEL", x: 0, y: 8, want: dimInk},
		{name: "what's on one scale", x: 10, y: 8, want: faintInk},
		{name: "a family's dot", x: 0, y: 10, want: ink{token: theme.VizFamily1}},
		{name: "a family's name", x: 2, y: 10, want: secondaryInk},
		{name: "a family's day, its first cell, in its steps", x: 14, y: 10, want: ink{on: stepInk(opus, 1).hue()}},
		{name: "its second cell", x: 15, y: 10, want: ink{on: stepInk(opus, 1).hue()}},
		{name: "its busiest", x: 20, y: 10, want: ink{on: stepInk(opus, 4).hue()}},
		{name: "a day to come", x: 127, y: 10, want: faintInk},
		{name: "a family's share", x: 137, y: 10, want: strongInk.numbered(Number{Kind: NumberShareOfUse, Of: "opus"})},
		{name: "of use", x: 141, y: 10, want: dimInk},
		{name: "BY ACCOUNT", x: 0, y: 19, want: dimInk},
		{name: "an account's name", x: 0, y: 21, want: titleInk},
		{name: "the primary's ◆", x: 9, y: 23, want: primaryInk},
		{name: "a day it hit a 5-hour limit, ▪ over its shade", x: 14, y: 21, want: limitOn(1)},
		{name: "the rest of that day", x: 15, y: 21, want: ink{on: dayInk(1).hue()}},
		{name: "an account's day in the days' shades", x: 16, y: 21, want: ink{on: dayInk(2).hue()}},
		{name: "an account's 5-hour limits", x: 138, y: 21, want: exhaustedInk.numbered(Number{Kind: NumberLimitsHit, Account: "work", Of: "5h"})},
		{name: "5-hour limits", x: 141, y: 21, want: dimInk},
		{name: "a day with nothing on it", x: 14, y: 27, want: ink{on: dayInk(0).hue()}},
		{name: "none hit", x: 139, y: 27, want: ink{token: theme.TextFaint, bold: true}.numbered(Number{Kind: NumberLimitsHit, Account: "spare", Of: "5h"})},
		{name: "the key's less", x: 0, y: 34, want: dimInk},
		{name: "the key's first shade", x: 5, y: 34, want: ink{on: dayInk(0).hue()}},
		{name: "its last", x: 18, y: 34, want: ink{on: dayInk(4).hue()}},
		{name: "what the shades are", x: 26, y: 34, want: faintInk},
		{name: "a day it hit a limit", x: 69, y: 34, want: limitOn(2)},
		{name: "what it is", x: 72, y: 34, want: dimInk},
		{name: "the days to come", x: 102, y: 34, want: faintInk},
		{name: "what they are", x: 105, y: 34, want: dimInk},
	}
	for _, tt := range tests {
		want, marked := tt.want, tt.want.mark
		want.mark = unique.Handle[mark]{}
		if got := c.at(tt.x, tt.y); got.ink != want || got.mark != marked {
			t.Errorf("%s, column %d of row %d, is %q in %+v, want %+v", tt.name, tt.x, tt.y+1, got.glyph, got.ink, tt.want)
		}
	}
}

// Where the look blends nothing, as the terminal theme and NO_COLOR, a day's
// level is told by its glyph's density, ·░▒▓█, in its shade's colour, a red
// ▪ in its first cell on a day it hit a 5-hour limit.
func TestDaysToldByGlyphsWhereTheLookBlendsNothing(t *testing.T) {
	page := daysWorld(views.DaysStrip{Of: "work", Days: stripOf("01234", 2)})
	for _, look := range []Look{NoColour(), Screen(builtin(t, "terminal"))} {
		f := daysFrame(t, page, 160, 37)
		f.Look = look
		c := historyDrawn(f)
		rows := textOf(c.rows(Look{}))
		if got, want := string([]rune(rows[21])[14:24]), "··░░▪▒▓▓██"; got != want {
			t.Errorf("work's first days read %q, want %q", got, want)
		}
		if got := c.at(18, 21).ink; got != (ink{token: theme.StateDestructive}) {
			t.Errorf("the limit's ▪ is in %+v, want state.destructive on nothing", got)
		}
		if got := c.at(16, 21).ink; got != dayInk(1) {
			t.Errorf("a day at level 1 is in %+v, want %+v", got, dayInk(1))
		}
		if got, want := string([]rune(rows[34])[:24]), "less ·· ░░ ▒▒ ▓▓ ██ more"; got != want {
			t.Errorf("the key reads %q, want %q", got, want)
		}
	}
}

// BY ACCOUNT shows six accounts, as many as fit over what's out of view, and
// scrolls within itself past them, saying what's out of view above and
// below.
func TestDaysByAccountScrollsPastSixAccounts(t *testing.T) {
	const toScroll = "   ·   j/k or the wheel to scroll them"
	tests := []struct {
		name          string
		accounts      []views.DaysStrip
		height        int
		scroll        int
		want          Scrolling
		first, says   string
		shownAccounts int
	}{
		{name: "three, all shown", accounts: tenDays[:3], height: 37, want: Scrolling{Page: 3}, first: "work", shownAccounts: 3},
		{name: "ten, the first six", accounts: tenDays, height: 37, want: Scrolling{Most: 4, Page: 6}, first: "work", says: "▼ 4 more accounts below" + toScroll, shownAccounts: 6},
		{
			name: "ten, scrolled by two", accounts: tenDays, height: 37, scroll: 2, want: Scrolling{Most: 4, Page: 6}, first: "side",
			says: "▲ 2 more accounts above   ·   ▼ 2 more accounts below" + toScroll, shownAccounts: 6,
		},
		{name: "ten, scrolled to the last", accounts: tenDays, height: 37, scroll: 9, want: Scrolling{Most: 4, Page: 6}, first: "spare", says: "▲ 4 more accounts above" + toScroll, shownAccounts: 6},
		{name: "a taller terminal shows six still", accounts: tenDays, height: 45, want: Scrolling{Most: 4, Page: 6}, first: "work", says: "▼ 4 more accounts below" + toScroll, shownAccounts: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := daysFrame(t, daysWorld(tt.accounts...), 160, tt.height)
			f.Scroll = tt.scroll
			if got := f.daysScrolling(); got != tt.want {
				t.Errorf("it scrolls %+v, want %+v", got, tt.want)
			}
			rows := textOf(historyDrawn(f).rows(f.Look))
			if got := strings.Fields(rows[21])[0]; got != tt.first {
				t.Errorf("the first account shown is %q, want %q", got, tt.first)
			}
			names := 0
			for _, row := range rows[21 : tt.height-4] {
				if strings.HasSuffix(row, "limits") || strings.HasSuffix(row, "limit") {
					names++
				}
			}
			if names != tt.shownAccounts {
				t.Errorf("it shows %d accounts, want %d", names, tt.shownAccounts)
			}
			if got := rows[tt.height-4]; got != tt.says {
				t.Errorf("row %d says %q, want %q", tt.height-3, got, tt.says)
			}
		})
	}
}

// Shorter than History's frames, or where BY ACCOUNT would show none of its
// accounts, Days is drawn whole, every account, and scrolls by rows over its
// key, which keeps the view's last rows: the row over the key says how many
// rows are out of view, above and below, and no line counts accounts it
// doesn't show; BY ACCOUNT never ends a page without its first strip; and
// the last shows scrolled to the end.
func TestDaysScrollsWholeWhereItsFramesDontFit(t *testing.T) {
	families := daysWorld(tenDays...)
	families.ByModel = slices.Repeat(families.ByModel, 3)
	tests := []struct {
		name          string
		page          views.DaysPage
		width, height int
		top, end      Scrolling
		keyRows       int
		below, above  string
	}{
		{
			name: "ten accounts, a row short of the frames", page: daysWorld(tenDays...), width: 160, height: 36,
			top: Scrolling{Most: 8, Page: 26}, end: Scrolling{Most: 8, Page: 26}, keyRows: 1,
			below: "▼ 8 more below   ·   j/k to scroll", above: "▲ 8 more above   ·   j/k to scroll",
		},
		{
			name: "ten accounts at 160×24, BY ACCOUNT left for the next page with its first strip", page: daysWorld(tenDays...), width: 160, height: 24,
			top: Scrolling{Most: 20, Page: 13}, end: Scrolling{Most: 20, Page: 14}, keyRows: 1,
			below: "▼ 21 more below   ·   j/k to scroll", above: "▲ 20 more above   ·   j/k to scroll",
		},
		{
			name: "ten accounts at 80×24, on a phone", page: daysWorld(tenDays...), width: 80, height: 24,
			top: Scrolling{Most: 23, Page: 12}, end: Scrolling{Most: 23, Page: 12}, keyRows: 2,
			below: "▼ 23 more below   ·   j/k to scroll", above: "▲ 23 more above   ·   j/k to scroll",
		},
		{
			name: "twelve families as tall as the frames", page: families, width: 160, height: 37,
			top: Scrolling{Most: 23, Page: 27}, end: Scrolling{Most: 23, Page: 27}, keyRows: 1,
			below: "▼ 23 more below   ·   j/k to scroll", above: "▲ 23 more above   ·   j/k to scroll",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := daysFrame(t, tt.page, tt.width, tt.height)
			for _, at := range []struct {
				scroll int
				want   Scrolling
				says   string
			}{{0, tt.top, tt.below}, {tt.top.Most, tt.end, tt.above}} {
				f.Scroll = at.scroll
				if got := f.daysScrolling(); got != at.want || !f.ScrollsRows(status.Document{}, now) {
					t.Errorf("scrolled %d, it scrolls %+v, a row at a time: %t; want %+v, a row at a time", at.scroll, got, f.ScrollsRows(status.Document{}, now), at.want)
				}
				view := textOf(historyDrawn(f).rows(Look{}))[f.top() : f.top()+f.viewRows()]
				if key := view[len(view)-tt.keyRows]; !strings.HasPrefix(key, "less ") {
					t.Errorf("scrolled %d, the key's first row reads %q, want it %d over the view's last", at.scroll, key, tt.keyRows-1)
				}
				if says := view[len(view)-tt.keyRows-1]; says != at.says {
					t.Errorf("scrolled %d, the row over the key reads %q, want %q", at.scroll, says, at.says)
				}
				if slices.ContainsFunc(view, func(row string) bool { return strings.Contains(row, "more account") }) {
					t.Errorf("scrolled %d, the view reads\n%s\nwant no line counting accounts out of view", at.scroll, strings.Join(view, "\n"))
				}
				if heading := slices.IndexFunc(view, func(row string) bool { return strings.HasPrefix(row, "BY ACCOUNT") }); heading >= 0 && heading+2 >= at.want.Page {
					t.Errorf("scrolled %d, BY ACCOUNT is on the page's row %d of %d, without its first strip:\n%s", at.scroll, heading, at.want.Page, strings.Join(view, "\n"))
				}
			}
			if last := textOf(historyDrawn(f).rows(Look{}))[f.top()+tt.end.Page-1]; !strings.HasPrefix(last, "test") && !strings.Contains(last, "······") {
				t.Errorf("scrolled to the end, the last row over the key reads %q, want the last account's", last)
			}
		})
	}
	if f := daysFrame(t, daysWorld(tenDays...), 160, 37); f.ScrollsRows(status.Document{}, now) {
		t.Error("ten accounts as tall as the frames scroll a row at a time, want an account at a time")
	}
}

// On a phone, Days is the last three weeks, each strip under its name, its
// count at the right, and the key over two rows.
func TestDaysOnAPhone(t *testing.T) {
	page := daysWorld(views.DaysStrip{Of: "work", Limits: 2, Days: stripOf(strings.Repeat("2", 53), 35, 52)}, views.DaysStrip{Of: "side", Limits: 1, Days: stripOf("", 50)})
	page.ByModel = []views.DaysStrip{{Of: "opus", Days: stripOf(strings.Repeat("3", 53)), Share: new(0.8)}, {Of: "sonnet", Days: stripOf(strings.Repeat("01", 26)), Share: new(0.2)}}
	f := daysFrame(t, page, 52, 35)
	f.Look = NoColour()
	top := f.top()
	rows := textOf(historyDrawn(f).rows(Look{}))
	wantFrom := []string{
		"14 Sep         21 Sep         28 Sep",
		"                              M T W T F S S",
		"BY MODEL",
		"",
		textsAt(52, map[int]string{0: "● Opus", 41: "80% of use"}),
		"▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓▓▓▓▓▓▓ ▓▓▓▓▓▓▓▓······",
		textsAt(52, map[int]string{0: "● Sonnet", 41: "20% of use"}),
		"░░··░░··░░··░░ ··░░··░░··░░·· ░░··░░········",
		"",
		"BY ACCOUNT",
		"",
		textsAt(52, map[int]string{0: "work", 36: "2 5-hour limits"}),
		"▪▒▒▒▒▒▒▒▒▒▒▒▒▒ ▒▒▒▒▒▒▒▒▒▒▒▒▒▒ ▒▒▒▒▒▒▪▒······",
		textsAt(52, map[int]string{0: "side", 37: "1 5-hour limit"}),
		"·············· ·············· ··▪···········",
	}
	for i, want := range wantFrom {
		if got := rows[top+i]; got != want {
			t.Errorf("row %d reads\n%q\nwant\n%q", top+i+1, got, want)
		}
	}
	key := []string{"less ·· ░░ ▒▒ ▓▓ ██ more", "▪▒ a day it hit a 5-hour limit   ·· days to come"}
	if got := rows[f.Height-4 : f.Height-2]; !slices.Equal(got, key) {
		t.Errorf("the key reads %q, want %q", got, key)
	}
}

// Till anything is recorded in its eight weeks, Days says so, in
// text.subtle, where the view starts.
func TestDaysSaysNothingIsRecordedYet(t *testing.T) {
	for name, page := range map[string]*views.DaysPage{"no page": nil, "an empty page": {Weeks: daysWorld().Weeks, ByAccount: []views.DaysStrip{{Of: "work", Days: stripOf("")}}}} {
		f := daysFrame(t, views.DaysPage{}, 160, 37)
		f.History.Days = page
		c := historyDrawn(f)
		rows := textOf(c.rows(Look{}))
		if got := rows[f.top()]; got != "nothing recorded yet" || c.at(0, f.top()).ink != dimInk {
			t.Errorf("%s: row %d reads %q in %+v, want nothing recorded yet in text.subtle", name, f.top()+1, got, c.at(0, f.top()).ink)
		}
		if under := strings.Join(rows[f.top()+1:], ""); under != "" {
			t.Errorf("%s: drew more under it: %q", name, under)
		}
	}
}

// Days' shares and its 5-hour limits change as today's use comes in, so a
// look brightens each as it changes.
func TestDaysNumbersBrighten(t *testing.T) {
	f := daysFrame(t, daysWorld(workDays, views.DaysStrip{Of: "spare", Days: stripOf("")}), 160, 37)
	want := map[Number]string{
		{Kind: NumberShareOfUse, Of: "opus"}: "59%", {Kind: NumberShareOfUse, Of: "sonnet"}: "37%",
		{Kind: NumberShareOfUse, Of: "fable"}: "2%", {Kind: NumberShareOfUse, Of: "haiku"}: "2%",
		{Kind: NumberLimitsHit, Account: "work", Of: "5h"}: "33", {Kind: NumberLimitsHit, Account: "spare", Of: "5h"}: "0",
	}
	if got := historyDrawn(f).numbers(); !maps.Equal(got, want) {
		t.Errorf("Days' numbers are %v, want %v", got, want)
	}
}

// A family's strip names it, its first letter capital, its dot in its hue,
// or text.muted for one without a hue of its own; a share not priced is —.
func TestDaysNamesEachFamily(t *testing.T) {
	tests := []struct {
		family string
		want   line
	}{
		{family: "opus", want: line{{"●", ink{token: theme.VizFamily1}}, {" Opus", secondaryInk}}},
		{family: "haiku", want: line{{"●", ink{token: theme.VizFamily4}}, {" Haiku", secondaryInk}}},
		{family: "claude-unknown-9", want: line{{"●", ink{token: theme.TextMuted}}, {" Claude-unknown-9", secondaryInk}}},
	}
	for _, tt := range tests {
		if got := familyDot(tt.family, secondaryInk); !slices.Equal(got, tt.want) {
			t.Errorf("familyDot(%q) = %+v, want %+v", tt.family, got, tt.want)
		}
	}
	unpriced := fitParts(80, shareOfUse(views.DaysStrip{Of: "fable"})...)
	if want := (line{{"—", faintInk.numbered(Number{Kind: NumberShareOfUse, Of: "fable"})}, {" of use", dimInk}}); !slices.Equal(unpriced, want) {
		t.Errorf("an unpriced share is %+v, want %+v", unpriced, want)
	}
}

// An account is named by its label, the primary's ◆ after it in
// text.tertiary, and never ▲, which is routing. A name too long for its room
// is cut, keeping ◆ and a blank cell before its strip, or on a phone before
// its count.
func TestDaysNamesEachAccount(t *testing.T) {
	long := views.DaysStrip{Of: "a-long-account-name-for-days", Limits: 12, Days: stripOf(busy)}
	tests := []struct {
		name    string
		account views.DaysStrip
		width   int
		want    string
	}{
		{name: "by its label", account: views.DaysStrip{Of: "work", Label: "Work", Days: stripOf(busy)}, width: 160, want: "Work "},
		{name: "the primary marked", account: views.DaysStrip{Of: "work", Label: "Work", Primary: true, Days: stripOf(busy)}, width: 160, want: "Work ◆ "},
		{name: "cut beside its strip", account: long, width: 160, want: "a-long-accou… "},
		{name: "cut, keeping its ◆", account: views.DaysStrip{Of: long.Of, Primary: true, Days: long.Days}, width: 160, want: "a-long-acc… ◆ "},
		{name: "on a phone, room beside its count", account: long, width: 52, want: "a-long-account-name-for-days       12 5-hour limits"},
		{name: "on a narrower phone, cut beside its count", account: long, width: 40, want: "a-long-account-name-f… 12 5-hour limits"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := daysFrame(t, daysWorld(tt.account), tt.width, 37)
			row := f.layOutDays(*f.History.Days, f.top()).accountRow(0)
			if got := textOf(historyDrawn(f).rows(f.Look))[row]; !strings.HasPrefix(got, tt.want) {
				t.Errorf("the account's row reads %q, want it to start %q", got, tt.want)
			}
		})
	}
}

// Narrower than a phone, Days shows the latest of its three weeks that fit
// short of the edge, each headed by its first day; and a strip's count, at
// the edge, is cut there, its share a figure, numbered only where it's
// drawn whole.
func TestDaysOnANarrowPhoneFitsItsWeeksAndCounts(t *testing.T) {
	f := daysFrame(t, daysWorld(workDays, personalDays, sideDays), 40, 40)
	if rows := textOf(historyDrawn(f).rows(Look{})); rows[f.top()] != "21 Sep         28 Sep" {
		t.Errorf("at 40 columns, Days' weeks read %q, want the last two, %q", rows[f.top()], "21 Sep         28 Sep")
	}
	opus := Number{Kind: NumberShareOfUse, Of: "opus"}
	for width, want := range map[int]string{4: "59%", 3: ""} {
		f := daysFrame(t, daysWorld(workDays, personalDays, sideDays), width, 40)
		if got := f.Numbers(threeRouted(), now)[opus]; got != want {
			t.Errorf("at %d columns, Opus's share is numbered %q, want %q", width, got, want)
		}
	}
}
