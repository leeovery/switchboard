package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// runwayThree is the router's document of three accounts, as Runway's frames
// have them: work's session running out at its pace at 14:42, back as it
// resets at 16:12; personal at its limit since 12:50, as the router told of
// it, until 14:12, its week running out at 01:12 on Tuesday, before it
// resets on Thursday; and side, where new sessions go, with room all along.
func runwayThree() status.Document {
	personal, reached := limitedAt("personal", clockAt(12, 50))
	side := readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day))
	return withEvents(routerDoc("side", 5, pressedAccount("work"), personal, side), reached)
}

// runwayFrame is a frame of Runway over the span, of a terminal the size
// given, in nord.
func runwayFrame(t *testing.T, span Span, width, height int) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.View, f.Span, f.Look = Runway, span, Screen(builtin(t, "nord"))
	return f
}

// drawnRunway is the frame f of doc drawn as text alone, each row without the
// blanks at its end, and Runway laid out in it: where its rows are.
func drawnRunway(f Frame, doc status.Document) ([]string, runwayLayout) {
	top := f.above(newCanvas(f.Width, f.Height), doc, now)
	return textOf(f.Draw(doc, now)), f.layOutRunway(doc, now, top)
}

// textOf is rows as text alone, each without the blanks at its end.
func textOf(rows []string) []string {
	text := make([]string, len(rows))
	for i, row := range rows {
		text[i] = strings.TrimRight(ansi.Strip(row), " ")
	}
	return text
}

// textsAt is a row of cells cells, blank but for the texts at their columns.
func textsAt(cells int, texts map[int]string) string {
	row := slices.Repeat([]string{" "}, cells)
	for col, text := range texts {
		for i, r := range []rune(text) {
			row[col+i] = string(r)
		}
	}
	return strings.TrimRight(strings.Join(row, ""), " ")
}

// repeated runs the glyphs given, each as many times as its count, in turn.
func repeated(runs ...any) string {
	var b strings.Builder
	for i := 0; i < len(runs); i += 2 {
		b.WriteString(strings.Repeat(runs[i].(string), runs[i+1].(int)))
	}
	return b.String()
}

func TestRunwayOverTheDay(t *testing.T) {
	rows := textOf(runwayFrame(t, Day, 160, 26).Draw(runwayThree(), now))

	ticks := []rune(rule(136))
	for col := 5; col < 136; col += 6 {
		ticks[col] = '┬'
	}
	hours := map[int]string{22 + 11: "14:00", 22 + 71: "Tue"}
	for i, hour := range []string{"16:00", "18:00", "20:00", "22:00"} {
		hours[22+23+12*i] = hour
	}
	for i, hour := range []string{"02:00", "04:00", "06:00", "08:00", "10:00"} {
		hours[22+83+12*i] = hour
	}
	want := map[int]string{
		7:  textsAt(160, map[int]string{27: "now"}),
		8:  textsAt(160, hours),
		9:  textsAt(160, map[int]string{22: string(ticks)}),
		10: "",
		11: " ROOM                 " + repeated("█", 4, "▃", 8, "█", 3, "▃", 9, "█", 54, "▃", 58),
		12: " accounts with room   " + repeated("█", 136),
		13: "",
		14: " 1 work               " + repeated("▆", 15, "─", 9, "▆", 112),
		15: textsAt(160, map[int]string{37: "runs out ~14:42  ·  back 16:12, as it resets"}),
		16: "",
		17: " 2 personal           " + repeated("▆", 4, "─", 8, "▆", 66, "─", 58),
		18: textsAt(160, map[int]string{26: "limit reached 12:50  ·  back 14:12", 100: "week runs out ~Tue 01:12  ·  back Thu 13:12, as it resets"}),
		19: "",
		20: " 3 side       ▲ next  " + repeated("▆", 136),
		21: textsAt(160, map[int]string{29: "room all day"}),
		22: "",
		23: " ▆ has room      ▆ has room, but running out      ─ no room      past hours dimmed · labels say when, and when it's back",
		24: "",
	}
	for row := 7; row <= 24; row++ {
		if rows[row] != want[row] {
			t.Errorf("row %d reads\n%q\nwant\n%q", row+1, rows[row], want[row])
		}
	}
}

