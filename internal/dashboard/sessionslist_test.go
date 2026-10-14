package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// listedSession is a session as the List's frames list it, at routedAt: its
// id, directory and account; what it's doing; when it started; its requests
// today and their worth, in cents; its mark; and the cost in cents of the
// move from work at its cap that brought it to side, 0 for none.
type listedSession struct {
	id, dir, on string
	doing       views.Activity
	started     time.Time
	requests    int
	worth       int
	mark        views.RowMark
	moved       int64
}

// listed is the session as the List has it.
func (s listedSession) listed() views.ListedSession {
	l := views.ListedSession{
		Session: s.id, Dir: s.dir, Account: s.on, Model: opus, Running: s.doing.Doing != views.Ended,
		Started: s.started, Requests: s.requests, Worth: worthOf(s.worth), Activity: s.doing, Mark: s.mark,
	}
	if s.moved > 0 {
		l.Routing = views.RoutingSaid{
			Words: status.RoutingWords(status.ReasonMovedOff+"work "+status.WhyReserve, status.Facts{From: "work", To: "side"}),
			From:  "work", Cost: cents(s.moved),
		}
	}
	return l
}

// idleFor is a session idle the time given at routedAt; endedAt one ended
// at the time given that day; and the others what each does.
func idleFor(d time.Duration) views.Activity {
	return views.Activity{Doing: views.Idle, Since: routedAt.Add(-d)}
}

func endedAt(hour, minute int) views.Activity {
	return views.Activity{Doing: views.Ended, Since: routedClock(1, hour, minute)}
}

var (
	asking    = views.Activity{Doing: views.Asking}
	answering = views.Activity{Doing: views.Answering}
)

// yesterdayClock is the time given the day before routedAt.
func yesterdayClock(hour, minute int) time.Time {
	return routedClock(1, hour, minute).AddDate(0, 0, -1)
}

// framedList is the List sessions-list-160 and sessions-list-phone-52 draw,
// at 14:42:07: six running, the oldest started first, and eight ended today,
// the latest ended first. 8a2c's move came yesterday, so the List gives it
// no routing, where the frames draw one.
func framedList() views.SessionList {
	sessions := []listedSession{
		{"db8a71e3", "~/Code/infra", "side", idleFor(50 * time.Second), yesterdayClock(15, 8), 126, 1137, views.MarkMoved, 66},
		{"9c2e47b1", "~/Code/website", "personal", idleFor(9 * time.Minute), routedClock(1, 11, 55), 267, 2715, views.NoMark, 0},
		{"d28c5e17", "~/Code/switchboard", "side", asking, routedClock(1, 12, 2), 445, 4820, views.MarkAsking, 182},
		{"41e0b6c2", "~/Code/docs", "personal", idleFor(34 * time.Second), routedClock(1, 12, 47), 158, 1786, views.NoMark, 0},
		{"7f3a0c94", "~/Code/api", "side", answering, routedClock(1, 14, 12), 84, 894, views.MarkAnswering, 98},
		{"c61b2f8d", "~/Code/mobile", "side", answering, routedClock(1, 14, 41), 5, 29, views.MarkAnswering, 0},
		{"5b19d3e7", "~/Code/mobile", "personal", endedAt(11, 21), routedClock(1, 10, 2), 38, 312, views.NoMark, 0},
		{"3d7a8e12", "~/Code/docs", "work", endedAt(10, 15), routedClock(1, 9, 40), 12, 84, views.NoMark, 0},
		{"e0c4f5a9", "~/Code/infra", "work", endedAt(10, 2), routedClock(1, 9, 12), 19, 147, views.NoMark, 0},
		{"a8f1b2c6", "~/Code/website", "side", endedAt(9, 28), routedClock(1, 8, 31), 24, 195, views.NoMark, 0},
		{"2b6d9f04", "~/Code/api", "work", endedAt(9, 5), routedClock(1, 7, 58), 31, 261, views.NoMark, 0},
		{"5e7d0a3b", "~/Code/api", "side", endedAt(0, 41), yesterdayClock(23, 47), 17, 128, views.NoMark, 0},
		{"4b0e6c1f", "~/Code/infra", "work", endedAt(0, 31), yesterdayClock(15, 52), 5, 31, views.NoMark, 0},
		{"8a2c5e7b", "~/Code/docs", "side", endedAt(0, 18), yesterdayClock(16, 55), 9, 63, views.NoMark, 0},
	}
	list := views.SessionList{GeneratedAt: routedAt}
	for _, s := range sessions {
		list.Sessions = append(list.Sessions, s.listed())
	}
	list.Today = views.SessionsToday{Sessions: 14, Requests: 1240, Worth: worthOf(12602)}
	return list
}

