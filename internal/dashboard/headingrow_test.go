package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// laidOn is the heading laid out from row 0 of a canvas width cells wide and
// two tall, on a phone where phone says, and drawn there.
func laidOn(h heading, width int, phone bool) (*canvas, []piece) {
	c := newCanvas(width, 2)
	pieces := h.pieces(0, width, phone)
	for _, p := range pieces {
		c.line(p.x, p.y, p.line)
	}
	return c, pieces
}

// The headings of the frames the tests hold the heading row to: the Log's,
// filtered, its count and f at its right; Runway's, stepped back a day, and
// over the week; Accounts', its period between its keys; and History's, over
// the last 12 months.
var (
	filteredLog = heading{
		name: "LOG", subTabs: []string{"Requests", "Events"},
		state: line{{"only d28c switchboard", faintInk}},
		right: line{{"830 hidden", secondaryInk}, spaces(3), {"f", keyInk}, {" what it shows", mutedInk}},
	}
	steppedBack = heading{name: "RUNWAY", state: line{{"Wed 30 Sep", faintInk}}, period: "5-hour"}
	overTheWeek = heading{name: "RUNWAY", period: "week"}
	comparing   = heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, period: "this week"}
	yearHistory = heading{name: "HISTORY", subTabs: historyViews, period: "last 12 months"}
)

func TestTheHeadingRow(t *testing.T) {
	tests := []struct {
		name    string
		heading heading
		width   int
		phone   bool
		want    []string
	}{
		{name: "the name alone, its rule to the last column", heading: heading{name: "SESSIONS"}, width: 160, want: []string{"SESSIONS  " + rule(150)}},
		{
			name: "the sub-tabs, [ ], the state, the rule a cell after it, and a line at the right, as log-requests-filtered-160 draws it", heading: filteredLog, width: 160,
			want: []string{"LOG    Requests  Events  [ ]   only d28c switchboard " + rule(77) + "  830 hidden   f what it shows"},
		},
		{
			name: "a state, and a window between its keys, as runway-stepped-back-160 draws it", heading: steppedBack, width: 160,
			want: []string{"RUNWAY   Wed 30 Sep " + rule(128) + "  < 5-hour >"},
		},
		{name: "a window, as runway-week-160 draws it", heading: overTheWeek, width: 160, want: []string{"RUNWAY  " + rule(142) + "  < week >"}},
		{
			name: "the sub-tabs, the rule two cells after [ ], and the period", heading: comparing, width: 160,
			want: []string{"ACCOUNTS    Compare  Detail  [ ]  " + rule(111) + "  < this week >"},
		},
		{
			name: "on a phone, the sub-tabs two cells after the name, without [ ] or a rule, the period at the right where it fits", heading: comparing, width: 52, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail" + blanks(13) + "< this week >"},
		},
		{
			name: "on a phone, the period on the row under the sub-tabs where it doesn't fit beside them", heading: comparing, width: 40, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail", blanks(27) + "< this week >"},
		},
		{
			name:    "on a phone, History's sub-tabs two cells after the name, padded, as Accounts' are",
			heading: yearHistory, width: 52, phone: true,
			want: []string{"HISTORY   Year  Weeks  Days  Tokens", blanks(34) + "< last 12 months >"},
		},
		{
			name: "on a phone, Accounts' padded sub-tabs leave loading… room before the period", heading: comparing.loading(), width: 73, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail    loading…" + blanks(22) + "< this week >"},
		},
		{
			name: "on a phone, History's padded sub-tabs leave loading… room, before the Year's filter", width: 52, phone: true,
			heading: HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Account: "work"}, Accounts: threeConfigured}.heading().loading(),
			want:    []string{"HISTORY   Year  Weeks  Days  Tokens    loading…", blanks(34) + "< last 12 months >"},
		},
		{
			name: "on a phone, the Year's filter cut to the room History's padded sub-tabs leave", width: 52, phone: true,
			heading: HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Account: "work"}, Accounts: threeConfigured}.heading(),
			want:    []string{"HISTORY   Year  Weeks  Days  Tokens    Work only, e…", blanks(34) + "< last 12 months >"},
		},
		{
			name: "History's sub-tabs wider than a phone", heading: yearHistory, width: 160,
			want: []string{"HISTORY    Year  Weeks  Days  Tokens  [ ]  " + rule(97) + "  < last 12 months >"},
		},
		{
			name: "on a phone, the sub-tabs two cells after the name, padded, and a line at the right on their row, as log-requests-phone-52 draws them", width: 52, phone: true,
			heading: heading{name: "LOG", subTabs: []string{"Requests", "Events"}, right: line{{"35 hidden", secondaryInk}, spaces(2), {"f", keyInk}}},
			want:    []string{"LOG   Requests  Events" + blanks(18) + "35 hidden  f"},
		},
		{
			name: "on a phone, a tab without sub-tabs keeps its rule and its window, as runway-phone-52 draws it", width: 52, phone: true,
			heading: heading{name: "RUNWAY", period: "5-hour"},
			want:    []string{"RUNWAY  " + rule(32) + "  < 5-hour >"},
		},
		{
			name: "the state cut short, to leave the period room", width: 100,
			heading: heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, state: line{{"accounts 1–6 of 10  ·  ", faintInk}, {"←→", keyInk}, {" moves along them; all accounts stays put", faintInk}}, period: "this week"},
			want:    []string{"ACCOUNTS    Compare  Detail  [ ]   accounts 1–6 of 10  ·  ←→ moves along them; all a…  < this week >"},
		},
		{
			name: "the right left out where it doesn't fit beside the name and the sub-tabs", heading: filteredLog, width: 40, phone: true,
			want: []string{"LOG   Requests  Events    only d28c swi…"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := laidOn(tt.heading, tt.width, tt.phone)
			got := c.rows(Look{})
			for i := range got {
				got[i] = strings.TrimRight(got[i], " ")
			}
			want := slices.Concat(tt.want, make([]string, 2-len(tt.want)))
			if !slices.Equal(got, want) {
				t.Errorf("the heading row reads\n%q\nwant\n%q", got, want)
			}
			if rows := tt.heading.rows(tt.width, tt.phone); rows != len(tt.want) {
				t.Errorf("rows() = %d, want %d", rows, len(tt.want))
			}
		})
	}
}

