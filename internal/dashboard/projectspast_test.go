package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// projectsWidths are the widths Projects' rules of a period gone are held
// at: a phone's, Blink's, an iPad's, a wide terminal's and a MacBook's.
var projectsWidths = []int{52, 73, 108, 160, 173}

// projectsTall is a height that leaves Projects' SESSIONS room for every
// line its tests draw, at every width.
const projectsTall = 90

// overGone is f over the period given, its dial as the watch builds it at
// routedAt, which the test holds to being gone.
func overGone(t *testing.T, f Frame, period views.Period) Frame {
	t.Helper()
	f.Period, f.Dial = period, views.DialOf(period, routedAt, time.Monday, time.Time{})
	if !f.Dial.Gone() {
		t.Fatalf("the dial of %s isn't gone at %s", f.Dial.Name(), routedAt)
	}
	return f
}

// weekBack is the period a week back from routedAt's.
func weekBack(t *testing.T) views.Period {
	t.Helper()
	return listStepsBack(t, views.LengthThisWeek, 1)
}

// projectsColumnsOf are Projects' list's columns as f lays them out at routedAt.
func projectsColumnsOf(f Frame) []projectColumn {
	return projectsListOf(f, len(f.Projects.rows()), true, routedAt).columns
}

// projectsColumn is the list's column of the part given among those given,
// reporting false where it isn't among them.
func projectsColumn(columns []projectColumn, p projectsPart) (projectColumn, bool) {
	i := slices.IndexFunc(columns, func(c projectColumn) bool { return c.part == p })
	if i < 0 {
		return projectColumn{}, false
	}
	return columns[i], true
}

// The list of a period gone has no live column, as no session of it runs
// now, its other columns all there as at now, taking the room, so the list
// still ends at the frame's edge; at now live shows. So at every width.
func TestTheProjectsListOfAPeriodGoneHasNoLiveColumn(t *testing.T) {
	for _, width := range projectsWidths {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			rows := projectRows(3)
			rows[0].Live = 2
			now := projectsFrame(t, width, projectsTall, rows, 0, nil)
			atNow := projectsColumnsOf(now)
			if _, ok := projectsColumn(atNow, listedLive); !ok {
				t.Fatalf("at %d at now, the list has no live column, want one", width)
			}
			rows[0].Live = 0
			gone := projectsColumnsOf(overGone(t, projectsFrame(t, width, projectsTall, rows, 0, nil), weekBack(t)))
			if _, ok := projectsColumn(gone, listedLive); ok {
				t.Errorf("at %d over last week, the list has a live column, want none", width)
			}
			for _, col := range atNow {
				if _, ok := projectsColumn(gone, col.part); col.part != listedLive && !ok {
					t.Errorf("at %d over last week, the list has no column %d, which shows at now", width, col.part)
				}
			}
			if last := gone[len(gone)-1]; last.x+last.width != width {
				t.Errorf("at %d over last week, the list ends at %d, want the frame's edge", width, last.x+last.width)
			}
			heads := textOf(overGone(t, projectsFrame(t, width, projectsTall, rows, 0, nil), weekBack(t)).page(framedDoc(nil, nil), routedAt).rows(Look{}))
			if y := rowWith(heads, "project"); y < 0 || strings.Contains(heads[y], " live ") {
				t.Errorf("at %d over last week, the list's heads read %q, want no live", width, heads[max(y, 0)])
			}
		})
	}
}