// listFrame is a frame width by height showing Sessions' List of list, in
// nord.
func listFrame(t *testing.T, width, height int, list views.SessionList) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.Look = Screen(builtin(t, "nord"))
	f.View, f.SessionsTab = Sessions, SessionsShown{Shown: SessionsList, List: list}
	return f
}

// listOf is a List of the sessions given, as built at routedAt, summing
// them as its today line does.
func listOf(sessions ...listedSession) views.SessionList {
	list := views.SessionList{GeneratedAt: routedAt}
	cents := 0
	for _, s := range sessions {
		list.Sessions = append(list.Sessions, s.listed())
		list.Today.Requests += s.requests
		cents += s.worth
	}
	list.Today.Sessions, list.Today.Worth = len(sessions), worthOf(cents)
	return list
}

// The List draws its frames cell for cell, in nord: its heads, its rows, its
// marks and inks, the ended rule, and the today line, on a row wide and
// broken onto two on a phone. 8a2c's routing, a move of yesterday's, is
// passed over: the List gives a move of today's alone.
func TestTheListIsDrawnAsItsFrames(t *testing.T) {
	tests := []struct {
		stem          string
		width, height int
		in            region
		passed        []spot
	}{
		{stem: "sessions-list-160", width: 160, height: 37, in: region{top: 6, bottom: 35, right: 159}, passed: []spot{{21, 91, 125}}},
		{stem: "sessions-list-phone-52", width: 52, height: 33, in: region{top: 7, bottom: 31, right: 51}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := listFrame(t, tt.width, tt.height, framedList())
			drawn := screenOf(t, f.Draw(framedDoc(nil, nil), routedAt))
			heldTo(t, tt.stem, reference(t, tt.stem), drawn, tt.in, shadeOf(f.Look.Canvas()), tt.passed, nil)
		})
	}
}

// d28cMoved is d28c, asking on side, which a move from work at its cap
// brought there for $1.82.
var d28cMoved = listedSession{"d28c5e17", "~/Code/switchboard", "side", asking, routedClock(1, 12, 2), 445, 4820, views.MarkAsking, 182}

// The List's columns are its frames' at 100 columns and wider, routing
// running from 91 to the last column, cut as the Log cuts it, its cost kept
// whole, down to 24 cells, at 115 columns, and left out below; under 100,
// as on a phone, its mark and four columns, the directory cut to 11.
func TestTheListsColumnsByWidth(t *testing.T) {
	wide := "  session            on        model      state        started          requests    worth"
	tests := []struct {
		width      int
		heads, row string
	}{
		{width: 200, heads: wide + "  routing", row: "↑ d28c switchboard   side      opus-5-5   asking       12:02                 445   $48.20  from work, at its cap  $1.82"},
		{width: 160, heads: wide + "  routing", row: "↑ d28c switchboard   side      opus-5-5   asking       12:02                 445   $48.20  from work, at its cap  $1.82"},
		{width: 115, heads: wide + "  routing", row: "↑ d28c switchboard   side      opus-5-5   asking       12:02                 445   $48.20  from work, at it…  $1.82"},
		{width: 114, heads: wide, row: "↑ d28c switchboard   side      opus-5-5   asking       12:02                 445   $48.20"},
		{width: 100, heads: wide, row: "↑ d28c switchboard   side      opus-5-5   asking       12:02                 445   $48.20"},
		{width: 99, heads: "  session          on        state          requests", row: "↑ d28c switchboard side      asking              445"},
		{width: 52, heads: "  session          on        state          requests", row: "↑ d28c switchboard side      asking              445"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := listFrame(t, tt.width, 37, listOf(d28cMoved))
			c := f.page(framedDoc(nil, nil), routedAt)
			top := f.top()
			if got := rowOf(c, top); got != tt.heads {
				t.Errorf("the heads read\n%q\nwant\n%q", got, tt.heads)
			}
			if got := rowOf(c, top+1); got != tt.row {
				t.Errorf("d28c's row reads\n%q\nwant\n%q", got, tt.row)
			}
		})
	}
}

