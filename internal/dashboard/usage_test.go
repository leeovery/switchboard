package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/theme"
)

// printNow is when the printouts are drawn: Thursday 1 October 2026,
// 14:42:07, an hour east of UTC.
var printNow = time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("UTC+1", 60*60))

// printPolicy judges windows as Claude's are judged.
var printPolicy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

// on is a time on a day of October 2026, in printNow's zone.
func on(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, printNow.Location())
}

// printAccount is an account labelled as its id, read as the document was,
// its 5-hour window, its week and Fable's week used as given.
func printAccount(id string, session, week, fable float64) status.Account {
	return status.Account{
		ID: id, Label: id, TokenSet: true, FetchedAt: printNow.Add(-4 * time.Second).UTC(),
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: session, ResetsAt: on(1, 17, 10).UTC()},
			{Key: "7d", Label: "Week", Utilization: week, ResetsAt: on(4, 21, 0).UTC()},
			{Key: "7d_oi", Label: "Fable week", Utilization: fable, ResetsAt: on(4, 21, 0).UTC()},
		},
	}
}

// trio is the router's document of three accounts, read 4 seconds before
// printNow, as change leaves it, with what's worked out of it: work, capped at
// 95%, at its cap in its 5-hour window; personal, the primary; and side,
// where new sessions go; six sessions.
func trio(change ...func(*status.Document)) status.Document {
	work := printAccount("work", 0.95, 0.34, 0.12)
	work.Reserve = 0.05
	personal := printAccount("personal", 0.39, 0.66, 0)
	personal.Primary = true
	doc := status.Document{
		GeneratedAt: printNow.Add(-4 * time.Second).UTC(), Source: status.SourceRouter, Best: "side", Primary: "personal",
		Router: status.Health{Healthy: true}, Sessions: 6,
		Accounts: []status.Account{work, personal, printAccount("side", 0.13, 0.78, 0.04)},
	}
	for _, c := range change {
		c(&doc)
	}
	return doc.WorkedOut(printPolicy)
}

// account changes doc's account with the given id as change says.
func account(id string, change func(*status.Account)) func(*status.Document) {
	return func(d *status.Document) {
		for i := range d.Accounts {
			if d.Accounts[i].ID == id {
				change(&d.Accounts[i])
			}
		}
	}
}

// pinnedTo pins doc's new sessions to the accounts with the ids given.
func pinnedTo(ids ...string) func(*status.Document) {
	return func(d *status.Document) { d.Pin = status.Pin{Accounts: ids, Since: printNow.UTC()} }
}

// printNames are the names printName gives Claude's windows.
var printNames = map[string]string{"5h": "5-hour", "7d": "Week", "7d_oi": "Fable week", "7d_sonnet": "Sonnet week"}

// printName names a window by its key as Claude's are named, or by its key
// where it isn't one of Claude's.
func printName(key string) string {
	return cmp.Or(printNames[key], key)
}

// printed is doc's printout width columns wide, grouped by, of the windows
// with the keys given, as text alone, its windows named as Claude's are.
func printed(doc status.Document, width int, by By, windows ...string) []string {
	return Printout{Width: width, By: by, Windows: windows, Name: printName}.Draw(doc.At(printNow, printPolicy))
}

// colAt is the n cells of row from column col: fewer where it ends sooner.
func colAt(row string, col, n int) string {
	cells := []rune(row)
	if col >= len(cells) {
		return ""
	}
	return string(cells[col:min(col+n, len(cells))])
}

// blockOn is doc's usage block width cells wide in the theme with the given
// slug, as usage prints it, drawn from row 0.
func blockOn(t *testing.T, doc status.Document, width int, slug string) *canvas {
	t.Helper()
	th, ok := theme.Builtin(slug)
	if !ok {
		t.Fatalf("no built-in theme %s", slug)
	}
	c := growing(width)
	printBlock(doc, width, Print(th, nil)).draw(c, 0)
	return c
}

// printBlock is doc's usage block width cells wide, drawn in look, as usage
// prints it at printNow, by window, of every window, named as Claude's are.
func printBlock(doc status.Document, width int, look Look) usageBlock {
	m := doc.At(printNow, printPolicy)
	return newUsageBlock(m, width, look, ByWindow, usageWindows(m, nil, printName))
}

// windowNowOf is the window with the given key of doc's account with the
// given id, as it stands at the moment given, as claudeLike judges it.
func windowNowOf(doc status.Document, id, key string, at time.Time) status.WindowNow {
	a, _ := doc.At(at, claudeLike).Account(id)
	w, _ := a.Window(key)
	return w
}