func TestTheHeadingRowsInks(t *testing.T) {
	c, _ := laidOn(heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, shown: 1, period: "this week"}, 160, false)
	tests := []struct {
		name string
		x    int
		want ink
	}{
		{name: "the name", x: 0, want: dimInk},
		{name: "a sub-tab not shown", x: 11, want: tabInk},
		{name: "the sub-tab shown", x: 20, want: shownSubTabInk},
		{name: "[ ]", x: 29, want: faintInk},
		{name: "the rule", x: 34, want: borderInk},
		{name: "<", x: 147, want: keyInk},
		{name: "the period", x: 149, want: mutedInk},
		{name: ">", x: 159, want: keyInk},
	}
	for _, tt := range tests {
		if got := c.at(tt.x, 0).ink; got != tt.want {
			t.Errorf("%s, at column %d, is in %+v, want %+v", tt.name, tt.x, got, tt.want)
		}
	}
	if want := (ink{token: theme.TextPrimary, on: hue{token: theme.BgSubtle}, bold: true}); shownSubTabInk != want || tabInk != (ink{token: theme.TextSubtle}) {
		t.Errorf("the sub-tab shown is in %+v and the rest in %+v, want %+v, and text.subtle", shownSubTabInk, tabInk, want)
	}
}

func TestTheHeadingRowsSubTabsAndStepsAreTargetsWhereTheyreDrawn(t *testing.T) {
	for _, phone := range []bool{false, true} {
		width := 160
		if phone {
			width = 52
		}
		c, pieces := laidOn(comparing, width, phone)
		rows := c.rows(Look{})
		var targets []Target
		for _, p := range pieces {
			if p.click.On != 0 {
				targets = append(targets, p.target())
			}
		}
		want := []struct {
			on   Clicked
			sub  int
			step int
			text string
		}{
			{on: OnSubTab, sub: 0, text: " Compare "}, {on: OnSubTab, sub: 1, text: " Detail "},
			{on: OnStep, step: -1, text: "<"}, {on: OnStep, step: 1, text: ">"},
		}
		if len(targets) != len(want) {
			t.Fatalf("on a phone: %v, the targets are %+v, want the sub-tabs, < and >", phone, targets)
		}
		for i, w := range want {
			tt := targets[i]
			if tt.On != w.on || tt.SubTab != w.sub || tt.Step != w.step || under(rows, tt) != w.text {
				t.Errorf("on a phone: %v, target %+v is over %q, want %q", phone, tt, under(rows, tt), w.text)
			}
		}
	}
}

