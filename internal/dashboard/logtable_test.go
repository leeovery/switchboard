package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// sessionsHeld reads the sessions of the lines given, every line of each, as
// a ledger holding them alone does.
type sessionsHeld []ledger.Line

func (s sessionsHeld) Sessions(_ []string, take func(date string, l ledger.Line)) map[string][]ledger.Line {
	got := make(map[string][]ledger.Line)
	for _, l := range s {
		if l.Session == "" {
			continue
		}
		got[l.Session] = append(got[l.Session], l)
		if take != nil {
			take(dayfile.DateOf(l.At), l)
		}
	}
	return got
}

// heldOf are the lines as the ledger holds them.
func heldOf(lines []ledger.Line) []ledger.Held {
	held := make([]ledger.Held, len(lines))
	for i, l := range lines {
		held[i] = ledger.Held{Line: l}
	}
	return held
}

// requestsOf is the Log's page showing Requests, as the watch builds it of
// what src says, its read of lines, the ledger holding them alone: the
// router answering, following, nothing chosen.
func requestsOf(src views.LogSources, lines []ledger.Line) LogPage {
	src.Read = views.ReadLogHeld(sessionsHeld(lines), heldOf(lines), ledger.Pricing, src.Now)
	return LogPage{
		Read: true, Requests: views.LogRows(src), Totals: src.Totals, From: views.Today(src.From), Today: views.Today(src.Now),
		Filter: src.Filter, Live: true, Written: src.Read.WrittenTo(),
	}
}

// logFrameOf is the frame of the Log's page p, width cells wide and height
// tall.
func logFrameOf(p LogPage, width, height int) Frame {
	f := frameOf(width, height)
	f.View, f.Log = Log, p
	return f
}

// The widths the Log's page is checked at, as the design's widths check
// has them, a phone's among them, and its heights.
var (
	pageWidths  = []int{52, 80, 100, 120, 160, 200}
	pageHeights = []int{24, 37}
)

// At every hard moment the Log's rows have to get right, at every width and
// height the design checks, the newest line is drawn at the foot of the
// table, the last of its lines in view, as many as the view shows from
// under its heads; the heads and the state line where they go; and nothing
// past the last column, the table drawn on a canvas wider than the frame,
// which drops nothing past it.
func TestTheLogDrawsEachHardMomentAtEveryWidth(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		src := views.LogSources{From: hm.From, Oldest: hm.Oldest, Stream: hm.Stream, Totals: hm.Totals, Filter: hm.Filter, Now: hm.Now}
		p := requestsOf(src, hm.Lines)
		lines := p.lines()
		for _, width := range pageWidths {
			for _, height := range pageHeights {
				t.Run(fmt.Sprintf("%s at %d×%d", hm.Name, width, height), func(t *testing.T) {
					f := logFrameOf(p, width, height)
					if x, y, ok := pastTheLastColumn(f, hm.Now); ok {
						t.Errorf("the table draws at column %d of row %d, past the frame's last, %d", x+1, y+1, width)
					}
					text := textOf(f.Draw(routerDoc("side", 0), hm.Now))
					if heads := text[f.top()]; !strings.HasPrefix(heads, "  at ") {
						t.Errorf("the heads' row reads %q, want the heads, from at", heads)
					}
					foot := f.top() + min(len(lines), f.LogRows())
					if got, want := text[foot], newestSaid(lines[len(lines)-1], hm.Now); !strings.Contains(got, want) {
						t.Errorf("the table's foot reads %q, want its newest line, %q", got, want)
					}
					if state := text[height-2]; !strings.HasPrefix(state, "● following") {
						t.Errorf("the state line reads %q, want following", state)
					}
				})
			}
		}
	}
}

// overhang is how many columns past a frame's last the canvas the table is
// drawn on runs, to catch what's drawn there.
const overhang = 80

