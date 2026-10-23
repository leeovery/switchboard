package dashboard

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/ledger"
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

// nowStates are the sessions a page's now rows are held in: running, moved
// with its why, idle, ended, pinned, its pin yielding, resumed, today and
// the day before, ended having moved on a day after its start's, and ended
// with models enough that row 7 sets where they switch.
func nowStates() []struct {
	name  string
	shown *SessionShown
} {
	pinned := func(reason, from, to string) *SessionShown {
		shown := moved()
		shown.Page.Account = to
		shown.Page.Moves = []views.SessionMove{{At: at(14, 31), Model: opus, From: from, To: to, Reason: reason}}
		return shown
	}
	overnight := resumed()
	overnight.Page.Resumed = yesterdayAt(22, 10)
	overnight.Page.Moves[0].At = yesterdayAt(22, 10)
	movedLater := ended()
	movedLater.Page.Started = yesterdayAt(13, 20).AddDate(0, 0, -1)
	manyModels := ended()
	for _, model := range []string{"claude-sonnet-5-5", "claude-fable-1", "claude-opus-4-1", "claude-sonnet-4-5", "claude-haiku-3-5", "claude-opus-4-5", "claude-sonnet-4", "claude-opus-4", "claude-sonnet-3-7", "claude-fable-2", "claude-haiku-3"} {
		manyModels.Page.Goes = append(manyModels.Page.Goes, views.SessionModel{Model: model, Account: "side"})
	}
	return []struct {
		name  string
		shown *SessionShown
	}{
		{name: "moved", shown: moved()},
		{name: "quiet", shown: quiet()},
		{name: "ended", shown: ended()},
		{name: "pinned here", shown: pinned(status.ReasonPinned, "work", "side")},
		{name: "its pin yielding", shown: pinned(status.ReasonPinYields+"side has no room", "side", "work")},
		{name: "resumed", shown: resumed()},
		{name: "resumed the day before", shown: overnight},
		{name: "ended, moved on a day after its start's", shown: movedLater},
		{name: "ended, many models", shown: manyModels},
	}
}

// The now rows take their wide form only where every part of each fits
// whole at its least, and switch once: so in every state, with short names
// and long, wherever they're wide, row 6 says what the session's doing,
// where it runs and why whole, and at its right one of the forms of when it
// started, whole, three cells or more apart, never a fuller one as the frame
// narrows; and row 7 its account's windows whole, or once it's over, its
// models and how long they're kept.
func TestTheNowRowsWideFormCutsNothing(t *testing.T) {
	for _, world := range pageWorlds {
		for _, state := range nowStates() {
			var switched []int
			was, fuller := true, 0
			for width := 220; width >= 40; width-- {
				d, drawn := pageDrawn(pageFrame(t, state.shown, width, 37), world.doc())
				if d.nowWide != was {
					switched = append(switched, width)
				}
				was = d.nowWide
				if !d.nowWide {
					continue
				}
				row6, row7 := strings.TrimRight(string(drawn[d.l.now]), " "), strings.TrimRight(string(drawn[d.l.now+1]), " ")
				where := d.page.Where(d.now)
				said := slices.Concat(d.doing(), line{spaces(sessionApart)}, d.onSince(where.OnSince), d.words(where.Why, mutedInk)).plain()
				form := slices.IndexFunc(d.startedForms(), func(l line) bool {
					return strings.HasSuffix(row6, strings.Repeat(" ", sessionApart)+l.plain())
				})
				if !strings.HasPrefix(row6, said) || form < fuller {
					t.Errorf("%s, %s, at %d columns, row 6 reads\n%q\nwant it to say\n%q\nand end in a form of when it started, no fuller than at %d", world.name, state.name, width, row6, said, width+1)
				}
				fuller = max(fuller, form)
				right := d.windows()
				if !d.page.Runs() {
					right = d.endedModels()
				}
				if !strings.HasSuffix(row7, right.plain()) {
					t.Errorf("%s, %s, at %d columns, row 7 reads\n%q\nwant it to end\n%q", world.name, state.name, width, row7, right.plain())
				}
			}
			if len(switched) != 1 {
				t.Errorf("%s, %s, the now rows switch form at %v, want once", world.name, state.name, switched)
			}
		}
	}
}

// The now rows take their wide form from a width each session's own words
// set, cell for cell, and nothing else moves it: not where new sessions go,
// ▲ on whichever account or none, nor ◆ on whichever, nor the clock a day,
// a week or a year on, its times' days with it.
func TestTheNowRowsSwitchWhereTheSessionsWordsSay(t *testing.T) {
	wideFrom := map[string][2]int{
		"moved":                  {96, 154},
		"quiet":                  {64, 80},
		"ended":                  {93, 151},
		"pinned here":            {70, 88},
		"its pin yielding":       {99, 155},
		"resumed":                {133, 165},
		"resumed the day before": {133, 165},
		"ended, moved on a day after its start's": {103, 161},
		"ended, many models":                      {168, 168},
	}
	for w, world := range pageWorlds {
		for _, state := range nowStates() {
			for _, best := range []string{"", "work", "personal", "side"} {
				for _, primary := range []string{"work", "personal", "side"} {
					for _, later := range []int{0, 1, 7, 365} {
						doc := world.doc()
						doc.Best = best
						for i := range doc.Accounts {
							doc.Accounts[i].Primary = doc.Accounts[i].ID == primary
						}
						now := sessionNow.AddDate(0, 0, later)
						switched := 0
						for width := 40; width <= 220 && switched == 0; width++ {
							if pageFrame(t, state.shown, width, 37).At(doc, now).sessionDrawn(doc, now).nowWide {
								switched = width
							}
						}
						if want := wideFrom[state.name][w]; switched != want {
							t.Errorf("%s, %s, ▲ on %q, ◆ on %s, %d days on, the now rows are wide from %d columns, want %d",
								world.name, state.name, best, primary, later, switched, want)
						}
					}
				}
			}
		}
	}
}