// accountNowOf is doc's account with the given id as it stands at the moment
// given, as claudeLike judges it.
func accountNowOf(doc status.Document, id string, at time.Time) status.AccountNow {
	a, _ := doc.At(at, claudeLike).Account(id)
	return a
}

func TestTheMatrixLaysEachWindowOutAsAGroupFromTheNameColumn(t *testing.T) {
	rows := printed(trio(), 160, ByWindow)
	checks := []struct {
		row, col int
		want     string
	}{
		{2, 16, "5-HOUR"}, {2, 64, "WEEK"}, {2, 112, "FABLE WEEK"},
		{4, 0, "1 work "}, {4, 34, "95%"}, {4, 39, "at cap · back 17:10"},
		{6, 0, "2 personal ◆ "}, {8, 0, "3 side ▲ "},
		{9, 0, strings.Repeat("─", 160)},
		{10, 0, "Σ pool "},
		{12, 16, "┃ even pace: where you would be now, used evenly    ╎ cap: work at 95%"},
	}
	for _, c := range checks {
		if got := colAt(rows[c.row], c.col, len([]rune(c.want))); got != c.want {
			t.Errorf("row %d reads %q from column %d, want %q", c.row, got, c.col, c.want)
		}
	}
	if len(rows) != 13 {
		t.Errorf("the printout is %d rows, want 13: the header, a blank, the headings, a blank, three accounts and the pool under a blank each, then the key line", len(rows))
	}
}

func TestTheMatrixsBarsFollowTheRule(t *testing.T) {
	tests := []struct {
		width, windows, names, want int
	}{
		{width: 160, windows: 3, names: 16, want: 16},
		{width: 160, windows: 1, names: 16, want: 88},
		{width: 160, windows: 3, names: 14, want: 17},
		{width: 216, windows: 3, names: 16, want: 35},
		{width: 300, windows: 1, names: 14, want: 90},
		{width: 160, windows: 4, names: 16, want: 4},
	}
	for _, tt := range tests {
		b := usageBlock{width: tt.width, windows: make([]shownWindow, tt.windows), names: tt.names}
		if got := b.matrixBar(); got != tt.want {
			t.Errorf("at %d columns, %d windows beside a %d-cell name column have bars %d cells long, want %d", tt.width, tt.windows, tt.names, got, tt.want)
		}
	}
}

func TestWindowsWhoseMatrixBarsWouldBeUnder12CellsStack(t *testing.T) {
	sonnet := func(a *status.Account) {
		a.Windows = append(a.Windows, quota.Window{Key: "7d_sonnet", Label: "Sonnet week", Utilization: 0.1, ResetsAt: on(4, 21, 0).UTC()})
	}
	four := trio(account("work", sonnet), account("personal", sonnet), account("side", sonnet))
	rows := printed(four, 160, ByWindow)
	var headings []string
	for _, r := range rows {
		if r != "" && strings.ToUpper(r) == r && !strings.ContainsAny(r, "┃╎─Σ") {
			headings = append(headings, r)
		}
	}
	if want := []string{"5-HOUR", "WEEK", "FABLE WEEK", "SONNET WEEK"}; !slices.Equal(headings, want) {
		t.Errorf("four windows at 160 columns are headed %q, want %q, each a section of its own", headings, want)
	}
}

func TestStackedWindowsBarsLeaveTheirWordsTheRowsEnd(t *testing.T) {
	tests := []struct {
		width, percentAt, wordsAt int
		words                     string
	}{
		{width: 149, percentAt: 16 + 103 + 2, wordsAt: 126, words: "at cap · back 17:10"},
		{width: 120, percentAt: 16 + 74 + 2, wordsAt: 97, words: "at cap · back 17:10"},
		{width: 80, percentAt: 16 + 34 + 2, wordsAt: 57, words: "at cap · back 17:10"},
		{width: 52, percentAt: 16 + 14 + 2, wordsAt: 37, words: "back 17:10"},
	}
	for _, tt := range tests {
		rows := printed(trio(), tt.width, ByWindow)
		first := slices.Index(rows, "5-HOUR")
		if first < 0 {
			t.Fatalf("at %d columns, no section is headed 5-HOUR:\n%s", tt.width, strings.Join(rows, "\n"))
		}
		work := rows[first+2]
		if got := colAt(work, 1, 6); got != "1 work" {
			t.Errorf("at %d columns, work's row starts %q a cell in, want %q", tt.width, got, "1 work")
		}
		if got := colAt(work, tt.percentAt, 3); got != "95%" {
			t.Errorf("at %d columns, work's row reads %q from column %d, want its percentage", tt.width, got, tt.percentAt)
		}
		if got := colAt(work, tt.wordsAt, 30); got != tt.words {
			t.Errorf("at %d columns, work's words read %q, want %q, ending a cell short of the edge", tt.width, got, tt.words)
		}
	}
}

