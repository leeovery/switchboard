package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// accountsOf are n accounts read a minute ago, each with a session and a
// week, and Fable's week, used where fable is set, else unused, and so
// hidden from every card. A recent event is told of.
func accountsOf(n int, fable bool) status.Document {
	used := 0.0
	if fable {
		used = 0.1
	}
	accounts := make([]status.Account, n)
	for i := range accounts {
		accounts[i] = readAccount(string(rune('a'+i)), sessionOf(0.2, 3*time.Hour), weekOf(0.3, 4*day), fableOf(used, 4*day))
	}
	doc := routerDoc("a", 0, accounts...)
	doc.Events = []status.Event{{ID: 1, At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "a"}}
	return doc
}

// laidOut is the Accounts view of doc laid out in the frame f, under its
// title and heading.
func laidOut(f Frame, doc status.Document) layout {
	top := f.above(newCanvas(f.Width, f.Height), doc, now)
	return f.layOut(doc, now, len(shownWindows(doc, now, f.Policy)), top)
}

func TestTheLayoutRulesAt160Columns(t *testing.T) {
	full, mid6, compact3, compact2 := density{fullCard, 4}, density{midCard, 6}, density{compactCard, 3}, density{compactCard, 2}
	tests := []struct {
		accounts int
		// rows are what the rules choose at 28, 40 and 50 rows: the density,
		// and whether the cards scroll.
		rows [3]density
		// scrolls are whether the cards scroll at each.
		scrolls [3]bool
	}{
		{accounts: 2, rows: [3]density{mid6, full, full}},
		{accounts: 3, rows: [3]density{mid6, full, full}},
		{accounts: 4, rows: [3]density{compact3, mid6, full}},
		{accounts: 5, rows: [3]density{compact3, mid6, full}},
		{accounts: 6, rows: [3]density{compact3, mid6, full}},
		{accounts: 7, rows: [3]density{compact2, compact3, mid6}, scrolls: [3]bool{true}},
		{accounts: 8, rows: [3]density{compact2, compact3, mid6}, scrolls: [3]bool{true}},
	}
	for _, tt := range tests {
		// The frames draw two and three accounts with Fable's week used, and
		// more without, as the design's table counts them.
		doc := accountsOf(tt.accounts, tt.accounts <= 3)
		for i, height := range []int{28, 40, 50} {
			l := laidOut(frameOf(160, height), doc)
			if l.density != tt.rows[i] || l.scrolls() != tt.scrolls[i] {
				t.Errorf("%d accounts at 160×%d: %+v, scrolling %v; want %+v, scrolling %v", tt.accounts, height, l.density, l.scrolls(), tt.rows[i], tt.scrolls[i])
			}
		}
	}
}

func TestCardsShareTheSpareWidth(t *testing.T) {
	tests := []struct {
		name            string
		width, accounts int
		// want is where each of the first row's cards starts, and wantWidth
		// how wide each is.
		want      []int
		wantWidth int
	}{
		{name: "160 columns hold 3", width: 160, accounts: 3, want: []int{1, 55, 109}, wantWidth: 50},
		{name: "never more than there are", width: 160, accounts: 2, want: []int{1, 82}, wantWidth: 77},
		{name: "two, sharing what a third would take", width: 120, accounts: 3, want: []int{1, 62}, wantWidth: 57},
		{name: "210 hold 4, closer together", width: 210, accounts: 4, want: []int{1, 53, 105, 157}, wantWidth: 50},
		{name: "214 hold 4 as far apart as 160 hold 3", width: 214, accounts: 4, want: []int{1, 55, 109, 163}, wantWidth: 50},
		{name: "a phone 1", width: 52, accounts: 3, want: []int{1}, wantWidth: 50},
		{name: "under 50 columns of room, one the whole width", width: 40, accounts: 3, want: []int{1}, wantWidth: 38},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := frameOf(tt.width, 60).gridOf(tt.accounts, 2)
			var starts []int
			for i := range g.across {
				x, _ := g.at(i, density{fullCard, 4})
				starts = append(starts, x)
			}
			if !slices.Equal(starts, tt.want) || g.width != tt.wantWidth {
				t.Errorf("%d accounts at %d columns: cards start at %v, %d wide; want %v, %d wide", tt.accounts, tt.width, starts, g.width, tt.want, tt.wantWidth)
			}
		})
	}
}

