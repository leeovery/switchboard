package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// pastWidths are the widths the List's rules of a period gone are held at:
// a phone's, Blink's, an iPad's and a wide terminal's.
var pastWidths = []int{52, 73, 108, 160}

// listStepsBack is the period of the length given stepped back from now the
// number of times given, as < steps it at routedAt, weeks starting on Monday.
func listStepsBack(t *testing.T, length views.Length, times int) views.Period {
	t.Helper()
	p := length.Now()
	for range times {
		var ok bool
		if p, ok = p.Back(routedAt, time.Monday, time.Time{}); !ok {
			t.Fatalf("%s won't step back %d times", length.Name(), times)
		}
	}
	return p
}

// pastFrame is a frame width cells wide showing Sessions' List of the log
// given, of the period given gone, its dial as the watch builds it at
// routedAt.
func pastFrame(t *testing.T, width int, period views.Period, log *views.SessionLog) Frame {
	t.Helper()
	log.Period = period
	f := listFrame(t, width, 37, log)
	f.Period, f.Dial = period, views.DialOf(period, routedAt, time.Monday, time.Time{})
	if !f.Dial.Gone() {
		t.Fatalf("the dial of %s isn't gone at %s", f.Dial.Name(), routedAt)
	}
	return f
}

// wentOn is the session as last week's log rows one that went on after it:
// leading, with no day it ended, its activity over at its last request in
// the period, as views' listedEnded has it.
func wentOn(s listedSession) views.PeriodSession {
	return views.PeriodSession{ListedSession: s.listed(), WentOn: true}
}

// lastWeeksLog is last week's log at routedAt: 7e2a, which started on Fri 25
// Sep, its last request of the week that day, and went on after the week,
// first; then ae31, ended on Sun 27 Sep,
// started that day; then under Sat 26 Sep, 9c04, started the day before,
// and 3b17, started on Wed 23 Sep, more than six days before today.
func lastWeeksLog() *views.SessionLog {
	sessions := []views.PeriodSession{
		wentOn(listedSession{"7e2a41c0", "~/Code/api", "work", endedAtOn(septemberClock(25, 17, 40)), septemberClock(25, 9, 30), 120, 1400, views.NoMark, 0}),
		listedSession{"ae31b7d2", "~/Code/docs", "side", endedAtOn(septemberClock(27, 11, 21)), septemberClock(27, 10, 2), 38, 312, views.NoMark, 0}.logged(),
		listedSession{"9c04e5f1", "~/Code/infra", "personal", endedAtOn(septemberClock(26, 16, 5)), septemberClock(25, 15, 0), 60, 650, views.NoMark, 0}.logged(),
		listedSession{"3b17a9d6", "~/Code/website", "work", endedAtOn(septemberClock(26, 12, 48)), septemberClock(23, 23, 47), 90, 1020, views.NoMark, 0}.logged(),
	}
	return &views.SessionLog{Sessions: sessions, All: views.ProjectTotals{Sessions: len(sessions), Requests: 308, Worth: worthOf(3382)}}
}

// In a period gone, no session sits first as running: one that went on
// after it does, with no rule over it, its state "went on" in text.muted in
// place of "ended …", and no mark; the rest under the rules of the days they
// ended. A start on its rule's day reads by the clock alone; one on another
// day, and a start of the session that went on, as prose.Past dates it,
// with its weekday within the six days before today, else its date, never
// as the rules name days. So at every width.
func TestTheListOfAPeriodGoneLeadsWithWhatWentOn(t *testing.T) {
	type row struct {
		line, state, started string
	}
	want := []row{
		{line: "  7e2a", state: "went on", started: "Fri 09:30"},
		{line: "  ended Sun  ─"},
		{line: "  ae31", state: "ended 11:21", started: "10:02"},
		{line: "  ended Sat  ─"},
		{line: "  9c04", state: "ended 16:05", started: "Fri 15:00"},
		{line: "  3b17", state: "ended 12:48", started: "23 Sep 23:47"},
	}
	for _, width := range pastWidths {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			f := pastFrame(t, width, listStepsBack(t, views.LengthThisWeek, 1), lastWeeksLog())
			doc := framedDoc(nil, nil)
			c, columns := f.page(doc, routedAt), f.listing(doc, routedAt).columns
			top := f.top()
			for i, tt := range want {
				y := top + 1 + i
				if got := rowOf(c, y); !strings.HasPrefix(got, tt.line) {
					t.Fatalf("at %d, line %d reads %q, want it to start %q", width, i+1, got, tt.line)
				}
				if tt.state == "" {
					continue
				}
				for _, col := range columns {
					said := strings.TrimSpace(glyphsAt(c, col.x, y, col.width))
					switch col.part {
					case listState:
						if said != tt.state || c.at(col.x, y).ink != mutedInk {
							t.Errorf("at %d, %s's state reads %q in %+v, want %q in %+v", width, tt.line, said, c.at(col.x, y).ink, tt.state, mutedInk)
						}
					case listStarted:
						if said != tt.started {
							t.Errorf("at %d, %s started %q, want %q", width, tt.line, said, tt.started)
						}
					}
				}
			}
			if !hasColumn(columns, listState) {
				t.Errorf("at %d, the List shows no state, want it at every width", width)
			}
			if shown, want := hasColumn(columns, listStarted), width >= startedFrom; shown != want {
				t.Errorf("at %d, the List shows started %t, want %t", width, shown, want)
			}
		})
	}
}

