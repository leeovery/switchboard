package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// poolNow is when the pool's charts are drawn in their tests: 14:00 on
// Thursday 1 October.
var poolNow = clockOn(14, 0)

// poolRoomOf is the room of a pool of two accounts, a column each of the
// rooms given, the last at now, each with the share of the pool it leaves
// used, as status.PoolUsed has it: a column whose room is below none isn't
// known.
func poolRoomOf(rooms ...float64) views.PoolRoom {
	room := views.PoolRoom{Of: 2}
	for _, r := range rooms {
		col := views.PoolRoomColumn{Room: max(r, 0), Known: r >= 0}
		if col.Known {
			col.Used = status.PoolUsed(r, room.Of)
		}
		room.Columns = append(room.Columns, col)
	}
	return room
}

// poolChartIn is the pool's chart of a window as long as given at poolNow of
// the room given, drawn as burn has it in look, width cells wide, h rows of
// chart between its top row and its axis, on a canvas of its own.
func poolChartIn(room views.PoolRoom, length time.Duration, burn Burn, look Look, width, h int) *canvas {
	pc := poolChart{key: "5h", name: "5-HOUR", length: length, room: room, now: poolNow, burn: burn}
	c := newCanvas(width, h+2)
	pc.draw(c, look, bounds{width: width, rows: h + 2})
	return c
}

// The pool's room, of 2 accounts, 0.6, 0.85, 1.25, 2 and 1.25 at now, leaves
// 0.7, 0.575, 0.375, none and 0.375 of all of it used: the burn-up fills
// each column to that, the burn-down to what's left.
func TestAPoolsChartFillsEachColumnToItsRoomAsItsBurnHasIt(t *testing.T) {
	tests := []struct {
		burn Burn
		want []int
	}{
		{burn: BurnUp, want: []int{22, 18, 12, 0, 12}},
		{burn: BurnDown, want: []int{10, 14, 20, 32, 20}},
	}
	nord := Screen(builtin(t, "nord"))
	room := poolRoomOf(0.6, 0.85, 1.25, 2, 1.25)
	for _, tt := range tests {
		t.Run(tt.burn.Name(), func(t *testing.T) {
			if got := filledOf(poolChartIn(room, 5*time.Hour, tt.burn, nord, 5, 4), 4); !slices.Equal(got, tt.want) {
				t.Errorf("the pool's %s fills its columns to %v eighths, want %v", tt.burn.Name(), got, tt.want)
			}
		})
	}
}

// Its columns end at its last cell, now's; a column whose room isn't known
// is blank; and a chart whose room is known at now alone says it has no
// history across its middle row.
func TestAPoolsChartIsBlankWhereItsRoomIsntKnownAndSaysSoWithNoHistory(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	known := filledOf(poolChartIn(poolRoomOf(-1, 1, 2, 1.25), 5*time.Hour, BurnDown, nord, 6, 4), 4)
	if want := []int{0, 0, 0, 16, 32, 20}; !slices.Equal(known, want) {
		t.Errorf("the pool's burn-down of four columns, its first not known, fills its six to %v eighths, want %v", known, want)
	}
	alone := poolChartIn(poolRoomOf(-1, -1, -1, 1.25), 5*time.Hour, BurnDown, nord, 25, 4)
	if got := filledOf(alone, 4); slices.ContainsFunc(got[:24], func(e int) bool { return e > 0 }) || got[24] != 20 {
		t.Errorf("the pool's burn-down known at now alone fills its columns to %v eighths, want none but now's, 20", got)
	}
	if got := rowOf(alone, 3); !strings.Contains(got, untracedLabel) {
		t.Errorf("its middle row reads %q, want %q across it", got, untracedLabel)
	}
}