// Every view tab of Accounts and History is a target over its name and a
// cell either side, on a phone and wider alike, and the one shown is picked
// out over them all, both cells with it, as the page's tabs are, and no
// further.
func TestViewTabsAreTargetsOverTheirNamesAndACellEitherSide(t *testing.T) {
	headings := []struct {
		name    string
		heading heading
	}{
		{name: "Accounts' Compare", heading: AccountsShown{Shown: AccountsCompare, Accounts: threeConfigured}.heading()},
		{name: "Accounts' Detail", heading: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured}.heading()},
		{name: "History's Year", heading: HistoryPage{Shown: HistoryYear}.heading()},
		{name: "History's Tokens", heading: HistoryPage{Shown: HistoryTokens}.heading()},
	}
	for _, hh := range headings {
		for _, width := range []int{52, 160} {
			t.Run(fmt.Sprintf("%s at %d", hh.name, width), func(t *testing.T) {
				c, pieces := laidOn(hh.heading, width, width == 52)
				rows := c.rows(Look{})
				var subTabs []string
				for _, p := range pieces {
					if p.click.On != OnSubTab {
						continue
					}
					tt := p.target()
					subTabs = append(subTabs, under(rows, tt))
					if p.click.SubTab == hh.heading.shown {
						pickedOut(t, c, tt)
					}
				}
				want := make([]string, len(hh.heading.subTabs))
				for i, name := range hh.heading.subTabs {
					want[i] = " " + name + " "
				}
				if !slices.Equal(subTabs, want) {
					t.Errorf("the sub-tabs' targets are over %q, want %q", subTabs, want)
				}
			})
		}
	}
}

// pickedOut fails t unless the cells under target tt alone, on row 0, are
// picked out as the sub-tab shown.
func pickedOut(t *testing.T, c *canvas, tt Target) {
	t.Helper()
	for x := tt.X - 1; x <= tt.X+tt.Width; x++ {
		within := x >= tt.X && x < tt.X+tt.Width
		if got := c.at(x, 0).ink; (got == shownSubTabInk) != within {
			t.Errorf("column %d is in %+v, where the sub-tab shown is picked out over columns %d to %d", x, got, tt.X, tt.X+tt.Width-1)
		}
	}
}

// headingFrame is a frame width cells wide showing view, as shown sets it.
func headingFrame(width int, view View, shown func(*Frame)) Frame {
	f := frameOf(width, 61)
	f.View = view
	shown(&f)
	return f
}

// accountsAt has a frame show Accounts' sub-tab given over the period given.
func accountsAt(sub AccountsView, period views.AccountsPeriod) func(*Frame) {
	return func(f *Frame) { f.Accounts = AccountsShown{Shown: sub, Accounts: threeConfigured, Period: period} }
}

// historyAt has a frame show History's page given.
func historyAt(page HistoryPage) func(*Frame) {
	return func(f *Frame) { f.History = page }
}