// startedFrom is the least of pastWidths at which last week's log shows its
// started column: narrower, it has left, as the List's columns give way.
const startedFrom = 108

// hasColumn reports whether the part given is among the columns.
func hasColumn(columns []listColumn, p listPart) bool {
	for _, col := range columns {
		if col.part == p {
			return true
		}
	}
	return false
}

// A period gone with no session says so in the table's place, from column
// 2, in text.subtle, by the dial's words for it, of every length and step.
func TestAnEmptyPeriodGoneSaysSoByTheDialsWords(t *testing.T) {
	tests := []struct {
		name   string
		length views.Length
		back   int
		want   string
	}{
		{name: "yesterday", length: views.LengthToday, back: 1, want: "no sessions yesterday"},
		{name: "two days back", length: views.LengthToday, back: 2, want: "no sessions on Tue 29 Sep"},
		{name: "last week", length: views.LengthThisWeek, back: 1, want: "no sessions last week"},
		{name: "two weeks back", length: views.LengthThisWeek, back: 2, want: "no sessions in the week of 14 Sep"},
		{name: "30 days back", length: views.LengthThirtyDays, back: 1, want: "no sessions in the 30 days to 1 Sep"},
		{name: "12 weeks back", length: views.LengthTwelveWeeks, back: 1, want: "no sessions in the 12 weeks to 12 Jul"},
		{name: "12 months back", length: views.LengthTwelveMonths, back: 1, want: "no sessions in the 12 months to 1 Oct 2025"},
	}
	for _, tt := range tests {
		for _, width := range pastWidths {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				f := pastFrame(t, width, listStepsBack(t, tt.length, tt.back), &views.SessionLog{})
				c := f.page(framedDoc(nil, nil), routedAt)
				y := f.top() + 1
				if got := rowOf(c, y); got != "  "+tt.want {
					t.Errorf("%s at %d reads %q, want %q", tt.name, width, got, "  "+tt.want)
				}
				if got := c.at(listFrom, y).ink; got != dimInk {
					t.Errorf("%s at %d is in %+v, want %+v", tt.name, width, got, dimInk)
				}
				if got := rowOf(c, y+1); got != "" {
					t.Errorf("%s at %d reads %q under its words, want nothing", tt.name, width, got)
				}
			})
		}
	}
}

