package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// tokenLeft has the account with the given id's token's year up the days
// given after routedAt, an hour on, so it's that many days off at routedAt.
func tokenLeft(id string, days int) func(*status.Document) {
	return account(id, func(a *status.Account) {
		a.TokenGoodUntil = routedAt.AddDate(0, 0, days).Add(time.Hour)
	})
}

// tokenUp has the account with the given id's token's year up an hour
// before routedAt.
func tokenUp(id string) func(*status.Document) {
	return account(id, func(a *status.Account) { a.TokenGoodUntil = routedAt.Add(-time.Hour) })
}

// inkFrom is the ink of the cell the text given starts at along row y of c,
// read from column x on: the zero ink where it isn't there.
func inkFrom(c *canvas, x, y int, text string) ink {
	row := rowOf(c, y)
	before, _, found := strings.Cut(ansi.Cut(row, x, c.width), text)
	if !found {
		return ink{}
	}
	return c.at(x+ansi.StringWidth(before), y).ink
}

// tokenInks are the inks Accounts gives a token line's words, what warns,
// and the command, as the line reads them.
type tokenInks struct {
	words, warns, command string
	warning               ink
}

// Detail tells of the account's token's year ending within 30 days, or up,
// on a line of its own under how the account stands, in line with its words:
// the notification's words, muted; the days left, or expired, bold
// accent.attention within 30 days and bold state.destructive within 7; and
// switchboard setup in text.secondary. USAGE moves down a row for it. A
// year further off, or unknown, tells nothing, and moves nothing.
func TestDetailTellsOfATokensYearEndingUnderHowTheAccountStands(t *testing.T) {
	tests := []struct {
		name   string
		change func(*status.Document)
		line   string
		inks   tokenInks
	}{
		{
			name: "at 21 days", change: tokenLeft("work", 21),
			line: "token expires 22 Oct 2026 · 21 days left — switchboard setup makes a new one",
			inks: tokenInks{words: "token expires", warns: "21 days left", command: "switchboard setup", warning: alertInk},
		},
		{
			name: "at 7 days", change: tokenLeft("work", 7),
			line: "token expires 8 Oct 2026 · 7 days left — switchboard setup makes a new one",
			inks: tokenInks{words: "token expires", warns: "7 days left", command: "switchboard setup", warning: exhaustedInk},
		},
		{
			name: "on its last day", change: tokenLeft("work", 0),
			line: "token expires 1 Oct 2026 · last day — switchboard setup makes a new one",
			inks: tokenInks{words: "token expires", warns: "last day", command: "switchboard setup", warning: exhaustedInk},
		},
		{
			name: "expired", change: tokenUp("work"),
			line: "token expired 1 Oct 2026 — switchboard setup makes a new one",
			inks: tokenInks{words: "token ", warns: "expired", command: "switchboard setup", warning: exhaustedInk},
		},
		{name: "at 31 days", change: tokenLeft("work", 31)},
		{name: "its year unknown", change: func(*status.Document) {}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := usageWorld(extraOff, tt.change)
			f := detailFrame(Look{}, framedAccounts, "work", "", nil, nil)
			d := f.detailDrawn(doc, routedAt, laidIn(f, doc))
			c := d.panel()
			usage := rowWith(textOf(c.rows(Look{})), "USAGE")
			if tt.line == "" {
				if row := rowOf(c, 1); strings.TrimSpace(ansi.Cut(row, d.l.panel, f.Width)) != "" {
					t.Errorf("the row under the head reads %q, want it blank", row)
				}
				if usage != 2 {
					t.Errorf("USAGE is on row %d, want 2", usage)
				}
				return
			}
			x := d.standsAt() + stateWordsAt
			if got := strings.TrimRight(ansi.Cut(rowOf(c, 1), x, f.Width), " "); got != tt.line {
				t.Errorf("the row under the head reads %q from column %d, want %q", got, x, tt.line)
			}
			if got := []rune(rowOf(c, 0))[x-stateWordsAt]; got != '●' {
				t.Errorf("the head's state starts %q, %d cells before the token line, want ●", got, stateWordsAt)
			}
			if usage != 3 {
				t.Errorf("USAGE is on row %d, want 3, under the token line and a blank", usage)
			}
			checkTokenInks(t, c, x, 1, tt.inks)
		})
	}
}

