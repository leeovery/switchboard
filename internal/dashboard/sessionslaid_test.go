package dashboard

import (
	"fmt"
	"slices"
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

func TestTheSwitchboardsColumnsAreTheNarrowingTables(t *testing.T) {
	tests := []struct {
		width                          int
		doing, from, plug, room, jacks int
		lines, bar, words              int
		fromShort, shortWords, lanes   bool
		dir, account                   int
	}{
		{width: 200, doing: 18, from: 31, plug: 48, room: 64, jacks: 124, lines: 126, bar: 32, words: 22, dir: 12, account: 8},
		{width: 160, doing: 18, from: 31, plug: 48, room: 24, jacks: 84, lines: 86, bar: 32, words: 22, dir: 12, account: 8},
		{width: 145, doing: 18, from: 31, plug: 48, room: 9, jacks: 69, lines: 71, bar: 32, words: 22, dir: 12, account: 8},
		{width: 129, doing: 18, from: 31, plug: 48, room: 9, jacks: 69, lines: 71, bar: 16, words: 22, dir: 12, account: 8},
		{width: 123, doing: 12, from: 25, plug: 42, room: 9, jacks: 63, lines: 65, bar: 16, words: 22, dir: 6, account: 8},
		{width: 122, doing: 12, from: 25, plug: 37, room: 13, jacks: 62, lines: 64, bar: 16, words: 22, dir: 6, account: 8, fromShort: true},
		{width: 118, doing: 12, from: 25, plug: 37, room: 9, jacks: 58, lines: 60, bar: 16, words: 22, dir: 6, account: 8, fromShort: true},
		{width: 117, doing: 12, from: 25, plug: 37, room: 16, jacks: 65, lines: 67, bar: 16, words: 14, dir: 6, account: 8, fromShort: true, shortWords: true},
		{width: 110, doing: 12, from: 25, plug: 37, room: 9, jacks: 58, lines: 60, bar: 16, words: 14, dir: 6, account: 8, fromShort: true, shortWords: true},
		{width: 102, doing: 12, from: 25, plug: 37, room: 9, jacks: 58, lines: 60, bar: 8, words: 14, dir: 6, account: 8, fromShort: true, shortWords: true},
		{width: 100, doing: 12, from: 25, plug: 35, room: 9, jacks: 56, lines: 58, bar: 8, words: 14, dir: 6, account: 6, fromShort: true, shortWords: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			l := sessionsLaid(tt.width, viewRows(37), views.Patched{}, threeLines)
			k := l.columns
			got := []int{k.doing, k.from, k.plug, k.room, k.jacks, k.lines, k.bar, k.words, k.dir, k.account}
			want := []int{tt.doing, tt.from, tt.plug, tt.room, tt.jacks, tt.lines, tt.bar, tt.words, tt.dir, tt.account}
			if !slices.Equal(got, want) {
				t.Errorf("doing, ▸, plug, room, jacks, lines, bar, words, dir, account are %v, want %v", got, want)
			}
			if k.fromShort != tt.fromShort || k.shortWords != tt.shortWords || l.lanes {
				t.Errorf("▸ short %t, words short %t, lanes %t; want %t, %t, cords", k.fromShort, k.shortWords, l.lanes, tt.fromShort, tt.shortWords)
			}
		})
	}
}

// narrowingTable is what the design's narrowing table gives at each width
// from 100 to 160: from 160's columns, the part each row names giving a cell
// for each column lost, and a step that frees several cells at once giving
// what it frees beyond what's lacking to the bends' room.
func narrowingTable(width int) boardColumns {
	k := boardColumns{dir: 12, account: 8, bar: 32, words: 22, room: width - 136}
	switch {
	case width >= 145:
	case width >= 129:
		k.room, k.bar = 9, 32-(145-width)
	case width >= 123:
		k.room, k.bar, k.dir = 9, 16, 12-(129-width)
	case width >= 118:
		k.bar, k.dir, k.fromShort, k.room = 16, 6, true, 13-(122-width)
	case width >= 110:
		k.bar, k.dir, k.fromShort, k.shortWords, k.words, k.room = 16, 6, true, true, 14, 16-(117-width)
	case width >= 102:
		k.dir, k.fromShort, k.shortWords, k.words, k.room, k.bar = 6, true, true, 14, 9, 16-(110-width)
	default:
		k.dir, k.fromShort, k.shortWords, k.words, k.room, k.bar, k.account = 6, true, true, 14, 9, 8, 8-(102-width)
	}
	return k
}