// Where some of a period gone's days are no longer kept, the List says so
// once, after its last row, in text.subtle, by the first day kept, its year
// where it isn't now's; under the empty period's words where it had no
// session. Where every day is, it says so in place of the empty period's
// words.
func TestThePeriodGoneSaysWhereItsSessionsStopBeingKept(t *testing.T) {
	ended := listedSession{"ae31b7d2", "~/Code/docs", "side", endedAtOn(septemberClock(27, 11, 21)), septemberClock(27, 10, 2), 38, 312, views.NoMark, 0}
	tests := []struct {
		name     string
		sessions []views.PeriodSession
		unkept   views.UnkeptDays
		want     []string
	}{
		{
			name: "some days, after the last row", sessions: []views.PeriodSession{ended.logged()},
			unkept: views.UnkeptDays{KeptFrom: civilDay(9, 24)},
			want:   []string{"  ended Sun  ─", "  ae31", "  sessions before 24 Sep no longer kept", ""},
		},
		{
			name: "some days, of another year", sessions: []views.PeriodSession{ended.logged()},
			unkept: views.UnkeptDays{KeptFrom: civilDay(9, 24).AddDate(-1, 0, 0)},
			want:   []string{"  ended Sun  ─", "  ae31", "  sessions before 24 Sep 2025 no longer kept", ""},
		},
		{
			name: "some days, no session", unkept: views.UnkeptDays{KeptFrom: civilDay(9, 24)},
			want: []string{"  no sessions last week", "  sessions before 24 Sep no longer kept", ""},
		},
		{
			name: "every day", unkept: views.UnkeptDays{KeptFrom: civilDay(9, 28), Every: true},
			want: []string{"  sessions no longer kept", ""},
		},
	}
	for _, tt := range tests {
		for _, width := range pastWidths {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				log := &views.SessionLog{Sessions: tt.sessions, All: views.ProjectTotals{Sessions: len(tt.sessions)}, Unkept: tt.unkept}
				f := pastFrame(t, width, listStepsBack(t, views.LengthThisWeek, 1), log)
				c := f.page(framedDoc(nil, nil), routedAt)
				top := f.top()
				for i, want := range tt.want {
					y := top + 1 + i
					if got := rowOf(c, y); !strings.HasPrefix(got, want) || want == "" && got != "" {
						t.Fatalf("%s at %d, line %d reads %q, want it to start %q", tt.name, width, i+1, got, want)
					}
					if strings.HasPrefix(want, "  sessions") {
						if got := c.at(listFrom, y).ink; got != dimInk {
							t.Errorf("%s at %d, %q is in %+v, want %+v", tt.name, width, want, got, dimInk)
						}
					}
				}
			})
		}
	}
}

// The sessions that hold still, ended or gone on after a period, are
// measured once as the watch builds the log, their starts said as the
// period has them: of a period gone, 3b17's as prose.PastAt dates it, "23
// Sep 23:47", and a start of another year with its year, "21 Sep 2025
// 12:08", the column's widest; of the same sessions at now, as the rules
// name days, "Wed 23 Sep 23:47". The session that went on is measured with
// them, as what it says holds still.
func TestALogIsMeasuredAsItsPeriodSaysItsStarts(t *testing.T) {
	lastYear := listedSession{"5d20c8e3", "~/Code/api", "side", endedAtOn(septemberClock(26, 9, 0)), septemberClock(21, 12, 8).AddDate(-1, 0, 0), 4, 40, views.NoMark, 0}
	tests := []struct {
		name    string
		gone    bool
		more    []views.PeriodSession
		started int
	}{
		{name: "gone", gone: true, started: len("23 Sep 23:47")},
		{name: "gone, a start of another year", gone: true, more: []views.PeriodSession{lastYear.logged()}, started: len("21 Sep 2025 12:08")},
		{name: "at now", started: len("Wed 23 Sep 23:47")},
	}
	for _, tt := range tests {
		log := lastWeeksLog()
		log.Sessions = append(log.Sessions, tt.more...)
		period := views.LengthThisWeek.Now()
		if tt.gone {
			period = listStepsBack(t, views.LengthThisWeek, 1)
		}
		if got := MeasureLog(log, routedAt, views.DialOf(period, routedAt, time.Monday, time.Time{})).started; got != tt.started {
			t.Errorf("%s, the log's starts measure %d cells, want %d", tt.name, got, tt.started)
		}
	}
	if !sessionStill(wentOn(listedSession{doing: endedAtOn(septemberClock(27, 17, 40))})) {
		t.Error("a session that went on after a period gone is measured as running, want it held still")
	}
}

// unkeptWeek is last week's log as lastWeeksLog has it, its days before 24
// Sep no longer kept, so the List says so after its last row.
func unkeptWeek() *views.SessionLog {
	log := lastWeeksLog()
	log.Unkept = views.UnkeptDays{KeptFrom: civilDay(9, 24)}
	return log
}

// unkeptSaid is what the List says of unkeptWeek's days no longer kept.
const unkeptSaid = "  sessions before 24 Sep no longer kept"

