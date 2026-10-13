package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// logNow is the moment the Log's frames are drawn at.
var logNow = time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("BST", 60*60))

// loggedOn is the time given today, or on a day back, in logNow's zone.
func loggedOn(daysBack, hour, minute, second int) time.Time {
	return time.Date(2026, 10, 1-daysBack, hour, minute, second, 0, logNow.Location())
}

// The heading's right, in the order it drops its parts: what alone hides
// the lines, then what f does; a filter's count alone, as several things
// hide lines; f alone with none hidden; and Events' count of today's, of
// all of them where some are hidden. On a phone, the count muted and f.
func TestTheLogsHeadingCountsInItsFormsInTheirDropOrder(t *testing.T) {
	tests := []struct {
		name  string
		count logHeadingCount
		forms []string
		phone string
	}{
		{
			name: "checks and counts hidden", count: hiddenCount(35, "checks and counts"),
			forms: []string{"35 hidden: checks and counts   f what it shows", "35 hidden   f what it shows", "35 hidden   f"},
			phone: "35 hidden  f",
		},
		{name: "counts hidden", count: hiddenCount(4, "counts"), forms: []string{"4 hidden: counts   f what it shows", "4 hidden   f what it shows", "4 hidden   f"}, phone: "4 hidden  f"},
		{name: "filtered", count: hiddenCount(830, ""), forms: []string{"830 hidden   f what it shows", "830 hidden   f"}, phone: "830 hidden  f"},
		{name: "a thousand hidden", count: hiddenCount(1240, ""), forms: []string{"1,240 hidden   f what it shows", "1,240 hidden   f"}, phone: "1,240 hidden  f"},
		{name: "none hidden", count: hiddenCount(0, ""), forms: []string{"f what it shows", "f"}, phone: "f"},
		{name: "every event", count: todayCount(20, 20), forms: []string{"20 today   f what it shows", "20 today   f"}, phone: "20 today  f"},
		{name: "some events hidden", count: todayCount(18, 20), forms: []string{"18 of 20 today   f what it shows", "18 of 20 today   f"}, phone: "18 of 20 today  f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := plains(tt.count.forms()); !slices.Equal(got, tt.forms) {
				t.Errorf("forms() = %q, want %q", got, tt.forms)
			}
			if got := tt.count.phone().plain(); got != tt.phone {
				t.Errorf("phone() = %q, want %q", got, tt.phone)
			}
		})
	}
}

// The heading's right is drawn as the frames draw it: the count in
// text.secondary and the words after it in text.muted, f bold accent.key;
// on a phone, the count all in text.muted.
func TestTheLogsHeadingCountIsInkedAsTheFramesDrawIt(t *testing.T) {
	want := line{
		{"35", secondaryInk}, {" hidden: checks and counts", mutedInk}, {"   ", ink{}}, {"f", keyInk}, {" what it shows", mutedInk},
	}
	if got := hiddenCount(35, "checks and counts").forms()[0].inked(); !slices.Equal(got, want) {
		t.Errorf("the whole form is inked %+v, want %+v", got, want)
	}
	if got, want := hiddenCount(35, "").phone().inked(), (line{{"35 hidden", mutedInk}, {"  ", ink{}}, {"f", keyInk}}); !slices.Equal(got, want) {
		t.Errorf("the phone's form is inked %+v, want %+v", got, want)
	}
	want = line{{"18", secondaryInk}, {" of ", mutedInk}, {"20", secondaryInk}, {" today", mutedInk}}
	if got := todayCount(18, 20).count.inked(); !slices.Equal(got, want) {
		t.Errorf("Events' count is inked %+v, want %+v", got, want)
	}
}