func TestOneAccountsWideCardHasAColumnBesideItFrom150Columns(t *testing.T) {
	doc := routerDoc("work", 3, pressedAccount("work"))
	doc.Events = []status.Event{{ID: 1, At: now.Add(-4 * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"}}
	tests := []struct {
		width      int
		wantBeside bool
		wantWidth  int
		// wantStrips are the strips' labels under the card.
		wantStrips []string
	}{
		{width: 160, wantBeside: true, wantWidth: wideCard},
		{width: 150, wantBeside: true, wantWidth: wideCard},
		{width: 149, wantWidth: 147, wantStrips: []string{"COMING UP", "RECENT"}},
	}
	for _, tt := range tests {
		l := laidOut(frameOf(tt.width, 40), doc)
		var strips []string
		for _, s := range l.strips {
			strips = append(strips, s.label)
		}
		if l.beside != tt.wantBeside || l.width != tt.wantWidth || !slices.Equal(strips, tt.wantStrips) {
			t.Errorf("at %d columns, a card %d wide, a column beside it: %v, strips %q; want %d wide, %v, strips %q", tt.width, l.width, l.beside, strips, tt.wantWidth, tt.wantBeside, tt.wantStrips)
		}
		if want := (density{fullCard, wideChart}); l.density != want {
			t.Errorf("at %d columns, one account's card is %+v, want %+v, its chart 6 rows tall", tt.width, l.density, want)
		}
	}
}

func TestOneAccountsColumnHasCOMINGUPAndRECENTLevelWithItsChart(t *testing.T) {
	doc := routerDoc("work", 3, pressedAccount("work"))
	doc.Prime = status.Prime{Day: "08:00-22:00", Slots: []status.Slot{{Account: "work", At: "03:50", Next: now.Add(14 * time.Hour).UTC()}}}
	doc.Events = []status.Event{
		{ID: 2, At: now.Add(-4 * time.Minute).UTC(), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: now.Add(90 * time.Minute).UTC()},
		{ID: 1, At: now.Add(-time.Hour).UTC(), Kind: status.EventRestart, Reason: "upgraded"},
	}
	rows := frameOf(160, 40).Draw(doc.WorkedOut(claudeLike), now)

	column := func(row int) string {
		cells := []rune(rows[row])
		return strings.TrimRight(string(cells[min(110, len(cells)):]), " ")
	}
	for _, tt := range []struct {
		row  int
		want string
	}{
		{row: 4, want: "COMING UP"},
		{row: 5, want: "14:42  work runs out at its pace        in 1h 30m"},
		{row: 6, want: "16:12  work's session resets                in 3h"},
		{row: 7, want: "03:12  work is primed                      in 14h"},
		{row: 8, want: ""},
		{row: 11, want: "RECENT"},
		{row: 12, want: "13:08  ● work"},
		{row: 13, want: "          came under pressure: its session runs…"},
		{row: 14, want: "12:12  ! restart due (upgraded)"},
		{row: 15, want: ""},
	} {
		if got := column(tt.row); got != tt.want {
			t.Errorf("row %d of the column reads %q, want %q", tt.row+1, got, tt.want)
		}
	}
}

func TestRECENTTakesTheFirstEmptyCellOfTheLastRow(t *testing.T) {
	tests := []struct {
		name     string
		accounts int
		// wantCell is set where RECENT takes a cell, and the cell's column.
		wantCell bool
		column   int
	}{
		{name: "four, three across", accounts: 4, wantCell: true, column: 56},
		{name: "five", accounts: 5, wantCell: true, column: 110},
		{name: "eight", accounts: 8, wantCell: true, column: 110},
		{name: "three: a strip", accounts: 3},
		{name: "six: a strip", accounts: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := frameOf(160, 50).Draw(accountsOf(tt.accounts, false), now)
			strip := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, " RECENT   13:11") })
			cell := slices.IndexFunc(rows, func(r string) bool { return strings.HasSuffix(r, " RECENT") })
			switch {
			case tt.wantCell && (cell < 0 || strip >= 0):
				t.Fatalf("rows are\n%s\nwant RECENT in an empty cell", strings.Join(rows, "\n"))
			case !tt.wantCell && (strip < 0 || cell >= 0):
				t.Fatalf("rows are\n%s\nwant RECENT a strip under the cards", strings.Join(rows, "\n"))
			case !tt.wantCell:
				return
			}
			if got := strings.Index(rows[cell], "RECENT"); len([]rune(rows[cell][:got])) != tt.column {
				t.Errorf("RECENT is at column %d, want %d, a cell in", len([]rune(rows[cell][:got])), tt.column)
			}
			if got, want := strings.TrimSpace(string([]rune(rows[cell+1])[tt.column:])), "13:11  ● a has room again"; got != want {
				t.Errorf("under RECENT, %q, want %q", got, want)
			}
		})
	}
}