func TestRunwaysInks(t *testing.T) {
	f := runwayFrame(t, Day, 160, 26)
	c := newCanvas(f.Width, f.Height)
	f.runway(c, runwayThree(), now, 7)

	subtle := hue{token: theme.BgSubtle}
	for _, tt := range []struct {
		name string
		x, y int
		want ink
	}{
		{name: "the past, with room, dimmed", x: 22, y: 14, want: ink{token: theme.StatePositive, fade: pastFade}},
		{name: "the past, without room, dimmed", x: 27, y: 17, want: ink{token: theme.StateDestructive, fade: pastFade}},
		{name: "now, draining, its column picked out", x: 28, y: 14, want: ink{token: theme.AccentAttention, on: subtle}},
		{name: "ahead, draining", x: 29, y: 14, want: warningInk},
		{name: "ahead, without room", x: 37, y: 14, want: errorInk},
		{name: "ahead, with room", x: 46, y: 14, want: positiveInk},
		{name: "the strip's past, every account with room, dimmed", x: 22, y: 11, want: ink{ramp: true, fade: pastFade}},
		{name: "the strip, now, most with room", x: 28, y: 12, want: ink{ramp: true, at: 1.0 / 3, on: subtle}},
		{name: "the strip, every account with room", x: 35, y: 12, want: ink{ramp: true}},
		{name: "now's label", x: 28, y: 7, want: ink{token: theme.TextPrimary, bold: true, on: subtle}},
		{name: "an hour's label", x: 33, y: 8, want: mutedInk},
		{name: "midnight's, the day it starts", x: 93, y: 8, want: strongInk},
		{name: "a tick", x: 27, y: 9, want: borderInk},
		{name: "what starts a stretch without room", x: 37, y: 15, want: exhaustedInk},
		{name: "when it's back", x: 56, y: 15, want: mutedInk},
		{name: "room all day", x: 29, y: 21, want: dimInk},
		{name: "a lane's place", x: 1, y: 14, want: dimInk},
		{name: "its name", x: 3, y: 14, want: titleInk},
		{name: "the next account's badge", x: 14, y: 20, want: nextInk},
		{name: "the strip's heading", x: 1, y: 11, want: labelInk},
		{name: "its label", x: 1, y: 12, want: mutedInk},
	} {
		if got := c.at(tt.x, tt.y).ink; got != tt.want {
			t.Errorf("%s: %q at %d,%d is in %+v, want %+v", tt.name, c.at(tt.x, tt.y).glyph, tt.x, tt.y, got, tt.want)
		}
	}
	for y := 7; y <= 22; y++ {
		if got, want := c.at(28, y).ink.on, subtle; (got == want) != (y < 22) {
			t.Errorf("row %d of now's column is on %+v, want bg.subtle from now's label to the last lane's words alone", y+1, got)
		}
	}
}

func TestTheColumnsAndTheirMinutesByWidth(t *testing.T) {
	long := runwayThree()
	long.Accounts[0].Label = "a-very-long-name-for-an-account"
	tests := []struct {
		name  string
		doc   status.Document
		width int
		// wantX is the column the lanes start at, and wantColumns how many
		// cells they take.
		wantX, wantColumns int
		wantStep           time.Duration
	}{
		{name: "160 columns: a column every 10 minutes", doc: runwayThree(), width: 160, wantX: 22, wantColumns: 136, wantStep: 10 * time.Minute},
		{name: "narrower: more minutes a column", doc: runwayThree(), width: 120, wantX: 22, wantColumns: 96, wantStep: 1360 * time.Minute / 96},
		{name: "wider: fewer", doc: runwayThree(), width: 200, wantX: 22, wantColumns: 176, wantStep: 1360 * time.Minute / 176},
		{name: "a long name, cut, moving the lanes along", doc: long, width: 160, wantX: 36, wantColumns: 122, wantStep: 1360 * time.Minute / 122},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, span := range []Span{Day, Week} {
				l := runwayFrame(t, span, tt.width, 26).layOutRunway(tt.doc, now, 7)
				if l.x != tt.wantX || l.timeline.columns != tt.wantColumns {
					t.Errorf("over the %s, the lanes take %d cells from column %d, want %d from %d", span.Name(), l.timeline.columns, l.x, tt.wantColumns, tt.wantX)
				}
				if span == Day && l.timeline.step != tt.wantStep {
					t.Errorf("a column is %s, want %s", l.timeline.step, tt.wantStep)
				}
			}
		})
	}
	if got := laneLabel(Frame{}.laneOf(long, long.Accounts[0], now)).plain(); got != "1 a-very-long-name-for-an…" {
		t.Errorf("a long name heads its lane as %q, want it cut to %d cells", got, nameCells)
	}
}