func TestByAccountEachAccountIsABlockOfItsWindows(t *testing.T) {
	wide := printed(trio(), 160, ByAccount)
	narrow := printed(trio(), 52, ByAccount)
	tests := []struct {
		name     string
		rows     []string
		row, col int
		want     string
	}{
		{name: "wide, two blank lines under the header", rows: wide, row: 3, col: 0, want: "1 work          5-hour"},
		{name: "wide, its bar 104 cells", rows: wide, row: 3, col: 26 + 104 + 2, want: "95%  at cap · back 17:10"},
		{name: "wide, its week", rows: wide, row: 5, col: 16, want: "Week"},
		{name: "wide, Fable's week", rows: wide, row: 7, col: 16, want: "Fable wk"},
		{name: "wide, two blank lines between blocks", rows: wide, row: 10, col: 0, want: "2 personal ◆    5-hour"},
		{name: "wide, the pool's rule in the second", rows: wide, row: 23, col: 0, want: strings.Repeat("─", 160)},
		{name: "wide, the pool last", rows: wide, row: 24, col: 0, want: "Σ pool          5-hour"},
		{name: "narrow, its name on a line of its own", rows: narrow, row: 5, col: 0, want: "1 work"},
		{name: "narrow, the label a cell in, its bar 18 cells", rows: narrow, row: 6, col: 0, want: " 5-hour"},
		{name: "narrow, its percentage", rows: narrow, row: 6, col: 12 + 18 + 2, want: "95%  back 17:10"},
		{name: "narrow, the next block", rows: narrow, row: 13, col: 0, want: "2 personal ◆"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colAt(tt.rows[tt.row], tt.col, len([]rune(tt.want))); got != tt.want {
				t.Errorf("row %d reads %q from column %d, want %q", tt.row, got, tt.col, tt.want)
			}
		})
	}
	if got := narrow[6]; strings.TrimRight(got, " ") != got || len([]rune(got)) > 51 {
		t.Errorf("a narrow row %q runs to the edge, want it to end a cell short", got)
	}
}

func TestTheNameColumnFitsTheNames(t *testing.T) {
	long := "someone.else@example.com"
	tests := []struct {
		name  string
		names []accountName
		width int
		want  int
	}{
		{name: "short names, at least 14", names: []accountName{{number: "1", label: "work"}}, width: 160, want: 14},
		{name: "short names under 80, at least 12", names: []accountName{{number: "1", label: "work"}}, width: 79, want: 12},
		{name: "the primary's mark and ▲'s room", names: []accountName{{number: "1", label: "work"}, {number: "2", label: "personal", primary: true}}, width: 160, want: 16},
		{name: "▲ counted once, kept or drawn", names: []accountName{{number: "2", label: "personal", primary: true, best: true}}, width: 160, want: 16},
		{name: "a long label, at most 24", names: []accountName{{number: "2", label: long, primary: true}}, width: 80, want: 24},
		{name: "a long label under 80, at most 16", names: []accountName{{number: "2", label: long, primary: true}}, width: 79, want: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nameColumn(tt.names, tt.width); got != tt.want {
				t.Errorf("the name column is %d cells, want %d", got, tt.want)
			}
		})
	}
}

func TestALongLabelIsCutKeepingItsNumberItsMarksAndRoomForTheBestsMark(t *testing.T) {
	n := accountName{number: "2", label: "someone.else@example.com", primary: true}
	elsewhere := n.line(24).plain()
	n.best = true
	here := n.line(24).plain()
	if want := "2 someone.else@ex… ◆"; elsewhere != want {
		t.Errorf("the name reads %q in a 24-cell column, want %q", elsewhere, want)
	}
	if want := elsewhere + " ▲"; here != want {
		t.Errorf("with ▲ the name reads %q, want %q: its label as it was", here, want)
	}
	if got := len([]rune(here)) + nameGap; got > 24 {
		t.Errorf("with ▲ the name and its gap take %d cells, more than its column's 24", got)
	}
}