// What a session is doing is said as its Activity's State says it, in its
// ink: asking and answering bold accent.key; a refusal bold
// state.destructive; throttled text.subtle, as the Switchboard has it; idle
// and ended text.muted.
func TestTheListsStateIsInkedByWhatItsDoing(t *testing.T) {
	tests := []struct {
		name  string
		doing views.Activity
		said  string
		ink   ink
	}{
		{name: "asking", doing: asking, said: "asking", ink: keyInk},
		{name: "answering", doing: answering, said: "answering", ink: keyInk},
		{name: "refused", doing: views.Activity{Doing: views.Refused, Status: 429}, said: "✕ 429", ink: exhaustedInk},
		{name: "throttled", doing: views.Activity{Doing: views.Throttled, Status: 429}, said: "… 429", ink: dimInk},
		{name: "idle", doing: idleFor(34 * time.Second), said: "idle 34s", ink: mutedInk},
		{name: "ended", doing: endedAt(11, 21), said: "ended 11:21", ink: mutedInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := d28cMoved
			s.doing = tt.doing
			f := listFrame(t, 160, 37, listOf(s))
			c := f.page(framedDoc(nil, nil), routedAt)
			y := rowSaying(c, 2, "d28c")
			if got := strings.TrimSpace(glyphsAt(c, 42, y, 11)); got != tt.said {
				t.Errorf("its state reads %q, want %q", got, tt.said)
			}
			if got := c.at(42, y).ink; got != tt.ink {
				t.Errorf("its state is in %+v, want %+v", got, tt.ink)
			}
		})
	}
}

// The List's lines are its sessions in its order, those running first,
// then, under the ended rule, those ended today, the rule only where any
// did; and with none, words saying so, and its today line saying none.
func TestTheListsLinesAndItsEndedRule(t *testing.T) {
	db8a := listedSession{"db8a71e3", "~/Code/infra", "side", idleFor(50 * time.Second), yesterdayClock(15, 8), 126, 1137, views.MarkMoved, 66}
	b519 := listedSession{"5b19d3e7", "~/Code/mobile", "personal", endedAt(11, 21), routedClock(1, 10, 2), 38, 312, views.NoMark, 0}
	tests := []struct {
		name  string
		list  views.SessionList
		lines []string
		today string
	}{
		{
			name: "running alone", list: listOf(db8a, d28cMoved),
			lines: []string{"▸ db8a", "↑ d28c", ""},
			today: "today  2 sessions · 571 requests · $59.57 at API prices",
		},
		{
			name: "running, then ended", list: listOf(d28cMoved, b519),
			lines: []string{"↑ d28c", "  ended today  ─", "  5b19", ""},
			today: "today  2 sessions · 483 requests · $51.32 at API prices",
		},
		{
			name: "ended alone", list: listOf(b519),
			lines: []string{"  ended today  ─", "  5b19", ""},
			today: "today  1 session · 38 requests · $3.12 at API prices",
		},
		{name: "none", list: listOf(), lines: []string{"  no sessions today", ""}, today: "today  no sessions"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := listFrame(t, 160, 37, tt.list)
			c := f.page(framedDoc(nil, nil), routedAt)
			for i, want := range tt.lines {
				if got := rowOf(c, f.top()+1+i); !strings.HasPrefix(got, want) || want == "" && got != "" {
					t.Errorf("line %d reads %q, want it to start %q", i+1, got, want)
				}
			}
			if got := strings.TrimSpace(rowOf(c, 35)); got != tt.today {
				t.Errorf("the today line reads %q, want %q", got, tt.today)
			}
		})
	}
}

