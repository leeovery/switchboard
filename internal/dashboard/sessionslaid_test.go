package dashboard

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// sat is a look at the Switchboard: who runs on each account, in the order
// views.RunningOf lists them, as "personal: 41e0 9c2e"; and the stubs each
// account's limit hangs, by its id.
type sat struct {
	on    []string
	stubs map[string]int
}

// patchedBy is the board the looks given keep, each after the last, as
// views.Patch keeps it.
func patchedBy(looks ...sat) views.Patched {
	var p views.Patched
	for _, look := range looks {
		var calls []views.Call
		for _, on := range look.on {
			account, sessions, _ := strings.Cut(on, ": ")
			for s := range strings.FieldsSeq(sessions) {
				calls = append(calls, views.Call{Account: account, Session: s})
			}
		}
		p = views.Patch(p, calls, look.stubs)
	}
	return p
}

// threeLines are the three accounts' lines the wide frames draw, each with
// three windows: 5-hour, Week and Fable wk.
var threeLines = boardLines{accounts: []string{"work", "personal", "side"}, windows: 3}

// accountsNamed are n accounts' ids, from a0.
func accountsNamed(n int) []string {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("a%d", i)
	}
	return ids
}

// viewRows are the view's rows at 160 by height: from row 6 to the row over
// the state line.
func viewRows(height int) int {
	return Frame{Width: 160, Height: height, View: Sessions}.viewRows()
}

// callWidths are what a width gives a call's names, and, on the board, the
// lines' parts: the directory's cells and the account's after ▸, whether ▸
// drops its from; the bars' cells, whether the words are short, and the
// bends' room; and whether it's laid on a lane.
type callWidths struct {
	dir, account      int
	fromShort         bool
	bar, room         int
	shortWords, lanes bool
}

// boardWidths are what the board's columns give as callWidths.
func boardWidths(k boardColumns) callWidths {
	return callWidths{dir: k.dir, account: k.account, fromShort: k.fromShort, bar: k.bar, room: k.room, shortWords: k.shortWords}
}

// The Switchboard's parts at their leasts, then a call's names raised to
// their naturals before the lines' bars: the directory to
// views.SessionDirMost, the account after ▸ to the widest name configured,
// of 8, 14 or 22 cells, none where no call has moved; then the bars, the
// lines' words whole, ▸'s from, the bars to 32 and the bends' room the rest.
// At 108, the 8 cells past the leasts go to the directory alone. Where the
// leasts don't fit, as under 100 columns whether or not a call has moved,
// it draws lanes, which lay a call's parts by the same rule, from column 2,
// what it's doing two cells after the directory, the spare at the row's end.
func TestACallsNamesAreRaisedBeforeTheLinesBars(t *testing.T) {
	tests := []struct {
		width, from int
		want        callWidths
	}{
		{width: 220, from: 0, want: callWidths{dir: 24, bar: 32, room: 96}},
		{width: 220, from: 8, want: callWidths{dir: 24, account: 8, bar: 32, room: 81}},
		{width: 220, from: 14, want: callWidths{dir: 24, account: 14, bar: 32, room: 75}},
		{width: 220, from: 22, want: callWidths{dir: 24, account: 22, bar: 32, room: 67}},
		{width: 160, from: 8, want: callWidths{dir: 24, account: 8, bar: 32, room: 21}},
		{width: 160, from: 14, want: callWidths{dir: 24, account: 14, bar: 29, room: 18}},
		{width: 160, from: 22, want: callWidths{dir: 24, account: 22, bar: 21, room: 18}},
		{width: 138, from: 8, want: callWidths{dir: 24, account: 8, fromShort: true, bar: 16, room: 20}},
		{width: 138, from: 14, want: callWidths{dir: 24, account: 14, fromShort: true, bar: 16, room: 22, shortWords: true}},
		{width: 138, from: 22, want: callWidths{dir: 24, account: 22, fromShort: true, bar: 12, room: 18, shortWords: true}},
		{width: 108, from: 0, want: callWidths{dir: 22, bar: 8, room: 18, shortWords: true}},
		{width: 108, from: 8, want: callWidths{dir: 14, account: 6, fromShort: true, bar: 8, room: 18, shortWords: true}},
		{width: 108, from: 14, want: callWidths{dir: 14, account: 6, fromShort: true, bar: 8, room: 18, shortWords: true}},
		{width: 108, from: 22, want: callWidths{dir: 14, account: 6, fromShort: true, bar: 8, room: 18, shortWords: true}},
		{width: 100, from: 22, want: callWidths{dir: 6, account: 6, fromShort: true, bar: 8, room: 18, shortWords: true}},
		{width: 100, from: 0, want: callWidths{dir: 14, bar: 8, room: 18, shortWords: true}},
		{width: 99, from: 0, want: callWidths{dir: 24, lanes: true}},
		{width: 99, from: 22, want: callWidths{dir: 24, account: 22, lanes: true}},
		{width: 73, from: 8, want: callWidths{dir: 24, account: 8, lanes: true}},
		{width: 73, from: 14, want: callWidths{dir: 24, account: 14, lanes: true}},
		{width: 73, from: 22, want: callWidths{dir: 24, account: 22, fromShort: true, lanes: true}},
		{width: 52, from: 0, want: callWidths{dir: 24, lanes: true}},
		{width: 52, from: 8, want: callWidths{dir: 22, account: 6, fromShort: true, lanes: true}},
		{width: 52, from: 14, want: callWidths{dir: 22, account: 6, fromShort: true, lanes: true}},
		{width: 52, from: 22, want: callWidths{dir: 22, account: 6, fromShort: true, lanes: true}},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d with %d", tt.width, tt.from), func(t *testing.T) {
			k, fits := boardColumnsOf(tt.width, tt.from)
			got := boardWidths(k)
			if !fits {
				k = boardColumns{callColumns: laneColumnsOf(tt.width, tt.from)}
				got = callWidths{dir: k.dir, account: k.account, fromShort: k.fromShort, lanes: true}
			}
			if got != tt.want {
				t.Errorf("laid %+v, want %+v", got, tt.want)
			}
			if err := keepsItsDistances(k, tt.from, tt.width); err != nil {
				t.Error(err)
			}
		})
	}
}

