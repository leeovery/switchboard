package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
)

// A session's page heads itself in the wide chrome's form from 100 columns,
// a phone's under them, as the chrome does. Its story is whole where its
// fixed rows, one account and three turns fit, below 29 rows under the wide
// chrome folded to its heading, and under a phone's left out, its blank row
// kept; below that it's the chrome alone. Its accounts take a row each while
// three turns fit beside them, then give way to a row saying how many more;
// its turns, under their column heads at every width, take the rest; and an
// open turn shows as many of its requests as the turns' rows hold under it,
// five at the most and one at the least.
func TestASessionsPageLaysOutItsRowsAtEachSize(t *testing.T) {
	tests := []struct {
		name                 string
		width, height, ranOn int
		want                 sessionLayout
	}{
		{
			name: "160×37, one account: the frames", width: 160, height: 37, ranOn: 1,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, accounts: 1,
				turnsHeading: 21, turnsAt: 24, turns: 11, moreTurns: 35, requests: 5},
		},
		{
			name: "160×37, ten accounts: eight rows and two more", width: 160, height: 37, ranOn: 10,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, accounts: 8, more: 2,
				turnsHeading: 29, turnsAt: 32, turns: 3, moreTurns: 35, requests: 1},
		},
		{
			name: "160×29, one account: its least whole", width: 160, height: 29, ranOn: 1,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, accounts: 1,
				turnsHeading: 21, turnsAt: 24, turns: 3, moreTurns: 27, requests: 1},
		},
		{
			name: "160×29, two accounts: the row of how many alone", width: 160, height: 29, ranOn: 2,
			want: sessionLayout{wide: true, story: true, heading: 4, now: 6, storyAt: 9, accountsAt: 19, more: 2,
				turnsHeading: 21, turnsAt: 24, turns: 3, moreTurns: 27, requests: 1},
		},
		{
			name: "160×24, one account: the story folded", width: 160, height: 24, ranOn: 1,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 1,
				turnsHeading: 13, turnsAt: 16, turns: 6, moreTurns: 22, requests: 3},
		},
		{
			name: "160×24, four accounts and three turns", width: 160, height: 24, ranOn: 4,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 4,
				turnsHeading: 16, turnsAt: 19, turns: 3, moreTurns: 22, requests: 1},
		},
		{
			name: "160×24, ten accounts: three rows and seven more", width: 160, height: 24, ranOn: 10,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 3, more: 7,
				turnsHeading: 16, turnsAt: 19, turns: 3, moreTurns: 22, requests: 1},
		},
		{
			name: "160×21: its least folded", width: 160, height: 21, ranOn: 1,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 1,
				turnsHeading: 13, turnsAt: 16, turns: 3, moreTurns: 19, requests: 1},
		},
		{
			name: "160×20: the chrome alone", width: 160, height: 20, ranOn: 1,
			want: sessionLayout{wide: true, chrome: true},
		},
		{
			name: "100×24, one account: the wide chrome's least width, its story folded", width: 100, height: 24, ranOn: 1,
			want: sessionLayout{wide: true, heading: 4, now: 6, storyAt: 9, accountsAt: 11, accounts: 1,
				turnsHeading: 13, turnsAt: 16, turns: 6, moreTurns: 22, requests: 3},
		},
		{
			name: "52×32, two accounts: the phone's frame", width: 52, height: 32, ranOn: 2,
			want: sessionLayout{story: true, heading: 5, now: 7, storyAt: 10, accountsAt: 18, accounts: 2,
				turnsHeading: 21, turnsAt: 24, turns: 6, moreTurns: 30, requests: 3},
		},
		{
			name: "52×28, one account: a phone's least whole", width: 52, height: 28, ranOn: 1,
			want: sessionLayout{story: true, heading: 5, now: 7, storyAt: 10, accountsAt: 18, accounts: 1,
				turnsHeading: 20, turnsAt: 23, turns: 3, moreTurns: 26, requests: 1},
		},
		{
			name: "52×20, one account: a phone's least, its story left out", width: 52, height: 20, ranOn: 1,
			want: sessionLayout{heading: 5, now: 7, storyAt: 10, accountsAt: 10, accounts: 1,
				turnsHeading: 12, turnsAt: 15, turns: 3, moreTurns: 18, requests: 1},
		},
		{
			name: "52×19: the chrome alone", width: 52, height: 19, ranOn: 1,
			want: sessionLayout{chrome: true},
		},
		{
			name: "99×27, one account: a phone's chrome, its story left out", width: 99, height: 27, ranOn: 1,
			want: sessionLayout{heading: 5, now: 7, storyAt: 10, accountsAt: 10, accounts: 1,
				turnsHeading: 12, turnsAt: 15, turns: 10, moreTurns: 25, requests: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, View: Sessions}
			if got := f.sessionLayout(tt.ranOn); got != tt.want {
				t.Errorf("sessionLayout(%d) =\n%+v\nwant\n%+v", tt.ranOn, got, tt.want)
			}
		})
	}
}