// pastTheLastColumn is the first cell the Log's table, as the frame f has
// it at now, draws past the frame's last column, drawn on a canvas running
// overhang columns further: false where it draws none.
func pastTheLastColumn(f Frame, now time.Time) (x, y int, ok bool) {
	c := newCanvas(f.Width+overhang, f.Height)
	f.log(c, routerDoc("side", 0), now, f.top())
	for y := range f.Height {
		for x := f.Width; x < f.Width+overhang; x++ {
			if cell := c.at(x, y); cell != nil && strings.TrimSpace(cell.glyph) != "" {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

// newestSaid is what the line l says, the newest of the table's, at now: a
// row its time, to the second; a rule, the day that began; the oldest day's
// line and a table's words, their first words.
func newestSaid(l logLine, now time.Time) string {
	switch {
	case l.row != nil:
		return prose.ToTheSecond(now, l.row.At)
	case l.rule != nil:
		return prose.Day(l.rule.Began.Start)
	case l.empty != nil:
		return l.empty[0].text
	}
	return "the ledger"
}

// A request's row is drawn in the columns its layout gives it, wherever they
// start, its mark two cells before the first: as a session's page draws a
// turn's requests, without the session, from column 5. A cell longer than
// its column is cut there, ending in an ellipsis.
func TestARequestsRowIsDrawnInTheColumnsItsLayoutGives(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 31, 13, 0, now.Location())
	l := ledger.Line{
		At: at, Request: "0a000001", Kind: ledger.KindMessage, Session: "d28c5e17-0000-4000-8000-000000000001", Model: "claude-opus-5-5",
		Account: "side", From: "work", Reason: "moved off work at its reserve", Status: 200, Attempts: 1, TotalMS: 9300,
	}
	row := views.LogRows(views.LogSources{
		From: at, Now: at.Add(time.Minute), Read: views.ReadLogHeld(sessionsHeld{l}, heldOf([]ledger.Line{l}), ledger.Pricing, at.Add(time.Minute)),
	}).Lines[0].Row
	layout := logLayout{places: []logPlace{{column: LogAt, x: 5, width: 8}, {column: LogOn, x: 15, width: 8}, {column: LogTook, x: 25, width: 6, aligned: alignRight}}}
	for name, want := range map[string]string{
		"SIDE":            "   ▸ 14:31:13  SIDE        9.3s",
		"PERSONAL-BACKUP": "   ▸ 14:31:13  PERSONA…    9.3s",
	} {
		c := newCanvas(40, 1)
		requestRow(c, 0, row, layout, func(string) string { return name }, at)
		if got := textOf(c.rows(Look{}))[0]; got != want {
			t.Errorf("named %s, the row reads %q, want %q", name, got, want)
		}
	}
}

// The Log's window, as it follows and as it's paused: following, its
// newest lines; ↑ from following pauses at the newest row, choosing it; ↑
// and ↓ move the choice a row at a time, passing over a day's rule and
// going no further than the first line held or the paused foot, the view
// scrolling to keep it in; scrolled up, it pauses with nothing chosen, and ↑
// then chooses the newest row in view; lines come since stay below the
// paused foot, counted; and end follows again, nothing chosen.
func TestTheLogsWindowFollowsAndPauses(t *testing.T) {
	yesterday := time.Date(2026, 9, 30, 23, 50, 0, 0, now.Location())
	today := time.Date(2026, 10, 1, 9, 0, 0, 0, now.Location())
	lines := []ledger.Line{asked("0", yesterday)}
	for i := range 6 {
		lines = append(lines, asked(fmt.Sprint(i+1), today.Add(time.Duration(i)*time.Minute)))
	}
	at := today.Add(time.Hour)
	src := views.LogSources{From: yesterday, Now: at}
	p := requestsOf(src, lines)
	const rows = 4

	steps := []struct {
		name  string
		do    func(LogPage) LogPage
		lines []ledger.Line
		// view are the times of the lines in view, a rule's as "rule", and
		// chosen the chosen line's, "" where none is.
		view   []string
		chosen string
		paused bool
		fresh  int
	}{
		{name: "following", do: identity, view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}},
		{name: "↓ following does nothing", do: chose(1, 4), view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}},
		{name: "↑ pauses at the newest", do: chose(-1, 4), view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}, chosen: "09:05:00", paused: true},
		{name: "↓ at the paused foot stays", do: chose(1, 4), view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}, chosen: "09:05:00", paused: true},
		{name: "↑ to the top of the view", do: times(chose(-1, 4), 3), view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}, chosen: "09:02:00", paused: true},
		{name: "↑ scrolls the view", do: chose(-1, 4), view: []string{"09:01:00", "09:02:00", "09:03:00", "09:04:00"}, chosen: "09:01:00", paused: true},
		{name: "↑ passes over the day's rule", do: times(chose(-1, 4), 2), view: []string{"23:50:00", "rule", "09:00:00", "09:01:00"}, chosen: "23:50:00", paused: true},
		{name: "↑ goes no further than the first line held", do: chose(-1, 4), view: []string{"23:50:00", "rule", "09:00:00", "09:01:00"}, chosen: "23:50:00", paused: true},
		{
			name: "lines come since stay below the paused foot, counted", do: identity,
			lines: []ledger.Line{asked("7", today.Add(10*time.Minute)), asked("8", today.Add(11*time.Minute))},
			view:  []string{"23:50:00", "rule", "09:00:00", "09:01:00"}, chosen: "23:50:00", paused: true, fresh: 2,
		},
		{name: "end follows again", do: LogPage.Followed, view: []string{"09:04:00", "09:05:00", "09:10:00", "09:11:00"}},
		{name: "scrolled up, it pauses with nothing chosen", do: scrolled(-2, 4), view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}, paused: true},
		{name: "↑ chooses the newest row in view", do: chose(-1, 4), view: []string{"09:02:00", "09:03:00", "09:04:00", "09:05:00"}, chosen: "09:05:00", paused: true},
		{name: "scrolled down no further than the paused foot", do: scrolled(5, 4), view: []string{"09:04:00", "09:05:00", "09:10:00", "09:11:00"}, chosen: "09:05:00", paused: true},
	}
	for _, step := range steps {
		if step.lines != nil {
			lines = append(lines, step.lines...)
			p = requestsOf(src, lines).withStands(p)
		}
		p = step.do(p)
		ls := p.lines()
		first, last := p.window(ls, p.stand(), rows)
		var view []string
		for _, l := range ls[first : last+1] {
			view = append(view, timeOf(l))
		}
		chosen := ""
		if i, ok := p.lit(ls, p.stand()); ok {
			chosen = timeOf(ls[i])
		}
		fresh := 0
		if p.Paused() {
			fresh = p.fresh(ls, p.stand())
		}
		if !slices.Equal(view, step.view) || chosen != step.chosen || p.Paused() != step.paused || fresh != step.fresh {
			t.Errorf("%s: in view %v, chosen %q, paused %v, %d new; want %v, %q, %v, %d",
				step.name, view, chosen, p.Paused(), fresh, step.view, step.chosen, step.paused, step.fresh)
		}
	}
}