func TestTheRowsLeftOverGrowTheRecentStripThenShowTheKey(t *testing.T) {
	doc := accountsOf(3, true)
	doc.Events = nil
	for i := range 4 {
		doc.Events = append(doc.Events, status.Event{ID: 4 - i, At: now.Add(-time.Duration(i+1) * time.Minute).UTC(), Kind: status.EventRoom, Account: "a"})
	}
	tests := []struct {
		height int
		// wantRecent is how many lines RECENT's strip shows, and wantKey
		// whether the key line shows, else ? for the key.
		wantRecent int
		wantKey    bool
	}{
		{height: 29, wantRecent: 1},
		{height: 30, wantRecent: 2},
		{height: 32, wantRecent: 4},
		{height: 33, wantRecent: 4},
		{height: 34, wantRecent: 4, wantKey: true},
		{height: 50, wantRecent: 4, wantKey: true},
	}
	for _, tt := range tests {
		f := frameOf(160, tt.height)
		l := laidOut(f, doc)
		if l.density != densities[0] || len(l.strips) != 1 || l.strips[0].lines != tt.wantRecent || l.key != tt.wantKey {
			t.Errorf("at 160×%d: %+v cards, RECENT %+v, key %v; want full cards, RECENT %d lines, key %v", tt.height, l.density, l.strips, l.key, tt.wantRecent, tt.wantKey)
		}
		rows := f.Draw(doc, now)
		key, over := rows[tt.height-3], rows[tt.height-2]
		if tt.wantKey != strings.HasPrefix(key, " KEY      ▆▆ room left") || tt.wantKey == strings.HasSuffix(over, "? for the key") {
			t.Errorf("at 160×%d, the rows over the footer are\n%s\n%s\nwant the key line: %v", tt.height, key, over, tt.wantKey)
		}
	}
}

func TestTheKeyLineExplainsNothingBeforeThereAreCards(t *testing.T) {
	rows := frameOf(160, 40).Draw(status.Document{}, now)
	if view := strings.Join(rows[7:39], "\n"); strings.TrimSpace(view) != "" {
		t.Errorf("before anything is read, the view reads\n%s\nwant it blank, the key with no cards to explain", view)
	}
}