// fromColumnCells are the cells the from column takes in the columns given,
// its account's natural from cells: none with none; "▸ " and the account
// where ▸ drops its from; else "▸ from " and the account.
func fromColumnCells(k callColumns, from int) int {
	switch {
	case from == 0:
		return 0
	case k.fromShort:
		return 2 + k.account
	}
	return 7 + k.account
}

// keepsItsDistances is an error where a part of the columns given isn't its
// distance from the part before it, the from column's natural from cells,
// none with none: on a lane, its lines 0, from column 2, what it's doing two
// cells after the directory; on the board, from column 0, a cell after it,
// the plugs two cells after the from column, the jacks 3 after the bends'
// room, and the lines ending at the last column.
func keepsItsDistances(k boardColumns, from, width int) error {
	id, doingGap := 0, 1
	if k.lines == 0 {
		id, doingGap = 2, 2
	}
	fromCells := fromColumnCells(k.callColumns, from)
	switch {
	case k.id != id, k.doing != id+4+1+k.dir+doingGap, k.from != k.doing+13:
		return fmt.Errorf("id at %d, doing at %d, ▸ at %d, want each its distance from the part before it", k.id, k.doing, k.from)
	case from == 0 && k.account != 0:
		return fmt.Errorf("with no call moved, the from column has %d cells, want none", k.account)
	case k.lines == 0:
		return nil
	case k.plug != k.from+fromCells+2, k.jacks != k.plug+k.room+3, k.lines != k.jacks+2:
		return fmt.Errorf("the plugs at %d, the jacks at %d and the lines at %d, want each its distance from the part before it", k.plug, k.jacks, k.lines)
	case k.lines+1+11+k.bar+1+4+2+k.words+1 != width:
		return fmt.Errorf("the lines from %d, bars %d and words %d, want them ending at the last column", k.lines, k.bar, k.words)
	}
	return nil
}

// At every width from 100 to 220, of names of each width, the board's parts
// fit and keep their distances; the lines' bars pass their least, their
// words are whole and ▸ keeps its from only where a call's names are whole;
// and as the frame widens a cell, no name narrows, nor does ▸ drop its from.
func TestAsTheBoardWidensNoNameNarrowsAndNamesComeFirst(t *testing.T) {
	for _, from := range []int{0, 8, 14, 22} {
		var before boardColumns
		for width := 100; width <= 220; width++ {
			k, fits := boardColumnsOf(width, from)
			if !fits {
				t.Fatalf("with %d, at %d the parts don't fit", from, width)
			}
			if err := keepsItsDistances(k, from, width); err != nil {
				t.Errorf("with %d, at %d: %v", from, width, err)
			}
			namesWhole := k.dir == views.SessionDirMost && k.account == from
			if !namesWhole && (k.bar > leastBar || !k.shortWords || from > 0 && !k.fromShort) {
				t.Errorf("with %d, at %d the names are %d and %d with the bars %d, words short %t, ▸ short %t: want the names whole first", from, width, k.dir, k.account, k.bar, k.shortWords, k.fromShort)
			}
			if width > 100 && (k.dir < before.dir || k.account < before.account || k.fromShort && !before.fromShort) {
				t.Errorf("with %d, at %d the names are %d and %d, ▸ short %t; a cell narrower, %d and %d, ▸ short %t", from, width, k.dir, k.account, k.fromShort, before.dir, before.account, before.fromShort)
			}
			before = k
		}
	}
}