// The page heads itself as the chrome does: in the wide chrome's form
// wherever the chrome's tabs share the title's row, and a phone's wherever
// they take a row of their own.
func TestASessionsPageHeadsItselfAsTheChromeDoes(t *testing.T) {
	for width := 20; width <= 230; width++ {
		f := Frame{Width: width, Height: 40, View: Sessions}
		if got, want := f.sessionLayout(1).wide, f.titleRows() == 1; got != want {
			t.Errorf("at %d columns, the page heads itself wide %t, want %t, as the chrome's title takes %d rows", width, got, want, f.titleRows())
		}
	}
}

// turnColumnOf is the column of a session's turns d lays out as part p,
// and whether it shows.
func (d sessionDrawn) turnColumnOf(p turnPart) (turnColumn, bool) {
	i := slices.IndexFunc(d.turnColumns, func(col turnColumn) bool { return col.part == p })
	if i < 0 {
		return turnColumn{}, false
	}
	return d.turnColumns[i], true
}

// turnText is what row says in the column of a session's turns d lays out
// as part p, without the blanks either side: nothing where it doesn't show.
func turnText(row []rune, d sessionDrawn, p turnPart) string {
	col, ok := d.turnColumnOf(p)
	if !ok {
		return ""
	}
	return strings.TrimSpace(string(row[col.x:col.end()]))
}

// stayText is what row says in the part p of an account's row as d lays it
// out, without the blanks either side: nothing where it doesn't show.
func stayText(row []rune, d sessionDrawn, p stayPart) string {
	i := slices.IndexFunc(d.stayColumns, func(col stayColumn) bool { return col.part == p })
	if i < 0 {
		return ""
	}
	col := d.stayColumns[i]
	return strings.TrimSpace(string(row[col.x : col.x+col.width]))
}

// pageDrawn is the page f draws of doc at the frames' now, as it lays it
// out, and drawn, as text, a row a slice of runes as wide as the frame.
func pageDrawn(f Frame, doc status.Document) (sessionDrawn, [][]rune) {
	at := f.At(doc, sessionNow)
	c := at.page(doc, sessionNow)
	rows := make([][]rune, f.Height)
	for y := range rows {
		rows[y] = []rune(rowOf(c, y) + strings.Repeat(" ", f.Width))
	}
	return at.sessionDrawn(doc, sessionNow), rows
}

// pageLabelled is the frames' document, its accounts named as long as a label
// is drawn at its most, or as the one given.
func pageLabelled(labels ...string) status.Document {
	doc := pageDoc()
	for i := range doc.Accounts {
		doc.Accounts[i].Label = labels[min(i, len(labels)-1)]
	}
	return doc
}