// pagedFrames are last week's List of the log given, at 108 columns, for
// each of rows given at the tallest height whose page holds that many rows,
// so the frame has room to draw the List.
func pagedFrames(t *testing.T, log func() *views.SessionLog, rows ...int) map[int]Frame {
	t.Helper()
	frames := make(map[int]Frame)
	for height := 6; height <= 30; height++ {
		f := pastFrame(t, 108, listStepsBack(t, views.LengthThisWeek, 1), log())
		f.Height = height
		if page := f.listing(framedDoc(nil, nil), routedAt).page; slices.Contains(rows, page) {
			frames[page] = f
		}
	}
	if len(frames) != len(rows) {
		t.Fatalf("pages of %v rows found at %d heights, want each of %v", slices.Collect(maps.Keys(frames)), len(frames), rows)
	}
	return frames
}

// pageRows are the rows of the List's page as f draws it, as text.
func pageRows(f Frame) []string {
	doc := framedDoc(nil, nil)
	l, c := f.listing(doc, routedAt), f.page(doc, routedAt)
	rows := make([]string, l.page)
	for i := range rows {
		rows[i] = rowOf(c, f.top()+1+i)
	}
	return rows
}

// No day's rule shows without a session under it, at any scroll, on a page
// of two, three or four rows: never over the words after the last row, nor
// on the page's last row.
func TestNoRuleShowsWithoutASessionUnderItOverTheWordsAfterTheRows(t *testing.T) {
	for page, f := range pagedFrames(t, unkeptWeek, 2, 3, 4) {
		most := f.listing(framedDoc(nil, nil), routedAt).paged().most()
		for offset := 0; offset <= most; offset++ {
			f.Scroll = offset
			rows := pageRows(f)
			for i, row := range rows {
				if !strings.HasPrefix(row, "  ended ") {
					continue
				}
				if i+1 == len(rows) || !isSessionRow(rows[i+1]) {
					t.Errorf("a page of %d scrolled %d reads\n%s\nwant a session under every rule", page, offset, strings.Join(rows, "\n"))
				}
			}
		}
	}
}

// isSessionRow reports whether the row is one of last week's sessions'.
func isSessionRow(row string) bool {
	for _, id := range []string{"7e2a", "ae31", "9c04", "3b17"} {
		if strings.Contains(row, " "+id+" ") {
			return true
		}
	}
	return false
}

// A step onto the last session shows the words after it too, where the
// page holds both beside its day's rule, held over it, from three rows; and
// the session alone where it doesn't. Kept in view otherwise, as a scroll or
// a click keeps it, it scrolls no further than shows the session.
func TestTheLastSessionChosenRevealsTheWordsAfterIt(t *testing.T) {
	for page, f := range pagedFrames(t, unkeptWeek, 1, 2, 3, 4) {
		f.SessionsTab.Chosen.Session = "3b17a9d6"
		kept := f
		kept.Scroll = f.ListScroll(framedDoc(nil, nil), routedAt)
		rows := strings.Join(pageRows(kept), "\n")
		if !strings.Contains(rows, " 3b17 ") || page == 3 && strings.Contains(rows, unkeptSaid) {
			t.Errorf("a page of %d, 3b17 kept in view, reads\n%s\nwant 3b17 shown, and on a page of 3 the words after it not", page, rows)
		}
		f.Scroll = f.ListStepScroll(framedDoc(nil, nil), routedAt)
		rows = strings.Join(pageRows(f), "\n")
		if !strings.Contains(rows, " 3b17 ") {
			t.Errorf("a page of %d, 3b17 chosen, reads\n%s\nwant 3b17 shown", page, rows)
		}
		if said, want := strings.Contains(rows, unkeptSaid), page > 2; said != want {
			t.Errorf("a page of %d, 3b17 chosen, reads\n%s\nwant the words after it shown %t", page, rows, want)
		}
	}
}

// Paged down from the top and up from the foot, the List shows every line,
// its words after the rows among them, either way, at every height.
func TestTheListPagesThroughItsWordsEitherWay(t *testing.T) {
	for page, f := range pagedFrames(t, unkeptWeek, 2, 3, 4, 5) {
		p := f.listing(framedDoc(nil, nil), routedAt).paged()
		seen := func(from, pages int) map[int]bool {
			shown := make(map[int]bool)
			for offset, before := from, -1; offset != before; before, offset = offset, p.scrolling(offset).Paged(offset, pages) {
				shownFrom, to := p.shown(offset)
				for r := p.top(shownFrom, to); r < to; r++ {
					shown[r] = true
				}
			}
			return shown
		}
		down, up := seen(0, 1), seen(p.most(), -1)
		if last := p.content - 1; !down[last] || !up[last] {
			t.Errorf("a page of %d shows the words after the rows paged down %t, up %t; want both", page, down[last], up[last])
		}
	}
}