// checkTokenInks checks the inks of a token line along row y, from column x.
func checkTokenInks(t *testing.T, c *canvas, x, y int, want tokenInks) {
	t.Helper()
	for _, tt := range []struct {
		text string
		ink  ink
	}{{want.words, mutedInk}, {want.warns, want.warning}, {want.command, secondaryInk}} {
		if tt.text == "" {
			continue
		}
		if got := inkFrom(c, x, y, tt.text); got != tt.ink {
			t.Errorf("%q is in %+v, want %+v", tt.text, got, tt.ink)
		}
	}
}

// All accounts' Detail tells of no token's year, whatever the accounts'.
func TestAllAccountsDetailTellsOfNoTokensYear(t *testing.T) {
	doc := usageWorld(extraOff, tokenLeft("work", 21))
	f := detailFrame(Look{}, framedAccounts, "", "", nil, nil)
	if rows := textOf(f.Draw(doc, routedAt)); rowWith(rows, "token") >= 0 {
		t.Errorf("all accounts' Detail tells of a token:\n%s", strings.Join(rows, "\n"))
	}
}

// Narrower than the line, wide Detail wraps it at its spaces to the frame's
// edge, and moves USAGE down as many rows as it takes.
func TestDetailWrapsItsTokenLineToTheFramesEdge(t *testing.T) {
	doc := usageWorld(extraOff, tokenLeft("work", 21))
	f := detailFrame(Look{}, framedAccounts, "work", "", nil, nil)
	f.Width = 108
	l, wide := f.detailIn(f.Moment(doc, routedAt))
	if !wide {
		t.Fatalf("Detail at %d columns is stacked, want it wide", f.Width)
	}
	d := f.detailDrawn(doc, routedAt, l)
	c := d.panel()
	x := d.standsAt() + stateWordsAt
	want := []string{"token expires 22 Oct 2026 · 21 days left — switchboard setup makes a new", "one"}
	for i, line := range want {
		if got := strings.TrimRight(ansi.Cut(rowOf(c, 1+i), x, f.Width), " "); got != line {
			t.Errorf("token line %d reads %q from column %d, want %q", i, got, x, line)
		}
	}
	if usage := rowWith(textOf(c.rows(Look{})), "USAGE"); usage != 4 {
		t.Errorf("USAGE is on row %d, want 4, under the token line's two rows and a blank", usage)
	}
}

// Stacked, as on the phone, Detail tells of the token's year under how the
// account stands, in line with its words, wrapped to the frame's edge, in
// the same inks; USAGE after it, a blank row between.
func TestStackedDetailTellsOfATokensYearUnderHowTheAccountStands(t *testing.T) {
	doc := usageWorld(extraOff, tokenLeft("work", 21))
	f := phoneFrame(t, 41, AccountsDetail, "work", phonePage(t))
	s := f.stackedOf(doc, routedAt)
	c := s.whole().c
	rows := textOf(c.rows(Look{}))
	want := []string{"  token expires 22 Oct 2026 · 21 days left —", "  switchboard setup makes a new one", "", "USAGE"}
	for i, line := range want {
		if rows[2+i] != line {
			t.Errorf("row %d reads %q, want %q", 2+i, rows[2+i], line)
		}
	}
	checkTokenInks(t, c, stateWordsAt, 2, tokenInks{words: "token expires", warns: "21 days left", warning: alertInk})
	checkTokenInks(t, c, stateWordsAt, 3, tokenInks{command: "switchboard setup"})
	plain := textOf(f.stackedOf(usageWorld(extraOff), routedAt).whole().c.rows(Look{}))
	if plain[3] != "USAGE" {
		t.Errorf("without a token line, row 3 reads %q, want USAGE, as before", plain[3])
	}
}

