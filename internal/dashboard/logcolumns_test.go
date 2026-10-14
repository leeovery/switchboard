package dashboard

import (
	"fmt"
	"strings"
	"testing"
)

// logWidths are the widths the Requests' layout is held at: a phone's, each
// side of each width a column comes or goes at, the design's 160, and past it.
var logWidths = []int{52, 80, 99, 100, 101, 102, 110, 111, 118, 119, 125, 126, 151, 152, 159, 160, 161, 200, 240}

// layoutSaid says a layout as the tests hold it: each column's name and the
// cells it takes, from its first to its last; then whether routing folds
// into stopped, and whether it's a phone's.
func layoutSaid(l logLayout) string {
	said := make([]string, len(l.places))
	for i, p := range l.places {
		said[i] = fmt.Sprintf("%s %d–%d", p.column.Name(), p.x, p.x+p.width-1)
	}
	s := strings.Join(said, " · ")
	if l.folded {
		s += ", folded"
	}
	if l.phone {
		s = "phone: " + s
	}
	return s
}

// The layouts at each of logWidths with the columns shown at first: on a
// phone, its five at its places; from 100, stopped running to the last
// column, routing folded into it, first, read, written and out each back
// where stopped keeps its 27 cells; from 152, routing beside it, cut to the
// room left; at 160 as the design draws it; and past it, the two sharing
// the room past their widths.
const (
	logLead = "at 2–9 · session 12–28 · model 31–39 · on 42–49 · status 52–57 · "
	logFull = logLead + "first 60–64 · took 67–72 · read 75–80 · written 83–89 · out 92–96 · "

	phone52 = "phone: at 2–9 · session 11–14 · on 20–27 · took 28–33 · stopped 37–51, folded"
	phone80 = "phone: at 2–9 · session 11–14 · on 20–27 · took 28–33 · stopped 37–79, folded"
	phone99 = "phone: at 2–9 · session 11–14 · on 20–27 · took 28–33 · stopped 37–98, folded"
	log100  = logLead + "took 60–65 · stopped 68–99, folded"
	log101  = logLead + "took 60–65 · stopped 68–100, folded"
	log102  = logLead + "took 60–65 · out 68–72 · stopped 75–101, folded"
	log110  = logLead + "took 60–65 · out 68–72 · stopped 75–109, folded"
	log111  = logLead + "took 60–65 · written 68–74 · out 77–81 · stopped 84–110, folded"
	log118  = logLead + "took 60–65 · written 68–74 · out 77–81 · stopped 84–117, folded"
	log119  = logLead + "took 60–65 · read 68–73 · written 76–82 · out 85–89 · stopped 92–118, folded"
	log125  = logLead + "took 60–65 · read 68–73 · written 76–82 · out 85–89 · stopped 92–124, folded"
	log126  = logFull + "stopped 99–125, folded"
	log151  = logFull + "stopped 99–150, folded"
	log152  = logFull + "stopped 99–125 · routing 128–151"
	log159  = logFull + "stopped 99–125 · routing 128–157"
	log160  = logFull + "stopped 99–125 · routing 128–157"
	log161  = logFull + "stopped 99–125 · routing 128–158"
	log200  = logFull + "stopped 99–145 · routing 148–197"
	log240  = logFull + "stopped 99–165 · routing 168–237"
)

// logAtFirst are the layouts at logWidths with the columns shown at first.
var logAtFirst = []string{
	phone52, phone80, phone99, log100, log101, log102, log110, log111, log118, log119,
	log125, log126, log151, log152, log159, log160, log161, log200, log240,
}

// withExtra are the layouts at logWidths with the extra given shown beside
// those shown at first, as given from the width it first fits at: the
// layouts at first below it, as an extra leaves before routing is cut short
// of its 24 cells or folds.
func withExtra(from int, wants ...string) []string {
	shown := make([]string, len(logWidths))
	for i, width := range logWidths {
		shown[i] = logAtFirst[i]
		if width >= from {
			shown[i], wants = wants[0], wants[1:]
		}
	}
	return shown
}