func TestTheDaysHoursFallFurtherApartAsItNarrows(t *testing.T) {
	tests := []struct {
		width int
		want  []string
		not   []string
	}{
		{width: 160, want: []string{"14:00", "16:00", "Tue"}, not: []string{"15:00"}},
		{width: 120, want: []string{"15:00", "18:00", "Tue"}, not: []string{"14:00", "16:00"}},
		{width: 80, want: []string{"18:00", "Tue", "06:00"}, not: []string{"15:00", "21:00"}},
	}
	for _, tt := range tests {
		rows, l := drawnRunway(runwayFrame(t, Day, tt.width, 40), runwayThree())
		hours := rows[l.top-lanesFrom+1]
		for _, want := range tt.want {
			if !strings.Contains(hours, want) {
				t.Errorf("at %d columns, the hours read %q, want %s among them", tt.width, hours, want)
			}
		}
		for _, not := range tt.not {
			if strings.Contains(hours, not) {
				t.Errorf("at %d columns, the hours read %q, want no %s, too close to the next", tt.width, hours, not)
			}
		}
	}
}

func TestTheStripCountsTheAccountsKnownToHaveRoom(t *testing.T) {
	unread := []room{roomUnknown, roomUnknown, roomUnknown, roomUnknown}
	tests := []struct {
		name  string
		cells [][]room
		// wantTop and wantFoot are the strip's rows, and wantTones where
		// along the ramp each of its columns is coloured.
		wantTop, wantFoot string
		wantTones         []float64
	}{
		{
			name: "of the accounts known, one not read yet left uncounted",
			cells: [][]room{
				{roomOpen, roomOpen, roomDraining, roomNone},
				{roomOpen, roomDraining, roomNone, roomNone},
				{roomDraining, roomNone, roomNone, roomNone},
				unread,
			},
			wantTop: " ROOM                 █▃", wantFoot: " accounts with room   ██▅▁",
			wantTones: []float64{0, 1.0 / 3, 2.0 / 3, 1},
		},
		{
			name:    "nothing known of any, as before anything is read: nothing drawn",
			cells:   [][]room{unread, unread},
			wantTop: " ROOM", wantFoot: " accounts with room",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := runwayLayout{x: 22, timeline: timeline{start: now, step: 10 * time.Minute, columns: 4, now: now}, cells: tt.cells}
			f := Frame{Width: 30, Height: 2}
			c := newCanvas(f.Width, f.Height)
			f.roomStrip(c, l, 0)

			rows := c.rows(Look{})
			if rows[0] != tt.wantTop || rows[1] != tt.wantFoot {
				t.Errorf("the strip reads\n%q\n%q\nwant\n%q\n%q", rows[0], rows[1], tt.wantTop, tt.wantFoot)
			}
			for col, want := range tt.wantTones {
				if got := c.at(l.x+col, 1).ink; got != (ink{ramp: true, at: want}) {
					t.Errorf("column %d is in %+v, want the ramp at %v", col, got, want)
				}
			}
		})
	}
}

func TestTheStripsToneByHowManyHaveRoom(t *testing.T) {
	for _, tt := range []struct {
		with, of int
		want     float64
	}{
		{with: 3, of: 3, want: 0},
		{with: 2, of: 3, want: 1.0 / 3},
		{with: 1, of: 2, want: 1.0 / 3},
		{with: 1, of: 3, want: 2.0 / 3},
		{with: 0, of: 3, want: 1},
	} {
		if got := stripTone(tt.with, tt.of); got != tt.want {
			t.Errorf("stripTone(%d, %d) = %v, want %v", tt.with, tt.of, got, tt.want)
		}
	}
}