// Its axis is its length back from now, a time of day for a window of a day
// or less, with its date for a longer one, as a week back is on now's
// weekday; and now.
func TestAPoolsChartsAxisIsItsLengthBackFromNowAndNow(t *testing.T) {
	tests := []struct {
		length time.Duration
		want   string
	}{
		{length: 5 * time.Hour, want: "09:00" + strings.Repeat(" ", 17) + "now"},
		{length: 7 * 24 * time.Hour, want: "24 Sep 14:00" + strings.Repeat(" ", 10) + "now"},
	}
	for _, tt := range tests {
		c := poolChartIn(poolRoomOf(1, 1.25), tt.length, BurnUp, Look{}, 25, 4)
		if got := rowOf(c, 5); got != tt.want {
			t.Errorf("the pool's chart %v long has its axis read %q, want %q", tt.length, got, tt.want)
		}
	}
}

// pooledRooms is the frame f, its pool's room in each window it charts of
// doc at now built, as the watch builds it, with nothing read before.
func pooledRooms(f Frame, doc status.Document, now time.Time) Frame {
	f.PoolRooms = make(map[string]views.PoolRoom)
	for _, ch := range f.PoolCharts(doc, now) {
		f.PoolRooms[ch.Key] = views.PoolRoom{}.Look(views.PoolRoomFrom{Moment: f.Moment(doc, now), Key: ch.Key, Columns: ch.Columns})
	}
	return f
}

func TestByAccountThePoolsChartsSitBesideItsBlock(t *testing.T) {
	f := besideOf(173)
	f.Models = false
	f = pooledRooms(f, trio(), printNow)
	rows, _ := usageBody(f, trio(), printNow, 40)
	i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, "Σ pool") })
	if i < 0 {
		t.Fatalf("USAGE by account reads\n%s\nwant the pool's block", strings.Join(rows, "\n"))
	}
	if got, want := colAt(rows[i], 107, 64), titlesRow(31, besideGap, "WEEK", "5-HOUR"); got != want {
		t.Errorf("beside the pool's block, the charts' top row reads\n%q\nwant\n%q", got, want)
	}
	if got := colAt(rows[i+6], 107, 6); got != "24 Sep" {
		t.Errorf("the second row under the pool's block reads %q beside it, want the week's axis, from a week back, 24 Sep, under its 5 rows of chart", got)
	}
}

// The pool's charts the watch builds the room of are those GRAPHS draws
// beside Σ pool by account, each window's as wide as it's drawn: none by
// window, under 150 columns, with GRAPHS hidden, or with one account.
func TestThePoolsChartsAreThoseDrawnBesideItsBlock(t *testing.T) {
	byWindow := besideOf(160)
	byWindow.By = ByWindow
	hidden := besideOf(160)
	hidden.Sections = Sections{}
	alone := trio()
	alone.Accounts = alone.Accounts[:1]
	tests := []struct {
		name  string
		frame Frame
		doc   status.Document
		want  []PoolChart
	}{
		{name: "by account at 173 columns, two a row", frame: besideOf(173), doc: trio(), want: []PoolChart{{Key: "7d", Columns: 31}, {Key: "5h", Columns: 31}, {Key: "7d_oi", Columns: 31}}},
		{name: "by account at 160 columns, one a row", frame: besideOf(160), doc: trio(), want: []PoolChart{{Key: "7d", Columns: 52}, {Key: "5h", Columns: 52}, {Key: "7d_oi", Columns: 52}}},
		{name: "by account at 150 columns, one a row", frame: besideOf(150), doc: trio(), want: []PoolChart{{Key: "7d", Columns: 42}, {Key: "5h", Columns: 42}, {Key: "7d_oi", Columns: 42}}},
		{name: "by window", frame: byWindow, doc: trio()},
		{name: "under 150 columns", frame: besideOf(149), doc: trio()},
		{name: "GRAPHS hidden", frame: hidden, doc: trio()},
		{name: "one account", frame: besideOf(160), doc: alone},
	}
	for _, tt := range tests {
		if got := tt.frame.PoolCharts(tt.doc, printNow); !slices.Equal(got, tt.want) {
			t.Errorf("%s: the pool's charts are %v, want %v", tt.name, got, tt.want)
		}
	}
}
