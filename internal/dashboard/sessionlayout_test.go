package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
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
			want: sessionLayout{story: true, heading: 5, now: 7, storyAt: 10, accountsAt: 19, accounts: 2,
				turnsHeading: 22, turnsAt: 25, turns: 5, moreTurns: 30, requests: 2},
		},
		{
			name: "52×29, one account: a phone's least whole", width: 52, height: 29, ranOn: 1,
			want: sessionLayout{story: true, heading: 5, now: 7, storyAt: 10, accountsAt: 19, accounts: 1,
				turnsHeading: 21, turnsAt: 24, turns: 3, moreTurns: 27, requests: 1},
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
			if got := f.layoutOf(tt.ranOn); got != tt.want {
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
		if got, want := f.layoutOf(1).wide, f.titleRows() == 1; got != want {
			t.Errorf("at %d columns, the page heads itself wide %t, want %t, as the chrome's title takes %d rows", width, got, want, f.titleRows())
		}
	}
}

// layoutOf is how the frame lays out the page of a session that ran on the
// accounts given, heading itself as chromeWide has it.
func (f Frame) layoutOf(accounts int) sessionLayout {
	return f.sessionLayout(accounts, f.chromeWide())
}

// drawnRows are the rows d draws, as text.
func drawnRows(d sessionDrawn) []string {
	c := newCanvas(d.Width, d.Height)
	d.draw(c)
	rows := make([]string, d.Height)
	for y := range rows {
		rows[y] = rowOf(c, y)
	}
	return rows
}

// The chrome decides the page's heading and its story's heading alone: with
// the page heading itself as the wide chrome does at every width, from 40
// columns to 220, its now rows, its story under its heading or label, its
// accounts, TURNS and its turns read as they do under the chrome's own.
func TestTheChromeDecidesNothingButTheHeadings(t *testing.T) {
	shown := moved()
	shown.Story = movedStory()
	for width := 40; width <= 220; width++ {
		f := pageFrame(t, shown, width, 60)
		d, _ := pageDrawn(f, pageDoc())
		wide := d
		wide.l = f.sessionLayout(len(shown.Page.Accounts), true)
		rows, wideRows := drawnRows(d), drawnRows(wide)
		same := func(what string, from, wideFrom, n int) {
			for i := range n {
				if rows[from+i] != wideRows[wideFrom+i] {
					t.Errorf("at %d columns, headed wide, %s's row %d reads\n%q\nwant it as under the chrome's own\n%q", width, what, i+1, wideRows[wideFrom+i], rows[from+i])
				}
			}
		}
		same("now", d.l.now, wide.l.now, 2)
		story := d.l.storyAt
		if d.l.wide {
			story++
		}
		same("the story", story, wide.l.storyAt+1, phoneStoryRows)
		same("the accounts and TURNS", d.l.accountsAt, wide.l.accountsAt, d.l.turnsAt-d.l.accountsAt)
		same("the turns", d.l.turnsAt, wide.l.turnsAt, min(d.l.turns, wide.l.turns))
	}
}

// What a session's page says takes its wide form where its own parts fit,
// switching once as the frame widens, never at the chrome's breakpoint: its
// now rows, and a move's label in the story.
func TestASessionsContentFormsSwitchOnceWhereTheyFit(t *testing.T) {
	shown := moved()
	shown.Story = movedStory()
	for _, world := range pageWorlds {
		var nowSwitched, labelSwitched []int
		wasNow, wasLabel := false, false
		for width := 40; width <= 220; width++ {
			f := pageFrame(t, shown, width, 60)
			d, _ := pageDrawn(f, world.doc())
			rows := drawnRows(d)
			label := strings.Contains(strings.Join(rows, "\n"), "cached again")
			if d.nowWide != wasNow {
				nowSwitched = append(nowSwitched, width)
			}
			if label != wasLabel {
				labelSwitched = append(labelSwitched, width)
			}
			wasNow, wasLabel = d.nowWide, label
		}
		if len(nowSwitched) != 1 || len(labelSwitched) != 1 {
			t.Errorf("%s, the now rows take their wide form from %v and the move's label from %v, want each to switch once", world.name, nowSwitched, labelSwitched)
		}
		for _, at := range slices.Concat(nowSwitched, labelSwitched) {
			if at == phoneUnder {
				t.Errorf("%s, a form switches at %d, the chrome's breakpoint", world.name, at)
			}
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
			tools, ok := d.turnColumnOf(turnTools)
			switch {
			case ok && tools.width > toolsMost:
				t.Errorf("%s, at %d columns, tools is %d cells wide, past its most, %d", world.name, width, tools.width, toolsMost)
			case ok && tools.width < toolsMost && last.end() != width:
				t.Errorf("%s, at %d columns, tools is short of its most and the row ends at %d, want it at the last column", world.name, width, last.end())
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

// ↑ asking keeps its cells on the page of a session that can still ask,
// between turns as while one runs, so tools holds still as they come and
// go; on an ended session's, which never asks, it has none, and tools takes
// them.
func TestOnlyASessionThatCanAskKeepsAskingsCells(t *testing.T) {
	over := moved()
	over.Page.Activity = views.Activity{Doing: views.Ended, Since: at(14, 0)}
	quiet := moved()
	quiet.Page.Activity = views.Activity{Since: sessionNow.Add(-9 * time.Minute)}
	for width := 40; width <= 220; width++ {
		running, _ := pageDrawn(pageFrame(t, moved(), width, 37), pageDoc())
		between, _ := pageDrawn(pageFrame(t, quiet, width, 37), pageDoc())
		ended, _ := pageDrawn(pageFrame(t, over, width, 37), pageDoc())
		if !slices.Equal(running.turnColumns, between.turnColumns) {
			t.Errorf("at %d columns, a quiet session's turns are laid out\n%+v\nwant them as a running one's\n%+v", width, between.turnColumns, running.turnColumns)
		}
		if _, ok := ended.turnColumnOf(turnAsking); ok {
			t.Errorf("at %d columns, an ended session's turns keep ↑ asking's cells", width)
		}
		tools, ok := running.turnColumnOf(turnTools)
		endedTools, _ := ended.turnColumnOf(turnTools)
		if ok && tools.width < toolsMost && endedTools.width <= tools.width {
			t.Errorf("at %d columns, an ended session's tools is %d cells wide, want more than a running one's %d", width, endedTools.width, tools.width)
		}
	}
}

// A move's second name gives way before read, written and out leave: cut
// with …, its first name whole, down to moveToLeast cells; and while any of
// them shows, it's cut no further. One account's name stays whole until
// they've left.
func TestAMovesSecondNameGivesWayBeforeTheDetails(t *testing.T) {
	doc := pageLabelled("work-team-plan", "personal-research", "side-hustle-consulting")
	cut := 0
	for width := 40; width <= 220; width++ {
		d, rows := pageDrawn(pageFrame(t, moved(), width, 37), doc)
		_, read := d.turnColumnOf(turnRead)
		move, one := turnText(rows[d.l.turnsAt+1], d, turnOn), turnText(rows[d.l.turnsAt+2], d, turnOn)
		if !read {
			continue
		}
		if !strings.HasPrefix(move, "work-team-plan → si") || one != "work-team-plan" {
			t.Errorf("at %d columns, read shows beside a move named %q and one account %q, want the first whole, si… at the least, and the one whole", width, move, one)
		}
		if strings.HasSuffix(move, prose.Ellipsis) {
			cut++
		}
	}
	if cut == 0 {
		t.Error("no width shows read beside a move cut, want its second name to give way first")
	}
}