func TestRunwayOverTheWeek(t *testing.T) {
	rows := textOf(runwayFrame(t, Week, 160, 26).Draw(runwayThree(), now))

	for _, tt := range []struct {
		name string
		row  int
		text string
		// at is the column it starts at.
		at int
	}{
		{name: "now", row: 7, text: "now", at: 40},
		{name: "today, Monday", row: 8, text: "Mon", at: 22 + 8},
		{name: "the strip's label", row: 12, text: "weeks with room", at: 1},
		{name: "work's week's use", row: 14, text: "34%", at: 17},
		{name: "where work's week resets", row: 14, text: "┃", at: 22 + 97},
		{name: "when, ending under it, and its use by then", row: 15, text: "resets Fri 13:12  ·  79% used by then", at: 22 + 97 - 36},
		{name: "personal's week's use", row: 17, text: "89%", at: 17},
		{name: "where personal's week runs out, its limit counting for nothing over the week", row: 18, text: "runs out ~Tue 01:12  ·  back Thu 13:12, as it resets", at: 22 + 29},
		{name: "where it resets", row: 17, text: "┃", at: 22 + 77},
		{name: "side's", row: 21, text: "resets Sat 13:12  ·  35% used by then", at: 22 + 116 - 36},
		{name: "the legend", row: 23, text: "▆ has room      ▆ has room, but running out      ─ no room      ┃ week resets      one column ≈ 75 min · yesterday dimmed", at: 1},
	} {
		if got := startOf(rows[tt.row], tt.text); got != tt.at {
			t.Errorf("%s: %q starts at %d on row %d, want %d:\n%s", tt.name, tt.text, got, tt.row+1, tt.at, rows[tt.row])
		}
	}
	if got, want := rows[17][strings.Index(rows[17], "▆"):], repeated("▆", 29, "─", 48, "┃", 1, "▆", 58); got != want {
		t.Errorf("personal's lane reads\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(rows[20], "▲ next") {
		t.Errorf("side's lane reads %q, want its week's use in place of ▲ next", rows[20])
	}
}

func TestAWeekResettingBeyondTheWeekShownHasRoomAllWeek(t *testing.T) {
	doc := routerDoc("", 1, readAccount("work", sessionOf(0.1, 4*time.Hour), weekOf(0.01, 6*day+20*time.Hour)))
	rows, l := drawnRunway(runwayFrame(t, Week, 160, 26), doc)

	if got, want := strings.TrimSpace(rows[l.top+1]), "room all week  ·  resets Mon 09:12"; got != want {
		t.Errorf("work's words read %q, want %q", got, want)
	}
}

func TestALaneDrainingAllDaySaysWhenItRunsOut(t *testing.T) {
	doc := routerDoc("", 1, readAccount("work", sessionOf(0.1, 4*time.Hour), weekRunningOut(2*day, 3*day)))
	rows, l := drawnRunway(runwayFrame(t, Day, 160, 26), doc)

	if got, want := strings.TrimSpace(rows[l.top+1]), "room all day  ·  week runs out ~Wed 13:12"; got != want {
		t.Errorf("work's words read %q, want %q", got, want)
	}
}

func TestOnAPhoneTheLanesLabelsGiveWayToTheTimeline(t *testing.T) {
	long := readAccount("a-long-account-named-so", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day))
	doc := routerDoc("a-long-account-named-so", 1, long, readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day)))
	rows, l := drawnRunway(runwayFrame(t, Day, 52, 26), doc)

	if l.x > 26 || l.timeline.columns < 24 {
		t.Errorf("the lanes start at %d, %d columns across, want halfway at most, leaving the timeline room", l.x, l.timeline.columns)
	}
	if got, want := rows[l.top], " 1 a-long-account-nam… ▲  "; !strings.HasPrefix(got, want) {
		t.Errorf("its lane starts %q, want %q: the name cut, and its badge brief", got, want)
	}
}

func TestTheWeeksAxisTicksTheQuartersOfItsFirstDayToo(t *testing.T) {
	tl := timelineOf(Week, now, 136)
	c := newCanvas(140, 2)
	weekdays(c, tl, 0, 0)
	evening := time.Date(tl.start.Year(), tl.start.Month(), tl.start.Day(), 18, 0, 0, 0, tl.start.Location())
	if got := c.at(tl.column(evening), 1).glyph; got != "╵" {
		t.Errorf("the first day's 18:00, in column %d, is ticked %q, want ╵", tl.column(evening), got)
	}
}