func TestTheKeyLineShowsOnlyWhereItFitsTheWidth(t *testing.T) {
	doc := accountsOf(3, true)
	for _, tt := range []struct {
		width   int
		wantKey bool
	}{{width: 160, wantKey: true}, {width: 147, wantKey: true}, {width: 146}, {width: 52}} {
		f := frameOf(tt.width, 80)
		rows := f.Draw(doc, now)
		if got := laidOut(f, doc).key; got != tt.wantKey || got == strings.HasSuffix(rows[78], "? for the key") {
			t.Errorf("at %d columns, the key line shows: %v, and the line over the footer reads %q; want it shown: %v", tt.width, got, rows[78], tt.wantKey)
		}
	}
}

func TestTheCardsScrollOnceTwoRowChartsDontFit(t *testing.T) {
	f := frameOf(160, 28)
	doc := accountsOf(8, false)
	if got, want := f.Scrolling(doc, now), (Scrolling{Most: 4, Page: 19}); got != want {
		t.Fatalf("Scrolling() = %+v, want %+v: three rows of 7, a blank between, in the 19 under the heading", got, want)
	}
	tests := []struct {
		name   string
		scroll int
		// wantBar is the scrollbar, top to bottom, wantFirst the first row
		// of the cards shown, and wantSays what the line over the footer says.
		wantBar, wantFirst, wantSays string
	}{
		{
			name: "at the top", scroll: 0, wantBar: strings.Repeat("┃", 15) + strings.Repeat("│", 4),
			wantFirst: " ╭─ 1 a ", wantSays: "▼ 2 more accounts below · j/k or wheel to scroll",
		},
		{
			name: "part way", scroll: 2, wantBar: strings.Repeat("│", 2) + strings.Repeat("┃", 15) + strings.Repeat("│", 2),
			wantFirst: " │  Week     30% ", wantSays: "▲ 3 more accounts above · ▼ 2 more accounts below · j/k or wheel to scroll",
		},
		{
			name: "at the foot", scroll: 4, wantBar: strings.Repeat("│", 4) + strings.Repeat("┃", 15),
			wantFirst: " │  ", wantSays: "▲ 3 more accounts above · j/k or wheel to scroll",
		},
		{
			name: "past the foot, as at it", scroll: 40, wantBar: strings.Repeat("│", 4) + strings.Repeat("┃", 15),
			wantFirst: " │  ", wantSays: "▲ 3 more accounts above · j/k or wheel to scroll",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			rows := f.Draw(doc, now)
			var bar strings.Builder
			for _, row := range rows[7:26] {
				bar.WriteString(string([]rune(row)[159:]))
			}
			if bar.String() != tt.wantBar {
				t.Errorf("the scrollbar reads %q, want %q", bar.String(), tt.wantBar)
			}
			if !strings.HasPrefix(rows[7], tt.wantFirst) {
				t.Errorf("the first row of cards shown is %q, want it to start %q", rows[7], tt.wantFirst)
			}
			if got := strings.TrimSpace(rows[26]); got != tt.wantSays {
				t.Errorf("the line over the footer says %q, want %q", got, tt.wantSays)
			}
		})
	}
}

func TestTheLineOverTheFooterCountsACardTallerThanTheViewOnce(t *testing.T) {
	l := layout{across: 1, down: 3, width: 50, gap: 1, bars: 1, density: density{compactCard, 2}, content: 23, view: 4}
	if above, below := l.hidden(3, 2); above != 1 || below != 2 {
		t.Errorf("hidden() = %d above, %d below, want 1 and 2: the card cut at both ends counted once", above, below)
	}
}