// At every width from 52 to 99, of names of each width, a lane's call keeps
// its parts' distances, and gives way in the board's turns: ▸ drops its
// from before the account narrows, and the account reaches its least before
// the directory narrows. A name cut leaves no cell spare at the row's end,
// and as the frame widens a cell, no name narrows, nor does ▸ drop its from.
func TestAsTheLanesWidenNoNameNarrowsAndNamesComeFirst(t *testing.T) {
	for _, from := range []int{0, 8, 14, 22} {
		var before callColumns
		for width := 52; width < 100; width++ {
			k := laneColumnsOf(width, from)
			if err := keepsItsDistances(boardColumns{callColumns: k}, from, width); err != nil {
				t.Errorf("with %d, at %d: %v", from, width, err)
			}
			fromCells, accountCut := fromColumnCells(k, from), k.account < from
			if accountCut && !k.fromShort || k.dir < views.SessionDirMost && k.account > min(from, leastAccount) {
				t.Errorf("with %d, at %d the names are %d and %d, ▸ short %t: want ▸'s from, then the account, then the directory to give", from, width, k.dir, k.account, k.fromShort)
			}
			if end := k.from + fromCells; (accountCut || k.dir < views.SessionDirMost) && max(end, k.doing+callDoing) != width {
				t.Errorf("with %d, at %d a name is cut with the row ending at %d", from, width, max(end, k.doing+callDoing))
			}
			if width > 52 && (k.dir < before.dir || k.account < before.account || k.fromShort && !before.fromShort) {
				t.Errorf("with %d, at %d the names are %d and %d, ▸ short %t; a cell narrower, %d and %d, ▸ short %t", from, width, k.dir, k.account, k.fromShort, before.dir, before.account, before.fromShort)
			}
			before = k
		}
	}
}

// Wherever the board draws cords, every bent cord turns down inside the
// bends' room, after the plugs and no further right than its last column, 3
// cells before the jacks, the first there, each on a column of its own: at
// every width from 100 to 300, of names of 8, 14 and 22 cells, with no call
// moved and with one, and 0 to 40 bent cords. So the steady room that
// decides they fit is never wider than the room laid, whichever way the from
// column's arrival moves it.
func TestEveryBentCordTurnsDownInsideTheRoomOnAColumnOfItsOwn(t *testing.T) {
	for _, labels := range []int{8, 14, 22} {
		for _, moved := range []bool{false, true} {
			for width := 100; width <= 300; width++ {
				for n := range 41 {
					l := bentIn(width, n, boardLines{accounts: []string{"a", "b"}, windows: 3, labels: labels, moved: moved})
					if l.lanes {
						continue
					}
					bends, k := l.bends(), l.columns
					seen := make(map[int]bool, len(bends))
					for _, bend := range bends {
						if bend <= k.plug || bend > k.plug+k.room || seen[bend] {
							t.Fatalf("names of %d, moved %t, at %d with %d bent: the cords bend at %v, want each on its own column in (%d, %d]", labels, moved, width, n, bends, k.plug, k.plug+k.room)
						}
						seen[bend] = true
					}
					if n > 0 && (bends[0] != k.jacks-bendClear || k.firstBend() != k.jacks-bendClear) {
						t.Fatalf("names of %d, moved %t, at %d with %d bent: the first bends at %d, the room ends at %d, want both at the jacks less 3, %d", labels, moved, width, n, bends[0], k.firstBend(), k.jacks-bendClear)
					}
				}
			}
		}
	}
}

// The first bent cord turns down 3 cells before the jacks, at every width the
// board draws, and the rest 3 apart where the room holds them: of the long
// names' world, a call moved, the room 18 columns at 108, its 6 bent cords
// at 61, 58, 55, 52, 49 and 46, 3 apart from 3 before the jacks at 64.
func TestTheFirstBentCordTurnsDownThreeCellsBeforeTheJacks(t *testing.T) {
	lines := boardLines{accounts: []string{"work", "personal", "side"}, windows: 2, labels: 22, moved: true}
	tests := []struct {
		width, room, jacks int
	}{
		{width: 100, room: 18, jacks: 56},
		{width: 108, room: 18, jacks: 64},
		{width: 138, room: 18, jacks: 90},
		{width: 160, room: 18, jacks: 95},
		{width: 220, room: 67, jacks: 144},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.width), func(t *testing.T) {
			l := sessionsLaid(tt.width, 100, patchedBy(sceneAt1442), lines)
			want := make([]int, 6)
			for i := range want {
				want[i] = tt.jacks - bendClear - i*bendsApart
			}
			if got := l.bends(); l.lanes || l.columns.room != tt.room || l.columns.jacks != tt.jacks || !slices.Equal(got, want) {
				t.Errorf("lanes %t, the room %d, the jacks at %d, the cords bending at %v; want cords, the room %d, the jacks at %d, bending at %v", l.lanes, l.columns.room, l.columns.jacks, got, tt.room, tt.jacks, want)
			}
		})
	}
}