func TestTheWeeksAxisTicksEachDaysStartAndNoQuarterBeforeItWhereTheClocksGoForwardAtMidnight(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("load America/Santiago: %v", err)
	}
	// Chile's clocks went from 00:00 to 01:00 on Sunday 6 September 2026: a
	// week of hours, from noon on the 4th.
	tl := timelineOf(Week, time.Date(2026, 9, 5, 12, 0, 0, 0, santiago), 168)
	c := newCanvas(168, 2)
	weekdays(c, tl, 0, 0)
	at := func(day, hour int) time.Time { return time.Date(2026, 9, day, hour, 0, 0, 0, santiago) }
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{name: "the 6th's start, as the clocks went forward", at: at(6, 1), want: "┬"},
		{name: "the hour before it, the 5th's last", at: at(5, 23), want: " "},
		{name: "the 6th's first quarter", at: at(6, 6), want: "╵"},
		{name: "the 7th's start", at: at(7, 0), want: "┬"},
	}
	for _, tt := range tests {
		if got := c.at(tl.column(tt.at), 1).glyph; got != tt.want {
			t.Errorf("%s, %v, in column %d, is ticked %q, want %q", tt.name, tt.at, tl.column(tt.at), got, tt.want)
		}
	}
}

func TestTheStripsFewWithRoomFillItsLowestEighth(t *testing.T) {
	cells := make([][]room, 40)
	for i := range cells {
		cells[i] = []room{roomNone}
	}
	cells[0] = []room{roomOpen}
	l := runwayLayout{x: 22, timeline: timeline{start: now, step: 10 * time.Minute, columns: 1, now: now}, cells: cells}
	c := newCanvas(30, 2)
	Frame{Width: 30, Height: 2}.roomStrip(c, l, 0)
	if got := c.at(22, 1); got.glyph != levels[1] || got.ink != (ink{ramp: true, at: stripTone(1, 40)}) {
		t.Errorf("one of 40 with room draws %q in %+v, want its lowest eighth in the tone of a few with room", got.glyph, got.ink)
	}
}