// Narrowing, the heading drops what hides the lines, then what f does,
// before the filter is cut: each form taken where it leaves the filter
// whole, the rule shrinking to nothing, in the cells from [ ] to the last
// column.
func TestTheLogsHeadingDropsItsRightsPartsBeforeCuttingTheFilter(t *testing.T) {
	hidden := hiddenCount(35, "checks and counts").forms()
	filtered := hiddenCount(830, "").forms()
	filter := logFilterNames{sessions: []string{"d28c switchboard"}}.said()
	long := logFilterNames{sessions: []string{"d28c switchboard"}, accounts: []string{"personal"}, failed: true}.said()
	tests := []struct {
		name   string
		forms  []line
		filter line
		room   int
		want   string
	}{
		{name: "at 160", forms: hidden, room: 132, want: "35 hidden: checks and counts   f what it shows"},
		{name: "the whole, its rule as short as none", forms: hidden, room: 50, want: "35 hidden: checks and counts   f what it shows"},
		{name: "why gone", forms: hidden, room: 49, want: "35 hidden   f what it shows"},
		{name: "why gone, its rule as short as none", forms: hidden, room: 31, want: "35 hidden   f what it shows"},
		{name: "what f does gone", forms: hidden, room: 30, want: "35 hidden   f"},
		{name: "the last, where nothing fits", forms: hidden, room: 16, want: "35 hidden   f"},
		{name: "filtered at 160", forms: filtered, filter: filter, room: 132, want: "830 hidden   f what it shows"},
		{name: "a long filter, whole", forms: filtered, filter: long, room: 75, want: "830 hidden   f what it shows"},
		{name: "a long filter, what f does gone", forms: filtered, filter: long, room: 74, want: "830 hidden   f"},
		{name: "a long filter, the filter whole", forms: filtered, filter: long, room: 61, want: "830 hidden   f"},
		{name: "a long filter to be cut", forms: filtered, filter: long, room: 60, want: "830 hidden   f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := logHeadingRight(tt.forms, tt.filter, tt.room).plain(); got != tt.want {
				t.Errorf("logHeadingRight(%d) = %q, want %q", tt.room, got, tt.want)
			}
		})
	}
}

// The filter is said as what it keeps: sessions, accounts, models, then
// failed, " · " between, in text.faint; nothing where it keeps every line.
func TestTheLogsFilterIsSaidAsWhatItKeeps(t *testing.T) {
	tests := []struct {
		name   string
		filter logFilterNames
		want   string
	}{
		{name: "a session", filter: logFilterNames{sessions: []string{"d28c switchboard"}}, want: "only d28c switchboard"},
		{
			name:   "one of each",
			filter: logFilterNames{failed: true, models: []string{"opus-5-5"}, accounts: []string{"side"}, sessions: []string{"d28c switchboard"}},
			want:   "only d28c switchboard · side · opus-5-5 · failed",
		},
		{name: "failed alone", filter: logFilterNames{failed: true}, want: "only failed"},
		{name: "every line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.filter.said()
			if got.plain() != tt.want || (got != nil && got[0].ink != faintInk) {
				t.Errorf("said() = %+v, want %q in text.faint", got, tt.want)
			}
		})
	}
}

// The state line at 160 columns, and on a phone, is as the frames draw it.
func TestTheLogsStateLineIsAsTheFramesDrawIt(t *testing.T) {
	tests := []struct {
		stem  string
		width int
		row   int
		state logState
	}{
		{stem: "log-requests-160", width: 160, row: 35, state: logState{standing: logFollowing, shown: 1240, noun: "line"}},
		{stem: "log-requests-phone-52", width: 52, row: 31, state: logState{standing: logFollowing, shown: 1240, noun: "line"}},
		{stem: "log-requests-filtered-160", width: 160, row: 35, state: logState{shown: 445, of: "d28c"}},
		{stem: "log-events-160", width: 160, row: 35, state: logState{shown: 20, noun: "event"}},
		{
			stem: "log-request-details-160", width: 160, row: 35,
			state: logState{standing: logPaused, at: loggedOn(0, 14, 31, 13), fresh: 214, shown: 1240, noun: "line"},
		},
		{stem: "log-not-live-160", width: 160, row: 35, state: logState{standing: logNotLive, at: loggedOn(0, 14, 40, 52), shown: 1198, noun: "line"}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			want := strings.TrimRight(referenceRows(t, tt.stem, "txt")[tt.row], " ")
			if got := tt.state.line(tt.width, logNow).plain(); got != want {
				t.Errorf("the state line reads\n%q\nwant\n%q", got, want)
			}
		})
	}
}