func TestALoneAccountCarriesTheBestsMarkAndTheHeaderNamesItWhateverItsRoom(t *testing.T) {
	tests := []struct {
		name   string
		change func(*status.Document)
	}{
		{name: "with room", change: func(d *status.Document) { d.Best = "work" }},
		{name: "its room spent", change: func(d *status.Document) { d.Best = "" }},
		{name: "nothing read of it yet", change: func(d *status.Document) {
			d.Best, d.Accounts[0].FetchedAt = "", time.Time{}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := printed(trio(func(d *status.Document) { d.Accounts = d.Accounts[:1] }, tt.change), 160, ByWindow)
			if got, want := colAt(rows[4], 0, 9), "1 work ▲ "; got != want {
				t.Errorf("the lone account's row starts %q, want %q", got, want)
			}
			if want := "new sessions → ▲ work"; !strings.HasSuffix(rows[0], want) {
				t.Errorf("the header reads %q, want it to end %q", rows[0], want)
			}
		})
	}
}

func TestEveryAccountIsNumberedRightAlignedOnceThereAreTen(t *testing.T) {
	ten := trio(func(d *status.Document) {
		for i := len(d.Accounts); i < 10; i++ {
			d.Accounts = append(d.Accounts, printAccount(fmt.Sprintf("extra%d", i+1), 0.1, 0.2, 0))
		}
	})
	rows := printed(ten, 160, ByWindow)
	starts := map[string]bool{}
	for _, r := range rows {
		starts[colAt(r, 0, 4)] = true
	}
	for _, want := range []string{" 1 w", " 9 e", "10 e", " Σ p"} {
		if !starts[want] {
			t.Errorf("no row starts %q, the numbers and Σ right-aligned in two cells", want)
		}
	}
}