// Paused on a quiet morning, the table's words saying it has nothing today
// stay in view under the paused foot, the newest row; as today's first row
// comes and they go, it's counted under the foot, and not drawn.
func TestPausedOnAQuietMorningTodaysFirstRowStaysBelowTheFoot(t *testing.T) {
	yesterday := []ledger.Line{asked("1", theDayBefore), asked("2", theDayBefore.Add(time.Minute))}
	src := views.LogSources{From: theDayBefore, Now: logDayNoon}
	pinned := status.Event{ID: 1, At: theDayBefore.Add(time.Minute), Kind: status.EventPin, Accounts: []string{"personal"}}
	tests := []struct {
		name          string
		before, after LogPage
		paused, came  []string
	}{
		{
			name: "Requests", before: requestsOf(src, yesterday), after: requestsOf(src, append(yesterday, asked("3", logDay))),
			paused: []string{"23:50:00", "23:51:00", "words"}, came: []string{"23:50:00", "23:51:00"},
		},
		{
			name: "Events", before: eventsShown(pinned), after: eventsShown(pinned, workAtItsCap),
			paused: []string{"23:51:00", "words"}, came: []string{"23:51:00"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.before.Chose(-1, 4)
			if got := inView(p, 4); !slices.Equal(got, tt.paused) {
				t.Errorf("paused, in view %v, want %v", got, tt.paused)
			}
			p = tt.after.withStands(p)
			if got, fresh := inView(p, 4), p.fresh(p.lines(), p.stand()); !slices.Equal(got, tt.came) || fresh != 1 {
				t.Errorf("today's first row come, in view %v, %d new; want %v, 1 new", got, fresh, tt.came)
			}
		})
	}
}