// Narrowing, the state line drops its last clause, then its count, then is
// cut; paused at a line of another day, or not live with lines to another,
// it names the day; and not live with no lines read, it says no more.
func TestTheLogsStateLineDropsItsClausesFromTheEndThenTheCount(t *testing.T) {
	paused := logState{standing: logPaused, at: loggedOn(0, 14, 31, 13), fresh: 214, shown: 1240, noun: "line"}
	notLive := logState{standing: logNotLive, at: loggedOn(0, 14, 40, 52), shown: 1198, noun: "line"}
	tests := []struct {
		name  string
		state logState
		width int
		want  string
	}{
		{name: "paused, whole", state: paused, width: 68, want: "‖ paused at 14:31:13   ▼ 214 new   end to follow   1,240 lines today"},
		{name: "paused, end to follow gone", state: paused, width: 67, want: "‖ paused at 14:31:13   ▼ 214 new" + strings.Repeat(" ", 18) + "1,240 lines today"},
		{name: "paused on a phone", state: paused, width: 52, want: "‖ paused at 14:31:13   ▼ 214 new   1,240 lines today"},
		{name: "paused, the count gone", state: paused, width: 51, want: "‖ paused at 14:31:13   ▼ 214 new"},
		{name: "paused, cut", state: paused, width: 31, want: "‖ paused at 14:31:13   ▼ 214 n…"},
		{
			name: "paused, none new", state: logState{standing: logPaused, at: loggedOn(0, 14, 31, 3), shown: 1240, noun: "line"}, width: 56,
			want: "‖ paused at 14:31:03   end to follow   1,240 lines today",
		},
		{
			name: "paused at another day", state: logState{standing: logPaused, at: loggedOn(1, 23, 47, 30), fresh: 3, shown: 1240, noun: "line"}, width: 80,
			want: "‖ paused at Wed 30 Sep 23:47:30   ▼ 3 new   end to follow" + strings.Repeat(" ", 6) + "1,240 lines today",
		},
		{name: "not live, whole from 112", state: notLive, width: 112, want: "○ not live   the ledger's lines to 14:40:52; those since come in as the router answers again   1,198 lines today"},
		{name: "not live, its clause gone", state: notLive, width: 111, want: "○ not live   the ledger's lines to 14:40:52" + strings.Repeat(" ", 51) + "1,198 lines today"},
		{name: "not live, the count gone", state: notLive, width: 62, want: "○ not live   the ledger's lines to 14:40:52"},
		{
			name: "not live to another day", state: logState{standing: logNotLive, at: loggedOn(1, 23, 47, 30), shown: 0, noun: "line"}, width: 80,
			want: "○ not live   the ledger's lines to Wed 30 Sep 23:47:30" + strings.Repeat(" ", 13) + "0 lines today",
		},
		{name: "not live with no lines read", state: logState{standing: logNotLive, shown: 0, noun: "line"}, width: 40, want: "○ not live" + strings.Repeat(" ", 17) + "0 lines today"},
		{name: "one line", state: logState{shown: 1, noun: "line"}, width: 30, want: "● following" + strings.Repeat(" ", 7) + "1 line today"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.TrimRight(tt.state.line(tt.width, logNow).plain(), " "); got != tt.want {
				t.Errorf("line(%d) =\n%q\nwant\n%q", tt.width, got, tt.want)
			}
		})
	}
}

// Paused, the state line is inked as the design draws it: ‖ bold
// accent.attention, paused in text.secondary and its time in text.muted; ▼
// in accent.key and its count bold; end bold accent.key.
func TestTheLogsPausedStateLineIsInkedAsTheDesignDrawsIt(t *testing.T) {
	state := logState{standing: logPaused, at: loggedOn(0, 14, 31, 13), fresh: 214, shown: 1240, noun: "line"}
	got := state.line(68, logNow).inked()
	want := line{
		{"‖", alertInk}, {" paused", secondaryInk}, {" at 14:31:13", mutedInk}, {"   ", ink{}}, {"▼ ", keyMarkInk}, {"214 new", keyInk},
		{"   ", ink{}}, {"end", keyInk}, {" to follow", mutedInk}, {"   ", ink{}}, {"1,240", secondaryInk}, {" lines today", mutedInk},
	}
	if !slices.Equal(got, want) {
		t.Errorf("line() is inked\n%+v\nwant\n%+v", got, want)
	}
}