// The ended rule runs from column 2 to the last, its words in text.subtle
// and its rule in border; the words saying there are no sessions are in
// text.faint.
func TestTheEndedRuleAndTheEmptyListsInks(t *testing.T) {
	ended := listedSession{"5b19d3e7", "~/Code/mobile", "personal", endedAt(11, 21), routedClock(1, 10, 2), 38, 312, views.NoMark, 0}
	f := listFrame(t, 120, 37, listOf(ended))
	c := f.page(framedDoc(nil, nil), routedAt)
	y := f.top() + 1
	if got := rowOf(c, y); got != "  ended today  "+rule(105) {
		t.Errorf("the ended rule reads %q, want it ruled to the last column", got)
	}
	if words, ruled := c.at(2, y).ink, c.at(119, y).ink; words != dimInk || ruled != borderInk {
		t.Errorf("the ended rule's words are in %+v and its rule %+v, want %+v and %+v", words, ruled, dimInk, borderInk)
	}
	f.SessionsTab.List = listOf()
	c = f.page(framedDoc(nil, nil), routedAt)
	if got := c.at(2, y).ink; got != faintInk {
		t.Errorf("no sessions today is in %+v, want %+v", got, faintInk)
	}
}

// The today line sums today's sessions, right-aligned to the last column on
// the row over the key line: on one row where it fits, as at 80 columns;
// else broken after the requests, the worth on the row over the key line,
// the rest on the row above, each right-aligned; the row above it blank.
// Where none of the worth is priced, it says so, with no "at API prices".
func TestTheTodayLine(t *testing.T) {
	unpriced := listOf(d28cMoved)
	unpriced.Today.Worth = views.Worth{Unpriced: []string{"no_usage"}}
	tests := []struct {
		name          string
		width, height int
		list          views.SessionList
		rows          map[int]string
	}{
		{name: "wide", width: 160, height: 37, list: framedList(), rows: map[int]string{
			34: "", 35: strings.Repeat(" ", 101) + "today  14 sessions · 1,240 requests · $126.02 at API prices",
		}},
		{name: "at 80 columns, a row", width: 80, height: 30, list: framedList(), rows: map[int]string{
			27: "", 28: strings.Repeat(" ", 21) + "today  14 sessions · 1,240 requests · $126.02 at API prices",
		}},
		{name: "on a phone, two rows", width: 52, height: 33, list: framedList(), rows: map[int]string{
			29: "", 30: strings.Repeat(" ", 17) + "today  14 sessions · 1,240 requests", 31: strings.Repeat(" ", 31) + "$126.02 at API prices",
		}},
		{name: "unpriced", width: 160, height: 37, list: unpriced, rows: map[int]string{
			35: strings.Repeat(" ", 118) + "today  1 session · 445 requests · unpriced",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := listFrame(t, tt.width, tt.height, tt.list)
			c := f.page(framedDoc(nil, nil), routedAt)
			for y, want := range tt.rows {
				if got := rowOf(c, y); got != want {
					t.Errorf("row %d reads\n%q\nwant\n%q", y+1, got, want)
				}
			}
		})
	}
}

// The today line's inks: today in text.subtle, the counts in
// text.secondary, the worth bold text.secondary, and the rest in
// text.muted.
func TestTheTodayLinesInks(t *testing.T) {
	f := listFrame(t, 160, 37, framedList())
	c := f.page(framedDoc(nil, nil), routedAt)
	for x, want := range map[int]ink{101: dimInk, 108: secondaryInk, 110: mutedInk, 122: secondaryInk, 128: mutedInk, 139: strongInk, 147: mutedInk} {
		if got := c.at(x, 35).ink; got != want {
			t.Errorf("column %d of the today line is in %+v, want %+v", x+1, got, want)
		}
	}
}

