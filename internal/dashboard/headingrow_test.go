package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

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
// over the week; and Accounts', its period's readout at its right. And the
// headings of a tab of four sub-tabs: over 12 months at now; filtered,
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
	comparing   = heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, period: weekAtNow}
	fourTabs    = heading{name: "EXAMPLE", subTabs: []string{"Feed", "Queue", "Logs", "Alerts"}, period: yearAtNow}
	filtering   = heading{
		name: fourTabs.name, subTabs: fourTabs.subTabs, period: fourTabs.period, least: "personal only",
		state:   line{{"Work only, every model  ·  ", faintInk}, {"v", keyInk}, {" for all, a model or an account", faintInk}},
		shorter: []line{{{"Work only", faintInk}}},
	}
	eightWeeks = heading{name: fourTabs.name, subTabs: fourTabs.subTabs, right: line{{"eight weeks", mutedInk}}}
)

// The readouts of the dial's period the tests draw, each keeping room for
// the widest readout views keeps room for then, as periodDial has it: this
// week and 12 months, at now; and a day gone, a week gone and 12 months
// gone.
var (
	weekAtNow = dialed{ranged: "week", when: "this week", widestRanged: "week", widestWhen: "this week", onOff: true}
	yearAtNow = dialed{ranged: "12 months", when: "now", widestRanged: "week", widestWhen: "this week", longerOff: true, onOff: true}
	dayGone   = dialed{ranged: "day", when: "yesterday", widestRanged: "week", widestWhen: widestWeekGone, gone: true, shorterOff: true}
	weekGone  = dialed{ranged: "week", when: "last week", widestRanged: "week", widestWhen: widestWeekGone, gone: true}
	yearGone  = dialed{ranged: "12 months", when: "to 1 Oct 2025", widestRanged: "week", widestWhen: widestWeekGone, gone: true, longerOff: true}
)