// Row 6's right takes its fullest form where it fits beside the left as
// it's said now, its times as they are, not at their widest: so a session
// resumed today says when it started whole at 140 columns. And at every
// width its form is the same whatever the session's doing.
func TestRow6sRightIsAsFullAsTheWordsSaidNowLeaveRoomFor(t *testing.T) {
	d, drawn := pageDrawn(pageFrame(t, resumed(), 140, 37), pageDoc())
	if row := strings.TrimRight(string(drawn[d.l.now]), " "); !d.nowWide || !strings.HasSuffix(row, "   started yesterday 16:40 · resumed 09:12") {
		t.Errorf("at 140 columns, row 6 reads\n%q\nwant it wide, ending in when it started and resumed", row)
	}
	doings := []views.Activity{
		{Doing: views.Asking}, {Doing: views.Answering}, {Doing: views.Throttled, Status: 429},
		{Doing: views.Refused, Status: 403}, {Doing: views.Idle, Since: sessionNow.Add(-59 * time.Minute)},
	}
	for _, state := range nowStates() {
		if !state.shown.Page.Runs() {
			continue
		}
		for width := 40; width <= 220; width++ {
			var forms []string
			for _, doing := range doings {
				shown := *state.shown
				shown.Page.Activity = doing
				d, _ := pageDrawn(pageFrame(t, &shown, width, 37), pageDoc())
				forms = append(forms, d.startedFit().plain())
			}
			if slices.ContainsFunc(forms, func(form string) bool { return form != forms[0] }) {
				t.Errorf("%s, at %d columns, row 6's right reads %q as the session's doing changes, want one form", state.name, width, forms)
			}
		}
	}
}

// At 138 columns a session resumed today says when it started and resumed
// whole beside where it runs, its right measured with the marks its name
// carries; with ▲ moved onto its account too, its row never runs past the
// frame, and the switch to the short form stays where it was.
func TestRow6sRightCountsTheMarksItsNameCarries(t *testing.T) {
	d, drawn := pageDrawn(pageFrame(t, resumed(), 138, 30), pageDoc())
	if row := strings.TrimRight(string(drawn[d.l.now]), " "); !d.nowWide || !strings.HasSuffix(row, "the most room    started yesterday 16:40 · resumed 09:12") {
		t.Errorf("at 138 columns, row 6 reads\n%q\nwant both its parts whole", row)
	}
	doc := pageDoc()
	doc.Best = "personal"
	switchedAt := func(doc status.Document) int {
		for width := 40; width <= 220; width++ {
			if d, _ := pageDrawn(pageFrame(t, resumed(), width, 30), doc); d.nowWide {
				return width
			}
		}
		return 0
	}
	if at, was := switchedAt(doc), switchedAt(pageDoc()); at != was {
		t.Errorf("with ▲ on its account, the now rows are wide from %d columns, want %d, as without it", at, was)
	}
	for width := 40; width <= 220; width++ {
		d, drawn := pageDrawn(pageFrame(t, resumed(), width, 30), doc)
		if row := strings.TrimRight(string(drawn[d.l.now]), " "); !strings.Contains(row, "▲") || ansi.StringWidth(row) > width {
			t.Errorf("at %d columns, with ▲ on its account, row 6 reads %q, want ▲ shown and the row within the frame", width, row)
		}
	}
}

// Row 7 decides the switch where it's the longer row: a session with
// little to say on row 6, with long names, its windows whole, and an ended
// one with many models, those models and how long their lines are kept
// whole, wherever the rows are wide.
func TestRow7IsWholeWhereItDecidesTheSwitch(t *testing.T) {
	plain := quiet()
	plain.Page.Goes, plain.Page.Started = nil, time.Time{}
	many := ended()
	for _, model := range []string{"claude-sonnet-5-5", "claude-fable-1", "claude-opus-4-1", "claude-sonnet-4-5", "claude-haiku-3-5", "claude-opus-4-5"} {
		many.Page.Goes = append(many.Page.Goes, views.SessionModel{Model: model, Account: "side"})
	}
	tests := []struct {
		name  string
		shown *SessionShown
		doc   status.Document
	}{
		{name: "running, little on row 6", shown: plain, doc: pageWorlds[1].doc()},
		{name: "ended, many models", shown: many, doc: pageDoc()},
	}
	for _, tt := range tests {
		decided := false
		for width := 40; width <= 220; width++ {
			d, drawn := pageDrawn(pageFrame(t, tt.shown, width, 37), tt.doc)
			_, row6 := flex.Lay(width, flex.Part{Least: d.saidCells(d.page.WhereSaidWidest(d.now)) + d.marksSpare(d.page.WhereSaidWidest(d.now))}, flex.Part{Least: d.ranWidest(), Gap: sessionApart})
			decided = decided || row6 && !d.nowWide
			if !d.nowWide {
				continue
			}
			right := d.windows()
			if !d.page.Runs() {
				right = d.endedModels()
			}
			if row7 := strings.TrimRight(string(drawn[d.l.now+1]), " "); !strings.HasSuffix(row7, right.plain()) || ansi.StringWidth(row7) > width {
				t.Errorf("%s, at %d columns, row 7 reads %q, want it to end %q whole", tt.name, width, row7, right.plain())
			}
		}
		if !decided {
			t.Errorf("%s, row 7 never decides the switch, want it the longer row", tt.name)
		}
	}
}

