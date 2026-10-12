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
// width given, the column its wide form ends before, what of it its phone
// form and its wide form draw, each drawn as that form lays it out, and its
// wide form drawn whole on the canvas given, where the Year's isn't cut at
// the frame's width as History's own canvas for it is.
type formCase struct {
	name        string
	frame       func(t *testing.T, width int) Frame
	reach       func(f Frame) int
	phone, wide func(f Frame) *canvas
	whole       func(c *canvas, f Frame)
}

// formCases are each sub-tab of the pages the frames draw.
func formCases() []formCase {
	year := func(ask views.YearAsk, page func() views.YearPage) func(t *testing.T, width int) Frame {
		return func(t *testing.T, width int) Frame { return yearFrame(t, width, 50, ask, page()) }
	}
	opusBefore := func() views.YearPage {
		page := opusYear()
		page.Before = &views.MonthsBefore{From: "2025-10", To: "2026-05", Then: "Opus 5"}
		return page
	}
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
		"the year":                    year(views.YearAsk{}, plainYear),
		"the year of a version":       year(views.YearAsk{Version: "Opus 5.5"}, opusBefore),
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
		cases[i].whole = func(c *canvas, f Frame) { f.history(c, f.top()) }
		switch {
		case strings.HasPrefix(c.name, "the year"):
			cases[i].reach, cases[i].phone, cases[i].wide = yearReach, yearForm(yearDrawn.phone), yearForm(yearDrawn.wide)
			cases[i].whole = func(c *canvas, f Frame) { yearOf(f).wide(c, f.top()) }
		case strings.HasPrefix(c.name, "weeks"):
			cases[i].reach, cases[i].phone, cases[i].wide = weeksReach, weeksForm(true), weeksForm(false)
		case strings.HasPrefix(c.name, "days"):
			cases[i].reach, cases[i].phone, cases[i].wide = daysReachOf, daysForm(true), daysForm(false)
		default:
			cases[i].reach, cases[i].phone, cases[i].wide = tokensReach, tokensForm(phoneTokens), tokensForm(wideTokens)
		}
	}
	return cases
}

// tokensReach is the column Tokens' wide form ends before, as the frame
// works it out.
func tokensReach(Frame) int {
	return wideTokens.reach()
}

// yearOf is the Year the frame shows, as it draws it.
func yearOf(f Frame) yearDrawn {
	return yearDrawn{Frame: f, page: *f.History.Year, ask: f.History.YearAsk}
}

// yearReach is the column the Year's wide form ends before, as the frame
// works it out.
func yearReach(f Frame) int {
	return yearOf(f).reach()
}

// yearForm is the Year of the frame as the form given draws it, from the
// view's top.
func yearForm(form func(yearDrawn, *canvas, int)) func(f Frame) *canvas {
	return func(f Frame) *canvas {
		c := newCanvas(f.Width, f.Height)
		form(yearOf(f), c, f.top())
		return c
	}
}

// weeksReach is the column Weeks' wide form ends before, in its shape, as the
// frame works it out.
func weeksReach(f Frame) int {
	return weeksDrawn{page: *f.History.Weeks, columns: f.History.WeeksShape == WeeksColumns, edge: f.edge()}.reach()
}

// weeksForm is Weeks of the frame in its phone form, or wide, in its shape,
// from the view's top.
func weeksForm(phone bool) func(f Frame) *canvas {
	return func(f Frame) *canvas {
		c := newCanvas(f.Width, f.Height)
		w := weeksDrawn{page: *f.History.Weeks, phone: phone, columns: !phone && f.History.WeeksShape == WeeksColumns, edge: f.edge()}
		top := f.top()
		foot := top + f.viewRows() - 1
		switch {
		case phone:
			f.weeksOnAPhone(c, w, top, foot)
		case w.columns:
			f.weeksColumns(c, w, top, foot)
		default:
			f.weeksTable(c, w, top, foot)
		}
		return c
	}
}

