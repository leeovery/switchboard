package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// logWidths are the widths the Requests' layout is held at: a phone's, each
// side of each width a column comes or goes at, the design's 160, and past it.
var logWidths = []int{
	52, 59, 60, 67, 68, 78, 79, 94, 95, 99, 100, 101, 102, 110, 111, 118, 119, 125, 126, 151, 152, 159, 160, 161, 200, 240,
}

// layoutSaid says a layout as the tests hold it: each column's name and the
// cells it takes, from its first to its last; then whether routing folds
// into stopped, and whether the session is its id alone; each after "brief:"
// where stopped says its words briefly.
func layoutSaid(l logLayout) string {
	said := make([]string, len(l.places))
	for i, p := range l.places {
		said[i] = fmt.Sprintf("%s %d–%d", p.column.Name(), p.x, p.x+p.width-1)
	}
	s := strings.Join(said, " · ")
	if l.folded {
		s += ", folded"
	}
	if !l.named && slices.ContainsFunc(l.places, func(p logPlace) bool { return p.column == LogSession }) {
		s += ", its id alone"
	}
	if l.brief {
		s = "brief: " + s
	}
	return s
}

// The layouts at each of logWidths with the columns shown at first: under
// 60, at, the session's id alone, on, took and stopped, as a phone's; from
// 60, the session's directory back; from 68, status; from 79, model; from
// 95, stopped saying its words whole; from 100 to 159 as before, first,
// read, written and out each back where stopped keeps its 27 cells; from
// 152, routing beside it, cut to the room left; at 160 as the design draws
// it; and past it, the two sharing the room past their widths. Each gap is
// two cells, at every width.
const (
	logLead = "at 2–9 · session 12–28 · model 31–39 · on 42–49 · status 52–57 · "
	logFull = logLead + "first 60–64 · took 67–72 · read 75–80 · written 83–89 · out 92–96 · "

	log52  = "brief: at 2–9 · session 12–18 · on 21–28 · took 31–36 · stopped 39–51, folded, its id alone"
	log59  = "brief: at 2–9 · session 12–18 · on 21–28 · took 31–36 · stopped 39–58, folded, its id alone"
	log60  = "brief: at 2–9 · session 12–28 · on 31–38 · took 41–46 · stopped 49–59, folded"
	log67  = "brief: at 2–9 · session 12–28 · on 31–38 · took 41–46 · stopped 49–66, folded"
	log68  = "brief: at 2–9 · session 12–28 · on 31–38 · status 41–46 · took 49–54 · stopped 57–67, folded"
	log78  = "brief: at 2–9 · session 12–28 · on 31–38 · status 41–46 · took 49–54 · stopped 57–77, folded"
	log79  = "brief: " + logLead + "took 60–65 · stopped 68–78, folded"
	log94  = "brief: " + logLead + "took 60–65 · stopped 68–93, folded"
	log95  = logLead + "took 60–65 · stopped 68–94, folded"
	log99  = logLead + "took 60–65 · stopped 68–98, folded"
	log100 = logLead + "took 60–65 · stopped 68–99, folded"
	log101 = logLead + "took 60–65 · stopped 68–100, folded"
	log102 = logLead + "took 60–65 · out 68–72 · stopped 75–101, folded"
	log110 = logLead + "took 60–65 · out 68–72 · stopped 75–109, folded"
	log111 = logLead + "took 60–65 · written 68–74 · out 77–81 · stopped 84–110, folded"
	log118 = logLead + "took 60–65 · written 68–74 · out 77–81 · stopped 84–117, folded"
	log119 = logLead + "took 60–65 · read 68–73 · written 76–82 · out 85–89 · stopped 92–118, folded"
	log125 = logLead + "took 60–65 · read 68–73 · written 76–82 · out 85–89 · stopped 92–124, folded"
	log126 = logFull + "stopped 99–125, folded"
	log151 = logFull + "stopped 99–150, folded"
	log152 = logFull + "stopped 99–125 · routing 128–151"
	log159 = logFull + "stopped 99–125 · routing 128–157"
	log160 = logFull + "stopped 99–125 · routing 128–157"
	log161 = logFull + "stopped 99–125 · routing 128–158"
	log200 = logFull + "stopped 99–145 · routing 148–197"
	log240 = logFull + "stopped 99–165 · routing 168–237"
)

// logAtFirst are the layouts at logWidths with the columns shown at first.
var logAtFirst = []string{
	log52, log59, log60, log67, log68, log78, log79, log94, log95, log99, log100, log101, log102, log110, log111,
	log118, log119, log125, log126, log151, log152, log159, log160, log161, log200, log240,
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
				if got := layoutSaid(logColumns(width, tt.ticked, labelLeast)); got != tt.want[i] {
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
		{
			name: "a phone without on, the directory kept", width: 52, ticked: ticks(LogOn),
			want: "brief: at 2–9 · session 12–28 · took 31–36 · stopped 39–51, folded",
		},
		{
			name: "a phone without routing", width: 52, ticked: ticks(LogRouting),
			want: "brief: at 2–9 · session 12–18 · on 21–28 · took 31–36 · stopped 39–51, its id alone",
		},
		{
			name: "a phone without stopped, routing said whole in the cells left", width: 52, ticked: ticks(LogStopped),
			want: "at 2–9 · session 12–18 · on 21–28 · took 31–36 · routing 39–51, its id alone",
		},
		{
			name: "a phone without either, the directory kept", width: 52, ticked: ticks(LogStopped, LogRouting),
			want: "at 2–9 · session 12–28 · on 31–38 · took 41–46",
		},
		{name: "a phone with every extra", width: 80, ticked: every, want: "brief: " + logLead + "took 60–65 · stopped 68–79, folded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutSaid(logColumns(tt.width, tt.ticked, labelLeast)); got != tt.want {
				t.Errorf("logColumns(%d) =\n%s\nwant\n%s", tt.width, got, tt.want)
			}
		})
	}
}