// A session resumed the day before is wide at 160 columns, each part
// measured for its own kind, its started part whole in its fullest form;
// and narrower, its started part gives way to how it has run alone before
// the rows take their short form.
func TestASessionResumedTheDayBeforeIsWideAt160(t *testing.T) {
	overnight := nowStates()[6].shown
	d, drawn := pageDrawn(pageFrame(t, overnight, 160, 37), pageDoc())
	if row := strings.TrimRight(string(drawn[d.l.now]), " "); !d.nowWide || !strings.HasSuffix(row, "   started yesterday 16:40 · resumed yesterday 22:10") {
		t.Errorf("at 160 columns, row 6 reads\n%q\nwant it wide, ending in when it started and resumed", row)
	}
	d, drawn = pageDrawn(pageFrame(t, overnight, 140, 37), pageDoc())
	if row := strings.TrimRight(string(drawn[d.l.now]), " "); !d.nowWide || !strings.HasSuffix(row, "   resumed yesterday 22:10") || strings.Contains(row, "started") {
		t.Errorf("at 140 columns, row 6 reads\n%q\nwant it wide, ending in when it resumed alone", row)
	}
}

// Rows 6 and 7 name an account at a label's most, cut with … past it, and
// keep its marks: so a label longer still leaves their rows on the frame.
func TestTheNowRowsNameAnAccountAtALabelsMost(t *testing.T) {
	long := strings.Repeat("s", 60)
	doc := pageLabelled("work", "personal", long)
	d, drawn := pageDrawn(pageFrame(t, moved(), 100, 37), doc)
	if d.nowWide {
		t.Fatal("at 100 columns, the now rows are wide, want them short")
	}
	cut := prose.Cut(long, labelMost)
	if row := strings.TrimRight(string(drawn[d.l.now]), " "); !strings.Contains(row, "on "+cut+" ▲ since 14:31") {
		t.Errorf("row 6 reads %q, want side named %q, its ▲ kept", row, cut)
	}
	wide, drawn := pageDrawn(pageFrame(t, moved(), 220, 37), doc)
	if !wide.nowWide {
		t.Fatal("at 220 columns, the now rows are short, want them wide")
	}
	for _, y := range []int{wide.l.now, wide.l.now + 1} {
		if row := string(drawn[y]); !strings.Contains(row, cut) || strings.Contains(row, strings.Repeat("s", labelMost)) {
			t.Errorf("row %d reads %q, want side named %q", y+1, strings.TrimRight(row, " "), cut)
		}
	}
	if row := string(drawn[wide.l.now]); !strings.Contains(row, cut+" ▲ since 14:31") {
		t.Errorf("row 6 reads %q, want side's ▲ kept", strings.TrimRight(row, " "))
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
					if namesShrank(namesShown(was), namesShown(on)) {
						t.Errorf("%s, at %d columns, a turn's accounts read %q, want each name shown no shorter than in %q a column narrower", world.name, width, on, was)
					}
				}
			}
			for i, turn := range shown.Page.Turns[:min(len(shown.Page.Turns), d.l.turns)] {
				row := rows[d.l.turnsAt+i]
				took := turn.TookSaid(sessionNow)
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

// An account's row keeps its name, its points and its worth at every width,
// its windows before its other figures, and its figures before its name: as
// the frame narrows, its points' words go short, then its name narrows to a
// label's least, then its requests leave, then its time there, then its
// points' windows leave from the end, down to one, each only where the row
// has no cell spare, and each step in turn, none skipped that its bounds
// leave room for. Where its requests or its time have left, what they free
// goes back to the name and the points in their turns. As the frame widens,
// no part leaves, nor does a name read shorter, but where a part comes back:
// the points and the name give it back the cells they took while it was
// gone.
func TestAnAccountsRowGivesWayInItsOrder(t *testing.T) {
	for _, world := range pageWorlds {
		var before sessionDrawn
		var beforeRows [][]rune
		var steps, want []int
		for width := 220; width >= 40; width-- {
			d, _ := pageDrawn(pageFrame(t, moved(), width, 37), world.doc())
			step := stayStep(t, world.name, width, d)
			if len(steps) == 0 || step != steps[len(steps)-1] {
				steps = append(steps, step)
			}
			if width == 40 {
				names, bounds := stayNameCells(d.names), d.windowsCells
				room := []bool{
					0:                 true,
					stayWordsNarrow:   bounds.wide > bounds.short,
					stayNameNarrows:   names.natural > names.least,
					stayRequestsLeave: true,
					stayTimeLeaves:    true,
					stayWindowsNarrow: bounds.short > bounds.one,
				}
				for s, has := range room {
					if has {
						want = append(want, s)
					}
				}
			}
		}
		if !slices.Equal(steps, want) {
			t.Errorf("%s, narrowing, an account's row gives way in the steps %v, want %v", world.name, steps, want)
		}
		for width := 40; width <= 220; width++ {
			d, rows := pageDrawn(pageFrame(t, moved(), width, 37), world.doc())
			shown := map[stayPart]stayColumn{}
			for _, col := range d.stayColumns {
				shown[col.part] = col
			}
			if width > 40 {
				returned := len(shown) > len(before.stayColumns)
				for _, was := range before.stayColumns {
					now, ok := shown[was.part]
					if !ok || now.width < was.width && !returned {
						t.Errorf("%s, at %d columns, part %d is %d cells wide, shown %t, want at least the %d it had a column narrower", world.name, width, was.part, now.width, ok, was.width)
					}
				}
				for i := range d.l.accounts {
					name, was := stayText(rows[d.l.accountsAt+i], d, stayName), stayText(beforeRows[before.l.accountsAt+i], before, stayName)
					if len([]rune(name)) < len([]rune(was)) && !returned {
						t.Errorf("%s, at %d columns, an account's name reads %q, want no shorter than %q a column narrower", world.name, width, name, was)
					}
				}
			}
			before, beforeRows = d, rows
		}
	}
}

// stayStep is the furthest step an account's row has given way to as d lays
// it out at width, as the turns name them, 0 where it's given way to none,
// failing t where it's given way out of its order, or narrows a part with a
// cell spare. Where its requests or its time have left, the name and the
// points' words may take back what they gave.
func stayStep(t *testing.T, world string, width int, d sessionDrawn) int {
	t.Helper()
	shown := map[stayPart]stayColumn{}
	used := 0
	for _, col := range d.stayColumns {
		shown[col.part] = col
		used += col.width + stayGap
	}
	if col, ok := shown[stayWorth]; !ok || col.width != stayWorthCells {
		t.Errorf("%s, at %d columns, an account's worth is %+v, shown %t, want it kept, %d cells", world, width, col, ok, stayWorthCells)
	}
	name, nameOK := shown[stayName]
	points, pointsOK := shown[stayPoints]
	if !nameOK || !pointsOK {
		t.Fatalf("%s, at %d columns, an account's name shows %t and its points %t, want both kept", world, width, nameOK, pointsOK)
	}
	_, requests := shown[stayRequests]
	_, time := shown[stayTime]
	names, bounds := stayNameCells(d.names), d.windowsCells
	given := []bool{
		stayWordsNarrow:   points.width < bounds.wide,
		stayNameNarrows:   name.width < names.natural,
		stayRequestsLeave: !requests,
		stayTimeLeaves:    !time,
		stayWindowsNarrow: points.width < bounds.short,
	}
	step := 0
	for s, way := range given {
		if way {
			step = s
		}
	}
	switch {
	case given[stayNameNarrows] && points.width > bounds.short:
		t.Errorf("%s, at %d columns, an account's name narrows with its points' words still wide", world, width)
	case given[stayWindowsNarrow] && (requests || time || name.width != names.least):
		t.Errorf("%s, at %d columns, its points' windows leave with its requests shown %t, its time %t and its name %d cells, over its least", world, width, requests, time, name.width)
	case requests && !time:
		t.Errorf("%s, at %d columns, its requests show without its time there, which leaves after them", world, width)
	}
	least := names.least + bounds.short + stayWorthCells + 2*stayGap
	switch {
	case !requests && least+stayTimeCells+stayRequestsCells+2*stayGap <= width:
		t.Errorf("%s, at %d columns, its requests left where they'd fit beside its name at its least and every window short", world, width)
	case !time && least+stayTimeCells+stayGap <= width:
		t.Errorf("%s, at %d columns, its time there left where it'd fit beside its name at its least and every window short", world, width)
	}
	if narrowed := given[stayWordsNarrow] || given[stayNameNarrows]; narrowed && used-stayGap < width {
		t.Errorf("%s, at %d columns, an account's row narrows a part with %d cells spare", world, width, width-used+stayGap)
	}
	return step
}

// An account's row lays out the same whatever its figures say, its parts and
// each one's cells set by what it can say at its most, and each row shows as
// many windows as fit at their most, so rows with as many windows show as
// many at one width: with short names and a 26-cell one, at every width,
// before its first point and after, at 1 to 1,234 points of each window,
// and worth a cent to its bound.
func TestAnAccountsRowHoldsStillAsItsFiguresChange(t *testing.T) {
	docs := map[string]status.Document{"short names": pageDoc(), "a 26-cell name": pageLabelled("work-team-plan-for-the-org", "personal", "side")}
	cents := func(n int64) *ledger.Picodollars {
		p := ledger.Picodollars(n) * ledger.Picodollars(10_000_000_000)
		return &p
	}
	type figure struct {
		set  func(*views.SessionAccount)
		took bool
	}
	var figures []figure
	for _, points := range []int{0, 1, 9, 10, 100, 999, 1000, 1234} {
		figures = append(figures, figure{set: func(a *views.SessionAccount) {
			for key := range a.Points {
				a.Points[key] = points
			}
		}, took: points > 0})
	}
	for _, amount := range []int64{1, 999, 1000, 14_106, 999_999} {
		figures = append(figures, figure{set: func(a *views.SessionAccount) { a.Amount = cents(amount) }, took: true})
	}
	figures = append(figures, figure{set: func(a *views.SessionAccount) { a.Points = nil }})
	for world, doc := range docs {
		for width := 40; width <= 220; width++ {
			d, drawn := pageDrawn(pageFrame(t, moved(), width, 37), doc)
			want, wantShown := d.stayColumns, windowsShown(d, drawn)
			for i, s := range d.page.Stays(d.now)[:d.l.accounts] {
				for j, o := range d.page.Stays(d.now)[:i] {
					if len(s.PointsKeys()) == len(o.PointsKeys()) && wantShown[i] != wantShown[j] {
						t.Errorf("%s, at %d columns, rows of %d windows show %d and %d of them", world, width, len(s.PointsKeys()), wantShown[j], wantShown[i])
					}
				}
			}
			for i, figure := range figures {
				shown := moved()
				for a := range shown.Page.Accounts {
					figure.set(&shown.Page.Accounts[a])
				}
				d, drawn := pageDrawn(pageFrame(t, shown, width, 37), doc)
				if !slices.Equal(d.stayColumns, want) {
					t.Errorf("%s, at %d columns, with figures %d, an account's row is laid out %+v, want %+v as with the frames'", world, width, i, d.stayColumns, want)
				}
				if got := windowsShown(d, drawn); figure.took && !slices.Equal(got, wantShown) {
					t.Errorf("%s, at %d columns, with figures %d, the accounts' rows show %v windows, want %v as with the frames'", world, width, i, got, wantShown)
				}
			}
		}
	}
}

// windowsShown are how many windows each account's row d draws shows.
func windowsShown(d sessionDrawn, drawn [][]rune) []int {
	shown := make([]int, d.l.accounts)
	for i := range shown {
		if points := stayText(drawn[d.l.accountsAt+i], d, stayPoints); points != "" {
			shown[i] = strings.Count(points, " · ") + 1
		}
	}
	return shown
}

// A model's own window the session used is measured with those listed,
// listed or not, as which windows it used is a fact of the session, not a
// figure: with the frame's models' windows hidden, a row that took points
// of Fable week says them whole wherever its bound fits; and where no
// account took any, the row is measured by the windows listed alone.
func TestAnAccountsRowMeasuresTheWindowsTheSessionUsed(t *testing.T) {
	used := resumed()
	used.Page.Accounts[1].Points = map[string]int{"5h": 9, "7d": 1, "7d_oi": 2}
	for width := 40; width <= 220; width++ {
		f := pageFrame(t, used, width, 37)
		f.Models = false
		d, drawn := pageDrawn(f, pageDoc())
		points, _ := d.stayColumnOf(stayPoints)
		got := stayText(drawn[d.l.accountsAt+1], d, stayPoints)
		if fits := points.width >= d.windowsCells.short; fits != strings.Contains(got, " · 2 of") {
			t.Errorf("at %d columns, with %d cells to every window's %d, the row's points read %q, want Fable week's said where they fit", width, points.width, d.windowsCells.short, got)
		}
		alone := pageFrame(t, resumed(), width, 37)
		alone.Models = false
		listed := alone.At(pageDoc(), sessionNow).sessionDrawn(pageDoc(), sessionNow)
		if listed.windowsCells != pointsCells(measuredKeys(listed.listedWindows(listed.m), nil)) {
			t.Errorf("at %d columns, with no model's own window used, the points are measured %+v, want by the windows listed alone", width, listed.windowsCells)
		}
	}
}

// An account's row keeps every part in its column, the blanks between
// them blank, and its worth whole at the right, stayGap cells after what
// comes before it, its figures past their most, 1,234 points of every
// window, or of the widest alone, and $9,999.99: at every width, even below
// the row's leasts, where the points give way, never the worth.
func TestAnAccountsPartsKeepToTheirColumns(t *testing.T) {
	most := moved()
	for i := range most.Page.Accounts {
		most.Page.Accounts[i].Points = map[string]int{"5h": 1234, "7d": 1234, "7d_oi": 1234}
		most.Page.Accounts[i].Worth = worthOf(999_999)
	}
	most.Page.Accounts[0].Points = map[string]int{"7d_oi": 1234}
	for _, world := range pageWorlds {
		for width := 40; width <= 220; width++ {
			d, drawn := pageDrawn(pageFrame(t, most, width, 37), world.doc())
			for i, s := range d.page.Stays(d.now)[:d.l.accounts] {
				row := drawn[d.l.accountsAt+i]
				said := strings.TrimRight(string(row), " ")
				if want := strings.Repeat(" ", stayGap) + s.Cents(); !strings.HasSuffix(said, want) || strings.Contains(strings.TrimSuffix(said, s.Cents()), "$") {
					t.Errorf("%s, at %d columns, an account's row reads %q, want it to end %q", world.name, width, said, want)
				}
				for c, col := range d.stayColumns[:len(d.stayColumns)-1] {
					if gap := string(row[col.x+col.width : d.stayColumns[c+1].x]); strings.TrimSpace(gap) != "" {
						t.Errorf("%s, at %d columns, an account's row reads %q, part %d running past its column into %q", world.name, width, said, col.part, gap)
					}
				}
			}
		}
	}
}

// pointsSessions are the frames' sessions whose accounts' rows say points.
func pointsSessions() map[string]*SessionShown {
	return map[string]*SessionShown{"moved": moved(), "quiet": quiet(), "ended": ended(), "resumed": resumed()}
}

// An account's row keeps its windows before its requests and its time
// there, and its requests before its name: with the frames' names and with
// a 26-cell one, from 58 columns up, every account's row says every window
// it took, uncut, and from 128 up, its requests whole too.
func TestAnAccountsWindowsComeBeforeItsRequests(t *testing.T) {
	docs := map[string]status.Document{"short names": pageDoc(), "a 26-cell name": pageLabelled("work-team-plan-for-the-org", "personal", "side")}
	for world, doc := range docs {
		for name, shown := range pointsSessions() {
			for width := 58; width <= 170; width++ {
				d, drawn := pageDrawn(pageFrame(t, shown, width, 37), doc)
				for i, s := range d.page.Stays(d.now)[:d.l.accounts] {
					row := drawn[d.l.accountsAt+i]
					points := stayText(row, d, stayPoints)
					if strings.Contains(points, prose.Ellipsis) || strings.Count(points, " · ") != len(s.PointsKeys())-1 {
						t.Errorf("%s, %s, at %d columns, an account's points read %q, want its %d windows whole", world, name, width, points, len(s.PointsKeys()))
					}
					if got := stayText(row, d, stayRequests); width >= 128 && got != s.RequestsSaid() {
						t.Errorf("%s, %s, at %d columns, an account's requests read %q, want %q", world, name, width, got, s.RequestsSaid())
					}
				}
			}
		}
	}
}

// Where an account's name is cut short of its natural, its row holds no
// cell spare: its points' column runs to its worth's, and the cells blank in
// it are only what's kept for its figures at their most, fewer than one
// window takes at its least in the row of an account that took every window
// listed.
func TestNoAccountsNameIsCutBesideBlankCells(t *testing.T) {
	for _, world := range pageWorlds {
		names := stayNameCells(namesOf(world.doc().At(sessionNow, claudeLike)))
		for name, shown := range pointsSessions() {
			for width := 40; width <= 220; width++ {
				d, drawn := pageDrawn(pageFrame(t, shown, width, 37), world.doc())
				col, _ := d.stayColumnOf(stayName)
				if col.width >= names.natural {
					continue
				}
				points, _ := d.stayColumnOf(stayPoints)
				if end := points.x + points.width + stayGap; end < width-stayWorthCells {
					t.Errorf("%s, %s, at %d columns, the names are cut to %d of %d with the points ending at %d, want them to run to the worth at %d", world.name, name, width, col.width, names.natural, end, width-stayWorthCells)
				}
				least, listed := width, len(d.listedWindows(d.m))
				for i, s := range d.page.Stays(d.now)[:d.l.accounts] {
					if len(s.PointsKeys()) < listed {
						continue
					}
					drawnPoints := ansi.StringWidth(strings.TrimRight(string(drawn[d.l.accountsAt+i][points.x:points.x+points.width]), " "))
					least = min(least, points.width-drawnPoints)
				}
				if least < width && least >= d.windowsCells.one {
					t.Errorf("%s, %s, at %d columns, the names are cut to %d of %d with %d cells blank in every row's points, want fewer than a window's %d", world.name, name, width, col.width, names.natural, least, d.windowsCells.one)
				}
			}
		}
	}
}

// stayColumnOf is the part p of an account's row as d lays it out, and
// whether it shows.
func (d sessionDrawn) stayColumnOf(p stayPart) (stayColumn, bool) {
	i := slices.IndexFunc(d.stayColumns, func(col stayColumn) bool { return col.part == p })
	if i < 0 {
		return stayColumn{}, false
	}
	return d.stayColumns[i], true
}

// The short now rows give every name one width, the same in both rows and
// whatever the session's doing or however long it has run, so an account
// reads one way and holds still as the session works.
func TestTheShortNowRowsGiveTheirNamesOneWidth(t *testing.T) {
	doings := []views.Activity{
		{Doing: views.Asking}, {Doing: views.Answering}, {Doing: views.Throttled, Status: 429},
		{Doing: views.Refused, Status: 403}, {Doing: views.Idle, Since: sessionNow.Add(-9 * time.Minute)},
	}
	runs := []time.Duration{59 * time.Minute, 61 * time.Minute, 10 * time.Hour}
	long := pageLabelled("work-team-plan-for-the-org", "personal-research-writing", "side-hustle-consulting")
	cases := map[string]struct {
		reason string
		doc    status.Document
	}{
		"its own move, row 8 the longer":     {doc: pageLabelled("work-team-plan-for-the-org", "personal", "side")},
		"the global pin's, row 7 the longer": {reason: status.ReasonMovedByPin, doc: long},
	}
	for case_, c := range cases {
		reason, doc := c.reason, c.doc
		between := false
		for width := 40; width <= 60; width++ {
			widths := map[int]bool{}
			for _, doing := range doings {
				for _, run := range runs {
					shown := moved()
					shown.Page.Activity, shown.Page.Started = doing, sessionNow.Add(-run)
					if reason != "" {
						shown.Page.Moves[0].Reason = reason
					}
					d, drawn := pageDrawn(pageFrame(t, shown, width, 37), doc)
					where := d.page.Where(d.now)
					cells := d.shortNamesCells(where)
					widths[cells], between = true, between || cells > labelLeast && cells < labelMost
					row7, row8 := string(drawn[d.l.now]), strings.TrimRight(string(drawn[d.l.now+1]), " ")
					if a, b := sideNamed(row7), sideNamed(row8); a != "" && b != "" && a != b {
						t.Errorf("%s, at %d columns, rows 7 and 8 read %q and %q, want side named alike", case_, width, strings.TrimRight(row7, " "), row8)
					}
					atLeast := d.wordsAt(where.Short, mutedInk, labelCutAt(labelLeast)).width() + sessionPhoneApart + ansi.StringWidth(d.page.RanSaidWidest(d.now))
					if why := d.wordsAt(where.Short, mutedInk, labelCutAt(cells)).plain(); atLeast <= width && !strings.HasPrefix(row8, why) {
						t.Errorf("%s, at %d columns, row 8 reads %q, want %q whole, as its names at their least let it", case_, width, row8, why)
					}
				}
			}
			if len(widths) != 1 {
				t.Errorf("%s, at %d columns, the short rows' names take %v cells as the session works, want one", case_, width, slices.Collect(maps.Keys(widths)))
			}
		}
		if !between {
			t.Errorf("%s, from 40 to 60 columns, the names are never between a label's least and its most, want them narrowing a cell at a time", case_)
		}
	}
}

// sideNamed is how row names side-hustle-consulting: the word it starts.
func sideNamed(row string) string {
	for word := range strings.FieldsSeq(row) {
		if strings.HasPrefix(word, "side-") {
			return word
		}
	}
	return ""
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

// A time the now rows say is never cut part-way: in every state, with
// short names and long, at every width, each time where says is whole on
// its row, or gone, where even the row's leasts can't hold it.
func TestTheNowRowsNeverCutATime(t *testing.T) {
	for _, world := range pageWorlds {
		for _, state := range nowStates() {
			for width := 40; width <= 220; width++ {
				d, drawn := pageDrawn(pageFrame(t, state.shown, width, 37), world.doc())
				where := d.page.Where(d.now)
				rows := string(drawn[d.l.now]) + "\n" + string(drawn[d.l.now+1])
				for _, p := range slices.Concat(where.OnSince, where.Why, where.Short) {
					if p.Kind != status.PartTime {
						continue
					}
					for cut := 1; cut < len([]rune(p.Text)); cut++ {
						if part := string([]rune(p.Text)[:cut]) + prose.Ellipsis; strings.Contains(rows, part) {
							t.Errorf("%s, %s, at %d columns, the now rows read\n%s\nwant %q whole or gone", world.name, state.name, width, rows, p.Text)
						}
					}
				}
			}
		}
	}
}

// The short form's names narrow toward a label's least before a date, a
// figure or the why is cut: at 73 columns, with long names, an ended
// session says when it moved and why whole, a pin yielding says why whole,
// and an idle session where it runs and since when, each name cut short
// instead, to no fewer than a label's least.
func TestTheShortNowRowsNarrowTheirNamesFirst(t *testing.T) {
	states := nowStates()
	tests := []struct {
		shown *SessionShown
		row   int
		whole string
	}{
		{shown: states[2].shown, row: 1, whole: " from 16:02: its cap"},
		{shown: states[4].shown, row: 1, whole: " has no room"},
		{shown: states[1].shown, row: 0, whole: " ◆ since it started"},
	}
	for _, tt := range tests {
		d, drawn := pageDrawn(pageFrame(t, tt.shown, 73, 37), pageWorlds[1].doc())
		row := strings.TrimRight(string(drawn[d.l.now+tt.row]), " ")
		if d.nowWide || !strings.Contains(row, tt.whole) {
			t.Errorf("at 73 columns, row %d reads %q, want it short, saying %q whole", d.l.now+tt.row+1, row, tt.whole)
		}
		for word := range strings.FieldsSeq(row) {
			if cut := strings.TrimSuffix(word, ","); strings.HasSuffix(cut, prose.Ellipsis) && ansi.StringWidth(cut) < labelLeast {
				t.Errorf("at 73 columns, row %d reads %q, a name cut under a label's least", d.l.now+tt.row+1, row)
			}
		}
	}
}

// A turn's worth is whole at every width to its bound: unpriced, part
// unpriced, and $999.99.
func TestATurnsWorthIsWholeToItsBound(t *testing.T) {
	shown := moved()
	plus := worthOf(11411)
	plus.Unpriced = []string{"claude-fable-1"}
	worths := []views.Worth{{Unpriced: []string{"claude-fable-1"}}, plus, worthOf(99999)}
	for i, w := range worths {
		shown.Page.Turns[i].Worth = w
	}
	for _, world := range pageWorlds {
		for width := 40; width <= 220; width++ {
			d, rows := pageDrawn(pageFrame(t, shown, width, 37), world.doc())
			for i, w := range worths {
				if got := turnText(rows[d.l.turnsAt+i], d, turnWorth); got != w.Cents() {
					t.Errorf("%s, at %d columns, turn %d's worth reads %q, want %q whole", world.name, width, shown.Page.Turns[i].Turn, got, w.Cents())
				}
			}
		}
	}
}

// From 40 columns to 52, in every session the frames draw, a turn's figures
// are whole, and a move between the frames' accounts is named whole: where
// on would narrow, the gaps narrow first, once, to one blank.
func TestANarrowTurnsRowKeepsItsFiguresAndMovesWhole(t *testing.T) {
	sessions := []*SessionShown{moved(), quiet(), ended(), resumed()}
	for width := 40; width <= 52; width++ {
		for _, shown := range sessions {
			d, rows := pageDrawn(pageFrame(t, shown, width, 44), pageDoc())
			shownTurns, _, _ := d.turnsShown()
			for i := range shownTurns {
				row := rows[d.l.turnsAt+i]
				for _, p := range []turnPart{turnStarted, turnRequests, turnWorth, turnOn} {
					if text := turnText(row, d, p); strings.Contains(text, prose.Ellipsis) {
						t.Errorf("at %d columns, turn row %q cuts %q", width, strings.TrimRight(string(row), " "), text)
					}
				}
			}
			gaps := slices.Collect(func(yield func(int) bool) {
				for i := 1; i < len(d.turnColumns); i++ {
					if !yield(d.turnColumns[i].x - d.turnColumns[i-1].end()) {
						return
					}
				}
			})
			if want := turnGap - min(1, max(0, 50-width)); slices.ContainsFunc(gaps, func(g int) bool { return g != want }) {
				t.Errorf("at %d columns, the turns' gaps are %v, want each %d", width, gaps, want)
			}
		}
	}
}

// A turn's tools leave whole from the end, each with its count, never cut
// part-way but for the first where even it doesn't fit.
func TestATurnsToolsLeaveWhole(t *testing.T) {
	shown := moved()
	shown.Page.Turns[0].Tools = []views.ToolCalls{{Tool: "mcp__server__a_tool_named_long", Times: 3}, {Tool: "Read", Times: 2}}
	for width := 40; width <= 220; width++ {
		d, rows := pageDrawn(pageFrame(t, shown, width, 37), pageDoc())
		col, ok := d.turnColumnOf(turnTools)
		if !ok {
			continue
		}
		for i, turn := range shown.Page.Turns[:d.l.turns] {
			got, each := turnText(rows[d.l.turnsAt+i], d, turnTools), turn.ToolsEach()
			whole := len(each) == 0 && got == ""
			for n := 1; n <= len(each); n++ {
				whole = whole || got == turn.ToolsSaidFirst(n)
			}
			if !whole && got != prose.Cut(each[0], col.width) {
				t.Errorf("at %d columns, turn %d's tools read %q, want whole tools, or the first alone cut", width, turn.Turn, got)
			}
		}
	}
}

// No row of a session's page reads "……", a cut landing right after a name
// cut short ending at the name's own ellipsis.
func TestNoRowReadsADoubledEllipsis(t *testing.T) {
	for _, world := range pageWorlds {
		for _, state := range nowStates() {
			for width := 40; width <= 220; width++ {
				_, drawn := pageDrawn(pageFrame(t, state.shown, width, 37), world.doc())
				for y, row := range drawn {
					if strings.Contains(string(row), prose.Ellipsis+prose.Ellipsis) {
						t.Errorf("%s, %s, at %d columns, row %d reads %q", world.name, state.name, width, y+1, strings.TrimRight(string(row), " "))
					}
				}
			}
		}
	}
}

// namesShrank reports whether a turn's on shows a name in fewer cells now
// than it did then, or names more or fewer.
func namesShrank(then, now []int) bool {
	if len(then) != len(now) {
		return true
	}
	for i := range now {
		if now[i] < then[i] {
			return true
		}
	}
	return false
}

// namesShown are the cells of each name on shows, the first and the last,
// an ellipsis that cuts one not counted.
func namesShown(on string) []int {
	names := strings.Split(strings.ReplaceAll(on, moveArrow, moveArrowTight), moveArrowTight)
	shown := make([]int, 0, 2)
	for _, name := range []string{names[0], names[len(names)-1]}[:min(2, len(names))] {
		shown = append(shown, ansi.StringWidth(strings.TrimSuffix(name, prose.Ellipsis)))
	}
	return shown
}

// A move's second name gives way before read, written and out leave: its
// arrow narrowed, then cut with …, its first name whole, down to
// moveToLeast cells; and while any of them shows, it's cut no further. One
// account's name stays whole until they've left.
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
		if spaced := move == "work-team-plan → side-hustle-consulting"; !spaced && !strings.HasPrefix(move, "work-team-plan→si") || one != "work-team-plan" {
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