// daysReachOf is the column Days' wide form ends before, as the frame works
// it out.
func daysReachOf(f Frame) int {
	return f.daysReach(*f.History.Days)
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
// at 160 columns, where its widest row fits the frame's width, and its phone
// form where it doesn't, as its wide form's parts are at fixed columns, and
// would fall off the frame. Each fits at 160.
func TestEachSubTabDrawsItsWideFormWhereItsWidestRowFits(t *testing.T) {
	for _, tt := range formCases() {
		t.Run(tt.name, func(t *testing.T) {
			reach := tt.reach(tt.frame(t, 160))
			if reach > 160 {
				t.Fatalf("its wide form ends before column %d, past 160 columns", reach)
			}
			for _, wide := range []Frame{tt.frame(t, 160), tt.frame(t, max(reach, phoneUnder))} {
				if differs := drawsAsTheForm(wide, tt.wide(wide)); differs != "" {
					t.Errorf("at %d columns, its wide form isn't drawn:\n%s", wide.Width, differs)
				}
			}
			phone := tt.frame(t, reach-1)
			if differs := drawsAsTheForm(phone, tt.phone(phone)); differs != "" {
				t.Errorf("at %d columns, its phone form isn't drawn:\n%s", phone.Width, differs)
			}
		})
	}
}

// The column each sub-tab's wide form is worked out to end before is where
// its widest row, drawn, ends. Drawn whole in a frame that wide, or a
// phone's width where that's wider, on a canvas wider than the frame, no
// row runs past the frame, as one worked out short would; and in a frame
// that wide, the widest row reaches its last column, as one worked out long
// wouldn't. Rows cut at the width, or as Weeks' verdicts, at the edge, end
// short of it or at it. Of Weeks as a table, whose widest row is shorter
// than a phone's width, this holds only that none runs past the frame.
func TestEachSubTabsWidestRowEndsWhereItsWorkedOutToEnd(t *testing.T) {
	for _, tt := range formCases() {
		t.Run(tt.name, func(t *testing.T) {
			reach := tt.reach(tt.frame(t, 160))
			f := tt.frame(t, max(reach, phoneUnder))
			c := newCanvas(f.Width+40, f.Height)
			tt.whole(c, f)
			drawn := 0
			for _, row := range textOf(c.rows(Look{})) {
				drawn = max(drawn, ansi.StringWidth(row))
			}
			if drawn > f.Width {
				t.Errorf("at %d columns, its widest row ends before column %d, past the frame", f.Width, drawn)
			}
			if f.Width == reach && drawn != reach {
				t.Errorf("its widest row ends before column %d, worked out as %d", drawn, reach)
			}
		})
	}
}

// The Year's numbers run as long as the ledger makes them, so they're cut at
// the frame's width, with an ellipsis, rather than reckoned in its widest
// row: at 160 columns, a Year of a world of many limits still draws its wide
// form, its limits cut short.
func TestTheYearsNumbersAreCutAtTheWidthNotDrawnOnAPhone(t *testing.T) {
	page := plainYear()
	page.Numbers.Limits = map[string]int{"5h": 4120, "7d": 3, "7d_oi": 2}
	f := yearFrame(t, 160, 50, views.YearAsk{}, page)
	named := f.WindowName
	f.WindowName = func(key string) string {
		if key == "7d_oi" {
			return "Fable week"
		}
		return named(key)
	}
	if differs := drawsAsTheForm(f, yearForm(yearDrawn.wide)(f)); differs != "" {
		t.Errorf("at 160 columns, its wide form isn't drawn:\n%s", differs)
	}
	rows := textOf(historyDrawn(f).rows(Look{}))
	limits := slices.IndexFunc(rows, func(row string) bool { return strings.Contains(row, "limits hit") })
	if want := "limits hit   5-hour ×4,120 · week…"; limits < 0 || !strings.HasSuffix(rows[limits], want) || ansi.StringWidth(rows[limits]) != 160 {
		t.Errorf("the limits row reads %q, want it to end %q at column 160", rows[max(limits, 0)], want)
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