// dayRuleOf is the rule between Wed 30 Sep and Thu 1 Oct, the day that
// ended's totals as given.
func dayRuleOf(totals views.DayTotals) views.DayRule {
	return views.DayRule{Ended: views.LedgerDay{Date: "2026-09-30"}, Began: views.LedgerDay{Date: "2026-10-01"}, Totals: totals}
}

// wedTotals are the day rule's figures as log-events-160 draws them.
var wedTotals = views.DayTotals{Date: "2026-09-30", Requests: 1912, Sessions: 17, Limits: 2, Amount: usd(212)}

// The day rule at 160 columns is as log-events-160 draws it, inked as the
// design says: the days bold text.primary, the figures text.secondary, the
// limits bold state.destructive and the worth bold text.secondary.
func TestTheDayRuleIsAsTheFrameDrawsIt(t *testing.T) {
	want := referenceRows(t, "log-events-160", "txt")[13]
	got := logDayRule(dayRuleOf(wedTotals), 158)
	if text := "  " + got.plain(); text != want {
		t.Errorf("the day rule reads\n%q\nwant\n%q", text, want)
	}
	inks := line{
		{"Wed 30 Sep", titleInk}, {"   ", ink{}}, {"1,912", secondaryInk}, {" requests · ", mutedInk}, {"17", secondaryInk}, {" sessions · ", mutedInk},
		{"2", exhaustedInk}, {" limits reached · ", mutedInk}, {"$212", strongInk}, {" at API prices", mutedInk}, {"  ", ink{}},
	}
	if inked := got.inked(); !slices.Equal(inked[:len(inks)], inks) || inked[len(inks)].ink != borderInk || inked[len(inked)-1] != (span{"Thu 1 Oct", titleInk}) {
		t.Errorf("the day rule is inked\n%+v\nwant it to start\n%+v\nthen its rule in border, ending with Thu 1 Oct bold text.primary", inked, inks)
	}
}

// Narrow, the day rule drops its summary's parts from the end, each where it
// would leave fewer than three cells of rule; then the two days stand alone,
// their rule as long as the room leaves; then they're cut.
func TestTheDayRuleDropsItsSummarysPartsFromTheEndKeepingThreeCellsOfRule(t *testing.T) {
	ruled := func(n int) string { return "  " + rule(n) + "  " }
	tests := []struct {
		name  string
		cells int
		want  string
	}{
		{name: "at 100", cells: 98, want: "Wed 30 Sep   1,912 requests · 17 sessions · 2 limits reached · $212 at API prices" + ruled(4) + "Thu 1 Oct"},
		{name: "whole", cells: 97, want: "Wed 30 Sep   1,912 requests · 17 sessions · 2 limits reached · $212 at API prices" + ruled(3) + "Thu 1 Oct"},
		{name: "the worth gone", cells: 96, want: "Wed 30 Sep   1,912 requests · 17 sessions · 2 limits reached" + ruled(23) + "Thu 1 Oct"},
		{name: "the worth gone, three cells of rule", cells: 76, want: "Wed 30 Sep   1,912 requests · 17 sessions · 2 limits reached" + ruled(3) + "Thu 1 Oct"},
		{name: "the limits gone", cells: 75, want: "Wed 30 Sep   1,912 requests · 17 sessions" + ruled(21) + "Thu 1 Oct"},
		{name: "the limits gone, three cells of rule", cells: 57, want: "Wed 30 Sep   1,912 requests · 17 sessions" + ruled(3) + "Thu 1 Oct"},
		{name: "the sessions gone", cells: 56, want: "Wed 30 Sep   1,912 requests" + ruled(16) + "Thu 1 Oct"},
		{name: "on a phone", cells: 50, want: "Wed 30 Sep   1,912 requests" + ruled(10) + "Thu 1 Oct"},
		{name: "the requests gone", cells: 42, want: "Wed 30 Sep" + ruled(19) + "Thu 1 Oct"},
		{name: "the days alone, three cells of rule", cells: 26, want: "Wed 30 Sep" + ruled(3) + "Thu 1 Oct"},
		{name: "the days alone, their rule shorter", cells: 24, want: "Wed 30 Sep" + ruled(1) + "Thu 1 Oct"},
		{name: "the days alone, their rule gone", cells: 23, want: "Wed 30 Sep    Thu 1 Oct"},
		{name: "cut", cells: 22, want: "Wed 30 Sep    Thu 1 O…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := logDayRule(dayRuleOf(wedTotals), tt.cells)
			if got.plain() != tt.want || got.width() > tt.cells {
				t.Errorf("logDayRule(%d) =\n%q\nwant\n%q", tt.cells, got.plain(), tt.want)
			}
		})
	}
}

