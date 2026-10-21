package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/views"
)

// formCase is a sub-tab of History of a page: its frame of a terminal the
// width given; the width its wide form takes, and the column its widest row
// ends before, as the frame works them out, the one past the other by the
// margin Weeks and Days keep clear; what of it its wide form draws, and
// what it draws where that doesn't fit, its phone form, but Weeks' columns,
// whose table is drawn there where it fits, each drawn as that form lays it
// out. The Year, whose arrangements are its parts', not a reach, has tests
// of its own.
type formCase struct {
	name           string
	frame          func(t *testing.T, width int) Frame
	reach, widest  func(f Frame) int
	narrower, wide func(f Frame) *canvas
}

// formCases are each sub-tab of the pages the frames draw.
func formCases() []formCase {
	weeks := func(page func() views.WeeksPage, shape WeeksShape) func(t *testing.T, width int) Frame {
		return func(_ *testing.T, width int) Frame { return weeksFrame(page(), shape, width, 40, NoColour()) }
	}
	days := func(page views.DaysPage) func(t *testing.T, width int) Frame {
		return func(t *testing.T, width int) Frame {
			f := daysFrame(t, page, width, 40)
			f.Look = NoColour()
			return f
		}
	}
	tokens := func(split TokensSplit) func(t *testing.T, width int) Frame {
		return func(_ *testing.T, width int) Frame { return tokensFrame(width, 40, split) }
	}
	var cases []formCase
	for name, frame := range map[string]func(*testing.T, int) Frame{
		"weeks of three in columns":   weeks(threeWeeks, WeeksColumns),
		"weeks of ten in columns":     weeks(tenWeeks, WeeksColumns),
		"weeks of three as a table":   weeks(threeWeeks, WeeksTable),
		"weeks of ten as a table":     weeks(tenWeeks, WeeksTable),
		"days of three":               days(daysWorld(workDays, personalDays, sideDays)),
		"days of ten":                 days(daysShared([4]float64{0.57, 0.39, 0.01, 0.03}, tenDays...)),
		"tokens by account":           tokens(TokensByAccount),
		"tokens by model":             tokens(TokensByModel),
		"tokens by account and model": tokens(TokensByAccountAndModel),
		"tokens unsplit":              tokens(TokensUnsplit),
	} {
		cases = append(cases, formCase{name: name, frame: frame})
	}
	for i, c := range cases {
		switch {
		case strings.HasPrefix(c.name, "weeks"):
			cases[i].reach, cases[i].widest, cases[i].narrower, cases[i].wide = weeksReach, weeksWidest, weeksForm(true), weeksForm(false)
			if strings.HasSuffix(c.name, "in columns") {
				cases[i].narrower = weeksTableForm
			}
		case strings.HasPrefix(c.name, "days"):
			cases[i].reach, cases[i].widest, cases[i].narrower, cases[i].wide = daysReachOf, daysWidestOf, daysForm(true), daysForm(false)
		default:
			cases[i].reach, cases[i].widest, cases[i].narrower, cases[i].wide = tokensReach, tokensReach, tokensForm(phoneTokens), tokensForm(wideTokens)
		}
	}
	return cases
}

// tokensReach is the width Tokens' wide form takes, and where its widest row
// ends, as the frame works it out.
func tokensReach(Frame) int {
	return wideTokens.reach()
}

// weeksReach and weeksWidest are the width Weeks' wide form takes, in its
// shape, and the column its widest row ends before, as the frame works them
// out.
func weeksReach(f Frame) int {
	return weeksWide(f).reach()
}

func weeksWidest(f Frame) int {
	return weeksWide(f).widest()
}

// weeksWide is Weeks of the frame in its wide form, in its shape.
func weeksWide(f Frame) weeksDrawn {
	return weeksDrawn{page: *f.History.Weeks, columns: f.History.WeeksShape == WeeksColumns, edge: f.edge()}
}