func TestEveryWidthFrom100To200NarrowsAsTheDesignsTableHasIt(t *testing.T) {
	for width := 100; width <= 200; width++ {
		k := sessionsLaid(width, viewRows(37), views.Patched{}, threeLines).columns
		want := narrowingTable(width)
		got := boardColumns{dir: k.dir, account: k.account, bar: k.bar, words: k.words, room: k.room, fromShort: k.fromShort, shortWords: k.shortWords}
		if got != want {
			t.Errorf("at %d, the parts are %+v, want %+v", width, got, want)
		}
		fromCells := 7 + k.account
		if k.fromShort {
			fromCells = 2 + k.account
		}
		switch {
		case k.doing != 5+k.dir+1, k.from != k.doing+13, k.plug != k.from+fromCells+2:
			t.Errorf("at %d, doing at %d, ▸ at %d, the plugs at %d, want each its distance from the part before it", width, k.doing, k.from, k.plug)
		case k.jacks != k.plug+k.room+12, k.lines != k.jacks+2:
			t.Errorf("at %d, the jacks at %d and the lines at %d, want the jacks 12 past the bends' room, the lines 2 past them", width, k.jacks, k.lines)
		case k.lines+1+11+k.bar+1+4+2+k.words+1 != width:
			t.Errorf("at %d, the lines from %d, bars %d and words %d, want them ending at the last column", width, k.lines, k.bar, k.words)
		}
	}
}

func TestUnder100ColumnsTheSwitchboardDrawsLanes(t *testing.T) {
	for _, width := range []int{99, 80, 52} {
		if l := sessionsLaid(width, viewRows(37), patchedBy(sat{on: []string{"side: c61b"}}), threeLines); !l.lanes {
			t.Errorf("at %d, the Switchboard draws cords, want lanes", width)
		}
	}
}

// bentOn is a board of n sessions on the second of two accounts, each its
// cord bent, as the first account's line, of three windows, stands over
// them all: laid width wide in a view tall enough for step 1.
func bentOn(width, n int) boardLayout {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("s%02d", i)
	}
	p := patchedBy(sat{on: []string{"b: " + strings.Join(ids, " ")}})
	return sessionsLaid(width, 100, p, boardLines{accounts: []string{"a", "b"}, windows: 3})
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