// A terminal too short for the table leaves its view no row, and each key
// still moves it as the rules have it, as a view a row tall would: ↑ pauses
// at the newest row, choosing it, k pauses with nothing chosen, g lands on
// the request it's given, a click chooses its line, and a card closing
// leaves the choice; none indexes past the lines. Grown, the table shows the
// line chosen.
func TestTheLogsKeysHoldInAViewWithNoRow(t *testing.T) {
	requests := threeAsked()
	events := eventsShown(status.Event{ID: 1, At: theDayBefore, Kind: status.EventPin, Accounts: []string{"personal"}}, workAtItsCap)
	tests := []struct {
		name   string
		page   LogPage
		do     func(LogPage) LogPage
		paused bool
		// chosen is the time of the line chosen: "" where none is.
		chosen string
	}{
		{name: "↓ following does nothing", page: requests, do: chose(1, 0)},
		{name: "↑ pauses at the newest", page: requests, do: chose(-1, 0), paused: true, chosen: "09:02:00"},
		{name: "↑ twice", page: requests, do: times(chose(-1, 0), 2), paused: true, chosen: "09:01:00"},
		{name: "↓ back to the foot", page: requests, do: then(times(chose(-1, 0), 2), chose(1, 0)), paused: true, chosen: "09:02:00"},
		{name: "k pauses with nothing chosen", page: requests, do: scrolled(-1, 0), paused: true},
		{name: "k then j", page: requests, do: then(scrolled(-1, 0), scrolled(1, 0)), paused: true},
		{name: "k then ↑", page: requests, do: then(scrolled(-1, 0), chose(-1, 0)), paused: true, chosen: "09:01:00"},
		{name: "g lands", page: requests.Turned(1), do: landed(logDay.Add(time.Minute), 0), paused: true, chosen: "09:01:00"},
		{name: "a click chooses", page: requests, do: func(p LogPage) LogPage { return p.ChoseLine(0, 0) }, paused: true, chosen: "09:00:00"},
		{name: "a card closes", page: requests, do: then(chose(-1, 0), func(p LogPage) LogPage { return p.Unlifted(0) }), paused: true, chosen: "09:02:00"},
		{name: "↑ on Events", page: events, do: chose(-1, 0), paused: true, chosen: "09:30:00"},
		{name: "k on Events", page: events, do: scrolled(-1, 0), paused: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.do(tt.page)
			lines := p.lines()
			chosen := ""
			at, ok := p.lit(lines, p.stand())
			if ok {
				chosen = timeOf(lines[at])
			}
			if p.Paused() != tt.paused || chosen != tt.chosen {
				t.Fatalf("with no row in view, paused %t, %q chosen; want %t, %q", p.Paused(), chosen, tt.paused, tt.chosen)
			}
			if first, last := p.window(lines, p.stand(), 4); ok && (at < first || at > last) {
				t.Errorf("grown to 4 rows, the view holds lines %d to %d, want the line chosen, %d, among them", first, last, at)
			}
		})
	}
}

// then is the step a, then the step b.
func then(a, b func(LogPage) LogPage) func(LogPage) LogPage {
	return func(p LogPage) LogPage { return b(a(p)) }
}

// landed is g's landing at the time given, in a view rows rows tall.
func landed(at time.Time, rows int) func(LogPage) LogPage {
	return func(p LogPage) LogPage { return p.Landed(at, rows) }
}

// inView are the times of the lines of p's table shown in view in rows
// rows, as timeOf has them, a table's words as "words".
func inView(p LogPage, rows int) []string {
	lines := p.lines()
	first, last := p.window(lines, p.stand(), rows)
	var view []string
	for _, l := range lines[first : last+1] {
		if l.empty != nil {
			view = append(view, "words")
			continue
		}
		view = append(view, timeOf(l))
	}
	return view
}

// asked is a line of a request with the id given of d28c's, on side,
// arriving at the time given.
func asked(id string, at time.Time) ledger.Line {
	return ledger.Line{
		At: at, Request: id, Kind: ledger.KindMessage, Session: "d28c5e17-0000-4000-8000-000000000001", Model: "claude-opus-5-5",
		Account: "side", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 1200,
	}
}

// withStands is p, its tables standing as was's do.
func (p LogPage) withStands(was LogPage) LogPage {
	p.Stands = was.Stands
	return p
}

// timeOf is the time of the line l, to the second: "rule" of a day's rule,
// and "" of any other line not a row.
func timeOf(l logLine) string {
	switch {
	case l.row != nil:
		return l.row.At.Format(time.TimeOnly)
	case l.event != nil:
		return l.event.At.Format(time.TimeOnly)
	case l.rule != nil:
		return "rule"
	}
	return ""
}

// The steps of the window's test, each a key.
func identity(p LogPage) LogPage { return p }

func chose(by, rows int) func(LogPage) LogPage {
	return func(p LogPage) LogPage { return p.Chose(by, rows) }
}

func scrolled(by, rows int) func(LogPage) LogPage {
	return func(p LogPage) LogPage { return p.Scrolled(by, rows) }
}

func times(step func(LogPage) LogPage, n int) func(LogPage) LogPage {
	return func(p LogPage) LogPage {
		for range n {
			p = step(p)
		}
		return p
	}
}