func TestATerminalTooShortForACardListsTheAccounts(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 12)
	rows := f.Draw(doc, now)
	want := []string{
		" 1 work  ● under pressure · new sessions go elsewhere",
		" 2 personal  ■ limit reached · back 14:12, in 1h",
		" 3 side  ● open · new sessions come here",
	}
	if !strings.HasPrefix(rows[0], "  SWITCHBOARD") || !slices.Equal(rows[1:4], want) {
		t.Errorf("drew\n%s\nwant the title row, then a line an account", strings.Join(rows, "\n"))
	}
	for i, row := range rows[4:11] {
		if row != "" {
			t.Errorf("row %d = %q, want nothing between the accounts and the footer", i+5, row)
		}
	}
	if !strings.HasPrefix(rows[11], " a auto   q quit") {
		t.Errorf("the last row = %q, want the footer", rows[11])
	}
	if got := f.Scrolling(doc, now); got != (Scrolling{}) {
		t.Errorf("Scrolling() = %+v, want none", got)
	}
	if _, ok := f.Cards(doc, now).Neighbour("work", 1, 0); ok {
		t.Error("a card's beside work's, want none: there are no cards to move between")
	}
	for _, height := range []int{1, 2} {
		rows := frameOf(160, height).Draw(doc, now)
		if !strings.HasPrefix(rows[0], "  SWITCHBOARD") || height == 2 && rows[1] != "" {
			t.Errorf("at %d rows, drew\n%s\nwant the title row alone", height, strings.Join(rows, "\n"))
		}
	}
	if rows := frameOf(160, 3).Draw(doc, now); rows[1] != want[0] || !strings.HasPrefix(rows[2], " a auto") {
		t.Errorf("at 3 rows, drew\n%s\nwant the title row, an account's line and the footer", strings.Join(rows, "\n"))
	}
}

func TestCardsThatFitDontScroll(t *testing.T) {
	f := frameOf(160, 40)
	doc := accountsOf(8, false)
	if got := f.Scrolling(doc, now); got != (Scrolling{Page: 26}) {
		t.Errorf("Scrolling() = %+v, want none, the cards' 26 rows fitting", got)
	}
	f.Scroll = 3
	if rows := f.Draw(doc, now); !strings.HasPrefix(rows[7], " ╭─ 1 a ") || strings.HasSuffix(rows[7], "┃") {
		t.Errorf("scrolled 3 rows, cards that fit start %q, want them unscrolled, without a scrollbar", rows[7])
	}
}

func TestAFramePrintedOnceNeverScrolls(t *testing.T) {
	f := Frame{Width: 160, View: Accounts, Policy: claudeLike, Scroll: 5}
	doc := accountsOf(8, false)
	if got := f.Scrolling(doc, now); got != (Scrolling{}) {
		t.Errorf("a printed frame's Scrolling() = %+v, want none", got)
	}
	rows := f.Draw(doc, now)
	if !strings.HasPrefix(rows[7], " ╭─ 1 a ") || strings.HasSuffix(rows[7], "┃") {
		t.Errorf("scrolled 5 rows, a printed frame's cards start %q, want them unscrolled, without a scrollbar", rows[7])
	}
	if got := strings.Count(strings.Join(rows, "\n"), "╭─ "); got != 8 {
		t.Errorf("printed %d cards, want every one of the 8", got)
	}
}

func TestThePhoneStacksCompactCardsWithTwoRowCharts(t *testing.T) {
	doc := accountsOf(3, true)
	doc.Events = append(doc.Events, status.Event{ID: 0, At: now.Add(-2 * time.Minute).UTC(), Kind: status.EventRoom, Account: "b"})
	f := frameOf(52, 36)
	l := laidOut(f, doc)
	if want := (density{compactCard, 2}); l.density != want || l.gap != 0 || l.across != 1 {
		t.Fatalf("on a phone, %+v cards, %d across, %d blank rows between; want %+v, one across, none between", l.density, l.across, l.gap, want)
	}
	rows := f.Draw(doc, now)
	var tops []int
	for i, row := range rows {
		if strings.HasPrefix(row, " ╭─ ") {
			tops = append(tops, i)
		}
	}
	if !slices.Equal(tops, []int{7, 15, 23}) {
		t.Errorf("cards start on rows %v, want 8, 16 and 24, stacked", tops)
	}
	if got, want := rows[32:35], []string{" 13:11 ● a has room again", " 13:10 ● b has room again", "                                      ? for the key"}; !slices.Equal(got, want) {
		t.Errorf("rows 33 to 35 are %q, want %q: RECENT unlabelled, two lines, and the key behind ?", got, want)
	}
}