// A part of the day rule's summary that would say none is left out; one is
// said as one; a worth part unpriced ends in +, and one all unpriced says
// so alone.
func TestTheDayRuleLeavesOutWhatWouldSayNone(t *testing.T) {
	tests := []struct {
		name   string
		totals views.DayTotals
		want   string
	}{
		{name: "no limits", totals: views.DayTotals{Requests: 1912, Sessions: 17, Amount: usd(212)}, want: "1,912 requests · 17 sessions · $212 at API prices"},
		{name: "one of each", totals: views.DayTotals{Requests: 1, Sessions: 1, Limits: 1, Amount: usd(1)}, want: "1 request · 1 session · 1 limit reached · $1 at API prices"},
		{name: "part unpriced", totals: views.DayTotals{Requests: 3, Amount: usd(212), Unpriced: []string{"fable-1"}}, want: "3 requests · $212+ at API prices"},
		{name: "all unpriced", totals: views.DayTotals{Requests: 3, Unpriced: []string{"fable-1"}}, want: "3 requests · unpriced"},
		{name: "no figures", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinedSummary(daySummary(tt.totals)).plain(); got != tt.want {
				t.Errorf("the summary reads %q, want %q", got, tt.want)
			}
		})
	}
	if got, want := logDayRule(dayRuleOf(views.DayTotals{}), 40).plain(), "Wed 30 Sep  "+rule(17)+"  Thu 1 Oct"; got != want {
		t.Errorf("the day rule of a day without figures reads %q, want %q, the days alone", got, want)
	}
}

// At the top of the oldest day kept, a line says how long the ledger keeps
// its lines, or, keeping them for good, that it's the ledger's first day; in
// text.faint, cut where it runs long.
func TestTheOldestDaysLineSaysHowLongTheLedgerKeepsItsLines(t *testing.T) {
	tests := []struct {
		name  string
		keep  time.Duration
		cells int
		want  string
	}{
		{name: "the default", keep: 400 * 24 * time.Hour, cells: 158, want: "the oldest day kept: the ledger keeps its lines 400 days"},
		{name: "a week and a day", keep: 8 * 24 * time.Hour, cells: 158, want: "the oldest day kept: the ledger keeps its lines 8 days"},
		{name: "forever", keep: dayfile.Forever, cells: 158, want: "the ledger's first day: it keeps its lines for good"},
		{name: "on a narrow phone", keep: 400 * 24 * time.Hour, cells: 40, want: "the oldest day kept: the ledger keeps i…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := oldestDay(tt.keep, tt.cells)
			if got.plain() != tt.want || got[0].ink != faintInk {
				t.Errorf("oldestDay(%v, %d) = %+v, want %q in text.faint", tt.keep, tt.cells, got, tt.want)
			}
		})
	}
}

// With nothing today, each table says so in text.faint.
func TestTheLogsTablesSayWhenTheyreEmpty(t *testing.T) {
	for _, tt := range []struct {
		l    line
		want string
	}{{noRequestsToday, "no requests today"}, {noEventsToday, "no events today"}} {
		t.Run(tt.want, func(t *testing.T) {
			if !slices.Equal(tt.l, line{{tt.want, faintInk}}) {
				t.Errorf("the empty table reads %+v, want %q in text.faint", tt.l, tt.want)
			}
		})
	}
}