// A session the List lists that made no request today, as one running on
// from yesterday past midnight, counts 0 requests, and its worth is — in
// text.faint; one whose start isn't known has none said.
func TestASessionOfNoRequestsTodayAndOfNoKnownStart(t *testing.T) {
	quiet := listedSession{"db8a71e3", "~/Code/infra", "side", idleFor(9 * time.Minute), time.Time{}, 0, 0, views.NoMark, 0}
	f := listFrame(t, 160, 37, listOf(quiet))
	c := f.page(framedDoc(nil, nil), routedAt)
	y := f.top() + 1
	if got, want := rowOf(c, y), "  db8a infra         side      opus-5-5   idle 9m"+strings.Repeat(" ", 30)+"0        —"; got != want {
		t.Errorf("its row reads\n%q\nwant\n%q", got, want)
	}
	if got := c.at(88, y).ink; got != faintInk {
		t.Errorf("its worth is in %+v, want %+v", got, faintInk)
	}
}

// The List's session and account are cut with … to their cells: the
// directory to 12 cells, 11 on a phone, the account to 8.
func TestTheListCutsItsCellsToTheirWidths(t *testing.T) {
	long := listedSession{"d28c5e17", "~/Code/a-directory-named-at-length", "an-account-named-at-length", asking, routedClock(1, 12, 2), 445, 4820, views.MarkAsking, 0}
	tests := []struct {
		width int
		row   string
	}{
		{width: 160, row: "↑ d28c a-directory…  an-acco…  opus-5-5   asking       12:02                 445   $48.20"},
		{width: 52, row: "↑ d28c a-director… an-acco…  asking              445"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := listFrame(t, tt.width, 37, listOf(long))
			c := f.page(framedDoc(nil, nil), routedAt)
			if got := rowOf(c, f.top()+1); got != tt.row {
				t.Errorf("its row reads\n%q\nwant\n%q", got, tt.row)
			}
		})
	}
}

// Past a screen, the List's lines scroll a row at a time under its heads,
// which hold, as its today line does, the row above it blank: at 160 by 24,
// fourteen rows, from 7 to 20, show fifteen lines, so they scroll a row, and
// no further, however far they're asked to.
func TestTheListScrollsUnderItsHeads(t *testing.T) {
	doc := framedDoc(nil, nil)
	f := listFrame(t, 160, 24, framedList())
	if got, want := f.Scrolling(doc, routedAt), (Scrolling{Most: 1, Page: 14}); got != want {
		t.Errorf("it scrolls %+v, want %+v", got, want)
	}
	if x, y, width, rows := f.scrolls(doc, routedAt); x != 0 || y != 7 || width != 160 || rows != 14 {
		t.Errorf("what scrolls is from %d along row %d, %d wide and %d rows, want its lines' rows, 8 to 21, across the frame", x, y+1, width, rows)
	}
	for _, scroll := range []int{1, 5} {
		f.Scroll = scroll
		c := f.page(doc, routedAt)
		rows := map[int]string{6: "  session", 7: "  9c2e", 12: "  ended today", 13: "  5b19", 20: "  8a2c", 21: "", 22: strings.Repeat(" ", 101) + "today  14"}
		for y, want := range rows {
			if got := rowOf(c, y); !strings.HasPrefix(got, want) || want == "" && got != "" {
				t.Errorf("scrolled %d, row %d reads %q, want it to start %q", scroll, y+1, got, want)
			}
		}
	}
}

// The List never ends a page on its ended rule: at 160 by 17, seven rows,
// the page that would end on it ends over it, six rows paged, so PgDn goes
// on from the rule, the first session ended under it.
func TestTheListEndsNoPageOnItsEndedRule(t *testing.T) {
	doc := framedDoc(nil, nil)
	f := listFrame(t, 160, 17, framedList())
	if got, want := f.Scrolling(doc, routedAt), (Scrolling{Most: 8, Page: 6}); got != want {
		t.Errorf("unscrolled, it scrolls %+v, want %+v", got, want)
	}
	for scroll, rows := range map[int]map[int]string{
		0: {7: "▸ db8a", 12: "↓ c61b", 13: ""},
		6: {7: "  ended today", 8: "  5b19", 13: "  5e7d"},
	} {
		f.Scroll = scroll
		c := f.page(doc, routedAt)
		for y, want := range rows {
			if got := rowOf(c, y); !strings.HasPrefix(got, want) || want == "" && got != "" {
				t.Errorf("scrolled %d, row %d reads %q, want it to start %q", scroll, y+1, got, want)
			}
		}
	}
}