// Compare tells of each account's token's year ending within 30 days, or up,
// in its column, on a row of its own under the states, short: the days left,
// or expired, coloured as Detail colours them. Other columns, all accounts'
// among them, leave it blank, and WEEK USED moves down a row. With no
// account shown telling of one, there's no row.
func TestCompareTellsOfATokensYearEndingUnderTheStates(t *testing.T) {
	f, _ := threeCompared(t, 160, 37)
	doc := usageWorld(extraOff, tokenLeft("work", 21), tokenLeft("personal", 5))
	c := f.At(doc, routedAt).page(doc, routedAt)
	rows := textOf(c.rows(Look{}))
	if got, want := rows[8], textsAt(102, map[int]string{26: "token: 21 days left", 60: "token: 5 days left"}); got != strings.TrimRight(want, " ") {
		t.Errorf("the row under the states reads\n%q, want\n%q", got, want)
	}
	if rows[10] != strings.TrimRight(rows[10], " ") || !strings.HasPrefix(rows[10], "WEEK USED") {
		t.Errorf("row 10 reads %q, want WEEK USED", rows[10])
	}
	checkTokenInks(t, c, 26, 8, tokenInks{words: "token:", warns: "21 days left", warning: alertInk})
	checkTokenInks(t, c, 60, 8, tokenInks{words: "token:", warns: "5 days left", warning: exhaustedInk})
	plain := textOf(f.At(usageWorld(extraOff), routedAt).Draw(usageWorld(extraOff), routedAt))
	if !strings.HasPrefix(plain[9], "WEEK USED") {
		t.Errorf("without a token's year to tell of, row 9 reads %q, want WEEK USED, as before", plain[9])
	}
	expired := textOf(f.Draw(usageWorld(extraOff, tokenUp("side")), routedAt))
	if got := columnOf(expired[8], "token expired"); got != 93 {
		t.Errorf("side's expired token is told from column %d, want 93: %q", got, expired[8])
	}
}

// Compare's heads, the token's row among them, are held as its rows scroll
// under them, and its scrolling counts the rows under the heads.
func TestComparesTokenRowIsHeldWithItsHeads(t *testing.T) {
	f, _ := threeCompared(t, 160, 29)
	doc := usageWorld(extraOff, tokenLeft("work", 21))
	f.Scroll = 3
	rows := textOf(f.Draw(doc, routedAt))
	if !strings.HasPrefix(strings.TrimSpace(rows[8]), "token: 21 days left") {
		t.Errorf("scrolled, row 8 reads %q, want the token's row held", rows[8])
	}
	if x, y, _, n := f.At(doc, routedAt).scrolls(doc, routedAt); x != 0 || y != 10 || n != 17 {
		t.Errorf("what scrolls is %d rows from column %d along row %d, want 17 from column 0 along row 10", n, x, y)
	}
}

// In a column too narrow for its words whole, Compare tells what warns
// alone, and narrower still, never cuts it part-way.
func TestComparesTokenRowGivesWayToWhatWarnsAlone(t *testing.T) {
	work, _ := usageWorld(extraOff, tokenLeft("work", 21)).At(routedAt, claudeLike).Account("work")
	note, noted := views.TokenNoteOf(work)
	if !noted {
		t.Fatal("work's token tells of nothing at 21 days")
	}
	for _, tt := range []struct {
		cells int
		want  string
	}{{19, "token: 21 days left"}, {18, "21 days left"}, {12, "21 days left"}, {11, "…"}} {
		if got := fitParts(tt.cells, tokenShortForms(note).fitting(tt.cells)...); got.plain() != tt.want {
			t.Errorf("in %d cells, the token's row reads %q, want %q", tt.cells, got.plain(), tt.want)
		}
	}
}

// Turned, as on the phone, Compare has no heads: it tells of each account's
// token's year under its WEEK USED row, from its bar, short, the next
// account a row further down.
func TestTurnedCompareTellsOfATokensYearUnderItsWeek(t *testing.T) {
	doc := usageWorld(extraOff, tokenLeft("work", 21))
	f := phoneFrame(t, 48, AccountsCompare, "", phonePage(t))
	cp := f.turned(doc, routedAt)
	columns := cp.columns()
	c := cp.turnedWhole().c
	rows := textOf(c.rows(Look{}))
	at := cp.turnedLaid(columns).at
	if got := strings.TrimRight(ansi.Cut(rows[3], at, f.Width), " "); got != "token: 21 days left" {
		t.Errorf("the row under work's week reads %q from column %d, want token: 21 days left", got, at)
	}
	checkTokenInks(t, c, at, 3, tokenInks{words: "token:", warns: "21 days left", warning: alertInk})
	if !strings.HasPrefix(rows[5], " personal") {
		t.Errorf("row 5 reads %q, want personal's week", rows[5])
	}
	plain := textOf(f.turned(usageWorld(extraOff), routedAt).turnedWhole().c.rows(Look{}))
	if !strings.HasPrefix(plain[4], " personal") {
		t.Errorf("without a token's year to tell of, row 4 reads %q, want personal's week, as before", plain[4])
	}
}