// A project's last in a period gone is when its last request came within
// it, never now, though a session of it is busy: over one day by its time
// alone; over more, with its weekday in the period's last seven days, else
// with its date, its year where it isn't now's. Its column is as wide as
// what it says. At now, a busy project's last is now. Each time is told in
// the local zone, as the period's days are.
func TestTheLastOfAPeriodGoneIsATimeNeverNow(t *testing.T) {
	today := routedAt.Local()
	yesterday := time.Date(today.Year(), today.Month(), today.Day()-1, 21, 40, 0, 0, time.Local)
	tests := []struct {
		name   string
		length views.Length
		last   time.Time
		want   string
	}{
		{name: "yesterday", length: views.LengthToday, last: yesterday, want: "21:40"},
		{name: "last week", length: views.LengthThisWeek, last: time.Date(2026, 9, 27, 21, 40, 0, 0, time.Local), want: "Sun 21:40"},
		{name: "30 days back", length: views.LengthThirtyDays, last: time.Date(2026, 8, 21, 21, 40, 0, 0, time.Local), want: "21 Aug 21:40"},
		{name: "12 months back", length: views.LengthTwelveMonths, last: time.Date(2025, 8, 21, 21, 40, 0, 0, time.Local), want: "21 Aug 2025 21:40"},
	}
	for _, tt := range tests {
		for _, width := range projectsWidths {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				rows := projectRows(2)
				for i := range rows {
					rows[i].Last = tt.last
				}
				rows[0].Activity = answering
				f := overGone(t, projectsFrame(t, width, projectsTall, rows, 0, nil), listStepsBack(t, tt.length, 1))
				col, ok := projectsColumn(projectsColumnsOf(f), listedLast)
				if !ok {
					if width >= 108 {
						t.Fatalf("%s at %d, the list has no last column, want one", tt.name, width)
					}
					return
				}
				text := textOf(f.page(framedDoc(nil, nil), routedAt).rows(Look{}))
				for _, name := range []string{"agentic", "project-1"} {
					y := rowWith(text, name)
					if got := cellsOf(text[y], col.x, col.width); got != tt.want {
						t.Errorf("%s at %d, %s's last reads %q, want %q", tt.name, width, name, got, tt.want)
					}
				}
				if col.width != len(tt.want) {
					t.Errorf("%s at %d, last is %d cells, want %d, as wide as it says", tt.name, width, col.width, len(tt.want))
				}
			})
		}
	}
	rows := projectRows(2)
	rows[0].Activity = answering
	f := projectsFrame(t, 173, projectsTall, rows, 0, nil)
	col, _ := projectsColumn(projectsColumnsOf(f), listedLast)
	text := textOf(f.page(framedDoc(nil, nil), routedAt).rows(Look{}))
	if got := cellsOf(text[rowWith(text, "agentic")], col.x, col.width); got != "now" {
		t.Errorf("at now, a busy project's last reads %q, want now", got)
	}
}

// cellsOf are the cells of the row from x, n of them, trimmed.
func cellsOf(row string, x, n int) string {
	r := []rune(row)
	return strings.TrimSpace(string(r[min(x, len(r)):min(x+n, len(r))]))
}

// pastDayPage is the chosen project's page over yesterday, as views gives
// it of a period gone: none of its sessions running, so no count of them;
// WORTH BY by the hour of that day's lines, 14:00 its peak, none of its
// hours now's nor to come; and its sessions as given.
func pastDayPage(sessions []views.PeriodSession) views.ProjectPage {
	page := chosenPage(sessions)
	by := views.WorthBy{Unit: views.ByHour, Peak: 14, Gone: true}
	for hour := range 24 {
		by.Buckets = append(by.Buckets, views.WorthBucket{
			Start: yesterdayClock(hour, 0), Label: fmt.Sprintf("%02d:00", hour), Axis: fmt.Sprintf("%02d", hour),
			Worth: worthOf(100 + min(hour, 28-hour)*10),
		})
	}
	page.WorthBy = views.WorthCharts{by}
	return page
}

