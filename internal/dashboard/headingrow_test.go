package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
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
// over the week; Accounts', its period between its keys; and History's, its
// sub-tabs set tight on a phone.
var (
	filteredLog = heading{
		name: "LOG", subTabs: []string{"Requests", "Events"},
		state: line{{"only d28c switchboard", faintInk}},
		right: line{{"830 hidden", secondaryInk}, spaces(3), {"f", keyInk}, {" what it shows", mutedInk}},
	}
	steppedBack  = heading{name: "RUNWAY", state: line{{"Wed 30 Sep", faintInk}}, period: "5-hour"}
	overTheWeek  = heading{name: "RUNWAY", period: "week"}
	comparing    = heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, period: "this week"}
	tightHistory = heading{name: "HISTORY", subTabs: historyViews, tight: true, period: "last 12 months"}
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
			name: "on a phone, the sub-tabs without [ ] or a rule, the period on the row under them", heading: comparing, width: 52, phone: true,
			want: []string{"ACCOUNTS    Compare  Detail", blanks(39) + "< this week >"},
		},
		{
			name:    "on a phone, the sub-tabs set tight, two cells after the name, each its name and a space, as history-phone-year-52 draws them",
			heading: tightHistory, width: 52, phone: true,
			want: []string{"HISTORY  Year Weeks Days Tokens", blanks(34) + "< last 12 months >"},
		},
		{
			name: "sub-tabs set tight on a phone, set as any others wider", heading: tightHistory, width: 160,
			want: []string{"HISTORY    Year  Weeks  Days  Tokens  [ ]  " + rule(97) + "  < last 12 months >"},
		},
		{
			name: "on a phone, a line at the right on the sub-tabs' row", width: 52, phone: true,
			heading: heading{name: "LOG", subTabs: []string{"Requests", "Events"}, right: line{{"35 hidden", secondaryInk}, spaces(2), {"f", keyInk}}},
			want:    []string{"LOG    Requests  Events" + blanks(17) + "35 hidden  f"},
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
			want: []string{"LOG    Requests  Events    only d28c sw…"},
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
			if rows := tt.heading.rows(tt.phone); rows != len(tt.want) {
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

// On a phone, sub-tabs set tight are each a target over its name and the
// space after it, the one shown picked out, its space with it, as
// history-phone-year-52 draws them.
func TestSubTabsSetTightAreTargetsOverTheirNamesAndTheSpaceAfter(t *testing.T) {
	c, pieces := laidOn(tightHistory, 52, true)
	rows := c.rows(Look{})
	var subTabs []string
	for _, p := range pieces {
		if p.click.On == OnSubTab {
			subTabs = append(subTabs, under(rows, p.target()))
		}
	}
	if want := []string{"Year ", "Weeks ", "Days ", "Tokens "}; !slices.Equal(subTabs, want) {
		t.Errorf("the sub-tabs' targets are over %q, want %q", subTabs, want)
	}
	for x, want := range map[int]ink{9: shownSubTabInk, 13: shownSubTabInk, 14: tabInk} {
		if got := c.at(x, 0).ink; got != want {
			t.Errorf("column %d is in %+v, want %+v", x, got, want)
		}
	}
}