func TestAFramePrintedOnceHasNoHeightToFit(t *testing.T) {
	doc := accountsOf(8, false)
	f := Frame{Width: 160, View: Accounts, Policy: claudeLike}
	l := laidOut(f, doc)
	if l.density != densities[0] || l.scrolls() || !l.key {
		t.Errorf("printed, %+v cards, scrolling %v, the key line %v; want full cards, no scrolling, and the key", l.density, l.scrolls(), l.key)
	}
	rows := f.Draw(doc, now)
	// The title row, a blank, the heading, a blank; three rows of cards 17
	// tall, a blank between; a blank, the key line; a blank, and which
	// windows hide.
	if want := 7 + 3*17 + 2 + 2 + 2; len(rows) != want {
		t.Fatalf("printed %d rows, want %d:\n%s", len(rows), want, strings.Join(rows, "\n"))
	}
	if got := rows[len(rows)-3]; !strings.HasPrefix(got, " KEY      ") {
		t.Errorf("the key line reads %q", got)
	}
	if got := rows[len(rows)-1]; !strings.HasSuffix(got, "Fable wk hidden: unused on every account") {
		t.Errorf("the last row reads %q, want which windows hide, and no footer", got)
	}
	if strings.Contains(strings.Join(rows, "\n"), "for the key") {
		t.Error("printed, it says the key's behind ?, which does nothing there")
	}
}

func TestAFramePrintedOnceForOneAccountGivesItsCardAndColumn(t *testing.T) {
	doc := routerDoc("work", 3, pressedAccount("work"))
	rows := Frame{Width: 160, View: Accounts, Policy: claudeLike}.Draw(doc, now)
	// The title row, a blank, the heading's line, a blank; the card 12 +
	// 6 + its bars' row tall, COMING UP beside it; a blank, the key line.
	if want := 4 + 19 + 2; len(rows) != want {
		t.Fatalf("printed %d rows, want %d:\n%s", len(rows), want, strings.Join(rows, "\n"))
	}
	if !strings.HasSuffix(rows[4], "╮     COMING UP") {
		t.Errorf("the card's top edge reads %q, want COMING UP beside it", rows[4])
	}
}

func TestEveryRowOfCardsLinesUpAtEveryWidth(t *testing.T) {
	for n := 1; n <= 8; n++ {
		t.Run(fmt.Sprintf("%d accounts", n), func(t *testing.T) {
			t.Parallel()
			doc := accountsOf(n, n%2 == 0)
			for width := 30; width <= 220; width++ {
				rows := Frame{Width: width, View: Accounts, Policy: claudeLike}.Draw(doc, now)
				if bad := misaligned(rows); bad != "" {
					t.Fatalf("at %d columns: %s:\n%s", width, bad, strings.Join(rows, "\n"))
				}
			}
		})
	}
}

func TestNeighbourIsTheCardBesideAboveOrBelow(t *testing.T) {
	// Four accounts at 160 columns: a, b and c in a row, and d under a.
	doc := accountsOf(4, false)
	tests := []struct {
		name         string
		from         string
		across, down int
		want         string
	}{
		{name: "along the row", from: "a", across: 1, want: "b"},
		{name: "back along it", from: "c", across: -1, want: "b"},
		{name: "past the row's end", from: "c", across: 1},
		{name: "before its start", from: "a", across: -1},
		{name: "past the end of a row too short to go on", from: "d", across: 1},
		{name: "down its column", from: "a", down: 1, want: "d"},
		{name: "down to a row too short to reach it: its last", from: "c", down: 1, want: "d"},
		{name: "up its column", from: "d", down: -1, want: "a"},
		{name: "above the first row", from: "b", down: -1},
		{name: "below the last", from: "d", down: 1},
		{name: "from an account the document lacks", from: "z", across: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := frameOf(160, 40).Cards(doc, now).Neighbour(tt.from, tt.across, tt.down)
			if got != tt.want || ok != (tt.want != "") {
				t.Errorf("Neighbour(%s, %d, %d) = %q, %v, want %q", tt.from, tt.across, tt.down, got, ok, tt.want)
			}
		})
	}
	if _, ok := (Frame{Width: 160, View: Accounts, Policy: claudeLike}).Cards(doc, now).Neighbour("a", 1, 0); ok {
		t.Error("a frame printed once has a card beside a's, want none: its cards take no focus")
	}
}