// The worlds the turns' and the accounts' rows are held in at every width:
// the frames', and one of long names, a label at its most among them.
var pageWorlds = []struct {
	name string
	doc  func() status.Document
}{
	{name: "short names", doc: pageDoc},
	{name: "long names", doc: func() status.Document {
		return pageLabelled("work-team-plan-for-the-org", "personal-research-writing", "side-hustle-consulting")
	}},
}

// A session's turns take one form at every width: their number, start,
// length, requests, worth and accounts always; and as the frame narrows,
// tools leaves first, then ↑ asking, read, written and out, in that order;
// then the start, the length and the requests narrow to their short heads,
// and last the accounts to a label's least. From 40 columns, what's kept
// fits; and wherever tools shows, its row runs to the last column, tools
// taking what's left, so no gap lies mid-row.
func TestATurnsColumnsLeaveInTheirOrder(t *testing.T) {
	kept := []turnPart{turnNumber, turnStarted, turnTook, turnRequests, turnWorth, turnOn}
	leaving := []turnPart{turnTools, turnAsking, turnRead, turnWritten, turnOut}
	for _, world := range pageWorlds {
		for width := 40; width <= 220; width++ {
			d, _ := pageDrawn(pageFrame(t, moved(), width, 37), world.doc())
			for _, p := range kept {
				if _, ok := d.turnColumnOf(p); !ok {
					t.Errorf("%s, at %d columns, column %d doesn't show, want it kept", world.name, width, p)
				}
			}
			for i, p := range leaving {
				_, shown := d.turnColumnOf(p)
				for _, later := range leaving[i+1:] {
					if _, ok := d.turnColumnOf(later); shown && !ok {
						t.Errorf("%s, at %d columns, column %d shows without column %d, which leaves after it", world.name, width, p, later)
					}
				}
			}
			last := d.turnColumns[len(d.turnColumns)-1]
			if last.end() > width {
				t.Errorf("%s, at %d columns, the turns' row runs to %d, past the frame", world.name, width, last.end())
			}
			if _, tools := d.turnColumnOf(turnTools); tools && last.end() != width {
				t.Errorf("%s, at %d columns, tools shows and the row ends at %d, want it at the last column", world.name, width, last.end())
			}
		}
	}
}

// As the frame widens, a session's turns never show fewer columns, nor a
// narrower one, tools and on among them, nor a shorter name of the
// accounts a turn went to, nor any figure cut, held from 40 columns to 220,
// with short names and long, over a turn running, turns with tools and one
// without.
func TestATurnsRowNeverSaysLessAsTheFrameWidens(t *testing.T) {
	shown := moved()
	shown.Page.Turns[3].Tools = nil
	for _, world := range pageWorlds {
		var before sessionDrawn
		var beforeRows [][]rune
		for width := 40; width <= 220; width++ {
			f := pageFrame(t, shown, width, 37)
			d, rows := pageDrawn(f, world.doc())
			if width > 40 {
				for _, was := range before.turnColumns {
					now, ok := d.turnColumnOf(was.part)
					if !ok || now.width < was.width {
						t.Errorf("%s, at %d columns, column %d is %d cells wide, shown %t, want at least the %d it had a column narrower", world.name, width, was.part, now.width, ok, was.width)
					}
				}
				for i := range min(d.l.turns, before.l.turns) {
					on, was := turnText(rows[d.l.turnsAt+i], d, turnOn), turnText(beforeRows[before.l.turnsAt+i], before, turnOn)
					if len([]rune(on)) < len([]rune(was)) {
						t.Errorf("%s, at %d columns, a turn's accounts read %q, want no shorter than %q a column narrower", world.name, width, on, was)
					}
				}
			}
			for i, turn := range shown.Page.Turns[:min(len(shown.Page.Turns), d.l.turns)] {
				row := rows[d.l.turnsAt+i]
				took, going := turn.Took(sessionNow)
				if going {
					took += prose.Ellipsis
				}
				figures := map[turnPart]string{
					turnNumber: strconv.Itoa(turn.Turn), turnStarted: prose.TimeOfDay(sessionNow, turn.Started), turnTook: took,
					turnRequests: prose.Grouped(turn.Requests), turnWorth: turn.Cents(),
				}
				for p, want := range figures {
					if got := turnText(row, d, p); got != want {
						t.Errorf("%s, at %d columns, turn %d's column %d reads %q, want %q whole", world.name, width, turn.Turn, p, got, want)
					}
				}
			}
			before, beforeRows = d, rows
		}
	}
}

