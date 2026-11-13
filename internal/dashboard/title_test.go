package dashboard

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

func TestTheTitleRow(t *testing.T) {
	tests := []struct {
		name  string
		width int
		shown View
		// clock is the frame's Clock: zero for none.
		clock time.Time
		// want are the rows it draws, and wantRows how many it takes.
		want     []string
		wantRows int
	}{
		{name: "the badge at the first column, the eight tabs two cells after it, and the clock ending at the last", width: 160, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log " + blanks(48) + "Mon 28 Sep  13:12:00",
		}, wantRows: 1},
		{name: "the clock at the frame's Clock, where it's set", width: 160, shown: Overview, clock: now.Add(10 * time.Minute), want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log " + blanks(48) + "Mon 28 Sep  13:22:00",
		}, wantRows: 1},
		{name: "the clock two cells clear of the tabs, where its date just fits", width: 114, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log   Mon 28 Sep  13:12:00",
		}, wantRows: 1},
		{name: "the date giving way first, where a date of two digits leaves too little room for the time", width: 113, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log " + blanks(13) + "13:12:00",
		}, wantRows: 1},
		{name: "the date kept a cell narrower, where the day has one digit", width: 113, shown: Overview, clock: oneDigitDay, want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log   Sat 3 Oct  13:12:00",
		}, wantRows: 1},
		{name: "the time alone, two cells clear of the tabs", width: 102, shown: Overview, clock: oneDigitDay, want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log   13:12:00",
		}, wantRows: 1},
		{name: "the tabs alone at the narrowest widths off a phone, too narrow for the time", width: 101, shown: Overview, want: []string{
			" SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log",
		}, wantRows: 1},
		{name: "on a phone, the clock on row 0, and the tabs filling a row of their own, touching", width: 70, shown: Sessions, want: []string{
			" SWITCHBOARD" + blanks(38) + "Mon 28 Sep  13:12:00",
			" Overview  Accounts  Projects  History  Sessions  Runway  Graphs  Log",
		}, wantRows: 2},
		{name: "on a phone too narrow for one row of tabs, two, as even as their order allows", width: 69, shown: Overview, want: []string{
			" SWITCHBOARD" + blanks(37) + "Mon 28 Sep  13:12:00",
			" Overview  Accounts  Projects  History",
			" Sessions  Runway  Graphs  Log",
		}, wantRows: 3},
		{name: "on a phone just wide enough for the date", width: 35, shown: Overview, want: []string{
			" SWITCHBOARD" + blanks(3) + "Mon 28 Sep  13:12:00",
		}, wantRows: 4},
		{name: "on a phone too narrow for a date of two digits, the time alone", width: 34, shown: Overview, want: []string{
			" SWITCHBOARD" + blanks(14) + "13:12:00",
		}, wantRows: 4},
		{name: "on a phone as narrow, a date of one digit kept", width: 34, shown: Overview, clock: oneDigitDay, want: []string{
			" SWITCHBOARD" + blanks(3) + "Sat 3 Oct  13:12:00",
		}, wantRows: 4},
		{name: "on a phone too narrow for two rows of tabs, three", width: 30, shown: Overview, want: []string{
			" SWITCHBOARD" + blanks(10) + "13:12:00",
			" Overview  Accounts",
			" Projects  History  Sessions",
			" Runway  Graphs  Log",
		}, wantRows: 4},
		{name: "on a phone too narrow for the time, the badge alone", width: 20, shown: Overview, want: []string{
			" SWITCHBOARD ",
		}, wantRows: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 5, View: tt.shown, Clock: tt.clock}
			c := newCanvas(f.Width, f.Height)
			f.title(c, now)
			if got := f.titleRows(); got != tt.wantRows {
				t.Errorf("titleRows() = %d, want %d", got, tt.wantRows)
			}
			if rows := c.rows(Look{})[:len(tt.want)]; !slices.Equal(rows, tt.want) {
				t.Errorf("the title reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// oneDigitDay is a clock on a day of one digit.
var oneDigitDay = time.Date(2026, 10, 3, 13, 12, 0, 0, now.Location())

// Off a phone, at every width, the title row reads as its parts in order, none
// over another: the tabs, then, where its time fits, at least two blank
// cells and the clock ending at the last column, its date where it shows.
func TestTheTitleRowsPartsNeverOverlap(t *testing.T) {
	tabs := " SWITCHBOARD    Overview   Accounts   Projects   History   Sessions   Runway   Graphs   Log "
	timeFits := len(tabs) + 2 + len("13:12:00")
	rest := regexp.MustCompile(`^  +([A-Z][a-z]{2} [0-9]{1,2} [A-Z][a-z]{2}  )?13:12:00$`)
	for _, clock := range []time.Time{now, oneDigitDay} {
		for width := 100; width <= 200; width++ {
			f := Frame{Width: width, Height: 1, View: Overview, Clock: clock}
			c := newCanvas(f.Width, f.Height)
			f.title(c, now)
			row := c.rows(Look{})[0]
			if width < timeFits {
				if want := strings.TrimSuffix(tabs, " "); row != want {
					t.Errorf("at %d columns on %s, the title row reads\n%q\nwant its tabs alone, too narrow for the time", width, prose.Day(clock), row)
				}
				continue
			}
			if after, ok := strings.CutPrefix(row, tabs); !ok || !rest.MatchString(after) || len([]rune(row)) != width {
				t.Errorf("at %d columns on %s, the title row reads\n%q\nwant its tabs, then the clock apart, ending at the last column", width, prose.Day(clock), row)
			}
		}
	}
}

func TestTheTitleRowsInks(t *testing.T) {
	tests := []struct {
		name  string
		width int
		x, y  int
		want  ink
	}{
		{name: "the badge", width: 160, x: 0, want: ink{token: theme.Canvas, on: hue{token: theme.AccentPrimary}, bold: true}},
		{name: "a tab not shown", width: 160, x: 15, want: tabInk},
		{name: "the tab shown", width: 160, x: 58, want: shownTabInk},
		{name: "the date", width: 160, x: 140, want: dateInk},
		{name: "the time, bold", width: 160, x: 159, want: timeInk},
		{name: "a phone's tab shown", width: 52, x: 1, y: 2, want: shownTabInk},
		{name: "a phone's time", width: 52, x: 51, want: timeInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 3, View: Sessions}
			c := newCanvas(f.Width, f.Height)
			f.title(c, now)
			if got := c.at(tt.x, tt.y).ink; got != tt.want {
				t.Errorf("column %d of row %d is in %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
	if shownTabInk.on != (hue{token: theme.BgSelection}) || timeInk != (ink{token: theme.TextSecondary, bold: true, kept: true}) {
		t.Errorf("the tab shown is on %v and the time in %+v, want bg.selection, and bold text.secondary, kept bright behind a card", shownTabInk.on, timeInk)
	}
	if dateInk != (ink{token: theme.TextMuted, kept: true}) {
		t.Errorf("the date is in %+v, want text.muted, kept bright behind a card", dateInk)
	}
}

// The tabs' widths, six and seven as there were and eight as there are.
var (
	sixTabs   = []int{10, 10, 9, 10, 8, 5}
	sevenTabs = []int{10, 10, 9, 10, 8, 8, 5}
	eightTabs = []int{10, 10, 10, 9, 10, 8, 8, 5}
)

func TestAPhonesTabsSplitOverRows(t *testing.T) {
	tests := []struct {
		name   string
		widths []int
		room   int
		want   []int
	}{
		{name: "six on one row where they fit", widths: sixTabs, room: 52, want: []int{6}},
		{name: "six a cell short, on two rows", widths: sixTabs, room: 51, want: []int{3, 3}},
		{name: "six on two rows while the wider fits", widths: sixTabs, room: 29, want: []int{3, 3}},
		{name: "six a cell short of that, on three", widths: sixTabs, room: 28, want: []int{2, 2, 2}},
		{name: "seven on one row where they fit", widths: sevenTabs, room: 60, want: []int{7}},
		{name: "seven a cell short, two rows, the wider as narrow as it can be", widths: sevenTabs, room: 59, want: []int{3, 4}},
		{name: "seven on two rows while the wider fits", widths: sevenTabs, room: 31, want: []int{3, 4}},
		{name: "seven a cell short of that, on three", widths: sevenTabs, room: 30, want: []int{2, 2, 3}},
		{name: "seven on three rows while the widest fits", widths: sevenTabs, room: 21, want: []int{2, 2, 3}},
		{name: "seven a cell short of that, on four, the earlier rows the fuller", widths: sevenTabs, room: 20, want: []int{1, 2, 2, 2}},
		{name: "seven narrower than a tab, split as where the widest just fits", widths: sevenTabs, room: 5, want: []int{1, 1, 1, 1, 1, 1, 1}},
		{name: "eight on one row where they fit", widths: eightTabs, room: 70, want: []int{8}},
		{name: "eight a cell short, on two rows", widths: eightTabs, room: 69, want: []int{4, 4}},
		{name: "eight on two rows while the wider fits", widths: eightTabs, room: 39, want: []int{4, 4}},
		{name: "eight a cell short of that, on three", widths: eightTabs, room: 38, want: []int{2, 3, 3}},
		{name: "eight on three rows while the widest fits", widths: eightTabs, room: 29, want: []int{2, 3, 3}},
		{name: "eight a cell short of that, on four", widths: eightTabs, room: 28, want: []int{2, 2, 2, 2}},
		{name: "none", room: 40},
	}
	if got := (Frame{}).tabWidths(); !slices.Equal(got, eightTabs) {
		t.Errorf("the tabs are %v cells wide, want %v", got, eightTabs)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tabRows(tt.widths, tt.room); !slices.Equal(got, tt.want) {
				t.Errorf("tabRows(%v, %d) = %v, want %v", tt.widths, tt.room, got, tt.want)
			}
		})
	}
}

// At every width, the tabs take the fewest rows they fit on whole, the widest
// as narrow as any split over that many rows makes it, the earlier rows the
// fuller where splits tie; and a split hangs on the count of rows alone.
func TestAPhonesTabsSplitOnTheFewestRowsAsEvenlyAsTheyMay(t *testing.T) {
	for _, widths := range [][]int{sixTabs, sevenTabs, eightTabs} {
		byRows := map[int][]int{}
		for room := 20; room <= 200; room++ {
			got := tabRows(widths, room)
			if want := evenest(widths, room); !slices.Equal(got, want) {
				t.Errorf("%v in %d cells split %v, want %v", widths, room, got, want)
			}
			if before, ok := byRows[len(got)]; ok && !slices.Equal(before, got) {
				t.Errorf("%v on %d rows split %v in %d cells, but %v in a narrower room", widths, len(got), got, room, before)
			}
			byRows[len(got)] = got
		}
	}
}

// evenest is the split of tabs of the widths given that tabRows should give
// in room cells, found by trying every split in order: of those on the
// fewest rows that fit, the one whose widest row is narrowest, the earlier
// rows the fuller.
func evenest(widths []int, room int) []int {
	var best []int
	bestWidest := 0
	for cuts := range 1 << (len(widths) - 1) {
		split, widest := splitAt(widths, cuts)
		switch {
		case widest > room:
		case best == nil, len(split) < len(best),
			len(split) == len(best) && widest < bestWidest,
			len(split) == len(best) && widest == bestWidest && slices.Compare(split, best) > 0:
			best, bestWidest = split, widest
		}
	}
	return best
}

// splitAt is the tabs of the widths given split after each tab whose bit is
// set in cuts: how many go to each row, and the widest row's width.
func splitAt(widths []int, cuts int) (split []int, widest int) {
	count, taken := 0, 0
	for i, w := range widths {
		count, taken = count+1, taken+w
		if i == len(widths)-1 || cuts&(1<<i) != 0 {
			split, widest = append(split, count), max(widest, taken)
			count, taken = 0, 0
		}
	}
	return split, widest
}

// blanks is n blank cells.
func blanks(n int) string {
	return strings.Repeat(" ", n)
}