// The chosen project's head over a period gone counts no session running,
// as its page has none; WORTH BY charts the day by the hour, no hour of its
// axis bold as now's. At now the head counts those running and the axis
// marks now's hour. So at every width.
func TestTheChosenProjectOverADayGoneMarksNothingAsNow(t *testing.T) {
	doc := framedDoc(nil, nil)
	for _, width := range projectsWidths {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			gone := pastDayPage(chosenSessions()[2:])
			f := overGone(t, projectsFrame(t, width, projectsTall, projectRows(3), 0, &gone), listStepsBack(t, views.LengthToday, 1))
			c := f.page(doc, routedAt)
			text := textOf(c.rows(Look{}))
			head := rowWith(text, "all projects") + 2
			if head < 2 || strings.Contains(text[head], "running") {
				t.Errorf("at %d over yesterday, the head reads %q, want no count running", width, text[max(head, 0)])
			}
			if bold := axisBold(t, c, text); bold != "" {
				t.Errorf("at %d over yesterday, WORTH BY's axis marks %q bold, want none", width, bold)
			}

			atNow := pastDayPage(chosenSessions())
			atNow.Live, atNow.WorthBy[0].Gone, atNow.WorthBy[0].ToCome = 1, false, 9
			f = projectsFrame(t, width, projectsTall, projectRows(3), 0, &atNow).over(views.LengthToday.Now())
			c = f.page(doc, routedAt)
			text = textOf(c.rows(Look{}))
			if head := rowWith(text, "all projects") + 2; !strings.HasSuffix(text[head], "● 1 running") {
				t.Errorf("at %d at now, the head reads %q, want it to end ● 1 running", width, text[head])
			}
			if bold := axisBold(t, c, text); bold != "14" {
				t.Errorf("at %d at now, WORTH BY's axis marks %q bold, want 14, now's hour", width, bold)
			}
		})
	}
}

// axisBold is what WORTH BY's axis draws bold, as now's bucket's name is,
// between its heading and its peak: "" where nothing is.
func axisBold(t *testing.T, c *canvas, text []string) string {
	t.Helper()
	from, to := rowWith(text, "WORTH BY HOUR"), rowWith(text, "peak ")
	if from < 0 || to < from {
		t.Fatalf("no WORTH BY HOUR drawn:\n%s", strings.Join(text, "\n"))
	}
	var bold strings.Builder
	for y := from + 1; y < to; y++ {
		for x := range c.width {
			if here := c.at(x, y); here != nil && here.ink == strongInk {
				bold.WriteString(here.glyph)
			}
		}
	}
	return bold.String()
}

// projectSessionsDrawn are SESSIONS' lines as f draws them at routedAt, under its
// heads, and its columns.
func projectSessionsDrawn(f Frame) ([]string, []listColumn, *canvas, int) {
	doc := framedDoc(nil, nil)
	s := f.projectsLaying(doc, routedAt).sessions
	c := f.page(doc, routedAt)
	var lines []string
	for y := s.top + 2; y < s.top+s.rows; y++ {
		lines = append(lines, rowOf(c, y))
	}
	return lines, s.columns, c, s.top + 2
}

// SESSIONS over a period gone lists as the List does then: the session that
// went on after it first, under no rule, its state "went on" in text.muted;
// the rest under the rules of the days they ended; a start on its rule's day
// by the clock alone, else as prose.PastAt dates it; and once, after its
// last row, in text.subtle, where the period reaches back before the first
// day whose sessions are kept. So at every width.
func TestSessionsOfAPeriodGoneListAsTheListDoes(t *testing.T) {
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
		{line: unkeptSaid},
	}
	for _, width := range projectsWidths {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			page := pastDayPage(lastWeeksLog().Sessions)
			page.Unkept = views.UnkeptDays{KeptFrom: civilDay(9, 24)}
			f := overGone(t, projectsFrame(t, width, projectsTall, projectRows(3), 0, &page), weekBack(t))
			lines, columns, c, top := projectSessionsDrawn(f)
			if len(lines) <= len(want) || lines[len(want)] != "" {
				t.Fatalf("at %d, SESSIONS reads\n%s\nwant %d lines, nothing after", width, strings.Join(lines, "\n"), len(want))
			}
			for i, tt := range want {
				if !strings.HasPrefix(lines[i], tt.line) {
					t.Fatalf("at %d, SESSIONS' line %d reads %q, want it to start %q", width, i+1, lines[i], tt.line)
				}
				for _, col := range columns {
					said := strings.TrimSpace(glyphsAt(c, col.x, top+i, col.width))
					switch {
					case tt.state == "":
					case col.part == listState && (said != tt.state || c.at(col.x, top+i).ink != mutedInk):
						t.Errorf("at %d, %s's state reads %q in %+v, want %q in %+v", width, tt.line, said, c.at(col.x, top+i).ink, tt.state, mutedInk)
					case col.part == listStarted && said != tt.started:
						t.Errorf("at %d, %s started %q, want %q", width, tt.line, said, tt.started)
					}
				}
			}
			if got := c.at(listFrom, top+len(want)-1).ink; got != dimInk {
				t.Errorf("at %d, %q is in %+v, want %+v", width, unkeptSaid, got, dimInk)
			}
			if width >= 108 && !hasColumn(columns, listStarted) {
				t.Errorf("at %d, SESSIONS shows no started, want it", width)
			}
		})
	}
}

