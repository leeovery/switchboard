package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

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
// over the week; and Accounts', its period between its keys. And the
// headings of a tab of four sub-tabs: over the last 12 months; filtered,
// its state naming what it's filtered to, and only that where there's less
// room, at its least as wide as the widest of the accounts it steps
// through; and of eight weeks, no period, eight weeks at its right.
var (
	filteredLog = heading{
		name: "LOG", subTabs: []string{"Requests", "Events"},
		state: line{{"only d28c switchboard", faintInk}}, free: true,
		right: line{{"830 hidden", secondaryInk}, spaces(3), {"f", keyInk}, {" what it shows", mutedInk}},
	}
	steppedBack = heading{name: "RUNWAY", state: line{{"Mon 21 Sep – Sun 27 Sep", faintInk}}}
	comparing   = heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, period: dialed{name: "this week"}}
	fourTabs    = heading{name: "EXAMPLE", subTabs: []string{"Feed", "Queue", "Logs", "Alerts"}, period: dialed{name: "last 12 months"}}
	filtering   = heading{
		name: fourTabs.name, subTabs: fourTabs.subTabs, period: fourTabs.period, least: "personal only",
		state:   line{{"Work only, every model  ·  ", faintInk}, {"v", keyInk}, {" for all, a model or an account", faintInk}},
		shorter: []line{{{"Work only", faintInk}}},
	}
	eightWeeks = heading{name: fourTabs.name, subTabs: fourTabs.subTabs, right: line{{"eight weeks", mutedInk}}}
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
			name: "a state, the rule a cell after it, as runway-stepped-back-160 draws it", heading: steppedBack, width: 160,
			want: []string{"RUNWAY   Mon 21 Sep – Sun 27 Sep " + rule(127)},
		},
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
			name:    "on a phone, four sub-tabs two cells after the name, padded, as Accounts' are",
			heading: fourTabs, width: 52, phone: true,
			want: []string{"EXAMPLE   Feed  Queue  Logs  Alerts", blanks(34) + "< last 12 months >"},
		},
		{
			name: "on a phone, Accounts' padded sub-tabs leave loading… room before the period", heading: comparing.loading(), width: 73, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail    loading…" + blanks(22) + "< this week >"},
		},
		{
			name: "on a phone, four padded sub-tabs leave loading… room, before the state", width: 52, phone: true,
			heading: filtering.loading(),
			want:    []string{"EXAMPLE   Feed  Queue  Logs  Alerts    loading…", blanks(34) + "< last 12 months >"},
		},
		{
			name: "on a phone, the state in its short form, where its full form doesn't fit the room four padded sub-tabs leave", width: 52, phone: true,
			heading: filtering,
			want:    []string{"EXAMPLE   Feed  Queue  Logs  Alerts    Work only", blanks(34) + "< last 12 months >"},
		},
		{
			name: "on a phone, the state in its short form beside the period, where its least would fit", width: 72, phone: true,
			heading: filtering,
			want:    []string{"EXAMPLE   Feed  Queue  Logs  Alerts    Work only" + blanks(6) + "< last 12 months >"},
		},
		{
			name: "four sub-tabs wider than a phone", heading: fourTabs, width: 160,
			want: []string{"EXAMPLE    Feed  Queue  Logs  Alerts  [ ]  " + rule(97) + "  < last 12 months >"},
		},
		{
			name: "wider than a phone, the state in its short form where its full form doesn't fit, then the rule", width: 108,
			heading: filtering,
			want:    []string{"EXAMPLE    Feed  Queue  Logs  Alerts  [ ]   Work only " + rule(34) + "  < last 12 months >"},
		},
		{
			name: "on a phone, the sub-tabs two cells after the name, padded, and a line at the right on their row, as log-requests-phone-52 draws them", width: 52, phone: true,
			heading: heading{name: "LOG", subTabs: []string{"Requests", "Events"}, right: line{{"35 hidden", secondaryInk}, spaces(2), {"f", keyInk}}},
			want:    []string{"LOG   Requests  Events" + blanks(18) + "35 hidden  f"},
		},
		{
			name: "on a phone, a tab without sub-tabs keeps its rule and its window, as runway-phone-52 draws it", width: 52, phone: true,
			heading: heading{name: "RUNWAY", period: dialed{name: "5-hour"}},
			want:    []string{"RUNWAY  " + rule(32) + "  < 5-hour >"},
		},
		{
			name: "the state cut short, to leave the period room", width: 100,
			heading: heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, state: line{{"accounts 1–6 of 10  ·  ", faintInk}, {"←→", keyInk}, {" moves along them; all accounts stays put", faintInk}}, period: dialed{name: "this week"}},
			want:    []string{"ACCOUNTS    Compare  Detail  [ ]   accounts 1–6 of 10  ·  ←→ moves along them; all a…  < this week >"},
		},
		{
			name:    "on a phone, a free state cut beside the name and the sub-tabs, loading…'s room kept, the right on the row under where it doesn't fit after that",
			heading: filteredLog, width: 40, phone: true,
			want: []string{"LOG   Requests  Events    only d28c swi…", blanks(12) + "830 hidden   f what it shows"},
		},
		{
			name:    "on a phone, loading… whole on the row under where it doesn't fit beside four sub-tabs, eight weeks after it",
			heading: eightWeeks.loading(), width: 40, phone: true,
			want: []string{"EXAMPLE   Feed  Queue  Logs  Alerts", "loading…" + blanks(21) + "eight weeks"},
		},
		{
			name:    "on a phone, eight weeks on the row under where it doesn't fit beside loading…'s room, whether it's loading or not",
			heading: eightWeeks, width: 59, phone: true,
			want: []string{"EXAMPLE   Feed  Queue  Logs  Alerts", blanks(48) + "eight weeks"},
		},
		{
			name:    "on a phone, loading… and eight weeks beside four sub-tabs where they fit",
			heading: eightWeeks.loading(), width: 60, phone: true,
			want: []string{"EXAMPLE   Feed  Queue  Logs  Alerts    loading…  eight weeks"},
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

// The dial's period is drawn as it stands: its name in text.muted at now
// and in accent.attention gone, so it never reads as now; < and > in bold
// accent.key, each faint, still bold, where it does nothing.
func TestTheDialIsDrawnAsItStands(t *testing.T) {
	tests := []struct {
		name           string
		dial           dialed
		back, said, on ink
	}{
		{name: "now", dial: dialed{name: "this week", onOff: true}, back: keyInk, said: mutedInk, on: stepOffInk},
		{name: "now, at the ledger's first day", dial: dialed{name: "this week", backOff: true, onOff: true}, back: stepOffInk, said: mutedInk, on: stepOffInk},
		{name: "a period gone", dial: dialed{name: "last week", gone: true}, back: keyInk, said: warningInk, on: keyInk},
		{name: "a period gone, at the ledger's first day", dial: dialed{name: "week of 14 Sep", gone: true, backOff: true}, back: stepOffInk, said: warningInk, on: keyInk},
	}
	for _, tt := range tests {
		pieces := periodPieces(tt.dial)
		got := []ink{pieces[0].line[0].ink, pieces[1].line[0].ink, pieces[2].line[0].ink}
		if want := []ink{tt.back, tt.said, tt.on}; !slices.Equal(got, want) || pieces[1].line.plain() != " "+tt.dial.name+" " {
			t.Errorf("%s: <, %q and > are in %+v, want %+v", tt.name, pieces[1].line.plain(), got, want)
		}
	}
	if want := (ink{token: theme.TextFaint, bold: true}); stepOffInk != want || warningInk != (ink{token: theme.AccentAttention}) {
		t.Errorf("a step that does nothing is in %+v and a period gone in %+v, want %+v and accent.attention", stepOffInk, warningInk, want)
	}
}

// The heading draws the dial as views says it: at now, its length's name,
// > doing nothing; gone, its own name; < doing nothing on a period holding
// the ledger's first day; and it keeps room for the widest of the lengths'
// names at now.
func TestTheHeadingDrawsTheDialAsViewsSaysIt(t *testing.T) {
	week := views.LengthThisWeek.Now()
	last, _ := week.Back(routedAt, time.Monday, time.Time{})
	began := last.From(routedAt, time.Monday)
	tests := []struct {
		name string
		dial views.Dial
		want dialed
	}{
		{name: "now", dial: views.DialOf(week, routedAt, time.Monday, time.Time{}), want: dialed{name: "this week", widest: "this week", onOff: true}},
		{name: "a week gone", dial: views.DialOf(last, routedAt, time.Monday, time.Time{}), want: dialed{name: "last week", widest: "this week", gone: true}},
		{name: "the week holding the ledger's first day", dial: views.DialOf(last, routedAt, time.Monday, began), want: dialed{name: "last week", widest: "this week", gone: true, backOff: true}},
	}
	for _, tt := range tests {
		if got := periodDial(tt.dial); got != tt.want {
			t.Errorf("%s: the heading draws %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// A phone's heading holds the room for the widest of the lengths' names at
// now, so stepping through them never moves it between one row and two; a
// period gone named wider may.
func TestAPhonesHeadingHoldsStillAtNowAndAWiderPastNameMayMoveIt(t *testing.T) {
	const edge = 61
	for _, length := range views.Lengths() {
		f := headingFrame(edge, Accounts, accountsAt(threeConfigured, AccountsCompare, length.Now()), false)
		if rows := f.heading().rows(f.Width, f.phone()); rows != 1 {
			t.Errorf("at %d columns over %q, the heading takes %d rows, want 1", edge, f.Dial.Name(), rows)
		}
	}
	year, _ := views.LengthTwelveMonths.Now().Back(now, time.Monday, time.Time{})
	f := headingFrame(edge, Accounts, accountsAt(threeConfigured, AccountsCompare, year), false)
	if rows := f.heading().rows(f.Width, f.phone()); rows != 2 {
		t.Errorf("at %d columns over %q, the heading takes %d rows, want 2, its name wider than now's", edge, f.Dial.Name(), rows)
	}
}

func TestTheHeadingRowsInks(t *testing.T) {
	c, _ := laidOn(heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, shown: 1, period: dialed{name: "this week"}}, 160, false)
	tests := []struct {
		name string
		x    int
		want ink
	}{
		{name: "the name", x: 0, want: headingInk},
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

// Every view tab of Accounts is a target over its name and a
// cell either side, on a phone and wider alike, and the one shown is picked
// out over them all, both cells with it, as the page's tabs are, and no
// further.
func TestViewTabsAreTargetsOverTheirNamesAndACellEitherSide(t *testing.T) {
	headings := []struct {
		name    string
		heading heading
	}{
		{name: "Accounts' Compare", heading: AccountsShown{Shown: AccountsCompare, Accounts: threeConfigured}.heading(views.DialOf(views.LengthThisWeek.Now(), routedAt, time.Monday, time.Time{}))},
		{name: "Accounts' Detail", heading: AccountsShown{Shown: AccountsDetail, Accounts: threeConfigured}.heading(views.DialOf(views.LengthThisWeek.Now(), routedAt, time.Monday, time.Time{}))},
		{name: "Accounts' Tokens", heading: AccountsShown{Shown: AccountsTokens, Accounts: threeConfigured}.heading(views.DialOf(views.LengthThisWeek.Now(), routedAt, time.Monday, time.Time{}))},
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
func accountsAt(accounts config.Accounts, sub AccountsView, period views.Period) func(*Frame) {
	return func(f *Frame) {
		*f = f.over(period)
		f.Accounts = AccountsShown{Shown: sub, Accounts: accounts}
	}
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
		{name: "Accounts' Compare", view: Accounts, shown: accountsAt(threeConfigured, AccountsCompare, views.LengthThisWeek.Now()), edge: 61},
		{name: "Accounts' Detail", view: Accounts, shown: accountsAt(threeConfigured, AccountsDetail, views.LengthToday.Now()), edge: 61},
		{name: "Accounts with one account", view: Accounts, shown: accountsAt(threeConfigured[:1], AccountsDetail, views.LengthThisWeek.Now()), edge: 52},
		{name: "Accounts' Tokens", view: Accounts, shown: accountsAt(threeConfigured, AccountsTokens, views.LengthThisWeek.Now()), edge: 61},
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
// widest form's and the state's least's to say: neither the page's loading,
// nor stepping the period with < and >, moves it, at any width; and where
// the period sits beside, the state shows its least form whole, and the
// period clears it.
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
	for _, period := range views.Lengths() {
		add("Accounts' Compare", Accounts, accountsAt(threeConfigured, AccountsCompare, period.Now()))
		add("Accounts' Detail", Accounts, accountsAt(threeConfigured, AccountsDetail, period.Now()))
		add("Accounts with one account", Accounts, accountsAt(threeConfigured[:1], AccountsDetail, period.Now()))
	}
	for _, period := range views.Lengths() {
		add("Accounts' Tokens", Accounts, accountsAt(threeConfigured, AccountsTokens, period.Now()))
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

// A heading's state is a bounded qualifier, never cut: beside four sub-tabs
// and eight weeks, from 40 columns, loading… reads whole, beside the
// sub-tabs or on the row under them, and the heading takes the same rows
// whether it's loading or not.
func TestLoadingReadsWholeBesideFourSubTabs(t *testing.T) {
	for width := 40; width <= 220; width++ {
		phone := frameOf(width, 61).phone()
		var rows []int
		for _, h := range []heading{eightWeeks, eightWeeks.loading()} {
			rows = append(rows, h.rows(width, phone))
			var said strings.Builder
			for _, p := range h.pieces(0, width, phone) {
				said.WriteString(p.line.plain() + "\n")
			}
			if h.reading && !strings.Contains(said.String(), loadingWord) {
				t.Errorf("at %d: the heading reads\n%s\nwant %q whole in it", width, said.String(), loadingWord)
			}
		}
		if rows[0] != rows[1] {
			t.Errorf("at %d: the heading takes %d rows, and loading %d, want the same", width, rows[0], rows[1])
		}
	}
}

// The Log's filter is free text, its length its data's: its heading takes
// the same rows whatever it keeps, or none, loading or not, and says its
// count whole at every width, beside the sub-tabs from where loading…'s room
// and the count fit after them, and on the row under narrower.
func TestTheLogsHeadingNeverHangsOnItsFilter(t *testing.T) {
	count := line{{"35 hidden", secondaryInk}, spaces(2), {"f", keyInk}}
	filters := map[string]line{
		"none":  nil,
		"short": {{"only side", faintInk}},
		"long":  {{"only d28c switchboard · work-team-plan · personal-research · opus", faintInk}},
	}
	for width := 40; width <= 99; width++ {
		want := 1
		if width < 48 {
			want = 2
		}
		for name, filter := range filters {
			h := heading{name: "LOG", subTabs: logViews, state: filter, free: true, right: count, compactRight: count}
			for _, h := range []heading{h, h.loading()} {
				if got := h.rows(width, true); got != want {
					t.Errorf("at %d, filtered %s, loading %t: the heading takes %d rows, want %d", width, name, h.reading, got, want)
				}
				c, _ := laidOn(h, width, true)
				if rows := strings.Join(textOf(c.rows(Look{})), "\n"); !strings.Contains(rows, count.plain()) {
					t.Errorf("at %d, filtered %s, loading %t: the heading reads\n%s\nwant %q whole", width, name, h.reading, rows, count.plain())
				}
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