// widestWeekGone is when the widest readout of a period gone is, as views
// keeps room for it: a week over a year's end, neither year now's.
const widestWeekGone = "30 Dec 2024–5 Jan 2025"

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
			name: "the sub-tabs, the rule two cells after [ ], and three short of the period's readout", heading: comparing, width: 160,
			want: []string{"ACCOUNTS    Compare  Detail  [ ]  " + rule(100) + "   - week +  < this week >"},
		},
		{
			name: "on a phone, the sub-tabs two cells after the name, without [ ] or a rule, the readout at the right where it fits beside them and loading…", heading: comparing, width: 63, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail" + blanks(14) + "- week +  < this week >"},
		},
		{
			name: "on a phone, the readout on the row under the sub-tabs where it doesn't fit beside them and loading…", heading: comparing, width: 62, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail", blanks(39) + "- week +  < this week >"},
		},
		{
			name: "on a phone, loading… whole between the sub-tabs and the readout, where they fit", heading: comparing.loading(), width: 63, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail    loading…  - week +  < this week >"},
		},
		{
			name: "on a phone, a past readout on the row under where the widest gone doesn't fit beside, however narrow its own", width: 73, phone: true,
			heading: heading{name: comparing.name, subTabs: comparing.subTabs, period: dayGone},
			want:    []string{"ACCOUNTS   Compare  Detail", blanks(51) + "- day +  < yesterday >"},
		},
		{
			name: "on a phone, a past readout beside the sub-tabs where the widest gone fits", width: 76, phone: true,
			heading: heading{name: comparing.name, subTabs: comparing.subTabs, period: dayGone},
			want:    []string{"ACCOUNTS   Compare  Detail" + blanks(28) + "- day +  < yesterday >"},
		},
		{
			name:    "on a phone, four sub-tabs two cells after the name, padded, as Accounts' are",
			heading: fourTabs, width: 52, phone: true,
			want: []string{"EXAMPLE   Feed  Queue  Logs  Alerts", blanks(30) + "- 12 months +  < now >"},
		},
		{
			name: "on a phone, Accounts' padded sub-tabs leave loading… room before the readout", heading: comparing.loading(), width: 73, phone: true,
			want: []string{"ACCOUNTS   Compare  Detail    loading…" + blanks(12) + "- week +  < this week >"},
		},
		{
			name: "on a phone, four padded sub-tabs leave loading… room, before the state", width: 52, phone: true,
			heading: filtering.loading(),
			want:    []string{"EXAMPLE   Feed  Queue  Logs  Alerts    loading…", blanks(30) + "- 12 months +  < now >"},
		},
		{
			name: "on a phone, the state in its short form, where its full form doesn't fit the room four padded sub-tabs leave", width: 52, phone: true,
			heading: filtering,
			want:    []string{"EXAMPLE   Feed  Queue  Logs  Alerts    Work only", blanks(30) + "- 12 months +  < now >"},
		},
		{
			name: "on a phone, the state in its short form beside the readout, where its least would fit", width: 77, phone: true,
			heading: filtering,
			want:    []string{"EXAMPLE   Feed  Queue  Logs  Alerts    Work only" + blanks(7) + "- 12 months +  < now >"},
		},
		{
			name: "four sub-tabs wider than a phone", heading: fourTabs, width: 160,
			want: []string{"EXAMPLE    Feed  Queue  Logs  Alerts  [ ]  " + rule(92) + "   - 12 months +  < now >"},
		},
		{
			name: "wider than a phone, the state in its short form where its full form doesn't fit, then the rule", width: 108,
			heading: filtering,
			want:    []string{"EXAMPLE    Feed  Queue  Logs  Alerts  [ ]   Work only " + rule(29) + "   - 12 months +  < now >"},
		},
		{
			name: "wider than a phone, a readout gone, the rule three short of it", heading: heading{name: "PROJECTS", period: yearGone}, width: 104,
			want: []string{"PROJECTS  " + rule(59) + "   - 12 months +  < to 1 Oct 2025 >"},
		},
		{
			name: "on a phone, the sub-tabs two cells after the name, padded, and a line at the right on their row, as log-requests-phone-52 draws them", width: 52, phone: true,
			heading: heading{name: "LOG", subTabs: []string{"Requests", "Events"}, right: line{{"35 hidden", secondaryInk}, spaces(2), {"f", keyInk}}},
			want:    []string{"LOG   Requests  Events" + blanks(18) + "35 hidden  f"},
		},
		{
			name: "on a phone, a tab without sub-tabs keeps its rule and its readout, where the widest gone fits after loading…'s room", width: 60, phone: true,
			heading: heading{name: "PROJECTS", period: dayGone},
			want:    []string{"PROJECTS  " + rule(25) + "   - day +  < yesterday >"},
		},
		{
			name: "on a phone, a tab without sub-tabs takes the row under for its readout, where the widest gone doesn't fit, its rule to the last column", width: 59, phone: true,
			heading: heading{name: "PROJECTS", period: dayGone},
			want:    []string{"PROJECTS  " + rule(49), blanks(37) + "- day +  < yesterday >"},
		},
		{
			name: "the state whole, never cut for the readout, which takes the row under where the state's least leaves it no room", width: 110,
			heading: heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, state: line{{"accounts 1–6 of 10  ·  ", faintInk}, {"←→", keyInk}, {" moves along them; all accounts stays put", faintInk}}, period: weekAtNow},
			want:    []string{"ACCOUNTS    Compare  Detail  [ ]   accounts 1–6 of 10  ·  ←→ moves along them; all accounts stays put " + rule(8), blanks(87) + "- week +  < this week >"},
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

// The readout is drawn as it stands: the range in text.muted; when in
// text.muted at now and in accent.attention gone, so it never reads as now;
// -, +, < and > in bold accent.key, each faint, still bold, where its step
// does nothing: - at the shortest range, + at the longest, < at the ledger's
// first day and > at now.
func TestTheReadoutIsDrawnAsItStands(t *testing.T) {
	tests := []struct {
		name                                    string
		dial                                    dialed
		said                                    string
		shorter, ranged, longer, back, when, on ink
	}{
		{name: "now", dial: weekAtNow, said: "- week +  < this week >", shorter: keyInk, ranged: mutedInk, longer: keyInk, back: keyInk, when: mutedInk, on: stepOffInk},
		{name: "now, at the longest range", dial: yearAtNow, said: "- 12 months +  < now >", shorter: keyInk, ranged: mutedInk, longer: stepOffInk, back: keyInk, when: mutedInk, on: stepOffInk},
		{name: "now, at the ledger's first day", dial: func() dialed { d := yearAtNow; d.backOff = true; return d }(), said: "- 12 months +  < now >",
			shorter: keyInk, ranged: mutedInk, longer: stepOffInk, back: stepOffInk, when: mutedInk, on: stepOffInk},
		{name: "a day gone, at the shortest range", dial: dayGone, said: "- day +  < yesterday >", shorter: stepOffInk, ranged: mutedInk, longer: keyInk, back: keyInk, when: warningInk, on: keyInk},
		{name: "a week gone, at the ledger's first day", dial: func() dialed { d := weekGone; d.backOff = true; return d }(), said: "- week +  < last week >",
			shorter: keyInk, ranged: mutedInk, longer: keyInk, back: stepOffInk, when: warningInk, on: keyInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pieces := periodPieces(tt.dial)
			var said strings.Builder
			for _, p := range pieces {
				said.WriteString(p.line.plain())
			}
			inkOf := func(i int) ink { return pieces[i].line[0].ink }
			got := []ink{inkOf(0), inkOf(1), inkOf(2), inkOf(4), inkOf(5), inkOf(6)}
			if want := []ink{tt.shorter, tt.ranged, tt.longer, tt.back, tt.when, tt.on}; !slices.Equal(got, want) || said.String() != tt.said {
				t.Errorf("the readout reads %q, -, the range, +, <, when and > in %+v; want %q in %+v", said.String(), got, tt.said, want)
			}
		})
	}
	if want := (ink{token: theme.TextFaint, bold: true}); stepOffInk != want || warningInk != (ink{token: theme.AccentAttention}) {
		t.Errorf("a step that does nothing is in %+v and a period gone in %+v, want %+v and accent.attention", stepOffInk, warningInk, want)
	}
}

