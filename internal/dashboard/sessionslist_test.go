package dashboard

import (
	"fmt"
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

// d28cMoved is d28c, asking on side, which a move from work at its cap
// brought there for $1.82.
var d28cMoved = listedSession{"d28c5e17", "~/Code/switchboard", "side", asking, routedClock(1, 12, 2), 445, 4820, views.MarkAsking, 182}

// The List's columns narrow in one narrowing, from column 2, 2 cells apart:
// routing, cut as the Log cuts it, its cost kept whole, to the last column
// down to 24 cells, at 127 columns beside the session whole, and left below;
// worth, at 101, and started, at 92, beside the session whole; and model as
// the leasts don't fit.
// What's past the leasts goes to the session, to its id and 24 cells of its
// directory, then to on, its natural the widest account's name; nothing but
// routing grows past its natural, so where routing's left the spare lies at
// the row's end.
func TestTheListsColumnsByWidth(t *testing.T) {
	whole := "  session                        on        model      state        started          requests    worth"
	moved := "↑ d28c switchboard               side      opus-5-5   asking       12:02                 445   $48.20"
	started := "  session                        on        model      state        started          requests"
	movedStarted := "↑ d28c switchboard               side      opus-5-5   asking       12:02                 445"
	tests := []struct {
		width      int
		heads, row string
	}{
		{width: 220, heads: whole + "  routing", row: moved + "  from work, at its cap  $1.82"},
		{width: 160, heads: whole + "  routing", row: moved + "  from work, at its cap  $1.82"},
		{width: 127, heads: whole + "  routing", row: moved + "  from work, at it…  $1.82"},
		{width: 126, heads: whole, row: moved},
		{width: 108, heads: whole, row: moved},
		{width: 101, heads: whole, row: moved},
		{width: 100, heads: started, row: movedStarted},
		{width: 92, heads: started, row: movedStarted},
		{
			width: 91,
			heads: "  session                        on        model      state        requests",
			row:   "↑ d28c switchboard               side      opus-5-5   asking            445",
		},
		{
			width: 73,
			heads: "  session                      on        model      state        requests",
			row:   "↑ d28c switchboard             side      opus-5-5   asking            445",
		},
		{
			width: 52,
			heads: "  session            on        state        requests",
			row:   "↑ d28c switchboard   side      asking            445",
		},
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

// listLeaving are the List's columns that leave, in the order they leave.
var listLeaving = []listPart{listRouting, listWorth, listStarted, listModel}

// listLeasts are the List's columns' leasts as the design gives them, on's
// natural on: the session its id and 6 of its directory; on its natural, 14
// at the most; and the others their widths.
func listLeasts(on int) map[listPart]int {
	return map[listPart]int{
		listSession: 11, listOn: min(on, 14), listModel: 9, listState: 11,
		listStarted: 15, listRequests: 8, listWorth: 7, listRouting: 24,
	}
}

// At every width from 52 to 220, of short names and long, the List's
// columns are laid out as listLaidAsDesigned has them, and widen as
// listWidensSteadily has them.
func TestTheListsColumnsLeaveInTheirOrder(t *testing.T) {
	for _, on := range []int{8, 14, 22} {
		before := listColumnsOf(51, on)
		for width := 52; width <= 220; width++ {
			columns := listColumnsOf(width, on)
			for _, wrong := range slices.Concat(listLaidAsDesigned(width, on), listWidensSteadily(before, columns)) {
				t.Errorf("on %d at %d, %s: %+v", on, width, wrong, columns)
			}
			before = columns
		}
	}
}

// listWidensSteadily says how the List's columns, a cell wider than before,
// narrow a name: the session never, but where model comes back; and on
// never, once the session's whole. The session and on are its first two
// columns, as they never leave.
func listWidensSteadily(before, after []listColumn) []string {
	var wrong []string
	modelBack := len(after) > len(before) && after[2].part == listModel && before[2].part != listModel
	if after[listSession].width < before[listSession].width && !modelBack {
		wrong = append(wrong, fmt.Sprintf("the session narrows from %d cells to %d", before[listSession].width, after[listSession].width))
	}
	if before[listSession].width == 29 && after[listOn].width < before[listOn].width {
		wrong = append(wrong, fmt.Sprintf("on narrows from %d cells to %d", before[listOn].width, after[listOn].width))
	}
	return wrong
}

// listLaidAsDesigned says how the List's columns at width, on's natural on,
// break the design: they start at column 2, 2 cells apart, each at least its
// least; they leave in the order routing, worth, started, model, the
// session, on, state and requests never; routing only while the leasts and
// the session and on whole don't fit, worth and started while the leasts
// and the session whole don't, and model while the leasts don't; what's
// past the leasts goes to the session, to 29, before on takes a cell past
// its least; neither worth nor started shows beside a session cut short;
// and routing, where it shows, ends at the last column, beside the session
// and on whole, and where it's left, no column is past its natural.
func listLaidAsDesigned(width, on int) []string {
	leasts, columns := listLeasts(on), listColumnsOf(width, on)
	shown := map[listPart]listColumn{}
	var wrong []string
	taken, x := -listGap, listFrom
	for _, col := range columns {
		if col.x != x || col.width < leasts[col.part] {
			wrong = append(wrong, fmt.Sprintf("%s is from %d, %d cells, want from %d, at least %d", col.head(), col.x, col.width, x, leasts[col.part]))
		}
		shown[col.part] = col
		taken += leasts[col.part] + listGap
		x = col.x + col.width + listGap
	}
	left := 0
	for left < len(listLeaving) && !hasPart(shown, listLeaving[left]) {
		left++
	}
	for _, p := range slices.Concat(listLeaving[left:], []listPart{listSession, listOn, listState, listRequests}) {
		if !hasPart(shown, p) {
			wrong = append(wrong, listPartFacts[p].head+" left out of turn")
		}
	}
	if room := width - listFrom; taken > room || left > 0 && taken+listGap+listReturns(leasts, listLeaving[left-1], on) <= room {
		wrong = append(wrong, fmt.Sprintf("the leasts shown take %d of %d cells, %d columns left", taken, room, left))
	}
	session, routing, last := shown[listSession], shown[listRouting], columns[len(columns)-1]
	switch {
	case shown[listOn].width > leasts[listOn] && session.width != 29:
		wrong = append(wrong, "on takes cells past its least before the session's whole")
	case (hasPart(shown, listWorth) || hasPart(shown, listStarted)) && session.width != 29:
		wrong = append(wrong, "worth or started shows beside a session cut short")
	case hasPart(shown, listRouting) && (routing.x+routing.width != width || session.width != 29 || shown[listOn].width != on):
		wrong = append(wrong, "routing shows short of the last column, or beside a session or on cut short")
	case !hasPart(shown, listRouting) && (session.width > 29 || shown[listOn].width > on || last.x+last.width > width):
		wrong = append(wrong, "with routing left, a column grows past its natural")
	}
	return wrong
}

// listReturns are the cells a column that left needs to show again, on's
// natural on: its least; worth and started the session's whole past its
// least too; and routing on's whole as well.
func listReturns(leasts map[listPart]int, p listPart, on int) int {
	switch p {
	case listRouting:
		return leasts[p] + 29 - leasts[listSession] + on - leasts[listOn]
	case listWorth, listStarted:
		return leasts[p] + 29 - leasts[listSession]
	}
	return leasts[p]
}

// hasPart reports whether the column is among those shown.
func hasPart(shown map[listPart]listColumn, p listPart) bool {
	_, ok := shown[p]
	return ok
}

// longListDoc is framedDoc, its accounts named at length: work-team-plan,
// personal-research and side-hustle-consulting.
func longListDoc() status.Document {
	doc := framedDoc(nil, nil)
	long := map[string]string{"work": "work-team-plan", "personal": "personal-research", "side": "side-hustle-consulting"}
	for i := range doc.Accounts {
		doc.Accounts[i].Label = long[doc.Accounts[i].ID]
	}
	return doc
}

// Of long names, the List holds every name whole where it fits: at 108,
// routing left, the 19 cells past the leasts give the session its 29, a
// directory of 23 whole, and on one more, 15, so work-team-plan is whole; at
// 73, worth and started left too, the session takes the 10 cells left, and
// on holds its 14, work-team-plan still whole; and at 138, routing still
// left, and 173, on as wide as the widest name, every name whole.
func TestTheListHoldsLongNamesWholeWhereTheyFit(t *testing.T) {
	infra := listedSession{"db8a71e3", "~/Code/infra-terraform-modules", "side", idleFor(50 * time.Second), yesterdayClock(15, 8), 126, 1137, views.MarkMoved, 0}
	docs := listedSession{"3d7a8e12", "~/Code/docs-site-getting-started", "work", endedAt(10, 15), routedClock(1, 9, 40), 12, 84, views.NoMark, 0}
	tests := []struct {
		width int
		rows  []string
	}{
		{width: 108, rows: []string{
			"▸ db8a infra-terraform-modules   side-hustle-co…  opus-5-5   idle 50s     yesterday 15:08       126   $11.37",
			"  3d7a docs-site-getting-start…  work-team-plan   opus-5-5   ended 10:15  09:40                  12    $0.84",
		}},
		{width: 73, rows: []string{
			"▸ db8a infra-terraform…  side-hustle-c…  opus-5-5   idle 50s          126",
			"  3d7a docs-site-getti…  work-team-plan  opus-5-5   ended 10:15        12",
		}},
		{width: 138, rows: []string{
			"▸ db8a infra-terraform-modules   side-hustle-consulting  opus-5-5   idle 50s     yesterday 15:08       126   $11.37",
			"  3d7a docs-site-getting-start…  work-team-plan          opus-5-5   ended 10:15  09:40                  12    $0.84",
		}},
		{width: 173, rows: []string{
			"▸ db8a infra-terraform-modules   side-hustle-consulting  opus-5-5   idle 50s     yesterday 15:08       126   $11.37",
			"  3d7a docs-site-getting-start…  work-team-plan          opus-5-5   ended 10:15  09:40                  12    $0.84",
		}},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			f := listFrame(t, tt.width, 30, listOf(infra, docs))
			c := f.page(longListDoc(), routedAt)
			for _, want := range tt.rows {
				id := string([]rune(want)[2:6])
				if got := rowOf(c, rowSaying(c, 2, id)); !strings.HasPrefix(got, want) {
					t.Errorf("%s's row reads\n%q\nwant it to start\n%q", id, got, want)
				}
			}
		})
	}
}

// The List's columns hold still: no natural comes from the rows shown, so
// they're the same scrolled as not, and as a session with a longer
// directory, on an account of a longer name, arrives.
func TestTheListsColumnsHoldStill(t *testing.T) {
	doc := longListDoc()
	arrives := listedSession{"f00d4e21", "~/Code/a-directory-named-at-great-length", "an-account-named-at-great-length", answering, routedClock(1, 14, 40), 3, 21, views.MarkAnswering, 0}
	for _, width := range []int{52, 73, 108, 160} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			f := listFrame(t, width, 17, framedList())
			still := f.listing(doc, routedAt).columns
			f.Scroll = 6
			if got := f.listing(doc, routedAt).columns; !slices.Equal(got, still) {
				t.Errorf("scrolled, its columns are\n%+v\nwant\n%+v", got, still)
			}
			f.SessionsTab.List.Sessions = slices.Concat([]views.ListedSession{arrives.listed()}, f.SessionsTab.List.Sessions)
			if got := f.listing(doc, routedAt).columns; !slices.Equal(got, still) {
				t.Errorf("with %s arrived, its columns are\n%+v\nwant\n%+v", arrives.dir, got, still)
			}
		})
	}
}