func TestTheWindowsGivenPrintAlone(t *testing.T) {
	tests := []struct {
		name    string
		windows []string
		want    []string
	}{
		{name: "none given: every one, in Claude's order", want: []string{"5-HOUR", "WEEK", "FABLE WEEK"}},
		{name: "two given", windows: []string{"5h", "7d_oi"}, want: []string{"5-HOUR", "FABLE WEEK"}},
		{name: "the week alone", windows: []string{"7d"}, want: []string{"WEEK"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, r := range printed(trio(), 120, ByWindow, tt.windows...) {
				if slices.Contains([]string{"5-HOUR", "WEEK", "FABLE WEEK"}, r) {
					got = append(got, r)
				}
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the sections are headed %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAnAccountWithNothingReadOfAWindowHasItsBarEmptyAndNoMarks(t *testing.T) {
	tests := []struct {
		name   string
		change func(*status.Account)
		group  int
		words  string
	}{
		{name: "no usable token", change: func(a *status.Account) { a.TokenSet = false }, group: 0, words: "no token"},
		{name: "nothing read yet", change: func(a *status.Account) { a.FetchedAt, a.Windows = time.Time{}, nil }, group: 0, words: "not read yet"},
		{name: "a window it hasn't reported", change: func(a *status.Account) { a.Windows = a.Windows[:2] }, group: 2, words: "not read yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := printed(trio(account("work", tt.change)), 160, ByWindow)[4]
			at := 16 + tt.group*48
			if got := colAt(row, at, 16); strings.TrimSpace(got) != "" {
				t.Errorf("work's bar reads %q, want it empty, with no marks", got)
			}
			if got := colAt(row, at+17, 4); got != "   —" {
				t.Errorf("work's percentage reads %q, want —", got)
			}
			if got := colAt(row, at+23, len([]rune(tt.words))); got != tt.words {
				t.Errorf("work's words read %q, want %q", got, tt.words)
			}
		})
	}
	c := blockOn(t, trio(account("work", func(a *status.Account) { a.TokenSet = false })), 160, "nord")
	if got := c.at(36, 2).ink; got != faintInk {
		t.Errorf("an account without a token has its percentage in %+v, want text.faint", got)
	}
}

func TestAStaleReadingShowsAsItWasLastReadFaded(t *testing.T) {
	stale := trio(account("work", func(a *status.Account) {
		a.Error, a.FetchedAt = "HTTP 529 · Overloaded", on(1, 14, 20).UTC()
	}))
	if got := colAt(printed(stale, 160, ByWindow)[4], 34, 25); got != "95%  not read since 14:20" {
		t.Errorf("a stale account's window reads %q, want its last reading, not read since then", got)
	}
	c := blockOn(t, stale, 160, "nord")
	for x := 16; x < 32; x++ {
		k := c.at(x, 2).ink
		if k.bold || k.fade != staleFade || (k.on != (hue{}) && k.on.fade != staleFade) {
			t.Errorf("cell %d of its bar is drawn in %+v, want it faded to 35%% of its colour, its surface too, nothing bold", x-16, k)
		}
	}
	for _, x := range []int{34, 39} {
		if got := c.at(x, 2).ink; got != dimInk {
			t.Errorf("its percentage and words are drawn in %+v at column %d, want text.subtle", got, x)
		}
	}
}

// USAGE marks a window's even pace as it stands: on the tick after work's
// session resets, before it's read again, the session has lapsed, and its
// bar has no pace mark, as once the router's next document has it empty,
// without a reset, rather than the mark the document worked out at its end.
func TestOnTheTickASessionLapsesUSAGEMarksNoEvenPaceOnIt(t *testing.T) {
	doc := threeRouted()
	session, _ := doc.Accounts[0].Window("5h")
	after := session.ResetsAt.Add(time.Second)

	if g := accountGauge(accountNowOf(doc, "work", after), "5h", after); g.bar.pace != nil {
		t.Errorf("work's session is marked at even pace %v, want no mark, as it has lapsed", *g.bar.pace)
	}
}

// USAGE words a window's allowance as it stands, as of when the document was
// read: on the tick after work's week resets, before it's read again, the
// week counts from empty, its allowance its whole room to its next reset, a
// week on, not the room it had left before, till a time already past.
func TestOnTheTickAWeekResetsUSAGEWordsItsAllowanceFromEmpty(t *testing.T) {
	doc := routerDoc("work", 0, readAccount("work", sessionOf(0.1, 4*time.Hour), weekOf(0.6, 2*time.Hour)))
	after := now.Add(2*time.Hour + time.Second)

	g := accountGauge(accountNowOf(doc, "work", after), "7d", after)
	if got, want := g.words.full.plain(), "14%/day till Mon 15:12"; got != want {
		t.Errorf("work's week's words read %q, want %q", got, want)
	}
}

func TestThePoolIsLastAndCountsTheAccountsNewSessionsCanGoTo(t *testing.T) {
	pinned := trio(pinnedTo("personal", "side"))
	rows := printed(pinned, 160, ByWindow)
	if got := colAt(rows[10], 39, 22); !strings.HasSuffix(got, " of 2 left · pinned") {
		t.Errorf("the pool's first window's words read %q, want its room of 2, pinned", got)
	}
	if got := colAt(rows[10], 87, 22); strings.Contains(got, "pinned") || !strings.Contains(got, " of 2 left") {
		t.Errorf("the pool's second window's words read %q, want its room of 2, not saying pinned again", got)
	}
	c := blockOn(t, pinned, 160, "nord")
	if got := c.at(0, 2).ink; got != faintInk {
		t.Errorf("work, out of the pool, is numbered in %+v, want text.faint", got)
	}
	if got := c.at(0, 4).ink; got != dimInk {
		t.Errorf("personal, in the pool, is numbered in %+v, want text.subtle", got)
	}
	if got := printed(trio(func(d *status.Document) { d.Accounts = d.Accounts[:1] }), 160, ByWindow); slices.ContainsFunc(got, func(r string) bool {
		return strings.Contains(r, "Σ") || strings.Contains(r, "─")
	}) {
		t.Errorf("one account's printout has a pool:\n%s", strings.Join(got, "\n"))
	}
}

func TestThePoolSaysItsPinnedOnTheFirstWindowItHasWordsFor(t *testing.T) {
	noSession := func(a *status.Account) { a.Windows = a.Windows[1:] }
	pinned := trio(pinnedTo("personal", "side"), account("personal", noSession), account("side", noSession))
	rows := printed(pinned, 160, ByWindow)
	if got := colAt(rows[10], 39, 22); strings.TrimSpace(got) != "" {
		t.Errorf("the pool's 5-hour window, which its accounts don't report, reads %q, want no words", got)
	}
	if got := colAt(rows[10], 87, 22); !strings.HasSuffix(got, " of 2 left · pinned") {
		t.Errorf("the pool's week, the first window it has words for, reads %q, want its room of 2, pinned", got)
	}
	if got := colAt(rows[10], 135, 22); strings.Contains(got, "pinned") || !strings.Contains(got, " of 2 left") {
		t.Errorf("the pool's Fable week reads %q, want its room of 2, not saying pinned again", got)
	}
}

func TestAPinSpendsTheCapsOfTheAccountsItNamesKeepingTheirMarks(t *testing.T) {
	for _, tt := range []struct {
		name   string
		doc    status.Document
		capped bool
	}{
		{name: "on auto", doc: trio(), capped: true},
		{name: "pinned to work", doc: trio(pinnedTo("work"))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			row := printed(tt.doc, 160, ByWindow)[4]
			if got := strings.HasPrefix(colAt(row, 39, 22), "at cap"); got != tt.capped {
				t.Errorf("work's 5-hour window reads %q: at cap %t, want %t", colAt(row, 39, 22), got, tt.capped)
			}
			if got := colAt(row, 16+15, 1); got != capMark {
				t.Errorf("work's bar has %q at cell 15, want its cap's mark", got)
			}
		})
	}
}

func TestABarOfNoCellsDrawsNoMarkOutsideIt(t *testing.T) {
	tests := []struct {
		name  string
		by    By
		width int
	}{
		{name: "stacked, the bar just gone", by: ByWindow, width: 38},
		{name: "stacked, narrower still", by: ByWindow, width: 30},
		{name: "by account, the bar just gone", by: ByAccount, width: 34},
		{name: "by account, narrower still", by: ByAccount, width: 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := printed(trio(), tt.width, tt.by)
			block := rows[:len(rows)-2]
			if i := slices.IndexFunc(block, func(r string) bool { return strings.ContainsAny(r, paceMark+capMark) }); i >= 0 {
				t.Errorf("at %d columns, with no cells to its bars, row %d draws a mark: %q", tt.width, i, block[i])
			}
		})
	}
}

func TestTheKeyLineSaysWhatTheMarksMean(t *testing.T) {
	pace := "┃ even pace: where you would be now, used evenly"
	capped := func(id string, reserve float64) func(*status.Document) {
		return account(id, func(a *status.Account) { a.Reserve = reserve })
	}
	tests := []struct {
		name  string
		doc   status.Document
		width int
		by    By
		want  []string
	}{
		{name: "from 150 columns, one line from the name column", doc: trio(), width: 160, want: []string{strings.Repeat(" ", 16) + pace + "    ╎ cap: work at 95%"}},
		{name: "narrower, two lines from column 0", doc: trio(), width: 149, want: []string{pace, "╎ cap: work at 95%"}},
		{name: "no account with a cap", doc: trio(capped("work", 0)), width: 120, want: []string{pace}},
		{name: "several caps, in the config's order", doc: trio(capped("side", 0.1)), width: 120, want: []string{pace, "╎ cap: work at 95%, side at 90%"}},
		{
			name: "a cap whose mark is off the bars, unnamed", doc: trio(capped("work", 0.004)), width: 160,
			want: []string{strings.Repeat(" ", 16) + pace},
		},
		{
			name: "the same cap, named where the bars are long enough to draw its mark", doc: trio(capped("work", 0.004)), width: 300, by: ByAccount,
			want: []string{strings.Repeat(" ", 16) + pace + "    ╎ cap: work at 100%"},
		},
		{
			name: "a cap of an account nothing is read of, its bars empty, unnamed", width: 120, want: []string{pace},
			doc: trio(account("work", func(a *status.Account) { a.FetchedAt, a.Windows = time.Time{}, nil })),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := printed(tt.doc, tt.width, tt.by)
			got := rows[len(rows)-len(tt.want):]
			if !slices.Equal(got, tt.want) || rows[len(rows)-len(tt.want)-1] != "" {
				t.Errorf("the printout ends\n%s\nwant a blank line, then\n%s", strings.Join(rows[len(rows)-len(tt.want)-1:], "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

func TestEveryAccountKeepsItsRowsBeforeAnyWindowIsRead(t *testing.T) {
	unread := func(a *status.Account) { a.FetchedAt, a.Windows = time.Time{}, nil }
	doc := trio(func(d *status.Document) { d.Accounts, d.Best = d.Accounts[:2], "" }, account("work", unread), account("personal", unread))
	names := map[By][]string{ByWindow: {"5-HOUR", "WEEK"}, ByAccount: {"5-hour", "Week"}}
	for _, by := range []By{ByWindow, ByAccount} {
		for _, width := range []int{160, 80} {
			t.Run(fmt.Sprintf("by %d at %d columns", by, width), func(t *testing.T) {
				text := strings.Join(printed(doc, width, by), "\n")
				for _, want := range append([]string{"1 work", "2 personal ◆"}, names[by]...) {
					if !strings.Contains(text, want) {
						t.Errorf("the printout has no %q:\n%s", want, text)
					}
				}
				if got := strings.Count(text, "not read yet"); got != 4 {
					t.Errorf("the printout reads \"not read yet\" %d times, want 4, each account's 5-hour window and week:\n%s", got, text)
				}
			})
		}
	}
}

func TestUsageShowsTheSharedWindowsAndEveryOneReportedOnceInClaudesOrder(t *testing.T) {
	unread := func(a *status.Account) { a.FetchedAt, a.Windows = time.Time{}, nil }
	tests := []struct {
		name   string
		doc    status.Document
		shared []string
		want   []string
	}{
		{name: "every one reported", doc: trio(), shared: []string{"5h", "7d"}, want: []string{"5h", "7d", "7d_oi"}},
		{name: "one shared that no account reports", doc: trio(), shared: []string{"5h", "1d"}, want: []string{"5h", "1d", "7d", "7d_oi"}},
		{
			name: "the shared alone, before any is read", shared: []string{"7d", "5h"}, want: []string{"5h", "7d"},
			doc: trio(account("work", unread), account("personal", unread), account("side", unread)),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := printPolicy
			policy.Shared = tt.shared
			if got := UsageWindows(tt.doc.At(printNow, policy)); !slices.Equal(got, tt.want) {
				t.Errorf("UsageWindows() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheWindowsAreNamedAsNameNamesThem(t *testing.T) {
	tests := []struct {
		name   string
		namer  func(string) string
		shared []string
		want   []shownWindow
	}{
		{
			name: "by Name, over their labels", namer: printName,
			want: []shownWindow{{"5h", "5-hour"}, {"7d", "Week"}, {"7d_oi", "Fable week"}},
		},
		{
			name: "by Name, cleaned", namer: func(key string) string { return "\x1b" + key + "\a" },
			want: []shownWindow{{"5h", "5h"}, {"7d", "7d"}, {"7d_oi", "7d_oi"}},
		},
		{
			name: "without Name, by their labels",
			want: []shownWindow{{"5h", "Session"}, {"7d", "Week"}, {"7d_oi", "Fable week"}},
		},
		{
			name: "a window every model shares that no account reports, by Name", namer: printName, shared: []string{"5h", "1d"},
			want: []shownWindow{{"5h", "5-hour"}, {"1d", "1d"}, {"7d", "Week"}, {"7d_oi", "Fable week"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := printPolicy
			policy.Shared = tt.shared
			m := trio().At(printNow, policy)
			if got := usageWindows(m, nil, Printout{Name: tt.namer}.namer(m)); !slices.Equal(got, tt.want) {
				t.Errorf("the windows are %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWordsACellTooLongInFullAreShortLeavingTheCellsAfterThemBlank(t *testing.T) {
	// Work's week at its cap reads "at cap · back Sun 21:00" in full: 23
	// cells, a cell more than the 22 the words have.
	capped := trio(account("work", func(a *status.Account) { a.Windows[1].Utilization = 0.96 }))
	tests := []struct {
		name      string
		width     int
		by        By
		row       int
		wordsAt   int
		blankFrom int
		blank     int
	}{
		{name: "the matrix, the 3 cells before the next group", width: 160, by: ByWindow, row: 4, wordsAt: 87, blankFrom: 87 + 22, blank: 3},
		{name: "stacked, the last column", width: 120, by: ByWindow, row: 15, wordsAt: 97, blankFrom: 119, blank: 1},
		{name: "by account, wide, the last column", width: 160, by: ByAccount, row: 5, wordsAt: 137, blankFrom: 159, blank: 1},
		{name: "by account, narrow, the last column", width: 100, by: ByAccount, row: 8, wordsAt: 77, blankFrom: 99, blank: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := printed(capped, tt.width, tt.by)
			row := rows[tt.row]
			if got := strings.TrimRight(colAt(row, tt.wordsAt, 22), " "); got != "back Sun" {
				t.Fatalf("work's week reads %q from column %d, want %q, short, as in full it runs a cell over:\n%s", got, tt.wordsAt, "back Sun", strings.Join(rows, "\n"))
			}
			if got := colAt(row, tt.blankFrom, tt.blank); strings.TrimSpace(got) != "" {
				t.Errorf("row %d reads %q from column %d, want it blank", tt.row, got, tt.blankFrom)
			}
		})
	}
}

func TestAProbedPrintoutMarksNoneOfSeveralAccountsAsWhereNewSessionsGo(t *testing.T) {
	probed := func(d *status.Document) {
		d.Source, d.Fallback = status.SourceProbe, status.Fallback{Router: status.RouterNotRunning}
	}
	if rows := printed(trio(probed), 160, ByWindow); slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, "▲") }) {
		t.Errorf("a probed printout of three accounts marks one ▲:\n%s", strings.Join(rows, "\n"))
	}
	lone := trio(probed, func(d *status.Document) { d.Accounts = d.Accounts[:1] })
	if got, want := colAt(printed(lone, 160, ByWindow)[4], 0, 9), "1 work ▲ "; got != want {
		t.Errorf("a probed printout's lone account's row starts %q, want %q, as there's nowhere else new sessions go", got, want)
	}
}

// USAGE says of each window what the moment has of it, at each of the hard
// moments: its words say its Kind, in its colour, at its At; its bar is empty,
// stale, filled and marked as the moment has it; and ▲ marks the account the
// moment's Next names.
func TestUSAGEAgreesWithTheMomentAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, claudeLike)
			b := newUsageBlock(m, 160, Look{}, ByWindow, usageWindows(m, nil, labelsOf(m)))
			for i, a := range m.Accounts {
				row := b.rows[i]
				if marked := strings.Contains(row.name.plain(), "▲"); marked != a.Next {
					t.Errorf("%s's row is marked ▲ %t, want %t, as the moment's Next is %q", a.ID, marked, a.Next, m.Next)
				}
				for j, shown := range b.windows {
					w, _ := a.Window(shown.key)
					checkGaugeAgrees(t, a, w, row.gauges[j], m.Now)
				}
			}
		})
	}
}

// checkGaugeAgrees checks that the gauge g USAGE draws of account a's window
// w, as it stands at now, says what w says.
func checkGaugeAgrees(t *testing.T, a status.AccountNow, w status.WindowNow, g gauge, now time.Time) {
	t.Helper()
	full := g.words.full.plain()
	if said, ink := kindSaid(full); said != w.Kind || len(g.words.full) > 0 && g.words.full[0].ink != ink {
		t.Errorf("%s's %s reads %q, as of kind %d in %+v, want kind %d", a.ID, w.Key, full, said, g.words.full[0].ink, w.Kind)
	}
	if at := timeSaid(w, now); at != "" && !strings.HasSuffix(full, at) {
		t.Errorf("%s's %s reads %q, want it to end at %q, its At", a.ID, w.Key, full, at)
	}
	if g.runsOut != w.RunsOut {
		t.Errorf("%s's %s runs out %t, want %t", a.ID, w.Key, g.runsOut, w.RunsOut)
	}
	want := usageBar{used: w.Used, capAt: a.Cap, pace: w.Pace, stale: w.Kind == status.WindowStale}
	if drawsEmpty(w.Kind) {
		want = usageBar{empty: true}
	}
	if got := g.bar; got.empty != want.empty || got.stale != want.stale || got.used != want.used || got.capAt != want.capAt || paceAt(got.pace) != paceAt(want.pace) {
		t.Errorf("%s's %s bar is %+v, pace %s, want %+v, pace %s", a.ID, w.Key, got, paceAt(got.pace), want, paceAt(want.pace))
	}
}

// kindSaid is the kind of window USAGE's words, in full, say it is, and the
// ink they say it in.
func kindSaid(full string) (status.WindowKind, ink) {
	for _, said := range []struct {
		lead string
		kind status.WindowKind
		ink  ink
	}{
		{"no token", status.WindowTokenless, errorInk},
		{"not read yet", status.WindowUnread, dimInk},
		{"not read since ", status.WindowStale, dimInk},
		{"limit", status.WindowLimited, errorInk},
		{"at cap", status.WindowCapped, warningInk},
		{"out ~", status.WindowRunsOut, warningInk},
		{"not started", status.WindowNotStarted, mutedInk},
	} {
		if strings.HasPrefix(full, said.lead) {
			return said.kind, said.ink
		}
	}
	return status.WindowOpen, mutedInk
}

// timeSaid is when USAGE's words, in full, say what w's Kind says comes, or
// came, as of now: "" where they say no time.
func timeSaid(w status.WindowNow, now time.Time) string {
	switch {
	case w.At.IsZero(), w.Kind == status.WindowOpen && w.Allowance == (quota.Allowance{}):
		return ""
	case w.Kind == status.WindowStale:
		full, _ := lastRead(now, w.At)
		return full
	}
	return prose.Dated(now, w.At)
}

// paceAt shows an even pace, "none" where it's nil.
func paceAt(pace *float64) string {
	if pace == nil {
		return "none"
	}
	return fmt.Sprint(*pace)
}
