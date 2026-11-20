package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// pastWidths are the widths the List's rules of a period gone are held at:
// a phone's, Blink's, an iPad's and a wide terminal's.
var pastWidths = []int{52, 73, 108, 160}

// stepsBack is the period of the length given stepped back from now the
// number of times given, as < steps it at routedAt, weeks starting on Monday.
func stepsBack(t *testing.T, length views.Length, times int) views.Period {
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
	log.Period, log.Gone = period, true
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
			f := pastFrame(t, width, stepsBack(t, views.LengthThisWeek, 1), lastWeeksLog())
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
		})
	}
}

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
	}
	for _, tt := range tests {
		for _, width := range pastWidths {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				f := pastFrame(t, width, stepsBack(t, tt.length, tt.back), &views.SessionLog{})
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
				f := pastFrame(t, width, stepsBack(t, views.LengthThisWeek, 1), log)
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
// period has them: of a period gone, 3b17's as prose.Past dates it, "23 Sep
// 23:47"; of the same sessions at now, as the rules name days, "Wed 23 Sep
// 23:47". The session that went on is measured with them, as what it says
// holds still.
func TestALogIsMeasuredAsItsPeriodSaysItsStarts(t *testing.T) {
	tests := []struct {
		name    string
		gone    bool
		started int
	}{
		{name: "gone", gone: true, started: len("23 Sep 23:47")},
		{name: "at now", started: len("Wed 23 Sep 23:47")},
	}
	for _, tt := range tests {
		log := lastWeeksLog()
		log.Gone = tt.gone
		if got := MeasureLog(log, routedAt).started; got != tt.started {
			t.Errorf("%s, the log's starts measure %d cells, want %d", tt.name, got, tt.started)
		}
	}
	if !sessionStill(wentOn(listedSession{doing: endedAtOn(septemberClock(27, 17, 40))})) {
		t.Error("a session that went on after a period gone is measured as running, want it held still")
	}
}