// What a session is doing is said as its Activity's State says it, in its
// ink: asking and answering bold accent.key; a refusal bold
// state.destructive; throttled text.subtle, as the Switchboard has it; idle
// and ended text.muted; and what's in flight as last known, the router not
// answering, as it was, in text.subtle, not bold, its mark too.
func TestTheListsStateIsInkedByWhatItsDoing(t *testing.T) {
	stale := func(a views.Activity) views.Activity {
		a.Stale = true
		return a
	}
	tests := []struct {
		name  string
		doing views.Activity
		said  string
		ink   ink
		mark  views.RowMark
	}{
		{name: "asking", doing: asking, said: "asking", ink: keyInk, mark: views.MarkAsking},
		{name: "answering", doing: answering, said: "answering", ink: keyInk, mark: views.MarkAnswering},
		{name: "asking as last known", doing: stale(asking), said: "asking", ink: dimInk, mark: views.MarkAsking},
		{name: "answering as last known", doing: stale(answering), said: "answering", ink: dimInk, mark: views.MarkAnswering},
		{name: "refused as last known", doing: stale(views.Activity{Doing: views.Refused, Status: 429}), said: "✕ 429", ink: dimInk},
		{name: "throttled as last known", doing: stale(views.Activity{Doing: views.Throttled, Status: 429}), said: "… 429", ink: dimInk},
		{name: "refused", doing: views.Activity{Doing: views.Refused, Status: 429}, said: "✕ 429", ink: exhaustedInk},
		{name: "throttled", doing: views.Activity{Doing: views.Throttled, Status: 429}, said: "… 429", ink: dimInk},
		{name: "idle", doing: idleFor(34 * time.Second), said: "idle 34s", ink: mutedInk},
		{name: "ended", doing: endedAt(11, 21), said: "ended 11:21", ink: mutedInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := d28cMoved
			s.doing = tt.doing
			if tt.mark != views.NoMark {
				s.mark = tt.mark
			}
			f := listFrame(t, 160, 37, listOf(s))
			c := f.page(framedDoc(nil, nil), routedAt)
			y, x := rowSaying(c, 2, "d28c"), strings.Index(rowOf(c, f.top()), "state")
			if got := strings.TrimSpace(glyphsAt(c, x, y, 11)); got != tt.said {
				t.Errorf("its state reads %q, want %q", got, tt.said)
			}
			if got := c.at(x, y).ink; got != tt.ink {
				t.Errorf("its state is in %+v, want %+v", got, tt.ink)
			}
			if mark := c.at(0, y); tt.mark != views.NoMark && (mark.glyph != tt.mark.String() || mark.ink != tt.ink) {
				t.Errorf("its mark is %q in %+v, want %q in %+v", mark.glyph, mark.ink, tt.mark, tt.ink)
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

// The today line sums today's sessions from column 0, as the key line is, on
// the row over it: on one row where it fits, as at 80 columns; else broken
// after the requests, the worth on the row over the key line, the rest on the
// row above; the row above it blank. Where none of the worth is priced, it
// says so, with no "at API prices".
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
			34: "", 35: "today  14 sessions · 1,240 requests · $126.02 at API prices",
		}},
		{name: "at 80 columns, a row", width: 80, height: 30, list: framedList(), rows: map[int]string{
			27: "", 28: "today  14 sessions · 1,240 requests · $126.02 at API prices",
		}},
		{name: "on a phone, two rows", width: 52, height: 33, list: framedList(), rows: map[int]string{
			29: "", 30: "today  14 sessions · 1,240 requests", 31: "$126.02 at API prices",
		}},
		{name: "unpriced", width: 160, height: 37, list: unpriced, rows: map[int]string{
			35: "today  1 session · 445 requests · unpriced",
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
	for x, want := range map[int]ink{0: dimInk, 7: secondaryInk, 9: mutedInk, 21: secondaryInk, 27: mutedInk, 38: strongInk, 46: mutedInk} {
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
	if got, want := rowOf(c, y), "  db8a infra"+strings.Repeat(" ", 21)+"side      opus-5-5   idle 9m"+strings.Repeat(" ", 30)+"0        —"; got != want {
		t.Errorf("its row reads\n%q\nwant\n%q", got, want)
	}
	if got := c.at(100, y).ink; got != faintInk {
		t.Errorf("its worth is in %+v, want %+v", got, faintInk)
	}
}

// The List's session and account are cut with … to their cells: the
// directory to 24 cells at its most, and to 12 at 52 columns; an account to
// the widest name configured, so one not configured is cut to it.
func TestTheListCutsItsCellsToTheirWidths(t *testing.T) {
	long := listedSession{"d28c5e17", "~/Code/a-directory-named-at-length", "an-account-named-at-length", asking, routedClock(1, 12, 2), 445, 4820, views.MarkAsking, 0}
	tests := []struct {
		width int
		row   string
	}{
		{width: 160, row: "↑ d28c a-directory-named-at-le…  an-acco…  opus-5-5   asking       12:02                 445   $48.20"},
		{width: 52, row: "↑ d28c a-directory…  an-acco…  asking            445"},
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
		rows := map[int]string{6: "  session", 7: "  9c2e", 12: "  ended today", 13: "  5b19", 20: "  8a2c", 21: "", 22: "today  14"}
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