// The words a List says in place of rows, or after them, take no click, and
// the row saying how many sessions are out of view counts sessions alone.
func TestTheListsWordsTakeNoClickAndCountAsNoSession(t *testing.T) {
	doc := framedDoc(nil, nil)
	for page, f := range pagedFrames(t, unkeptWeek, 3) {
		f.Scroll = f.listing(doc, routedAt).paged().most()
		c := f.page(doc, routedAt)
		rows := pageRows(f)
		at := slices.Index(rows, unkeptSaid)
		if at < 0 {
			t.Fatalf("a page of %d at its foot reads\n%s\nwant the words after the rows", page, strings.Join(rows, "\n"))
		}
		shown := 0
		for _, row := range rows {
			if isSessionRow(row) {
				shown++
			}
		}
		for _, target := range f.Targets(doc, routedAt) {
			if target.On == OnSession && target.Y == f.top()+1+at {
				t.Errorf("the words after the rows take a click, %+v; want none", target)
			}
		}
		if got, want := strings.TrimSpace(rowOf(c, f.stateRow())), fmt.Sprintf("▲ %d more sessions", 4-shown); got != want {
			t.Errorf("a page of %d at its foot says %q out of view, want %q", page, got, want)
		}
	}
	for _, unkept := range []views.UnkeptDays{{}, {KeptFrom: civilDay(9, 28), Every: true}} {
		f := pastFrame(t, 108, listStepsBack(t, views.LengthThisWeek, 1), &views.SessionLog{Unkept: unkept})
		for _, target := range f.Targets(doc, routedAt) {
			if target.On == OnSession {
				t.Errorf("an empty week, %+v no longer kept, takes a click, %+v; want none", unkept, target)
			}
		}
	}
}

// The period line of a period gone names it as the dial does, in now's
// inks: its name in text.subtle, the counts in text.secondary and the worth
// bold; on one row where it fits, and on a phone, broken after the requests;
// and where every day of it is no longer kept, the period alone.
func TestThePeriodLineOfAPeriodGone(t *testing.T) {
	dial := views.DialOf(listStepsBack(t, views.LengthThisWeek, 1), routedAt, time.Monday, time.Time{})
	tests := []struct {
		name  string
		log   *views.SessionLog
		width int
		want  []string
	}{
		{name: "wide", log: lastWeeksLog(), width: 108, want: []string{"last week  4 sessions · 308 requests · $33.82 at API prices"}},
		{name: "a phone", log: lastWeeksLog(), width: 52, want: []string{"last week  4 sessions · 308 requests", "$33.82 at API prices"}},
		{name: "every day gone", log: &views.SessionLog{Unkept: views.UnkeptDays{KeptFrom: civilDay(9, 28), Every: true}}, width: 52, want: []string{"last week"}},
	}
	for _, tt := range tests {
		rows := periodLine(*tt.log, dial, tt.width)
		var got []string
		for _, r := range rows {
			got = append(got, r.plain())
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s, the period line reads %q, want %q", tt.name, got, tt.want)
		}
		if rows[0][0].ink != dimInk {
			t.Errorf("%s, the period's name is in %+v, want %+v", tt.name, rows[0][0].ink, dimInk)
		}
		for _, r := range rows {
			for _, s := range r {
				if ink, ok := map[string]ink{"4": secondaryInk, "308": secondaryInk, "$33.82": strongInk}[s.text]; ok && s.ink != ink {
					t.Errorf("%s, %q is in %+v, want %+v", tt.name, s.text, s.ink, ink)
				}
			}
		}
	}
}