// weeksForm is Weeks of the frame in its phone form, or wide, in its shape,
// from the view's top.
func weeksForm(phone bool) func(f Frame) *canvas {
	return func(f Frame) *canvas {
		c := newCanvas(f.Width, f.Height)
		w := weeksDrawn{page: *f.History.Weeks, phone: phone, columns: !phone && f.History.WeeksShape == WeeksColumns, edge: f.edge()}
		foot := f.top() + f.viewRows() - 1
		shown, above, below := f.weeksShown(w)
		w.draw(c, f.Look, f.top(), shown)
		w.accountsOut(c, foot-w.footRows(), above, below)
		if !phone {
			w.put(c, 0, foot, w.legend()...)
		}
		return c
	}
}

// weeksTableForm is Weeks of the frame as a table, wide, as it's drawn where
// its columns are chosen and don't fit, and its table does.
func weeksTableForm(f Frame) *canvas {
	f.History.WeeksShape = WeeksTable
	return weeksForm(false)(f)
}

// daysReachOf is the width Days' wide form takes, the narrowest frame it
// fits, as flex lays it; and daysWidestOf the column its widest row ends
// before there, a margin short of it.
func daysReachOf(f Frame) int {
	for width := 1; ; width++ {
		f.Width = width
		if f.daysWideFits(*f.History.Days) {
			return width
		}
	}
}

func daysWidestOf(f Frame) int {
	return daysReachOf(f) - margin
}

// daysForm is what of Days, laid out in its phone form or wide, tells the
// forms apart: the weeks over the days and the key.
func daysForm(phone bool) func(f Frame) *canvas {
	return func(f Frame) *canvas {
		c := newCanvas(f.Width, f.Height)
		l := f.daysLaidOut(*f.History.Days, f.top(), phone)
		f.daysWeeks(c, l)
		f.daysKey(c, l)
		return c
	}
}

// tokensForm is what of Tokens, at the columns given, cut at the frame's
// width, tells the forms apart: its heads and its newest period's first row.
func tokensForm(cols tokensColumns) func(f Frame) *canvas {
	return func(f Frame) *canvas {
		cols.width = f.Width
		c := newCanvas(f.Width, f.Height)
		f.tokensHeads(c, f.top(), cols)
		cols.draw(c, f.top()+2, f.tokensRows(f.History.Tokens.Periods[0])[0])
		return c
	}
}

// drawsAsTheForm reports how the sub-tab f shows differs from what of it
// the form drew, row by row, where the form drew anything: "" where none
// does.
func drawsAsTheForm(f Frame, form *canvas) string {
	drawn, want := textOf(historyDrawn(f).rows(Look{})), textOf(form.rows(Look{}))
	var differs strings.Builder
	for y, row := range want {
		if row != "" && drawn[y] != row {
			fmt.Fprintf(&differs, "row %d reads\n%q\nwant\n%q\n", y+1, drawn[y], row)
		}
	}
	return differs.String()
}

// Each of History's sub-tabs draws its wide form, as the design lays it out
// at 160 columns, where the width it takes, its widest row and the margin
// after it that Weeks and Days keep clear, fits the frame's, and where it
// doesn't, its phone form, as its wide form's parts are at fixed columns,
// and would fall off the frame; but Weeks' columns, whose table takes their
// place where it fits. Each fits at 160.
func TestEachSubTabDrawsItsWideFormWhereItsWidestRowFits(t *testing.T) {
	for _, tt := range formCases() {
		t.Run(tt.name, func(t *testing.T) {
			reach := tt.reach(tt.frame(t, 160))
			if reach > 160 {
				t.Fatalf("its wide form takes %d columns, past 160", reach)
			}
			for _, wide := range []Frame{tt.frame(t, 160), tt.frame(t, max(reach, phoneUnder))} {
				if differs := drawsAsTheForm(wide, tt.wide(wide)); differs != "" {
					t.Errorf("at %d columns, its wide form isn't drawn:\n%s", wide.Width, differs)
				}
			}
			narrower := tt.frame(t, reach-1)
			if differs := drawsAsTheForm(narrower, tt.narrower(narrower)); differs != "" {
				t.Errorf("at %d columns, the form it takes there isn't drawn:\n%s", narrower.Width, differs)
			}
		})
	}
}