// An account's row keeps its name, its points and its worth at every width:
// as the frame narrows, its points take their short words, then its
// requests leave, then its time there, and last its name narrows, to a
// label's least. Its name's natural is the widest name configured, its
// label at most labelMost, never the accounts the session ran on; and as
// the frame widens, no part leaves or narrows, nor does a name read
// shorter.
func TestAnAccountsRowGivesWayInItsOrder(t *testing.T) {
	for _, world := range pageWorlds {
		var before sessionDrawn
		var beforeRows [][]rune
		for width := 40; width <= 220; width++ {
			d, rows := pageDrawn(pageFrame(t, moved(), width, 37), world.doc())
			shown := map[stayPart]stayColumn{}
			for _, col := range d.stayColumns {
				shown[col.part] = col
			}
			for _, p := range []stayPart{stayName, stayPoints, stayWorth} {
				if _, ok := shown[p]; !ok {
					t.Errorf("%s, at %d columns, part %d of an account's row doesn't show, want it kept", world.name, width, p)
				}
			}
			_, requests := shown[stayRequests]
			_, time := shown[stayTime]
			names := stayNameCells(namesOf(world.doc().At(sessionNow, claudeLike)))
			points := pointsCells(d.listedWindows(d.m))
			switch {
			case requests && !time:
				t.Errorf("%s, at %d columns, its requests show without its time there, which leaves after them", world.name, width)
			case !requests && shown[stayPoints].width > points.least:
				t.Errorf("%s, at %d columns, its requests left with its points at %d, before they narrowed to %d", world.name, width, shown[stayPoints].width, points.least)
			case time && shown[stayName].width < names.natural:
				t.Errorf("%s, at %d columns, its time there shows with its name narrowed to %d, short of %d", world.name, width, shown[stayName].width, names.natural)
			}
			if width > 40 {
				for _, was := range before.stayColumns {
					if now, ok := shown[was.part]; !ok || now.width < was.width {
						t.Errorf("%s, at %d columns, part %d is %d cells wide, shown %t, want at least the %d it had a column narrower", world.name, width, was.part, now.width, ok, was.width)
					}
				}
				for i := range d.l.accounts {
					name, was := stayText(rows[d.l.accountsAt+i], d, stayName), stayText(beforeRows[before.l.accountsAt+i], before, stayName)
					if len([]rune(name)) < len([]rune(was)) {
						t.Errorf("%s, at %d columns, an account's name reads %q, want no shorter than %q a column narrower", world.name, width, name, was)
					}
				}
			}
			before, beforeRows = d, rows
		}
	}
}

// An account's name in its row is a label's: its number and marks, ▲'s
// cells kept, as headCells counts them, and its label, labelLeast cells at
// the least, and at the natural the widest configured, at most labelMost.
func TestAnAccountsNameIsALabels(t *testing.T) {
	long := strings.Repeat("x", labelMost+6)
	tests := []struct {
		name          string
		labels        []string
		least, natual int
	}{
		{name: "the frames' three, personal the primary", labels: []string{"work", "personal", "side"}, least: 14, natual: 14},
		{name: "a label past its most", labels: []string{"work", long, "side"}, least: 14, natual: 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stayNameCells(namesOf(pageLabelled(tt.labels...).At(sessionNow, claudeLike)))
			if got != (partCells{least: tt.least, natural: tt.natual}) {
				t.Errorf("an account's name is %+v, want least %d and natural %d", got, tt.least, tt.natual)
			}
		})
	}
}