// A phone's period sits at the right of the heading's name and sub-tabs
// wherever they fit whole beside it at its widest, and takes the row under
// them where they don't, the view starting a row on.
func TestAPhonesPeriodSitsBesideTheHeadingWhereItFits(t *testing.T) {
	compare, detail := accountsAt(AccountsCompare, views.AccountsThisWeek), accountsAt(AccountsDetail, views.AccountsToday)
	year, calendar := historyAt(HistoryPage{Shown: HistoryYear}), historyAt(HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Year: 2025}})
	tokens := historyAt(HistoryPage{Shown: HistoryTokens})
	tests := []struct {
		name   string
		view   View
		shown  func(*Frame)
		width  int
		beside bool
	}{
		{name: "Accounts' Compare", view: Accounts, shown: compare, width: 73, beside: true},
		{name: "Accounts' Compare", view: Accounts, shown: compare, width: 52, beside: true},
		{name: "Accounts' Compare", view: Accounts, shown: compare, width: 40},
		{name: "Accounts' Detail", view: Accounts, shown: detail, width: 73, beside: true},
		{name: "Accounts' Detail", view: Accounts, shown: detail, width: 52, beside: true},
		{name: "Accounts' Detail", view: Accounts, shown: detail, width: 40},
		{name: "History's Year", view: History, shown: year, width: 73, beside: true},
		{name: "History's Year", view: History, shown: year, width: 52},
		{name: "History's Year, over a calendar year", view: History, shown: calendar, width: 52},
		{name: "History's Tokens", view: History, shown: tokens, width: 73, beside: true},
		{name: "History's Tokens", view: History, shown: tokens, width: 52, beside: true},
		{name: "History's Tokens", view: History, shown: tokens, width: 40},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s at %d", tt.name, tt.width), func(t *testing.T) {
			f := headingFrame(tt.width, tt.view, tt.shown)
			rows := 2
			if tt.beside {
				rows = 1
			}
			var steps []int
			for _, p := range f.headingPieces() {
				if p.click.On == OnStep {
					steps = append(steps, p.y)
				}
			}
			periodRow := f.headingY() + rows - 1
			if want := []int{periodRow, periodRow}; !slices.Equal(steps, want) {
				t.Errorf("< and > are on rows %v, want %v", steps, want)
			}
			if got := f.heading().rows(f.Width, f.phone()); got != rows {
				t.Errorf("the heading takes %d rows, want %d", got, rows)
			}
			if got, want := f.top(), f.headingY()+rows+1; got != want {
				t.Errorf("the view starts at row %d, want %d", got, want)
			}
		})
	}
}

// Whether the period sits beside the name is the width's and the period's
// widest form's to say, never the period shown's: stepping through every
// period a view has never moves its heading between one row and two, at any
// width, and where the period sits beside, each form clears the sub-tabs.
func TestSteppingThePeriodNeverMovesTheHeadingsRows(t *testing.T) {
	forms := map[string][]func(*Frame){}
	for _, period := range views.AccountsPeriods() {
		forms["Accounts' Compare"] = append(forms["Accounts' Compare"], accountsAt(AccountsCompare, period))
		forms["Accounts' Detail"] = append(forms["Accounts' Detail"], accountsAt(AccountsDetail, period))
	}
	for _, year := range []int{0, 2025} {
		forms["History's Year"] = append(forms["History's Year"], historyAt(HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Year: year}}))
	}
	for _, period := range tokensPeriods {
		forms["History's Tokens"] = append(forms["History's Tokens"], historyAt(HistoryPage{Shown: HistoryTokens, TokensAsk: views.TokensAsk{Period: period}}))
	}
	for name, shown := range forms {
		view := Accounts
		if strings.HasPrefix(name, "History") {
			view = History
		}
		for width := 40; width <= 120; width++ {
			var rows []int
			for _, form := range shown {
				f := headingFrame(width, view, form)
				h := f.heading()
				rows = append(rows, h.rows(width, f.phone()))
				if h.compact(f.phone()) && h.rows(width, f.phone()) == 1 {
					cleared(t, f, name)
				}
			}
			if slices.Min(rows) != slices.Max(rows) {
				t.Errorf("%s at %d: its periods' headings take %v rows, want the same for each", name, width, rows)
			}
		}
	}
}

// cleared fails t unless the period on the frame's heading row starts two
// cells or more after the last sub-tab.
func cleared(t *testing.T, f Frame, name string) {
	t.Helper()
	var subTabsEnd, stepAt int
	for _, p := range f.headingPieces() {
		switch {
		case p.click.On == OnSubTab:
			subTabsEnd = p.x + p.line.width()
		case p.click.On == OnStep && p.click.Step < 0:
			stepAt = p.x
		}
	}
	if stepAt < subTabsEnd+ruleGap {
		t.Errorf("%s at %d: < is at column %d, want it two cells or more after the sub-tabs, which end at %d", name, f.Width, stepAt, subTabsEnd)
	}
}