// The Requests' layout at every width, with no extras, and with each extra
// ticked: the extras leave first as it narrows, worth last of them, before
// routing is cut short of 24 cells or folds; then first, read, written and
// out leave; and past 160, the extras keep their widths and stopped and
// routing share what's left.
func TestTheLogsRequestsAreLaidOutAtEveryWidth(t *testing.T) {
	tests := []struct {
		name   string
		ticked LogTicks
		want   []string
	}{
		{name: "no extras", want: logAtFirst},
		{name: "worth", ticked: ticks(LogWorth), want: withExtra(160,
			logFull+"worth 99–104 · stopped 107–133 · routing 136–159",
			logFull+"worth 99–104 · stopped 107–133 · routing 136–160",
			logFull+"worth 99–104 · stopped 107–149 · routing 152–197",
			logFull+"worth 99–104 · stopped 107–169 · routing 172–237",
		)},
		{name: "input", ticked: ticks(LogInput), want: withExtra(160,
			logFull+"input 99–104 · stopped 107–133 · routing 136–159",
			logFull+"input 99–104 · stopped 107–133 · routing 136–160",
			logFull+"input 99–104 · stopped 107–149 · routing 152–197",
			logFull+"input 99–104 · stopped 107–169 · routing 172–237",
		)},
		{name: "attempts", ticked: ticks(LogAttempts), want: withExtra(200,
			logFull+"attempts 99–106 · stopped 109–150 · routing 153–197",
			logFull+"attempts 99–106 · stopped 109–170 · routing 173–237",
		)},
		{name: "size", ticked: ticks(LogSize), want: withExtra(160,
			logFull+"size 99–104 · stopped 107–133 · routing 136–159",
			logFull+"size 99–104 · stopped 107–133 · routing 136–160",
			logFull+"size 99–104 · stopped 107–149 · routing 152–197",
			logFull+"size 99–104 · stopped 107–169 · routing 172–237",
		)},
		{name: "messages", ticked: ticks(LogMessages), want: withExtra(200,
			logFull+"messages 99–106 · stopped 109–150 · routing 153–197",
			logFull+"messages 99–106 · stopped 109–170 · routing 173–237",
		)},
		{name: "tools", ticked: ticks(LogTools), want: withExtra(159,
			logFull+"tools 99–103 · stopped 106–132 · routing 135–158",
			logFull+"tools 99–103 · stopped 106–132 · routing 135–159",
			logFull+"tools 99–103 · stopped 106–132 · routing 135–160",
			logFull+"tools 99–103 · stopped 106–148 · routing 151–197",
			logFull+"tools 99–103 · stopped 106–168 · routing 171–237",
		)},
		{name: "thinking", ticked: ticks(LogThinking), want: withExtra(200,
			logFull+"thinking 99–106 · stopped 109–150 · routing 153–197",
			logFull+"thinking 99–106 · stopped 109–170 · routing 173–237",
		)},
		{name: "agent", ticked: ticks(LogAgent), want: withExtra(200,
			logFull+"agent 99–114 · stopped 117–154 · routing 157–197",
			logFull+"agent 99–114 · stopped 117–174 · routing 177–237",
		)},
		{name: "request id", ticked: ticks(LogRequestID), want: withExtra(200,
			logFull+"request id 99–108 · stopped 111–151 · routing 154–197",
			logFull+"request id 99–108 · stopped 111–171 · routing 174–237",
		)},
	}
	for _, tt := range tests {
		for i, width := range logWidths {
			t.Run(fmt.Sprintf("%s at %d", tt.name, width), func(t *testing.T) {
				if got := layoutSaid(logColumns(width, tt.ticked)); got != tt.want[i] {
					t.Errorf("logColumns(%d, %s) =\n%s\nwant\n%s", width, tt.name, got, tt.want[i])
				}
			})
		}
	}
}

// ticks are the columns shown at first with those given toggled.
func ticks(toggled ...LogColumn) LogTicks {
	var t LogTicks
	for _, c := range toggled {
		t = t.Toggled(c)
	}
	return t
}