// The column each sub-tab's wide form is worked out to end before is where
// its widest row, drawn, ends. Drawn whole in a frame as wide as it takes,
// or a phone's width where that's wider, on a canvas wider than the frame,
// no row runs into the margin after its rows it keeps clear, or past the
// frame, as one worked out short would; and in a frame as wide as it takes,
// the widest row reaches the column worked out, as one worked out long
// wouldn't. Rows cut at the width, or as Weeks' verdicts, at the edge, end
// short of it or at it. Of Weeks as a table, whose widest row is shorter
// than a phone's width, this holds only that none runs past its rows' end.
func TestEachSubTabsWidestRowEndsWhereItsWorkedOutToEnd(t *testing.T) {
	for _, tt := range formCases() {
		t.Run(tt.name, func(t *testing.T) {
			reach, widest := tt.reach(tt.frame(t, 160)), tt.widest(tt.frame(t, 160))
			f := tt.frame(t, max(reach, phoneUnder))
			c := newCanvas(f.Width+40, f.Height)
			f.history(c, f.top())
			drawn := 0
			for _, row := range textOf(c.rows(Look{})) {
				drawn = max(drawn, ansi.StringWidth(row))
			}
			if drawn > f.Width-(reach-widest) {
				t.Errorf("at %d columns, its widest row ends before column %d, past where its rows end", f.Width, drawn)
			}
			if f.Width == reach && drawn != widest {
				t.Errorf("its widest row ends before column %d, worked out as %d", drawn, widest)
			}
		})
	}
}

// Tokens' worth against its price runs as long as the plans' cost makes it,
// so it's cut at the frame's width, with an ellipsis, rather than reckoned in
// its widest row: as wide as its heads, Tokens draws its wide form, its
// newest week's against its price cut short.
func TestTokensAgainstItsPriceIsCutAtTheWidth(t *testing.T) {
	f := tokensFrame(wideTokens.reach(), 37, TokensByAccount)
	row := tokensRowsOf(f)[2]
	if want := "4.6× its $138 we…"; !strings.HasSuffix(row, want) || ansi.StringWidth(row) != f.Width {
		t.Errorf("the newest week's row reads %q, want it to end %q at column %d", row, want, f.Width)
	}
}

// A sub-tab drawn in its phone form, in a frame wider than a phone's,
// scrolls what that form draws: Weeks' accounts from the first's row, under
// the verdict's rows; Days' BY ACCOUNT to the row over the scroll line, over
// its key's two rows.
func TestASubTabInItsPhoneFormScrollsWhatThatFormDraws(t *testing.T) {
	tests := []struct {
		name  string
		frame func(t *testing.T, width int) Frame
		reach func(f Frame) int
		at    func(y, rows int) int
		reads string
	}{
		{
			name: "weeks",
			frame: func(_ *testing.T, width int) Frame {
				return weeksFrame(threeWeeks(), WeeksColumns, width, 40, NoColour())
			},
			reach: weeksReach, at: func(y, _ int) int { return y }, reads: "work",
		},
		{
			name: "days",
			frame: func(t *testing.T, width int) Frame {
				f := daysFrame(t, daysWorld(workDays, personalDays, sideDays), width, 40)
				f.Look = NoColour()
				return f
			},
			reach: daysReachOf, at: func(y, rows int) int { return y + rows + 1 }, reads: "less",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.frame(t, tt.reach(tt.frame(t, 160))-1)
			drawn := textOf(historyDrawn(f).rows(Look{}))
			if row := tt.at(f.historyScrolls()); !strings.HasPrefix(drawn[row], tt.reads) {
				t.Errorf("at %d columns, row %d reads %q, want it to start %q", f.Width, row+1, drawn[row], tt.reads)
			}
		})
	}
}