// The heading draws the readout as views says it: at now, the range and
// when at now, > doing nothing, keeping room for the widest readout at now;
// gone, its own when, keeping room for the widest gone; - doing nothing at
// the shortest range and + at the longest; < doing nothing on a period
// holding the ledger's first day.
func TestTheHeadingDrawsTheReadoutAsViewsSaysIt(t *testing.T) {
	week := views.LengthThisWeek.Now()
	last, _ := week.Back(routedAt, time.Monday, time.Time{})
	yesterday, _ := views.LengthToday.Now().Back(routedAt, time.Monday, time.Time{})
	yearBack, _ := views.LengthTwelveMonths.Now().Back(routedAt, time.Monday, time.Time{})
	began := last.From(routedAt, time.Monday)
	tests := []struct {
		name string
		dial views.Dial
		want dialed
	}{
		{name: "now", dial: views.DialOf(week, routedAt, time.Monday, time.Time{}), want: weekAtNow},
		{name: "12 months at now", dial: views.DialOf(views.LengthTwelveMonths.Now(), routedAt, time.Monday, time.Time{}), want: yearAtNow},
		{name: "a week gone", dial: views.DialOf(last, routedAt, time.Monday, time.Time{}), want: weekGone},
		{name: "yesterday", dial: views.DialOf(yesterday, routedAt, time.Monday, time.Time{}), want: dayGone},
		{name: "12 months gone", dial: views.DialOf(yearBack, routedAt, time.Monday, time.Time{}), want: yearGone},
		{name: "the week holding the ledger's first day", dial: views.DialOf(last, routedAt, time.Monday, began), want: func() dialed { d := weekGone; d.backOff = true; return d }()},
	}
	for _, tt := range tests {
		if got := periodDial(tt.dial); got != tt.want {
			t.Errorf("%s: the heading draws %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

// A phone's heading with sub-tabs keeps room for the widest readout at now
// and, apart, for the widest gone: so no step at now, nor any step among
// periods gone, moves it between one row and two, on Accounts or Sessions'
// List. At 73 columns, as a phone draws them, the readout at now sits
// beside the sub-tabs, and every past readout on the row under them.
func TestAPhonesHeadingHoldsStillAtNowAndAmongPeriodsGone(t *testing.T) {
	const phone = 73
	tabs := []struct {
		name  string
		view  View
		shown func(period views.Period) func(*Frame)
	}{
		{name: "Accounts' Compare", view: Accounts, shown: func(p views.Period) func(*Frame) { return accountsAt(threeConfigured, AccountsCompare, p) }},
		{name: "Accounts' Detail", view: Accounts, shown: func(p views.Period) func(*Frame) { return accountsAt(threeConfigured, AccountsDetail, p) }},
		{name: "Sessions' List", view: Sessions, shown: sessionsListAt},
	}
	for _, tab := range tabs {
		for _, length := range views.Lengths() {
			p := length.Now()
			for back := range 60 {
				f := headingFrame(phone, tab.view, tab.shown(p), false)
				want := 2
				if back == 0 {
					want = 1
				}
				if rows := f.heading().rows(f.Width, f.phone()); rows != want {
					t.Errorf("%s at %d over %q, %d back, takes %d rows, want %d", tab.name, phone, f.Dial.Name(), back, rows, want)
				}
				p, _ = p.Back(now, time.Monday, time.Time{})
			}
		}
	}
}

// projectsAt has a frame show Projects over the period given.
func projectsAt(period views.Period) func(*Frame) {
	return func(f *Frame) { *f = f.over(period) }
}

// widestGonePeriod is the period gone whose readout is the widest a period
// gone takes, as views keeps room for it: a week stepped back from now to
// one over a year's end, neither year now's.
func widestGonePeriod(t *testing.T) views.Period {
	t.Helper()
	p, _ := views.LengthThisWeek.Now().Back(now, time.Monday, time.Time{})
	ranged, when := dialOf(p).Widest()
	for range 200 {
		if d := dialOf(p); d.Range() == ranged && d.When() == when {
			return p
		}
		p, _ = p.Back(now, time.Monday, time.Time{})
	}
	t.Fatalf("no week within 200 back reads - %s +  < %s >", ranged, when)
	return p
}

// readoutDrawn is the period's readout as the heading pieces given draw
// it: its text, from the first column of - to the last of >, and the row
// and column it starts at; false where none of its signs is drawn. It fails
// t where some of its signs are drawn and not all, or where they aren't
// laid end to end on one row.
func readoutDrawn(t *testing.T, pieces []piece) (said string, x, y int, ok bool) {
	t.Helper()
	first := slices.IndexFunc(pieces, func(p piece) bool { return p.click.On == OnPress && p.click.Key == "-" })
	signs := 0
	for _, p := range pieces {
		if p.click.On == OnStep || p.click.On == OnPress {
			signs++
		}
	}
	if first < 0 {
		if signs > 0 {
			t.Errorf("the heading draws %d of the readout's signs without its -", signs)
		}
		return "", 0, 0, false
	}
	readout := pieces[first:min(first+7, len(pieces))]
	var b strings.Builder
	at := readout[0].x
	for _, p := range readout {
		if p.y != readout[0].y || p.x != at {
			t.Errorf("the readout's pieces aren't end to end on one row: %+v", readout)
			break
		}
		b.WriteString(p.line.plain())
		at += p.line.width()
	}
	if signs != 4 || len(readout) != 7 {
		t.Errorf("the heading draws %d of the readout's signs, in %d pieces, want all four, in 7", signs, len(readout))
	}
	return b.String(), readout[0].x, readout[0].y, true
}

// Every tab with the dial draws the readout whole, at the last column, at
// every width from 36 columns, at now and gone, the widest gone among
// them; narrower, whole or not at all, never cut. On the row under, the
// state gives way to it.
func TestEveryDialTabDrawsItsReadoutWhole(t *testing.T) {
	periods := []views.Period{widestGonePeriod(t)}
	for _, length := range views.Lengths() {
		p := length.Now()
		for range 3 {
			periods = append(periods, p)
			p, _ = p.Back(now, time.Monday, time.Time{})
		}
	}
	tabs := []struct {
		name  string
		view  View
		shown func(views.Period) func(*Frame)
	}{
		{name: "Compare", view: Accounts, shown: func(p views.Period) func(*Frame) { return accountsAt(threeConfigured, AccountsCompare, p) }},
		{name: "Detail", view: Accounts, shown: func(p views.Period) func(*Frame) { return accountsAt(threeConfigured, AccountsDetail, p) }},
		{name: "Tokens", view: Accounts, shown: func(p views.Period) func(*Frame) { return accountsAt(threeConfigured, AccountsTokens, p) }},
		{name: "Projects", view: Projects, shown: projectsAt},
		{name: "the List", view: Sessions, shown: sessionsListAt},
	}
	for _, tab := range tabs {
		for _, period := range periods {
			d := dialOf(period)
			want := "- " + d.Range() + " +  < " + d.When() + " >"
			for width := 20; width <= 140; width++ {
				for _, loading := range []bool{false, true} {
					f := headingFrame(width, tab.view, tab.shown(period), loading)
					where := fmt.Sprintf("%s at %d over %q, loading: %t", tab.name, width, d.Name(), loading)
					said, x, y, ok := readoutDrawn(t, f.headingPieces())
					switch {
					case !ok && width >= ansi.StringWidth(want):
						t.Errorf("%s, draws no readout, want %q", where, want)
					case ok && (said != want || x < 0 || x+ansi.StringWidth(said) != width):
						t.Errorf("%s, draws %q from column %d, want %q whole, ending at the last column", where, said, x, want)
					case ok && y == f.headingY() && !f.heading().compact(f.phone()):
						ruledClear(t, f, x, where)
					}
				}
			}
		}
	}
}

// ruledClear fails t unless the frame's heading row, a heading with a rule
// whose readout starts at column x of it, draws a cell of rule at least,
// ending readoutGap cells before the readout, and its state, where it has
// one, loading… among its forms, whole in one of them.
func ruledClear(t *testing.T, f Frame, x int, where string) {
	t.Helper()
	c := newCanvas(f.Width, f.headingY()+2)
	for _, p := range f.headingPieces() {
		c.line(p.x, p.y, p.line)
	}
	row := []rune(c.rows(Look{})[f.headingY()])
	if gap := string(row[x-readoutGap : x]); row[x-readoutGap-1] != '─' || strings.TrimSpace(gap) != "" {
		t.Errorf("%s, the heading reads %q, want a cell of rule ending %d cells before the readout", where, string(row), readoutGap)
	}
	forms := f.heading().stateForms()
	if forms != nil && !slices.ContainsFunc(forms, func(l line) bool { return strings.Contains(string(row), l.plain()+" ─") }) {
		t.Errorf("%s, the heading reads %q, want its state whole in one of its forms, the rule after it", where, string(row))
	}
}

// sessionsListAt has a frame show Sessions' List over the period given.
func sessionsListAt(period views.Period) func(*Frame) {
	return func(f *Frame) {
		*f = f.over(period)
		f.SessionsTab = SessionsShown{Shown: SessionsList}
	}
}

func TestTheHeadingRowsInks(t *testing.T) {
	c, _ := laidOn(heading{name: "ACCOUNTS", subTabs: []string{"Compare", "Detail"}, shown: 1, period: weekGone}, 160, false)
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
		{name: "the rule's last cell", x: 133, want: borderInk},
		{name: "the cell after it", x: 134, want: ink{}},
		{name: "-", x: 137, want: keyInk},
		{name: "the range", x: 139, want: mutedInk},
		{name: "+", x: 144, want: keyInk},
		{name: "<", x: 147, want: keyInk},
		{name: "when", x: 149, want: warningInk},
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
			key  string
			text string
		}{
			{on: OnSubTab, sub: 0, text: " Compare "}, {on: OnSubTab, sub: 1, text: " Detail "},
			{on: OnPress, key: "-", text: "-"}, {on: OnPress, key: "+", text: "+"},
			{on: OnStep, step: -1, text: "<"}, {on: OnStep, step: 1, text: ">"},
		}
		if len(targets) != len(want) {
			t.Fatalf("on a phone: %v, the targets are %+v, want the sub-tabs, -, +, < and >", phone, targets)
		}
		for i, w := range want {
			tt := targets[i]
			if tt.On != w.on || tt.SubTab != w.sub || tt.Step != w.step || tt.Key != w.key || under(rows, tt) != w.text {
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

// A phone's readout sits at the right of the heading's row from the width
// its name, its sub-tabs, three cells on loading… whole, or the state's
// least form where that's wider, and two cells on the widest readout it
// keeps room for fit, at now or gone, and takes the row under them a cell
// narrower, the view starting a row on.
func TestAPhonesReadoutSitsBesideTheHeadingWhereItFits(t *testing.T) {
	yesterday, _ := views.LengthToday.Now().Back(now, time.Monday, time.Time{})
	lastWeek, _ := views.LengthThisWeek.Now().Back(now, time.Monday, time.Time{})
	tests := []struct {
		name  string
		view  View
		shown func(*Frame)
		edge  int
	}{
		{name: "Accounts' Compare", view: Accounts, shown: accountsAt(threeConfigured, AccountsCompare, views.LengthThisWeek.Now()), edge: 71},
		{name: "Accounts' Detail", view: Accounts, shown: accountsAt(threeConfigured, AccountsDetail, views.LengthToday.Now()), edge: 71},
		{name: "Accounts with one account", view: Accounts, shown: accountsAt(threeConfigured[:1], AccountsDetail, views.LengthThisWeek.Now()), edge: 62},
		{name: "Accounts' Tokens", view: Accounts, shown: accountsAt(threeConfigured, AccountsTokens, views.LengthThisWeek.Now()), edge: 71},
		{name: "Sessions' List", view: Sessions, shown: sessionsListAt(views.LengthThisWeek.Now()), edge: 65},
		{name: "Accounts' Tokens, gone", view: Accounts, shown: accountsAt(threeConfigured, AccountsTokens, lastWeek), edge: 84},
		{name: "Accounts' Detail, gone", view: Accounts, shown: accountsAt(threeConfigured, AccountsDetail, yesterday), edge: 84},
		{name: "Accounts with one account, gone", view: Accounts, shown: accountsAt(threeConfigured[:1], AccountsDetail, lastWeek), edge: 75},
		{name: "Sessions' List, gone", view: Sessions, shown: sessionsListAt(yesterday), edge: 78},
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
					var signs []int
					for _, p := range f.headingPieces() {
						if p.click.On == OnStep || p.click.On == OnPress {
							signs = append(signs, p.y)
						}
					}
					readoutRow := f.headingY() + rows - 1
					if want := []int{readoutRow, readoutRow, readoutRow, readoutRow}; !slices.Equal(signs, want) {
						t.Errorf("-, +, < and > are on rows %v, want %v", signs, want)
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

// Whether the readout sits beside the name is the width's, the widest
// readout's it keeps room for and the state's least's to say: neither the
// page's loading, nor any step at now, nor any among periods gone, moves
// it, at any width; and where the readout sits beside, the state shows its
// least form whole, and the readout clears it.
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
	for _, length := range views.Lengths() {
		period := length.Now()
		for back := range 30 {
			when := " at now"
			if back > 0 {
				when = " gone"
			}
			add("Accounts' Compare"+when, Accounts, accountsAt(threeConfigured, AccountsCompare, period))
			add("Accounts' Detail"+when, Accounts, accountsAt(threeConfigured, AccountsDetail, period))
			add("Accounts with one account"+when, Accounts, accountsAt(threeConfigured[:1], AccountsDetail, period))
			add("Accounts' Tokens"+when, Accounts, accountsAt(threeConfigured, AccountsTokens, period))
			add("Sessions' List"+when, Sessions, sessionsListAt(period))
			add("Projects"+when, Projects, projectsAt(period))
			period, _ = period.Back(now, time.Monday, time.Time{})
		}
	}
	widest := widestGonePeriod(t)
	add("Accounts' Compare gone", Accounts, accountsAt(threeConfigured, AccountsCompare, widest))
	add("Accounts' Detail gone", Accounts, accountsAt(threeConfigured, AccountsDetail, widest))
	add("Accounts with one account gone", Accounts, accountsAt(threeConfigured[:1], AccountsDetail, widest))
	add("Accounts' Tokens gone", Accounts, accountsAt(threeConfigured, AccountsTokens, widest))
	add("Sessions' List gone", Sessions, sessionsListAt(widest))
	add("Projects gone", Projects, projectsAt(widest))
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

// ←→ steps along Compare's accounts without moving its heading between one
// row and two, though which it shows gains a digit as it steps: of ten
// accounts and twelve, this week, last week and the widest period gone, from
// 20 columns to 200.
func TestSteppingAlongComparesAccountsNeverMovesTheHeadingsRows(t *testing.T) {
	lastWeek, _ := views.LengthThisWeek.Now().Back(now, time.Monday, time.Time{})
	periods := []views.Period{views.LengthThisWeek.Now(), lastWeek, widestGonePeriod(t)}
	for _, n := range []int{10, 12} {
		for _, period := range periods {
			for width := 20; width <= 200; width++ {
				shown := headingFrame(width, Accounts, accountsAt(numberedConfigured(n), AccountsCompare, period), false)
				var rows []int
				for first := range shown.compareLast() + 1 {
					f := shown
					f.Accounts.First = first
					rows = append(rows, f.heading().rows(width, f.phone()))
				}
				if slices.Min(rows) != slices.Max(rows) {
					t.Errorf("Compare of %d at %d over %q: its headings, stepped along the accounts, take %v rows, want the same for each", n, width, dialOf(period).Name(), rows)
				}
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
// form whole, where it has a state, and the readout's - starts two cells or
// more after what's before it.
func besideWhole(t *testing.T, f Frame, name string) {
	t.Helper()
	var said strings.Builder
	var before, stepAt int
	for _, p := range f.headingPieces() {
		if p.click.On == OnPress && p.click.Key == "-" {
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
		t.Errorf("%s at %d, loading: %t: - is at column %d, want it two cells or more after column %d", name, f.Width, f.Loading, stepAt, before)
	}
}