// The steady room never narrows as the width grows, from 100 to 300, of
// names of 8, 14 and 22 cells, and is never wider than the room laid with the
// from column or without it: it's the widest room that never narrows, the
// least of the rooms laid at its width and at every width past it. Short of
// every part's natural, it's the room's least, 18 columns, as at 108, 138
// and 150 with names of 8 cells; past them, as at 160, it grows a column a
// column.
func TestTheSteadyRoomNeverNarrowsAsTheWidthGrows(t *testing.T) {
	const widest = 300
	for _, labels := range []int{8, 14, 22} {
		laid := make([]int, widest+1)
		for width := 100; width <= widest; width++ {
			without, _ := boardColumnsOf(width, fromOf(labels, false))
			with, _ := boardColumnsOf(width, fromOf(labels, true))
			laid[width] = min(without.room, with.room)
		}
		before := 0
		for width := 100; width <= widest; width++ {
			steady := steadyRoom(width, labels)
			if least := slices.Min(laid[width:]); steady != least {
				t.Errorf("names of %d at %d: the steady room is %d, want %d, the least laid from there on", labels, width, steady, least)
			}
			if steady < before {
				t.Errorf("names of %d at %d: the steady room is %d, a column narrower %d", labels, width, steady, before)
			}
			before = steady
		}
	}
	for width, want := range map[int]int{108: 18, 138: 18, 150: 18, 160: 21} {
		if got := steadyRoom(width, 8); got != want {
			t.Errorf("names of 8 at %d: the steady room is %d, want %d", width, got, want)
		}
	}
}

// As the width grows from 100 to 300, the board's form changes once at the
// most, from lanes to cords, for any number of bent cords from 0 to 40, of
// names of 8, 14 and 22 cells, with no call moved and with one: the steady
// room never narrows, so cords never give way to lanes on a wider frame.
func TestTheBoardsFormChangesOnceAsTheWidthGrows(t *testing.T) {
	for _, labels := range []int{8, 14, 22} {
		for _, moved := range []bool{false, true} {
			for n := range 41 {
				lines := boardLines{accounts: []string{"a", "b"}, windows: 3, labels: labels, moved: moved}
				cords := false
				for width := 100; width <= 300; width++ {
					l := bentIn(width, n, lines)
					if cords && l.lanes {
						t.Fatalf("names of %d, moved %t, %d bent: lanes at %d after cords on a narrower frame", labels, moved, n, width)
					}
					cords = !l.lanes
				}
			}
		}
	}
}

// bentOn is a board of n sessions on the second of two accounts, each its
// cord bent, as the first account's line, of three windows, stands over
// them all: laid width wide in a view tall enough for step 1.
func bentOn(width, n int) boardLayout {
	return bentIn(width, n, boardLines{accounts: []string{"a", "b"}, windows: 3})
}

// bentIn is a board of n sessions on the second of the lines given, each
// its cord bent, as the first line stands over them all: laid width wide in
// a view tall enough for step 1.
func bentIn(width, n int, lines boardLines) boardLayout {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("s%02d", i)
	}
	p := patchedBy(sat{on: []string{lines.accounts[1] + ": " + strings.Join(ids, " ")}})
	return sessionsLaid(width, 100, p, lines)
}

// bends are the columns the layout's bent cords turn down at, top to bottom.
func (l boardLayout) bends() []int {
	var bends []int
	for _, c := range l.calls {
		if c.bent() {
			bends = append(bends, c.cord.bend)
		}
	}
	return bends
}