// A sub-tab's form never hangs on its data: each row whose length is
// bounded is reckoned at the widest its format draws, not at the width it's
// drawn, so at the one width, Days of accounts that hit one 5-hour limit
// each and of accounts that hit more take the one form.
func TestASubTabsFormNeverHangsOnItsData(t *testing.T) {
	oneEach := daysWorld(workDays, personalDays, sideDays)
	oneEach.ByAccount = slices.Clone(oneEach.ByAccount)
	for i := range oneEach.ByAccount {
		oneEach.ByAccount[i].Limits = 1
	}
	daysPhone := func(f Frame) bool { return f.layOutDays(*f.History.Days, f.top()).phone }
	tests := []struct {
		name   string
		width  int
		phone  func(f Frame) bool
		frames map[string]Frame
	}{
		{
			name: "days", width: 154, phone: daysPhone,
			frames: map[string]Frame{
				"of limits hit many times": daysFrame(t, daysWorld(workDays, personalDays, sideDays), 154, 40),
				"of a limit hit once each": daysFrame(t, oneEach, 154, 40),
			},
		},
	}
	for _, tt := range tests {
		forms := map[bool][]string{}
		for name, f := range tt.frames {
			forms[tt.phone(f)] = append(forms[tt.phone(f)], name)
		}
		if len(forms) > 1 {
			t.Errorf("at %d columns, %s draws its phone form %v, and its wide form %v; want the one form", tt.width, tt.name, forms[true], forms[false])
		}
	}
}

// Weeks and Days end their rows a margin short of the frame's width, as
// their scroll lines do, so each takes its wide form only where its widest
// row ends there: at a width equal to the width it takes, and a cell
// narrower, nothing of it is drawn in the margin column.
func TestWeeksAndDaysDrawNothingInTheMarginColumn(t *testing.T) {
	for _, tt := range formCases() {
		if !strings.HasPrefix(tt.name, "weeks") && !strings.HasPrefix(tt.name, "days") {
			continue
		}
		reach := tt.reach(tt.frame(t, 160))
		for _, width := range []int{reach, reach - 1} {
			f := tt.frame(t, width)
			for y, row := range textOf(historyDrawn(f).rows(Look{})) {
				if ansi.StringWidth(row) > f.edge() {
					t.Errorf("%s at %d columns, row %d runs into the margin column:\n%q", tt.name, width, y+1, row)
				}
			}
		}
	}
}

// On a phone, Weeks and Days scrolled to accounts above and below say what's
// out of view in the longest form that fits, short of the margin, cut
// nowhere.
func TestAPhoneSaysWhatsOutOfViewInALineThatFits(t *testing.T) {
	days := daysFrame(t, daysShared([4]float64{0.57, 0.39, 0.01, 0.03}, tenDays...), 52, 37)
	tests := []struct {
		name  string
		frame Frame
		want  string
	}{
		{name: "weeks", frame: weeksFrame(manyWeeks(18), WeeksTable, 52, 37, NoColour()), want: "▲ 2 more accounts above · ▼ 1 more account below"},
		{name: "days", frame: days, want: "▲ 2 more accounts above · ▼ 3 more accounts below"},
	}
	for _, tt := range tests {
		f := tt.frame
		f.Scroll = 2
		rows := textOf(historyDrawn(f).rows(Look{}))
		i := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "more account") })
		if i < 0 || rows[i] != tt.want || ansi.StringWidth(rows[i]) > f.edge() {
			t.Errorf("%s scrolled on a phone reads\n%s\nwant a row %q", tt.name, strings.Join(rows, "\n"), tt.want)
		}
	}
}

// Nothing History draws is left to the canvas to clip: each sub-tab, in
// either form, at any width, its accounts scrolled to the middle, draws
// nothing past its edge, the frame's width for Tokens, and a
// margin short of it for Weeks and Days, as each cuts what it draws to fit.
func TestHistoryDrawsNothingPastItsEdge(t *testing.T) {
	for _, tt := range formCases() {
		reach := tt.reach(tt.frame(t, 160))
		for _, width := range []int{1, 4, 12, 20, 28, 36, 44, 52, 60, 80, 100, reach - 1, reach} {
			f := tt.frame(t, width)
			f.Scroll = 2
			edge := f.Width
			if strings.HasPrefix(tt.name, "weeks") || strings.HasPrefix(tt.name, "days") {
				edge = f.edge()
			}
			c := newCanvas(width+80, f.Height)
			f.history(c, f.top())
			for row, cells := range c.cells {
				if x := slices.IndexFunc(cells[max(edge, 0):], func(here cell) bool { return here != blank }); x >= 0 {
					t.Errorf("%s at %d columns draws %q past its edge, at column %d of row %d:\n%q",
						tt.name, width, cells[max(edge, 0)+x].glyph, max(edge, 0)+x, row, textOf(c.rows(Look{}))[row])
				}
			}
		}
	}
}