// SESSIONS over a period gone with no session kept of the project says so
// in place of its rows, as the List does: the period had none, by the
// dial's words, or every day of it is no longer kept; in text.subtle.
func TestSessionsOfAPeriodGoneWithNoSessionSaysSoAsTheListDoes(t *testing.T) {
	tests := []struct {
		name   string
		unkept views.UnkeptDays
		want   []string
	}{
		{name: "none", want: []string{"  no sessions last week"}},
		{name: "some days no longer kept", unkept: views.UnkeptDays{KeptFrom: civilDay(9, 24)}, want: []string{"  no sessions last week", unkeptSaid}},
		{name: "every day no longer kept", unkept: views.UnkeptDays{KeptFrom: civilDay(9, 28), Every: true}, want: []string{"  sessions no longer kept"}},
	}
	for _, tt := range tests {
		for _, width := range projectsWidths {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				page := pastDayPage(nil)
				page.Unkept = tt.unkept
				f := overGone(t, projectsFrame(t, width, projectsTall, projectRows(3), 0, &page), weekBack(t))
				lines, _, c, top := projectSessionsDrawn(f)
				if got := lines[:len(tt.want)+1]; !slices.Equal(got, append(slices.Clone(tt.want), "")) {
					t.Errorf("%s at %d, SESSIONS reads %q, want %q", tt.name, width, got, tt.want)
				}
				for i := range tt.want {
					if got := c.at(listFrom, top+i).ink; got != dimInk {
						t.Errorf("%s at %d, %q is in %+v, want %+v", tt.name, width, tt.want[i], got, dimInk)
					}
				}
			})
		}
	}
}

// SESSIONS' sessions that hold still are measured as the watch builds the
// page, their starts said as the dial's period has them: of a period gone,
// as prose.PastAt dates them, 3b17's "23 Sep 23:47" the widest; at now, as
// the rules name days, "Wed 23 Sep 23:47".
func TestSessionsAreMeasuredAsTheDialsPeriodSaysTheirStarts(t *testing.T) {
	page := pastDayPage(lastWeeksLog().Sessions)
	m := frameOf(173, 53).Moment(framedDoc(nil, nil), routedAt)
	started := slices.Index(projectColumns, listStarted)
	for _, tt := range []struct {
		name string
		dial views.Dial
		want int
	}{
		{name: "gone", dial: views.DialOf(weekBack(t), routedAt, time.Monday, time.Time{}), want: len("23 Sep 23:47")},
		{name: "at now", dial: views.DialOf(views.LengthThisWeek.Now(), routedAt, time.Monday, time.Time{}), want: len("Wed 23 Sep 23:47")},
	} {
		if got := MeasureSessions(&page, m, routedAt, tt.dial).widest[started]; got != tt.want {
			t.Errorf("%s, SESSIONS' starts measure %d cells, want %d", tt.name, got, tt.want)
		}
	}
}