// The session chosen is lit on bg.selection across the width, its glyphs
// and their inks as they were; nothing is, with none chosen, or with the one
// chosen scrolled out of view.
func TestTheSessionChosenIsLitAcrossTheWidth(t *testing.T) {
	doc := framedDoc(nil, nil)
	tests := []struct {
		name   string
		chosen string
		scroll int
		want   []int
	}{
		{name: "none chosen"},
		{name: "d28c", chosen: "d28c5e17", want: []int{9}},
		{name: "8a2c, ended", chosen: "8a2c5e7b", scroll: 1, want: []int{20}},
		{name: "db8a, scrolled out of view", chosen: "db8a71e3", scroll: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := listFrame(t, 160, 24, framedList())
			f.SessionsTab.Chosen, f.Scroll = Plug{Session: tt.chosen}, tt.scroll
			c := f.page(doc, routedAt)
			var lit []int
			for y := range f.Height {
				if c.at(0, y).ink.on == litInk && c.at(f.Width-1, y).ink.on == litInk {
					lit = append(lit, y)
				}
			}
			if !slices.Equal(lit, tt.want) {
				t.Errorf("rows %v are lit, want %v", lit, tt.want)
			}
		})
	}
	f := listFrame(t, 160, 37, framedList())
	f.SessionsTab.Chosen = Plug{Session: "d28c5e17"}
	if got, want := f.page(doc, routedAt).at(0, 9).ink, (ink{token: keyInk.token, bold: true, on: litInk}); got != want {
		t.Errorf("d28c's mark, chosen, is in %+v, want %+v", got, want)
	}
}

// A click on a session's row on the List chooses it, and opens the one
// chosen: each row in view is a target across the width, by its session,
// on no account; the heads, the ended rule and the today line are none.
func TestAClickOnTheListsRowChoosesItsSession(t *testing.T) {
	doc := framedDoc(nil, nil)
	f := listFrame(t, 160, 24, framedList())
	f.Scroll = 1
	var got []Target
	for _, target := range f.Targets(doc, routedAt) {
		if target.On == OnSession {
			got = append(got, target)
		}
	}
	ids := []string{"9c2e47b1", "d28c5e17", "41e0b6c2", "7f3a0c94", "c61b2f8d", "", "5b19d3e7", "3d7a8e12", "e0c4f5a9", "a8f1b2c6", "2b6d9f04", "5e7d0a3b", "4b0e6c1f", "8a2c5e7b"}
	var want []Target
	for i, id := range ids {
		if id != "" {
			want = append(want, Target{Y: 7 + i, Width: 160, On: OnSession, Session: id})
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("the List's targets are\n%+v\nwant\n%+v", got, want)
	}
}

// Till the watch has built the List, as before its first read lands, it
// draws nothing, scrolls nowhere, and nothing on it takes a click.
func TestTheListUnbuiltDrawsNothing(t *testing.T) {
	doc := framedDoc(nil, nil)
	f := listFrame(t, 160, 37, views.SessionList{})
	c := f.page(doc, routedAt)
	for y := f.top(); y < f.Height-1; y++ {
		if got := rowOf(c, y); got != "" {
			t.Errorf("row %d reads %q, want it blank", y+1, got)
		}
	}
	if got := f.Scrolling(doc, routedAt); got != (Scrolling{}) {
		t.Errorf("it scrolls %+v, want nowhere", got)
	}
	for _, target := range f.Targets(doc, routedAt) {
		if target.On == OnSession {
			t.Errorf("%+v takes a click, want none", target)
		}
	}
}