// The period line is never cut: of the widest names a period gone has, and
// counts in the thousands, at 52 and 73 columns, its parts give way whole,
// in two rows at most, its worth always shown.
func TestThePeriodLineIsNeverCut(t *testing.T) {
	log := &views.SessionLog{All: views.ProjectTotals{Sessions: 1234, Requests: 123456, Worth: worthOf(1234567)}}
	for _, length := range []views.Length{views.LengthTwelveMonths, views.LengthTwelveWeeks, views.LengthThirtyDays} {
		dial := views.DialOf(listStepsBack(t, length, 1), routedAt, time.Monday, time.Time{})
		for _, width := range []int{52, 73} {
			rows := periodLine(*log, dial, width)
			var said []string
			for _, r := range rows {
				said = append(said, r.plain())
			}
			whole := strings.Join(said, "\n")
			if len(rows) > 2 || strings.Contains(whole, "…") || !strings.HasPrefix(whole, dial.Name()) || !strings.Contains(whole, "$12,345.67 at API prices") {
				t.Errorf("%s at %d, the period line reads %q; want two rows at most, nothing cut, its name and worth whole", dial.Name(), width, said)
			}
			for _, r := range rows {
				if r.width() > width {
					t.Errorf("%s at %d, %q runs past the width", dial.Name(), width, r.plain())
				}
			}
		}
	}
}

// The period line takes the first of its forms whose rows fit whole: of 12
// months to 1 Oct 2025 with 29 sessions, 5,423 requests and $646.09, at 108
// all on one row; at 73 broken after the requests; at 52 the name over the
// rest; narrower, the requests giving way, the sessions after the name, then
// under it with the worth, then the worth alone under the name. Of no
// sessions, saying so after the name, or under it.
func TestThePeriodLineTakesTheFirstFormThatFits(t *testing.T) {
	dial := views.DialOf(listStepsBack(t, views.LengthTwelveMonths, 1), routedAt, time.Monday, time.Time{})
	counted := &views.SessionLog{All: views.ProjectTotals{Sessions: 29, Requests: 5423, Worth: worthOf(64609)}}
	tests := []struct {
		name  string
		log   *views.SessionLog
		width int
		want  []string
	}{
		{name: "one row", log: counted, width: 108, want: []string{"12 months to 1 Oct 2025  29 sessions · 5,423 requests · $646.09 at API prices"}},
		{name: "broken after the requests", log: counted, width: 73, want: []string{"12 months to 1 Oct 2025  29 sessions · 5,423 requests", "$646.09 at API prices"}},
		{name: "the name over the rest", log: counted, width: 52, want: []string{"12 months to 1 Oct 2025", "29 sessions · 5,423 requests · $646.09 at API prices"}},
		{name: "the sessions after the name", log: counted, width: 40, want: []string{"12 months to 1 Oct 2025  29 sessions", "$646.09 at API prices"}},
		{name: "the sessions under the name", log: counted, width: 35, want: []string{"12 months to 1 Oct 2025", "29 sessions · $646.09 at API prices"}},
		{name: "the worth alone", log: counted, width: 30, want: []string{"12 months to 1 Oct 2025", "$646.09 at API prices"}},
		{name: "none, after the name", log: &views.SessionLog{}, width: 52, want: []string{"12 months to 1 Oct 2025  no sessions"}},
		{name: "none, under the name", log: &views.SessionLog{}, width: 30, want: []string{"12 months to 1 Oct 2025", "no sessions"}},
	}
	for _, tt := range tests {
		var got []string
		for _, r := range periodLine(*tt.log, dial, tt.width) {
			got = append(got, r.plain())
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s at %d, the period line reads %q, want %q", tt.name, tt.width, got, tt.want)
		}
	}
}

// A day's rule names a day in another year than now's with its year, at
// every width, in a period gone as at now.
func TestADaysRuleInAnotherYearCarriesItsYear(t *testing.T) {
	ended := listedSession{"5d20c8e3", "~/Code/api", "side", endedAtOn(septemberClock(21, 18, 0).AddDate(-1, 0, 0)), septemberClock(21, 12, 8).AddDate(-1, 0, 0), 4, 40, views.NoMark, 0}
	for _, width := range pastWidths {
		for _, gone := range []bool{false, true} {
			log := &views.SessionLog{Sessions: []views.PeriodSession{ended.logged()}, All: views.ProjectTotals{Sessions: 1}}
			var f Frame
			if gone {
				f = pastFrame(t, width, listStepsBack(t, views.LengthTwelveMonths, 1), log)
			} else {
				log.Period = views.LengthTwelveMonths.Now()
				f = listFrame(t, width, 37, log)
			}
			c := f.page(framedDoc(nil, nil), routedAt)
			if got, want := rowOf(c, f.top()+1), "  ended Sun 21 Sep 2025  ─"; !strings.HasPrefix(got, want) {
				t.Errorf("at %d, gone %t, the rule reads %q, want it to start %q", width, gone, got, want)
			}
		}
	}
}