// Bent cords sit 3 apart where the bends' room holds them so, else 2, else
// 1, else the Switchboard draws lanes, the first bend 3 cells left of the
// jacks: with no call moved, the room is 18 columns at 108, 21 at 145, 36 at
// 160 and 76 at 200.
func TestBentCordsSitThreeTwoOrOneApartElseTheSwitchboardDrawsLanes(t *testing.T) {
	tests := []struct {
		width, bent int
		apart       int
	}{
		{width: 108, bent: 6, apart: 3},
		{width: 108, bent: 7, apart: 2},
		{width: 108, bent: 9, apart: 2},
		{width: 108, bent: 10, apart: 1},
		{width: 108, bent: 18, apart: 1},
		{width: 108, bent: 19},
		{width: 145, bent: 7, apart: 3},
		{width: 145, bent: 8, apart: 2},
		{width: 145, bent: 11, apart: 2},
		{width: 145, bent: 12, apart: 1},
		{width: 145, bent: 21, apart: 1},
		{width: 145, bent: 22},
		{width: 160, bent: 12, apart: 3},
		{width: 160, bent: 13, apart: 2},
		{width: 160, bent: 18, apart: 2},
		{width: 160, bent: 19, apart: 1},
		{width: 160, bent: 36, apart: 1},
		{width: 160, bent: 37},
		{width: 200, bent: 26, apart: 3},
		{width: 200, bent: 27, apart: 2},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d bent at %d", tt.bent, tt.width), func(t *testing.T) {
			l := bentOn(tt.width, tt.bent)
			if tt.apart == 0 {
				if !l.lanes {
					t.Errorf("the cords bend at %v, want lanes", l.bends())
				}
				return
			}
			want := make([]int, tt.bent)
			for i := range want {
				want[i] = l.columns.jacks - bendClear - i*tt.apart
			}
			if got := l.bends(); l.lanes || !slices.Equal(got, want) {
				t.Errorf("lanes %t, the cords bend at %v, want cords bending at %v", l.lanes, got, want)
			}
			if last := want[len(want)-1]; last <= l.columns.plug {
				t.Errorf("the last cord bends at %d, want it in the bends' room, after the plugs at %d", last, l.columns.plug)
			}
		})
	}
}

// Bent cords turn down in the order of their jacks' rows, the first at the
// bends' room's last column, so none crosses another: s2, joining b's foot
// on the jack s0 left above s1's, takes the jack below s1's, s1 moving up to
// the first, as a line's calls hold its jacks in their order.
func TestBentCordsAreOrderedByTheirJacksRowsThenTheirCalls(t *testing.T) {
	p := patchedBy(
		sat{on: []string{"b: s0 s1"}},
		sat{on: []string{"b: s1", "c: s0"}},
		sat{on: []string{"b: s1 s2", "c: s0"}},
	)
	l := sessionsLaid(160, 100, p, boardLines{accounts: []string{"a", "b", "c"}, windows: 3})
	want := []string{"s1 6→15 @81", "s2 7→16 @78", "s0 9→24 @75"}
	if got := l.cordsOf(); !slices.Equal(got, want) {
		t.Errorf("s2 at b's foot, the cords are %q, want %q", got, want)
	}
}

// The key ends on the row over the key line, two blank rows after the lines
// at the least, so at 160 by 37, steps 1 to 4 hold 3, 5, 13 and 26
// accounts.
func TestAtOneHundredAndSixtyBy37StepsOneToFourHoldThreeFiveThirteenAndTwentySixAccounts(t *testing.T) {
	tests := []struct{ accounts, step int }{
		{accounts: 3, step: 1}, {accounts: 4, step: 2},
		{accounts: 5, step: 2}, {accounts: 6, step: 3},
		{accounts: 13, step: 3}, {accounts: 14, step: 4},
		{accounts: 26, step: 4}, {accounts: 27, step: 5},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.accounts), func(t *testing.T) {
			l := sessionsLaid(160, viewRows(37), views.Patched{}, boardLines{accounts: accountsNamed(tt.accounts), windows: 3})
			if l.step != tt.step {
				t.Errorf("%d accounts take step %d, want %d", tt.accounts, l.step, tt.step)
			}
			switch {
			case tt.step < compactedStep && 6+l.keyAt != 37-3:
				t.Errorf("the key starts on row %d, want it on 34, ending over the key line", 6+l.keyAt)
			case tt.step < compactedStep && l.keyAt < l.rows+boardKeyGap:
				t.Errorf("the key starts on row %d of the view, want two blank rows after the lines at the least, from %d", l.keyAt, l.rows+boardKeyGap)
			case tt.step == compactedStep && l.keyAt >= 0:
				t.Errorf("at step 5, the key starts on row %d of the view, want it giving way", l.keyAt)
			}
		})
	}
}

// The wide frames' scene: personal's limit moved none yet, as at 14:42:07;
// 41e0 and 9c2e on personal, c61b, d28c, 7f3a and db8a on side.
var sceneAt1442 = sat{on: []string{"personal: 41e0 9c2e", "side: c61b d28c 7f3a db8a"}}

// tenAccounts are the ten accounts sessions-ten-accounts-160 draws, with its
// sessions on them.
var (
	tenLines = boardLines{
		accounts: []string{"work", "personal", "side", "client", "spare", "lab", "team", "extra", "ops", "studio"}, windows: 2,
	}
	tenAt1442 = sat{on: []string{
		"personal: 41e0 9c2e", "side: c61b d28c 7f3a db8a", "client: 1f9a 6c2e 0d4b", "lab: b7e5 93fa", "extra: 4ae8", "studio: c0d1 f3b1",
	}}
)

