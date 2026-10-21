package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
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
			name: "on a phone, the sub-tabs two cells after the name, without [ ] or a rule, the period at the right where it fits beside them and loading…", heading: comparing, width: 53, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail" + blanks(14) + "< this week >"},
		},
		{
			name: "on a phone, the period on the row under the sub-tabs where it doesn't fit beside them and loading…", heading: comparing, width: 52, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail", blanks(39) + "< this week >"},
		},
		{
			name: "on a phone, loading… whole between the sub-tabs and the period, where they fit", heading: comparing.loading(), width: 53, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail    loading…  < this week >"},
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
			name: "on a phone, the Year's filter in its short form, where its full form doesn't fit the room History's padded sub-tabs leave", width: 52, phone: true,
			heading: HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Account: "work"}, Accounts: threeConfigured}.heading(),
			want:    []string{"HISTORY   Year  Weeks  Days  Tokens    Work only", blanks(34) + "< last 12 months >"},
		},
		{
			name: "on a phone, the Year's filter in its short form beside the period, where the widest configured account's would fit", width: 72, phone: true,
			heading: HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Account: "work"}, Accounts: threeConfigured}.heading(),
			want:    []string{"HISTORY   Year  Weeks  Days  Tokens    Work only" + blanks(6) + "< last 12 months >"},
		},
		{
			name: "History's sub-tabs wider than a phone", heading: yearHistory, width: 160,
			want: []string{"HISTORY    Year  Weeks  Days  Tokens  [ ]  " + rule(97) + "  < last 12 months >"},
		},
		{
			name: "wider than a phone, the Year's filter in its short form where its full form doesn't fit, then the rule", width: 108,
			heading: HistoryPage{Shown: HistoryYear, YearAsk: views.YearAsk{Account: "work"}, Accounts: threeConfigured}.heading(),
			want:    []string{"HISTORY    Year  Weeks  Days  Tokens  [ ]   Work only " + rule(34) + "  < last 12 months >"},
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

// headingFrame is a frame width cells wide showing view, as shown sets it,
// loading where loading says.
func headingFrame(width int, view View, shown func(*Frame), loading bool) Frame {
	f := frameOf(width, 61)
	f.View, f.Loading = view, loading
	shown(&f)
	return f
}

// accountsAt has a frame show Accounts' sub-tab given over the period given,
// of the accounts given.
func accountsAt(accounts config.Accounts, sub AccountsView, period views.AccountsPeriod) func(*Frame) {
	return func(f *Frame) { f.Accounts = AccountsShown{Shown: sub, Accounts: accounts, Period: period} }
}

// historyAt has a frame show History's page given.
func historyAt(page HistoryPage) func(*Frame) {
	return func(f *Frame) { f.History = page }
}

// longLabelled are the accounts configured, side's label long.
var longLabelled = config.Accounts{{ID: "work", Label: "Work"}, {ID: "personal"}, {ID: "side", Label: "side-hustle-consulting"}}

// The Year's filters: none; an account; and a version, Opus 5.5 or the wider
// Sonnet 5.5.
var (
	yearAll    = views.YearAsk{}
	yearWork   = views.YearAsk{Account: "work"}
	yearOpus   = views.YearAsk{Version: "Opus 5.5"}
	yearSonnet = views.YearAsk{Version: "Sonnet 5.5"}
)

// yearAt has a frame show the Year over the year given, filtered as ask
// says, of the accounts configured, its last 12 months read where read says.
func yearAt(accounts config.Accounts, ask views.YearAsk, year int, read bool) func(*Frame) {
	ask.Year = year
	return historyAt(HistoryPage{Shown: HistoryYear, YearAsk: ask, Accounts: accounts, Read: read})
}

// tokensAt has a frame show Tokens by the period given.
func tokensAt(period views.TokensPeriod) func(*Frame) {
	return historyAt(HistoryPage{Shown: HistoryTokens, TokensAsk: views.TokensAsk{Period: period}})
}

// A phone's period sits at the right of the heading's row from the width its
// name, its sub-tabs, three cells on loading… whole, or the state's least
// form where that's wider, and two cells on the period at its widest fit,
// and takes the row under them a cell narrower, the view starting a row on.
func TestAPhonesPeriodSitsBesideTheHeadingWhereItFits(t *testing.T) {
	tests := []struct {
		name  string
		view  View
		shown func(*Frame)
		edge  int
	}{
		{name: "Accounts' Compare", view: Accounts, shown: accountsAt(threeConfigured, AccountsCompare, views.AccountsThisWeek), edge: 53},
		{name: "Accounts' Detail", view: Accounts, shown: accountsAt(threeConfigured, AccountsDetail, views.AccountsToday), edge: 53},
		{name: "Accounts with one account", view: Accounts, shown: accountsAt(threeConfigured[:1], AccountsDetail, views.AccountsThisWeek), edge: 44},
		{name: "History's Tokens", view: History, shown: tokensAt(views.TokensByWeek), edge: 61},
		{name: "History's Year", view: History, shown: yearAt(threeConfigured, yearAll, 0, true), edge: 67},
		{name: "History's Year over a calendar year", view: History, shown: yearAt(threeConfigured, yearAll, 2025, true), edge: 67},
		{name: "History's Year, Work's alone, personal only the widest", view: History, shown: yearAt(threeConfigured, yearWork, 0, true), edge: 72},
		{name: "History's Year, Work's alone, a label configured long", view: History, shown: yearAt(longLabelled, yearWork, 0, true), edge: 86},
		{name: "History's Year, Opus 5.5's alone", view: History, shown: yearAt(threeConfigured, yearOpus, 0, true), edge: 72},
		{name: "History's Year, Opus 5.5's alone, not read yet", view: History, shown: yearAt(threeConfigured, yearOpus, 0, false), edge: 72},
		{name: "History's Year, Sonnet 5.5's alone", view: History, shown: yearAt(threeConfigured, yearSonnet, 0, true), edge: 74},
		{name: "History's Year, Sonnet 5.5's alone, not read yet", view: History, shown: yearAt(threeConfigured, yearSonnet, 0, false), edge: 74},
	}
	for _, tt := range tests {
		for _, width := range []int{tt.edge - 1, tt.edge} {
			for _, loading := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s at %d, loading: %t", tt.name, width, loading), func(t *testing.T) {
					f := headingFrame(width, tt.view, tt.shown, loading)
					rows := 1
					if width < tt.edge {
						rows = 2
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
	}
}

// Whether the period sits beside the name is the width's, the period's
// widest form's and the state's least's to say: only v moves the heading
// between one row and two, as it turns a filter on, off, or from one kind to
// the other. Neither a read, the page's loading or its last 12 months', nor
// stepping the period with < and >, nor stepping the Year's account with ←
// and →, moves it, at any width; and where the period sits beside, the state
// shows its least form whole, and the period clears it.
func TestSteppingThePeriodNeverMovesTheHeadingsRows(t *testing.T) {
	type sweep struct {
		view  View
		forms []func(*Frame)
	}
	sweeps := map[string]sweep{}
	add := func(name string, view View, form func(*Frame)) {
		s := sweeps[name]
		s.view, s.forms = view, append(s.forms, form)
		sweeps[name] = s
	}
	for _, period := range views.AccountsPeriods() {
		add("Accounts' Compare", Accounts, accountsAt(threeConfigured, AccountsCompare, period))
		add("Accounts' Detail", Accounts, accountsAt(threeConfigured, AccountsDetail, period))
		add("Accounts with one account", Accounts, accountsAt(threeConfigured[:1], AccountsDetail, period))
	}
	for _, year := range []int{0, 2025} {
		for _, read := range []bool{false, true} {
			add("History's Year", History, yearAt(threeConfigured, yearAll, year, read))
			for _, ask := range []views.YearAsk{yearOpus, yearSonnet} {
				add("History's Year, "+ask.Version+"'s alone", History, yearAt(threeConfigured, ask, year, read))
			}
			for name, accounts := range map[string]config.Accounts{"short": threeConfigured, "long": longLabelled} {
				for _, id := range accounts.IDs() {
					add("History's Year, an account's alone, labels "+name, History, yearAt(accounts, views.YearAsk{Account: id}, year, read))
				}
			}
		}
	}
	for _, period := range tokensPeriods {
		add("History's Tokens", History, tokensAt(period))
	}
	for name, s := range sweeps {
		for width := 30; width <= 140; width++ {
			var rows []int
			for _, form := range s.forms {
				for _, loading := range []bool{false, true} {
					f := headingFrame(width, s.view, form, loading)
					h := f.heading()
					rows = append(rows, h.rows(width, f.phone()))
					if h.compact(f.phone()) && h.rows(width, f.phone()) == 1 {
						besideWhole(t, f, name)
					}
				}
			}
			if slices.Min(rows) != slices.Max(rows) {
				t.Errorf("%s at %d: its headings, read, loading and stepped, take %v rows, want the same for each", name, width, rows)
			}
		}
	}
}

// besideWhole fails t unless the frame's heading row says the state's least
// form whole, where it has a state, and < starts two cells or more after
// what's before it.
func besideWhole(t *testing.T, f Frame, name string) {
	t.Helper()
	var said strings.Builder
	var before, stepAt int
	for _, p := range f.headingPieces() {
		if p.click.On == OnStep {
			stepAt = p.x
			break
		}
		said.WriteString(p.line.plain())
		before = p.x + p.line.width()
	}
	if forms := f.heading().stateForms(); forms != nil {
		if least := forms[len(forms)-1].plain(); !strings.Contains(said.String(), least) {
			t.Errorf("%s at %d, loading: %t: the heading row says %q, want %q whole in it", name, f.Width, f.Loading, said.String(), least)
		}
	}
	if stepAt < before+ruleGap {
		t.Errorf("%s at %d, loading: %t: < is at column %d, want it two cells or more after column %d", name, f.Width, f.Loading, stepAt, before)
	}
}