func TestALanesBadge(t *testing.T) {
	held := runwayThree()
	held.Accounts[2].Windows[1].Utilization, held.Accounts[2].Windows[1].Status = 1, quota.StatusRejected
	tests := []struct {
		name string
		doc  status.Document
		span Span
		i    int
		want line
	}{
		{name: "over the day, the account new sessions go to", doc: runwayThree(), i: 2, want: line{{nextBadge, nextInk}}},
		{name: "over the day, another", doc: runwayThree(), i: 0},
		{name: "over the day, the one account there is", doc: routerDoc("work", 1, pressedAccount("work")), i: 0},
		{name: "over the week, its week's use", doc: runwayThree(), span: Week, i: 0, want: line{{"34%", strongInk}}},
		{name: "over the week, a week running out", doc: runwayThree(), span: Week, i: 1, want: line{{"89%", alertInk}}},
		{name: "over the week, a week at its limit", doc: held, span: Week, i: 2, want: line{{"100%", exhaustedInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Policy: claudeLike, Span: tt.span}
			l := f.laneOf(tt.doc, tt.doc.Accounts[tt.i], now)
			if got := f.badge(tt.doc, l, now); !slices.Equal(got, tt.want) {
				t.Errorf("badge() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestEachCauseOfAStretchIsToldWhereItStarts(t *testing.T) {
	both := pressedAccount("work")
	both.Windows[1] = weekRunningOut(2*time.Hour, 3*day)
	rows, l := drawnRunway(runwayFrame(t, Day, 160, 26), routerDoc("", 1, both))

	if got, want := rows[l.top], " 1 work               "+repeated("▆", 15, "─", 121); got != want {
		t.Errorf("work's lane reads\n%q\nwant\n%q: room till 14:42, then one stretch without it, to the end", got, want)
	}
	want := textsAt(160, map[int]string{22 + 15: "runs out ~14:42", 22 + 32: "week runs out ~15:12  ·  back Thu 13:12, as it resets"})
	if got := rows[l.top+1]; got != want {
		t.Errorf("work's words read\n%q\nwant\n%q: its session's where it runs out, then its week's, clear of them, back once, from the week", got, want)
	}
}

func TestTheWordsUnderALaneLeaveOffWhatDoesntFitWhole(t *testing.T) {
	rows, l := drawnRunway(runwayFrame(t, Day, 120, 40), runwayThree())

	if got := rows[l.top+laneAt(1)+1]; !strings.HasSuffix(got, "   week runs out ~Tue 01:12") {
		t.Errorf("personal's words read %q, want when its week runs out alone, there being no room for when it's back", got)
	}
}

func TestTheLegend(t *testing.T) {
	tests := []struct {
		name  string
		span  Span
		width int
		look  Look
		want  string
	}{
		{name: "over the day", width: 160, want: "▆ has room      ▆ has room, but running out      ─ no room      past hours dimmed · labels say when, and when it's back"},
		{name: "over the week", span: Week, width: 160, want: "▆ has room      ▆ has room, but running out      ─ no room      ┃ week resets      one column ≈ 75 min · yesterday dimmed"},
		{name: "what it explains besides left off where it doesn't fit", width: 120, want: "▆ has room      ▆ has room, but running out      ─ no room"},
		{name: "on a phone, closer", width: 52, want: "▆ has room  ▆ has room, but running out  ─ no room"},
		{name: "without colour, running out in shade, and the past not dimmed", width: 160, look: NoColour(), want: "▆ has room      ▒ has room, but running out      ─ no room      labels say when, and when it's back"},
		{name: "over the week, without colour", span: Week, width: 160, look: NoColour(), want: "▆ has room      ▒ has room, but running out      ─ no room      ┃ week resets      one column ≈ 75 min"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := runwayFrame(t, tt.span, tt.width, 26)
			if tt.look.styled {
				f.Look = tt.look
			}
			tl := f.layOutRunway(runwayThree(), now, 7).timeline
			if got := f.legend(tl).plain(); got != tt.want {
				t.Errorf("the legend reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
	inks := make(map[string]ink)
	for _, g := range runwayFrame(t, Week, 160, 26).legendGlyphs() {
		inks[g.means] = g.drawn[0].ink
	}
	for means, want := range map[string]ink{
		"has room": {token: theme.StatePositive, bold: true}, "has room, but running out": alertInk,
		"no room": exhaustedInk, "week resets": titleInk,
	} {
		if inks[means] != want {
			t.Errorf("%q is drawn in %+v, want %+v", means, inks[means], want)
		}
	}
}

func TestALegendThatDoesntFitWholeLeavesTheKeyBehindQuestionMark(t *testing.T) {
	doc := routerDoc("work", 1, readAccount("work", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day)))
	for _, tt := range []struct {
		width      int
		wantLegend bool
	}{
		{width: 52, wantLegend: true},
		{width: 51, wantLegend: false},
	} {
		rows := textOf(runwayFrame(t, Day, tt.width, 40).Draw(doc, now))
		screen := strings.Join(rows, "\n")
		if shown := strings.Contains(screen, "─ no room"); shown != tt.wantLegend || strings.Contains(rows[37], "…") {
			t.Errorf("at %d columns, the screen is\n%s\nwant the legend shown whole: %v, and never cut short", tt.width, screen, tt.wantLegend)
		}
		if behind := strings.HasSuffix(rows[38], "? for the key"); behind == tt.wantLegend {
			t.Errorf("at %d columns, the line over the footer reads %q, want the key behind ?: %v", tt.width, rows[38], !tt.wantLegend)
		}
	}
}

func TestAboutHowLongAColumnIs(t *testing.T) {
	for _, tt := range []struct {
		d    time.Duration
		want string
	}{
		{d: weekSpan / 136, want: "75 min"},
		{d: 23 * time.Minute, want: "25 min"},
		{d: weekSpan / 28, want: "6h"},
	} {
		if got := about(tt.d); got != tt.want {
			t.Errorf("about(%s) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestRunwaysLanesScrollWhereTheyDontFit(t *testing.T) {
	var accounts []status.Account
	for _, id := range []string{"work", "personal", "side", "client", "spare", "lab", "team", "extra"} {
		accounts = append(accounts, readAccount(id, sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*day)))
	}
	eight := routerDoc("side", 8, accounts...)
	f := runwayFrame(t, Day, 160, 26)

	if got, want := f.Scrolling(eight, now), (Scrolling{Most: 13, Page: 10}); got != want {
		t.Errorf("Scrolling() = %+v, want %+v", got, want)
	}
	for _, tt := range []struct {
		scroll int
		// wantFirst is the account whose lane leads the view, and wantSays
		// what the line over the footer says.
		wantFirst, wantSays string
	}{
		{scroll: 0, wantFirst: " 1 work", wantSays: "▼ 5 more accounts below · j/k or wheel to scroll"},
		{scroll: 13, wantFirst: "", wantSays: "▲ 5 more accounts above · j/k or wheel to scroll"},
		{scroll: 99, wantFirst: "", wantSays: "▲ 5 more accounts above · j/k or wheel to scroll"},
	} {
		f.Scroll = tt.scroll
		rows := textOf(f.Draw(eight, now))
		if tt.wantFirst != "" && !strings.HasPrefix(rows[14], tt.wantFirst) {
			t.Errorf("scrolled %d, the lanes start\n%s\nwant %s first", tt.scroll, rows[14], tt.wantFirst)
		}
		if got := strings.TrimSpace(rows[24]); got != tt.wantSays {
			t.Errorf("scrolled %d, the line over the footer reads %q, want %q", tt.scroll, got, tt.wantSays)
		}
		if strings.Contains(strings.Join(rows, "\n"), "has room, but running out") {
			t.Errorf("scrolled %d, the screen is\n%s\nwant no legend while the lanes scroll", tt.scroll, strings.Join(rows, "\n"))
		}
		if got := cellsOf(rows[16], '┃'); !slices.Equal(got, []int{159}) && tt.scroll == 0 {
			t.Errorf("scrolled %d, row 17 has ┃ at %v, want the scrollbar's at the right edge", tt.scroll, got)
		}
	}
	f = runwayFrame(t, Day, 160, 27)
	if rows := textOf(f.Draw(routerDoc("side", 8, accounts[:4]...), now)); !strings.HasSuffix(rows[25], "? for the key") {
		t.Errorf("with four lanes, fitting without the legend, the line over the footer reads %q, want the key behind ?", rows[25])
	}
}

func TestRunwayFitsEveryWidth(t *testing.T) {
	docs := map[string]status.Document{
		"three":       runwayThree(),
		"probed":      probed(readAccount("work", sessionOf(0.2, time.Hour)), status.Account{ID: "side", Label: "side"}),
		"nothing yet": {},
	}
	for name, doc := range docs {
		t.Run(name, func(t *testing.T) {
			for _, span := range []Span{Day, Week} {
				for width := 1; width <= 200; width++ {
					for _, height := range []int{3, 8, 40} {
						f := frameOf(width, height)
						f.View, f.Span = Runway, span
						rows := f.Draw(doc, now)
						if len(rows) != height {
							t.Errorf("over the %s, at %d×%d, drew %d rows", span.Name(), width, height, len(rows))
						}
						for i, row := range rows {
							if got := ansi.StringWidth(row); got > width {
								t.Errorf("over the %s, at %d×%d, row %d is %d cells wide:\n%s", span.Name(), width, height, i+1, got, row)
							}
						}
					}
				}
			}
		})
	}
}

func TestTheHelpOverRunwayExplainsItsGlyphs(t *testing.T) {
	for _, span := range []Span{Day, Week} {
		f := runwayFrame(t, span, 160, 34)
		f.Help = []Key{{Key: "w", Does: "switch between the day and the week, now the " + span.Name()}, {Key: "q", Does: "quit"}}
		screen := strings.Join(textOf(f.Draw(runwayThree(), now)), "\n")

		for _, want := range []string{"▆   has room", "▆   has room, but running out", "─   no room"} {
			if !strings.Contains(screen, want) {
				t.Errorf("over the %s, the screen is\n%s\nwant the help to explain %q", span.Name(), screen, want)
			}
		}
		if got := strings.Contains(screen, "┃   week resets"); got != (span == Week) {
			t.Errorf("over the %s, the help explains ┃: %v, want it to over the week alone", span.Name(), got)
		}
		if strings.Contains(screen, "room left") {
			t.Errorf("over the %s, the screen is\n%s\nwant none of the cards' glyphs", span.Name(), screen)
		}
	}
}

func TestRunwayIsDrawnAfterTheHeadingThatEveryViewHas(t *testing.T) {
	for _, span := range []Span{Day, Week} {
		f := runwayFrame(t, span, 160, 26)
		cards := f
		cards.View = Accounts
		accounts := textOf(cards.Draw(runwayThree(), now))
		rows := textOf(f.Draw(runwayThree(), now))
		for i := range 6 {
			if i != 0 && rows[i] != accounts[i] {
				t.Errorf("over the %s, row %d reads\n%q\nwant it as every view has it\n%q", span.Name(), i+1, rows[i], accounts[i])
			}
		}
		if got, want := rows[25], " a auto   q quit"; !strings.HasPrefix(got, want) {
			t.Errorf("over the %s, the footer reads %q, want the keys given", span.Name(), got)
		}
	}
}

// startOf is the cell row has text starting at, or -1 where it hasn't.
func startOf(row, text string) int {
	before, _, found := strings.Cut(row, text)
	if !found {
		return -1
	}
	return ansi.StringWidth(before)
}