// cordsOf are the layout's calls, as "41e0 6→14 @72", the rows absolute, as
// a frame from row 6 draws them: each session, its row, its jack's, and
// where it bends, if it does.
func (l boardLayout) cordsOf() []string {
	var cords []string
	for _, c := range l.calls {
		k := c.cord
		said := fmt.Sprintf("%s %d→%d", c.session, 6+k.from, 6+k.to)
		if c.bent() {
			said += fmt.Sprintf(" @%d", k.bend)
		}
		cords = append(cords, said)
	}
	return cords
}

// linesOf are the layout's lines, as "personal 14–19", the rows absolute,
// as a frame from row 6 draws them, with the rows of the stubs hanging from
// each, as "╌14".
func (l boardLayout) linesOf() []string {
	var lines []string
	for _, ln := range l.lines {
		said := fmt.Sprintf("%s %d–%d", ln.account, 6+ln.top, 6+ln.top+ln.rows-1)
		for _, s := range ln.stubs {
			said += fmt.Sprintf(" ╌%d", 6+s.row)
		}
		lines = append(lines, said)
	}
	return lines
}

func TestTheFramesWorldsAreLaidOutAsTheFramesDrawThem(t *testing.T) {
	tests := []struct {
		name        string
		height      int
		patched     views.Patched
		lines       boardLines
		step, keyAt int
		wantLines   []string
		wantCords   []string
	}{
		{
			name: "sessions-switchboard-160", height: 37, patched: patchedBy(sceneAt1442), lines: threeLines, step: 1, keyAt: 34,
			wantLines: []string{"work 6–11", "personal 15–20", "side 24–29"},
			wantCords: []string{"41e0 6→15 @81", "9c2e 7→16 @78", "c61b 9→24 @75", "d28c 10→25 @72", "7f3a 11→26 @69", "db8a 12→27 @66"},
		},
		{
			name:   "sessions-moved-160: 41e0 at side's foot, its stub on personal's first jack, 9c2e kept on its second",
			height: 37, lines: threeLines, step: 1, keyAt: 34,
			patched:   patchedBy(sceneAt1442, sat{on: []string{"personal: 9c2e", "side: c61b d28c 7f3a db8a 41e0"}, stubs: map[string]int{"personal": 1}}),
			wantLines: []string{"work 6–11", "personal 15–20 ╌15", "side 24–29"},
			wantCords: []string{"9c2e 6→16 @81", "c61b 8→24 @78", "d28c 9→25 @75", "7f3a 10→26 @72", "db8a 11→27 @69", "41e0 12→28 @66"},
		},
		{
			name: "sessions-pinned-160: all six on side, two stubs on personal", height: 37, lines: threeLines, step: 1, keyAt: 34,
			patched: patchedBy(
				sat{on: []string{"side: c61b d28c 7f3a db8a"}},
				sat{on: []string{"side: c61b d28c 7f3a db8a 41e0 9c2e"}, stubs: map[string]int{"personal": 2}},
			),
			wantLines: []string{"work 6–11", "personal 15–20 ╌15 ╌16", "side 24–29"},
			wantCords: []string{
				"c61b 6→24 @81", "d28c 7→25 @78", "7f3a 8→26 @75", "db8a 9→27 @72", "41e0 10→28 @69", "9c2e 11→29 @66",
			},
		},
		{
			name: "sessions-one-account-160: a jack a call, straight", height: 37, step: 1, keyAt: 34,
			patched:   patchedBy(sat{on: []string{"work: 41e0 9c2e c61b d28c 7f3a"}}),
			lines:     boardLines{accounts: []string{"work"}, windows: 3},
			wantLines: []string{"work 6–11"},
			wantCords: []string{"41e0 6→6", "9c2e 7→7", "c61b 8→8", "d28c 9→9", "7f3a 10→10"},
		},
		{
			name: "sessions-ten-accounts-160: step 3, side growing two rows and client one", height: 37, step: 3, keyAt: 34,
			patched: patchedBy(tenAt1442), lines: tenLines,
			wantLines: []string{
				"work 6–7", "personal 8–9", "side 10–13", "client 14–16", "spare 17–18", "lab 19–20", "team 21–22", "extra 23–24",
				"ops 25–26", "studio 27–28",
			},
			wantCords: []string{
				"41e0 6→8 @81", "9c2e 7→9 @78", "c61b 9→10 @75", "d28c 10→11 @72", "7f3a 11→12 @69", "db8a 12→13 @66",
				"1f9a 14→14", "6c2e 15→15", "0d4b 16→16", "b7e5 18→19 @63", "93fa 19→20 @60", "4ae8 21→23 @57", "c0d1 23→27 @54",
				"f3b1 24→28 @51",
			},
		},
		{
			name: "three accounts at 160 by 29: step 2", height: 29, patched: patchedBy(sceneAt1442), lines: threeLines, step: 2, keyAt: 26,
			wantLines: []string{"work 6–9", "personal 13–16", "side 20–23"},
			wantCords: []string{"41e0 6→13 @81", "9c2e 7→14 @78", "c61b 9→20 @75", "d28c 10→21 @72", "7f3a 11→22 @69", "db8a 12→23 @66"},
		},
		{
			name: "three accounts at 160 by 24: step 3, side growing two rows", height: 24, patched: patchedBy(sceneAt1442), lines: threeLines,
			step: 3, keyAt: 21,
			wantLines: []string{"work 6–7", "personal 10–11", "side 14–17"},
			wantCords: []string{"41e0 6→10 @81", "9c2e 7→11 @78", "c61b 9→14 @75", "d28c 10→15 @72", "7f3a 11→16 @69", "db8a 12→17 @66"},
		},
		{
			name:   "ten accounts at 160 by 29: step 5, scrolling, the key giving way, each group starting below the line above",
			height: 29, patched: patchedBy(tenAt1442), lines: tenLines, step: 5, keyAt: -1,
			wantLines: []string{
				"work 6–6", "personal 7–8", "side 10–13", "client 15–17", "spare 18–18", "lab 19–20", "team 21–21", "extra 22–22",
				"ops 23–23", "studio 24–25",
			},
			wantCords: []string{
				"41e0 7→7", "9c2e 8→8", "c61b 10→10", "d28c 11→11", "7f3a 12→12", "db8a 13→13", "1f9a 15→15", "6c2e 16→16",
				"0d4b 17→17", "b7e5 19→19", "93fa 20→20", "4ae8 22→22", "c0d1 24→24", "f3b1 25→25",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := sessionsLaid(160, viewRows(tt.height), tt.patched, tt.lines)
			keyAt := -1
			if l.keyAt >= 0 {
				keyAt = 6 + l.keyAt
			}
			if l.lanes || l.step != tt.step || keyAt != tt.keyAt {
				t.Errorf("lanes %t, step %d, the key from row %d; want cords, step %d, the key from row %d", l.lanes, l.step, keyAt, tt.step, tt.keyAt)
			}
			if got := l.linesOf(); !slices.Equal(got, tt.wantLines) {
				t.Errorf("the lines are\n%q\nwant\n%q", got, tt.wantLines)
			}
			if got := l.cordsOf(); !slices.Equal(got, tt.wantCords) {
				t.Errorf("the cords are\n%q\nwant\n%q", got, tt.wantCords)
			}
		})
	}
}