func TestBentCordsSitThreeTwoOrOneApartElseTheSwitchboardDrawsLanes(t *testing.T) {
	tests := []struct {
		width, bent int
		apart       int
	}{
		{width: 160, bent: 8, apart: 3},
		{width: 160, bent: 9, apart: 2},
		{width: 160, bent: 12, apart: 2},
		{width: 160, bent: 13, apart: 1},
		{width: 160, bent: 24, apart: 1},
		{width: 160, bent: 25},
		{width: 145, bent: 3, apart: 3},
		{width: 145, bent: 5, apart: 2},
		{width: 145, bent: 9, apart: 1},
		{width: 145, bent: 10},
		{width: 200, bent: 13, apart: 3},
		{width: 200, bent: 22, apart: 3},
		{width: 200, bent: 23, apart: 2},
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
				want[i] = l.columns.jacks - 12 - i*tt.apart
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

func TestBentCordsAreOrderedByTheirJacksRowsThenTheirCalls(t *testing.T) {
	p := patchedBy(
		sat{on: []string{"b: s0 s1"}},
		sat{on: []string{"b: s1", "c: s0"}},
		sat{on: []string{"b: s1 s2", "c: s0"}},
	)
	l := sessionsLaid(160, 100, p, boardLines{accounts: []string{"a", "b", "c"}, windows: 3})
	want := []string{"s1 6→15 @69", "s2 7→14 @72", "s0 9→22 @66"}
	if got := l.cordsOf(); !slices.Equal(got, want) {
		t.Errorf("s2 at b's foot on its first jack, the cords are %q, want %q", got, want)
	}
}

func TestAtOneHundredAndSixtyBy37StepsOneToFourHoldThreeFiveTwelveAndTwentyFiveAccounts(t *testing.T) {
	tests := []struct{ accounts, step int }{
		{accounts: 3, step: 1}, {accounts: 4, step: 2},
		{accounts: 5, step: 2}, {accounts: 6, step: 3},
		{accounts: 12, step: 3}, {accounts: 13, step: 4},
		{accounts: 25, step: 4}, {accounts: 26, step: 5},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.accounts), func(t *testing.T) {
			l := sessionsLaid(160, viewRows(37), views.Patched{}, boardLines{accounts: accountsNamed(tt.accounts), windows: 3})
			if l.step != tt.step {
				t.Errorf("%d accounts take step %d, want %d", tt.accounts, l.step, tt.step)
			}
			switch {
			case tt.step < compactedStep && 6+l.keyAt+boardKeyRows-1 > 37-3:
				t.Errorf("the key ends on row %d, want it over the row over the key line, 34", 6+l.keyAt+boardKeyRows-1)
			case tt.step < compactedStep && l.keyAt != l.rows+2:
				t.Errorf("the key starts on row %d of the view, want two blank rows after the lines, %d", l.keyAt, l.rows+2)
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
// as a frame from row 6 draws them, with the stubs hanging from each, as
// "╌14×7".
func (l boardLayout) linesOf() []string {
	var lines []string
	for _, ln := range l.lines {
		said := fmt.Sprintf("%s %d–%d", ln.account, 6+ln.top, 6+ln.top+ln.rows-1)
		for _, s := range ln.stubs {
			said += fmt.Sprintf(" ╌%d×%d", 6+s.row, s.cells)
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
			name: "sessions-switchboard-160", height: 37, patched: patchedBy(sceneAt1442), lines: threeLines, step: 1, keyAt: 30,
			wantLines: []string{"work 6–11", "personal 14–19", "side 22–27"},
			wantCords: []string{"41e0 6→14 @72", "9c2e 7→15 @69", "c61b 9→22 @66", "d28c 10→23 @63", "7f3a 11→24 @60", "db8a 12→25 @57"},
		},
		{
			name:   "sessions-moved-160: 41e0 at side's foot, its stub on personal's first jack, 9c2e kept on its second",
			height: 37, lines: threeLines, step: 1, keyAt: 30,
			patched:   patchedBy(sceneAt1442, sat{on: []string{"personal: 9c2e", "side: c61b d28c 7f3a db8a 41e0"}, stubs: map[string]int{"personal": 1}}),
			wantLines: []string{"work 6–11", "personal 14–19 ╌14×7", "side 22–27"},
			wantCords: []string{"9c2e 6→15 @72", "c61b 8→22 @69", "d28c 9→23 @66", "7f3a 10→24 @63", "db8a 11→25 @60", "41e0 12→26 @57"},
		},
		{
			name: "sessions-pinned-160: all six on side, two stubs on personal", height: 37, lines: threeLines, step: 1, keyAt: 30,
			patched: patchedBy(
				sat{on: []string{"side: c61b d28c 7f3a db8a"}},
				sat{on: []string{"side: c61b d28c 7f3a db8a 41e0 9c2e"}, stubs: map[string]int{"personal": 2}},
			),
			wantLines: []string{"work 6–11", "personal 14–19 ╌14×7 ╌15×6", "side 22–27"},
			wantCords: []string{
				"c61b 6→22 @72", "d28c 7→23 @69", "7f3a 8→24 @66", "db8a 9→25 @63", "41e0 10→26 @60", "9c2e 11→27 @57",
			},
		},
		{
			name: "sessions-one-account-160: a jack a call, straight", height: 37, step: 1, keyAt: 14,
			patched:   patchedBy(sat{on: []string{"work: 41e0 9c2e c61b d28c 7f3a"}}),
			lines:     boardLines{accounts: []string{"work"}, windows: 3},
			wantLines: []string{"work 6–11"},
			wantCords: []string{"41e0 6→6", "9c2e 7→7", "c61b 8→8", "d28c 9→9", "7f3a 10→10"},
		},
		{
			name: "sessions-ten-accounts-160: step 3, side growing two rows and client one", height: 37, step: 3, keyAt: 31,
			patched: patchedBy(tenAt1442), lines: tenLines,
			wantLines: []string{
				"work 6–7", "personal 8–9", "side 10–13", "client 14–16", "spare 17–18", "lab 19–20", "team 21–22", "extra 23–24",
				"ops 25–26", "studio 27–28",
			},
			wantCords: []string{
				"41e0 6→8 @72", "9c2e 7→9 @70", "c61b 9→10 @68", "d28c 10→11 @66", "7f3a 11→12 @64", "db8a 12→13 @62",
				"1f9a 14→14", "6c2e 15→15", "0d4b 16→16", "b7e5 18→19 @60", "93fa 19→20 @58", "4ae8 21→23 @56", "c0d1 23→27 @54",
				"f3b1 24→28 @52",
			},
		},
		{
			name: "three accounts at 160 by 29: step 2", height: 29, patched: patchedBy(sceneAt1442), lines: threeLines, step: 2, keyAt: 22,
			wantLines: []string{"work 6–9", "personal 11–14", "side 16–19"},
			wantCords: []string{"41e0 6→11 @72", "9c2e 7→12 @69", "c61b 9→16 @66", "d28c 10→17 @63", "7f3a 11→18 @60", "db8a 12→19 @57"},
		},
		{
			name: "three accounts at 160 by 24: step 3, side growing two rows", height: 24, patched: patchedBy(sceneAt1442), lines: threeLines,
			step: 3, keyAt: 16,
			wantLines: []string{"work 6–7", "personal 8–9", "side 10–13"},
			wantCords: []string{"41e0 6→8 @72", "9c2e 7→9 @69", "c61b 9→10 @66", "d28c 10→11 @63", "7f3a 11→12 @60", "db8a 12→13 @57"},
		},
		{
			name: "ten accounts at 160 by 29: step 5, scrolling, the key giving way", height: 29, patched: patchedBy(tenAt1442), lines: tenLines,
			step: 5, keyAt: -1,
			wantLines: []string{
				"work 6–6", "personal 7–8", "side 9–12", "client 14–16", "spare 17–17", "lab 18–19", "team 20–20", "extra 21–21",
				"ops 22–22", "studio 23–24",
			},
			wantCords: []string{
				"41e0 6→7 @72", "9c2e 7→8 @69", "c61b 9→9", "d28c 10→10", "7f3a 11→11", "db8a 12→12", "1f9a 14→14", "6c2e 15→15",
				"0d4b 16→16", "b7e5 18→18", "93fa 19→19", "4ae8 21→21", "c0d1 23→23", "f3b1 24→24",
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

func TestALineMovesDownToMeetItsGroupSoEveryCordRunsRightThenDown(t *testing.T) {
	lines := boardLines{accounts: []string{"work", "personal"}, windows: 3}
	tests := []struct {
		name    string
		patched views.Patched
		want    []string
		first   []int
	}{
		{
			name:    "a group starting below its line's first jack",
			patched: patchedBy(sat{on: []string{"work: s1 s2 s3", "personal: s4"}}),
			want:    []string{"s1 6→6", "s2 7→7", "s3 8→8", "s4 10→10"},
			first:   []int{0, 4},
		},
		{
			name: "a call at its group's foot plugged into a jack above it",
			patched: patchedBy(
				sat{on: []string{"work: s1", "personal: s2 s3"}},
				sat{on: []string{"work: s1 s2", "personal: s3"}},
				sat{on: []string{"work: s1 s2", "personal: s3 s4"}},
			),
			want:  []string{"s1 6→6", "s2 7→7", "s3 9→11 @72", "s4 10→10"},
			first: []int{0, 3},
		},
		{
			name: "a line whose first call keeps a jack past its first, starting over its group",
			patched: patchedBy(
				sat{on: []string{"work: s1", "personal: s2 s3"}},
				sat{on: []string{"work: s1 s2", "personal: s3"}},
			),
			want:  []string{"s1 6→6", "s2 7→7", "s3 9→9"},
			first: []int{0, 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := sessionsLaid(160, 10, tt.patched, lines)
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

func TestAStubHangsAShorterCordForEachSessionTheLimitMovedNeverUnderACell(t *testing.T) {
	l := sessionsLaid(160, viewRows(37), patchedBy(sat{stubs: map[string]int{"personal": 9}}), threeLines)
	ln, _ := l.line("personal")
	var cells []int
	for _, s := range ln.stubs {
		cells = append(cells, s.cells)
	}
	if want := []int{7, 6, 5, 4, 3, 2, 1, 1, 1}; !slices.Equal(cells, want) {
		t.Errorf("nine stubs are %v cells long, want %v", cells, want)
	}
	if ln.rows != 9 {
		t.Errorf("personal's line takes %d rows, want 9, a jack for each stub", ln.rows)
	}
}