func TestRevealScrollsNoFurtherThanItMustToShowACardWhole(t *testing.T) {
	doc := accountsOf(8, false)
	f := frameOf(160, 28)
	l := laidOut(f, doc)
	if !l.scrolls() {
		t.Fatal("eight accounts in 28 rows don't scroll")
	}
	_, last := l.at(7, l.density)
	tests := []struct {
		name   string
		scroll int
		id     string
		want   int
	}{
		{name: "a card in view: left where it is", scroll: 2, id: "d", want: 2},
		{name: "a card below: down till it shows whole", id: "h", want: last + l.density.rows(l.bars) - l.view},
		{name: "a card above: up to its top", scroll: l.content - l.view, id: "a", want: 0},
		{name: "an account the document lacks: left where it is", scroll: 2, id: "z", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f.Scroll = tt.scroll
			if got := f.Cards(doc, now).Reveal(tt.id); got != tt.want {
				t.Errorf("Reveal(%s) from %d = %d, want %d", tt.id, tt.scroll, got, tt.want)
			}
		})
	}
}

func TestInViewIsTheFirstCardTheViewShowsWhole(t *testing.T) {
	doc := accountsOf(8, false)
	tests := []struct {
		name   string
		height int
		scroll int
		want   string
	}{
		{name: "the first, unscrolled", height: 28, want: "a"},
		{name: "the first of the row under one cut short", height: 28, scroll: 1, want: "d"},
		{name: "where none shows whole, the first shown", height: 18, scroll: 9, want: "d"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, tt.height)
			f.Scroll = tt.scroll
			if got := f.Cards(doc, now).InView(); got != tt.want {
				t.Errorf("InView() = %q, want %q", got, tt.want)
			}
		})
	}
	if got := frameOf(160, 28).Cards(status.Document{}, now).InView(); got != "" {
		t.Errorf("InView() of no accounts = %q, want none", got)
	}
}

// misaligned says how a frame's rows of cards don't line up, or "" where
// each row's cards start and end on the same rows, their sides in line.
func misaligned(rows []string) string {
	for i := 0; i < len(rows); i++ {
		lefts := cellsOf(rows[i], '╭')
		if len(lefts) == 0 {
			continue
		}
		rights := cellsOf(rows[i], '╮')
		j := i + 1
		for ; j < len(rows) && !strings.Contains(rows[j], "╰"); j++ {
			cells := []rune(rows[j])
			for _, side := range slices.Concat(lefts, rights) {
				if side >= len(cells) || cells[side] != '│' {
					return fmt.Sprintf("row %d has no side at %d", j+1, side)
				}
			}
		}
		switch {
		case j == len(rows):
			return fmt.Sprintf("cards opened on row %d never close", i+1)
		case !slices.Equal(cellsOf(rows[j], '╰'), lefts) || !slices.Equal(cellsOf(rows[j], '╯'), rights):
			return fmt.Sprintf("cards opened at %v on row %d close at %v on row %d", lefts, i+1, cellsOf(rows[j], '╰'), j+1)
		}
		i = j
	}
	return ""
}

// cellsOf lists the cells of a row where glyph is drawn.
func cellsOf(row string, glyph rune) []int {
	var cells []int
	for i, r := range []rune(row) {
		if r == glyph {
			cells = append(cells, i)
		}
	}
	return cells
}