// At step 5, the lines take the rows they can: where the board's rows pass
// the view's, the blank row after each group of calls gives way. At 138 by
// 15, the scene's three accounts all show, where with the blank row the
// third was out of view over 4 blank rows; by 17, where they show with it,
// it stays. Twelve accounts of a call each at 160 by 24 all show, the key
// under them where it still fits, the board whole.
func TestAtStep5TheLinesTakeTheRowsTheyCan(t *testing.T) {
	twelve := make([]string, 12)
	for i, a := range accountsNamed(12) {
		twelve[i] = fmt.Sprintf("%s: s%d", a, i)
	}
	tests := []struct {
		name          string
		width, height int
		patched       views.Patched
		lines         boardLines
		keyAt         int
		want          []string
	}{
		{
			name: "three at 138 by 15", width: 138, height: 15, patched: patchedBy(sceneAt1442), keyAt: -1,
			lines: boardLines{accounts: []string{"work", "personal", "side"}, windows: 2},
			want:  []string{"work 6–6", "personal 7–8", "side 9–12"},
		},
		{
			name: "three at 138 by 17", width: 138, height: 17, patched: patchedBy(sceneAt1442), keyAt: -1,
			lines: boardLines{accounts: []string{"work", "personal", "side"}, windows: 2},
			want:  []string{"work 6–6", "personal 7–8", "side 10–13"},
		},
		{
			name: "twelve at 160 by 24", width: 160, height: 24, patched: patchedBy(sat{on: twelve}), keyAt: 21,
			lines: boardLines{accounts: accountsNamed(12), windows: 2},
			want: []string{
				"a0 6–6", "a1 7–7", "a2 8–8", "a3 9–9", "a4 10–10", "a5 11–11", "a6 12–12", "a7 13–13", "a8 14–14", "a9 15–15",
				"a10 16–16", "a11 17–17",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := sessionsLaid(tt.width, viewRows(tt.height), tt.patched, tt.lines)
			keyAt := -1
			if l.keyAt >= 0 {
				keyAt = 6 + l.keyAt
			}
			if l.step != compactedStep || l.rows > viewRows(tt.height) || keyAt != tt.keyAt {
				t.Errorf("step %d, %d rows in a view of %d, the key from row %d; want step 5, every account in view, the key from %d", l.step, l.rows, viewRows(tt.height), keyAt, tt.keyAt)
			}
			if got := l.linesOf(); !slices.Equal(got, tt.want) {
				t.Errorf("the lines are %q, want %q", got, tt.want)
			}
		})
	}
}