// on at its natural width, the widest configured account's: at least
// labelLeast; in the first pass, whole until first, read and written have
// left, then cut before out leaves; in the second, taking what's past the
// leasts of the columns the first kept before any other, as the spike's 73
// and 108 columns draw a table of long names.
func TestTheLogsOnIsLaidOutAtItsNaturalWidth(t *testing.T) {
	tests := []struct {
		name      string
		width, on int
		want      string
	}{
		{
			name: "long names at 73, on taking the 5 cells left", width: 73, on: 22,
			want: "brief: at 2–9 · session 12–28 · on 31–43 · status 46–51 · took 54–59 · stopped 62–72, folded",
		},
		{
			name: "long names at 108, on taking the 6 cells past stopped's 27", width: 108, on: 22,
			want: "at 2–9 · session 12–28 · model 31–39 · on 42–55 · status 58–63 · took 66–71 · out 74–78 · stopped 81–107, folded",
		},
		{
			name: "long names at 116, on whole", width: 116, on: 22,
			want: "at 2–9 · session 12–28 · model 31–39 · on 42–63 · status 66–71 · took 74–79 · out 82–86 · stopped 89–115, folded",
		},
		{
			name: "long names at 173, on whole", width: 173, on: 22,
			want: "at 2–9 · session 12–28 · model 31–39 · on 42–63 · status 66–71 · first 74–78 · took 81–86 · read 89–94 · " +
				"written 97–103 · out 106–110 · stopped 113–139 · routing 142–171",
		},
		{name: "accounts shorter than on's least", width: 160, on: 3, want: log160},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := layoutSaid(logColumns(tt.width, LogTicks{}, tt.on)); got != tt.want {
				t.Errorf("logColumns(%d, on %d) =\n%s\nwant\n%s", tt.width, tt.on, got, tt.want)
			}
		})
	}
}

// From 116 columns, a table of long names has on whole at every width,
// whatever's ticked: first, read and written leave before it's cut.
func TestTheLogsOnIsWholeFrom116Columns(t *testing.T) {
	for _, ticked := range []LogTicks{{}, ticks(LogRouting), ticks(LogStopped), everyExtra()} {
		for width := 116; width <= 220; width++ {
			l := logColumns(width, ticked, 22)
			i := slices.IndexFunc(l.places, func(p logPlace) bool { return p.column == LogOn })
			if i < 0 || l.places[i].width != 22 {
				t.Errorf("at %d, ticked %v, the layout is %s, want on whole, 22 cells", width, ticked, layoutSaid(l))
			}
		}
	}
}

// The second pass lays out the columns the first kept and no others, and
// the row runs to the last column and no further, at every width from a
// phone's, of names short and long, whatever's ticked.
func TestTheLogsSecondPassKeepsTheFirstsColumnsWithinTheRow(t *testing.T) {
	for _, ticked := range []LogTicks{{}, ticks(LogRouting), ticks(LogStopped), ticks(LogStopped, LogRouting), everyExtra()} {
		for _, on := range []int{labelLeast, 15, labelMost} {
			for width := 52; width <= 240; width++ {
				_, kept := logSpread.decided(width, ticked.shown(), on)
				var want []LogColumn
				for _, p := range kept {
					if p.role != routingMargin && !slices.Contains(want, p.column) {
						want = append(want, p.column)
					}
				}
				l := logColumns(width, ticked, on)
				got := make([]LogColumn, len(l.places))
				for i, p := range l.places {
					got[i] = p.column
				}
				last := l.places[len(l.places)-1]
				if !slices.Equal(got, want) || last.x+last.width > width {
					t.Errorf("at %d, on %d, ticked %v, the layout is %s, want the columns the first pass kept, %v, within the row", width, on, ticked, layoutSaid(l), want)
				}
			}
		}
	}
}

// A column of names' natural is the widest name of the accounts configured,
// as the dashboard names them, labelMost at the most.
func TestTheWidestLabelIsTheWidestConfiguredAccounts(t *testing.T) {
	tests := []struct {
		name     string
		accounts []status.Account
		want     int
	}{
		{name: "none", want: 0},
		{name: "by label, else id", accounts: []status.Account{{ID: "work", Label: "personal-research"}, {ID: "side"}}, want: 17},
		{name: "a label past labelMost", accounts: []status.Account{{ID: "work", Label: "an-account-named-at-great-length"}}, want: labelMost},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := frameOf(160, 24).Moment(routerDoc("side", 0, tt.accounts...), now)
			if got := widestLabel(m); got != tt.want {
				t.Errorf("widestLabel = %d, want %d", got, tt.want)
			}
		})
	}
}