// The hard moments of narrowing: extras leaving the last first, however
// many are ticked; and the table with stopped, routing or both hidden, a
// lone one running to the last column at its least, or neither, the columns
// before them fitting alone, on a phone too.
func TestTheLogsRequestsNarrowWhateverIsTicked(t *testing.T) {
	every := ticks(LogWorth, LogInput, LogAttempts, LogSize, LogMessages, LogTools, LogThinking, LogAgent, LogRequestID)
	tests := []struct {
		name   string
		width  int
		ticked LogTicks
		want   string
	}{
		{
			name: "worth and agent at 160, agent leaving", width: 160, ticked: ticks(LogWorth, LogAgent),
			want: logFull + "worth 99–104 · stopped 107–133 · routing 136–159",
		},
		{
			name: "every extra at 240, request id leaving", width: 240, ticked: every,
			want: logFull + "worth 99–104 · input 107–112 · attempts 115–122 · size 125–130 · messages 133–140 · tools 143–147 · " +
				"thinking 150–157 · agent 160–175 · stopped 178–204 · routing 207–237",
		},
		{
			name: "every extra at 200, routing cut to the room left", width: 200, ticked: every,
			want: logFull + "worth 99–104 · input 107–112 · attempts 115–122 · size 125–130 · messages 133–140 · stopped 143–169 · routing 172–199",
		},
		{name: "every extra at 151, each gone", width: 151, ticked: every, want: log151},
		{name: "routing hidden at 160, stopped to the last column", width: 160, ticked: ticks(LogRouting), want: logFull + "stopped 99–159"},
		{name: "routing hidden at 125, first leaving", width: 125, ticked: ticks(LogRouting), want: logLead + "took 60–65 · read 68–73 · written 76–82 · out 85–89 · stopped 92–124"},
		{name: "stopped hidden at 160, routing to the last column", width: 160, ticked: ticks(LogStopped), want: logFull + "routing 99–159"},
		{
			name: "stopped hidden at 100, routing keeping 24 cells", width: 100, ticked: ticks(LogStopped),
			want: logLead + "took 60–65 · out 68–72 · routing 75–99",
		},
		{name: "both hidden at 100", width: 100, ticked: ticks(LogStopped, LogRouting), want: logFull[:len(logFull)-len(" · ")]},
		{
			name: "worth with both hidden at 100, worth leaving", width: 100, ticked: ticks(LogWorth, LogStopped, LogRouting),
			want: logFull[:len(logFull)-len(" · ")],
		},
		{
			name: "worth with both hidden at 105, worth fitting", width: 105, ticked: ticks(LogWorth, LogStopped, LogRouting),
			want: logFull + "worth 99–104",
		},
		{
			name: "at alone", width: 160,
			ticked: ticks(LogSession, LogModel, LogOn, LogStatus, LogFirst, LogTook, LogRead, LogWritten, LogOut, LogStopped, LogRouting),
			want:   "at 2–9",
		},
		{name: "a phone without on", width: 52, ticked: ticks(LogOn), want: "phone: at 2–9 · session 11–14 · took 28–33 · stopped 37–51, folded"},
		{name: "a phone without routing", width: 52, ticked: ticks(LogRouting), want: "phone: at 2–9 · session 11–14 · on 20–27 · took 28–33 · stopped 37–51"},
		{name: "a phone without stopped", width: 52, ticked: ticks(LogStopped), want: "phone: at 2–9 · session 11–14 · on 20–27 · took 28–33 · routing 37–51"},
		{name: "a phone without either", width: 52, ticked: ticks(LogStopped, LogRouting), want: "phone: at 2–9 · session 11–14 · on 20–27 · took 28–33"},
		{name: "a phone with every extra", width: 80, ticked: every, want: phone80},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutSaid(logColumns(tt.width, tt.ticked)); got != tt.want {
				t.Errorf("logColumns(%d) =\n%s\nwant\n%s", tt.width, got, tt.want)
			}
		})
	}
}

// The column heads, each at its column's start, or ending at its end where
// the column is right-aligned, are as the design's frames draw them: at 160,
// and on a phone.
func TestTheLogsHeadsSitAsTheFramesDrawThem(t *testing.T) {
	tests := []struct {
		stem  string
		width int
		row   int
	}{
		{stem: "log-requests-160", width: 160, row: 6},
		{stem: "log-requests-phone-52", width: 52, row: 7},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			want := referenceRows(t, tt.stem, "txt")[tt.row]
			if got := headsOf(logColumns(tt.width, LogTicks{})); got != strings.TrimRight(want, " ") {
				t.Errorf("the heads at %d read\n%q\nwant\n%q", tt.width, got, want)
			}
		})
	}
}

// headsOf is the layout's heads, as a row of text: each column's name at its
// start, or ending at its end where it's right-aligned.
func headsOf(l logLayout) string {
	row := []rune(strings.Repeat(" ", 300))
	for _, p := range l.places {
		x := p.x
		if p.aligned == alignRight {
			x = p.x + p.width - len(p.column.Name())
		}
		copy(row[x:], []rune(p.column.Name()))
	}
	return strings.TrimRight(string(row), " ")
}