// Each world in a view of step 3's rows and its key's, no row spare, so its
// lines are no further apart than the step's gap and their groups move them.
func TestALineMovesDownToMeetItsGroupSoEveryCordRunsRightThenDown(t *testing.T) {
	lines := boardLines{accounts: []string{"work", "personal"}, windows: 3}
	tests := []struct {
		name    string
		patched views.Patched
		view    int
		want    []string
		first   []int
	}{
		{
			name:    "a group starting below its line's first jack",
			patched: patchedBy(sat{on: []string{"work: s1 s2 s3", "personal: s4"}}),
			view:    9,
			want:    []string{"s1 6→6", "s2 7→7", "s3 8→8", "s4 10→10"},
			first:   []int{0, 4},
		},
		{
			name: "a call joining its group's foot on a jack above the one before's, the jacks kept in the calls' order",
			patched: patchedBy(
				sat{on: []string{"work: s1", "personal: s2 s3"}},
				sat{on: []string{"work: s1 s2", "personal: s3"}},
				sat{on: []string{"work: s1 s2", "personal: s3 s4"}},
			),
			view:  8,
			want:  []string{"s1 6→6", "s2 7→7", "s3 9→9", "s4 10→10"},
			first: []int{0, 3},
		},
		{
			name: "a line whose first call keeps a jack past its first, starting over its group",
			patched: patchedBy(
				sat{on: []string{"work: s1", "personal: s2 s3"}},
				sat{on: []string{"work: s1 s2", "personal: s3"}},
			),
			view:  7,
			want:  []string{"s1 6→6", "s2 7→7", "s3 9→9"},
			first: []int{0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := sessionsLaid(160, tt.view, tt.patched, lines)
			if l.step != 3 {
				t.Fatalf("the board takes step %d, want step 3, its lines two rows each", l.step)
			}
			if got := l.cordsOf(); !slices.Equal(got, tt.want) {
				t.Errorf("the cords are %q, want %q", got, tt.want)
			}
			for _, c := range l.calls {
				if c.cord.to < c.cord.from {
					t.Errorf("%s's cord runs up, from row %d to %d", c.session, c.cord.from, c.cord.to)
				}
			}
			var first []int
			for _, ln := range l.lines {
				first = append(first, ln.first)
			}
			if !slices.Equal(first, tt.first) {
				t.Errorf("the accounts' first rows are %v, want %v", first, tt.first)
			}
		})
	}
}

func TestALineGrowsARowForEachJackPastItsOwn(t *testing.T) {
	ids := []string{"s1", "s2", "s3", "s4", "s5", "s6", "s7", "s8"}
	l := sessionsLaid(160, viewRows(37), patchedBy(sat{on: []string{"work: " + strings.Join(ids, " ")}}), boardLines{accounts: []string{"work"}, windows: 3})
	if got := l.linesOf(); !slices.Equal(got, []string{"work 6–13"}) {
		t.Errorf("one account with 8 sessions, its line is %q, want it grown two rows past its six, 6–13", got)
	}
	if row, ok := l.jackRow("work", 7); !ok || row != 7 {
		t.Errorf("its eighth jack is on row %d of the view, %t; want 7", row, ok)
	}
	if _, ok := l.jackRow("side", 0); ok {
		t.Error("a line the board has none of has a jack")
	}
}

// A limit hangs a stub from a jack of its line for each session it moved,
// each 2 cells, as the next tests draw them, the line growing a row for each
// jack past its own.
func TestAStubHangsForEachSessionTheLimitMoved(t *testing.T) {
	l := sessionsLaid(160, viewRows(37), patchedBy(sat{stubs: map[string]int{"personal": 9}}), threeLines)
	ln, _ := l.line("personal")
	if len(ln.stubs) != 9 || ln.rows != 9 {
		t.Errorf("nine sessions moved, personal hangs %d stubs over %d rows, want 9 over 9, a jack for each stub", len(ln.stubs), ln.rows)
	}
}
