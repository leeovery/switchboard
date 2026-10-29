package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// The rows of the Accounts page at 160×37, as frame 3 has them.
const (
	accountsHeadsRow = 8
	accountsFirstRow = 10
	accountsAddRow   = 16
	accountsNotesRow = 19
)

// yearLeft is a token's year with days left of it at now, its days gone of
// a year before.
func yearLeft(days int) status.TokenYear {
	until := now.AddDate(0, 0, days)
	return status.TokenYear{Taken: until.AddDate(-1, 0, 0), GoodUntil: until}
}

// frame3Review is the review as frame 3 has it, each token checked: work,
// the primary, its year unknown; personal, no plan set, 21 days left of its
// token's year; and side, its token refused.
func frame3Review() setup.Review {
	return setup.Review{At: now, Accounts: []setup.Reviewed{
		{ID: "work", Label: "Work", Plan: "max20x", Reserve: 0.05, Primary: true, Token: setup.TokenStanding{State: setup.TokenWorking}},
		{ID: "personal", Label: "Personal", Token: setup.TokenStanding{State: setup.TokenWorking, Year: yearLeft(21)}},
		{ID: "side", Label: "Side", Plan: "pro", Token: setup.TokenStanding{State: setup.TokenNotTaken}},
	}}
}

// accountsScreen is the screen at Accounts, the review given, its row chosen
// as given.
func accountsScreen(review setup.Review, chosen int) Setup {
	return Setup{
		Now:      now,
		Step:     SetupAccounts,
		Status:   []setup.Part{{Standing: setup.PartUp, Words: "router up, healthy"}, {Words: "3 accounts, 2 working"}},
		Accounts: &SetupAccountsPage{Review: &review, Chosen: chosen},
	}
}

func TestTheAccountsPageIsFrame3sTable(t *testing.T) {
	c := setupDrawn(accountsScreen(frame3Review(), 2), 160, 37)
	want := map[int]string{
		4:                "ACCOUNTS  " + strings.Repeat("─", 138) + "  3 accounts",
		6:                "  Each token was checked with the API just now.",
		accountsHeadsRow: "     ACCOUNT         LABEL         PLAN       RESERVE    TOKEN",
		accountsFirstRow: "  1  work ◆          Work          Max 20x    5%         " + strings.Repeat("╌", 24) + "  working · expiry date unknown",
		12:               "  2  personal        Personal      not set    none       " + strings.Repeat("█", 23) + "   expires 19 Oct 2026  ·  21 days left",
		14:               "▌ 3  side            Side          Pro        none       ✗ rejected: revoked or expired",
		accountsAddRow:   "  +  add an account",
	}
	for y, w := range want {
		if got := rowOf(c, y); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", y, got, w)
		}
	}
}

func TestTheAccountsPagesInks(t *testing.T) {
	c := setupDrawn(accountsScreen(frame3Review(), 2), 160, 37)
	on := func(k ink) ink {
		k.on = reviewChosenOn
		return k
	}
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{"a head, subtle", 5, accountsHeadsRow, dimInk},
		{"a number, subtle", 2, accountsFirstRow, dimInk},
		{"an id, bold text.primary", 5, accountsFirstRow, titleInk},
		{"the primary's ◆, muted", 10, accountsFirstRow, mutedInk},
		{"a label, muted", 21, accountsFirstRow, mutedInk},
		{"a plan, text.secondary", 35, accountsFirstRow, secondaryInk},
		{"a reserve, text.secondary", 46, accountsFirstRow, secondaryInk},
		{"a year unknown's bar, faint", 57, accountsFirstRow, faintInk},
		{"its words, muted", 83, accountsFirstRow, mutedInk},
		{"a plan not set, faint", 35, 12, faintInk},
		{"a reserve of none, faint", 46, 12, faintInk},
		{"expires, muted", 83, 12, mutedInk},
		{"the date, text.secondary", 91, 12, secondaryInk},
		{"the dot, faint", 105, 12, faintInk},
		{"days left under 30, bold accent.attention", 108, 12, alertInk},
		{"the row chosen's ▌, accent.key on bg.selection", 0, 14, ink{token: theme.AccentKey, on: reviewChosenOn}},
		{"its id on bg.selection", 5, 14, on(titleInk)},
		{"a refusal's ✗, bold state.destructive", 57, 14, on(exhaustedInk)},
		{"its words, text.secondary", 59, 14, on(secondaryInk)},
		{"its blank cells to the edge on bg.selection", 159, 14, on(ink{})},
		{"+, bold accent.key", 2, accountsAddRow, keyInk},
		{"add an account, muted", 5, accountsAddRow, mutedInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inkAt(c, tt.x, tt.y); got != tt.want {
				t.Errorf("column %d of row %d is in %+v, want %+v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestAYearsBarFillsTheDaysGoneAlongTheRamp(t *testing.T) {
	look := Screen(builtin(t, "nord"))
	review := frame3Review()
	c, _ := accountsScreen(review, 0).page(160, 37, look)
	for i := range 24 {
		k := inkAt(c, 57+i, 12)
		want := hue{token: theme.VizTrack}
		if i < 23 {
			want = hue{ramp: true, at: along(i, 24)}
		}
		if k.on != want {
			t.Errorf("cell %d of the bar is on %+v, want %+v: 23 of 24 cells gone, each along the ramp where it sits", i, k.on, want)
		}
	}
}

func TestTheDaysLeftAreInkedByHowFewTheyAre(t *testing.T) {
	tests := []struct {
		name string
		days int
		said string
		want ink
	}{
		{"31 days: muted", 31, "31 days left", mutedInk},
		{"30 days: bold accent.attention, as the note says it", 30, "30 days left", alertInk},
		{"8 days: bold accent.attention", 8, "8 days left", alertInk},
		{"7 days: bold state.destructive", 7, "7 days left", exhaustedInk},
		{"its last day: bold state.destructive", 0, "last day", exhaustedInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review := frame3Review()
			review.Accounts[1].Token.Year = yearLeft(tt.days)
			if tt.days == 0 {
				review.Accounts[1].Token.Year.GoodUntil = now.Add(time.Hour)
			}
			c := setupDrawn(accountsScreen(review, 0), 160, 37)
			row := rowOf(c, 12)
			if !strings.HasSuffix(row, "  ·  "+tt.said) {
				t.Fatalf("personal's row reads %q, want it to end %q", row, tt.said)
			}
			if got := inkAt(c, len([]rune(row))-len([]rune(tt.said)), 12); got != tt.want {
				t.Errorf("its days left are in %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestATokensWordsWhereItsNotToldByItsYear(t *testing.T) {
	tests := []struct {
		name  string
		token setup.TokenStanding
		want  string
	}{
		{"its check running", setup.TokenStanding{State: setup.TokenRead, Year: yearLeft(21)}, "checking…"},
		{"the API didn't answer", setup.TokenStanding{State: setup.TokenUnchecked, Year: yearLeft(21)}, "unchecked"},
		{"no token", setup.TokenStanding{}, "no token"},
		{"a token file setup can't read", setup.TokenStanding{State: setup.TokenUnreadable, Why: "chmod 600"}, "can't read"},
		{"its year up", setup.TokenStanding{State: setup.TokenWorking, Year: status.TokenYear{GoodUntil: now.Add(-time.Hour)}}, "expired 28 Sep 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review := frame3Review()
			review.Accounts[0].Token = tt.token
			c := setupDrawn(accountsScreen(review, 2), 160, 37)
			row := rowOf(c, accountsFirstRow)
			if !strings.HasSuffix(row, " "+tt.want) {
				t.Errorf("work's row reads %q, want it to end %q", row, tt.want)
			}
			if x := len([]rune(row)) - len([]rune(tt.want)); tt.token.State != setup.TokenWorking && inkAt(c, x, accountsFirstRow) != mutedInk {
				t.Errorf("its words are in %+v, want muted", inkAt(c, x, accountsFirstRow))
			}
		})
	}
}

func TestTheReviewsNotes(t *testing.T) {
	c := setupDrawn(accountsScreen(frame3Review(), 2), 160, 37)
	want := []string{
		"  side has no working token, so no requests are routed to it.",
		"  Press t to create a new one, approving it in the browser signed in to Side.",
		"",
		"  personal has a token that expires 19 Oct 2026: 21 days left. Press t to create a new one, approving it in the browser signed in to Personal.",
		"  personal has no plan set, so History can't value its usage. Press e to set one.",
	}
	for i, w := range want {
		if got := rowOf(c, accountsNotesRow+i); got != w {
			t.Errorf("row %d reads\n%q\nwant\n%q", accountsNotesRow+i, got, w)
		}
	}
	for _, blank := range []int{accountsAddRow + 1, accountsAddRow + 2} {
		if got := rowOf(c, blank); got != "" {
			t.Errorf("row %d reads %q, want it blank: two blank rows under + add an account", blank, got)
		}
	}
	inks := []struct {
		name string
		x, y int
		want ink
	}{
		{"an id, bold text.primary", 2, accountsNotesRow, titleInk},
		{"what stops its requests, text.secondary", 7, accountsNotesRow, secondaryInk},
		{"how to see to it, muted", 2, accountsNotesRow + 1, mutedInk},
		{"what's missing besides, muted", 11, accountsNotesRow + 4, mutedInk},
	}
	for _, tt := range inks {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s: column %d of row %d is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

func TestNoAccountThePrimaryOfTwoSaysOneMustBeAndEnterWaits(t *testing.T) {
	review := frame3Review()
	review.Unmarked, review.Accounts[0].Primary = true, false
	c := setupDrawn(accountsScreen(review, 0), 160, 37)
	if got, want := rowOf(c, accountsNotesRow), "  No account is the primary, the one the browser and the Claude apps are signed in to."; got != want {
		t.Errorf("the first note reads\n%q\nwant\n%q", got, want)
	}
	if got, want := rowOf(c, accountsNotesRow+1), "  Choose it, and press p to make it the primary."; got != want {
		t.Errorf("under it reads\n%q\nwant\n%q", got, want)
	}
	if got := rowOf(c, 36); !strings.HasSuffix(got, "p make primary"+blanks(94)+"q quit") {
		t.Errorf("the key line reads %q, want p make primary at its left, and q quit alone at its right", got)
	}
}

func TestTheAccountsPagesKeys(t *testing.T) {
	tests := []struct {
		name   string
		chosen int
		want   string
	}{
		{"an account chosen", 0, "↑↓ select   t new token   e edit   r remove   p make primary" + blanks(72) + "enter next: Priming   q quit"},
		{"+ add an account chosen", 3, "↑↓ select" + blanks(122) + "enter add an account   q quit"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := setupDrawn(accountsScreen(frame3Review(), tt.chosen), 160, 37)
			if got := rowOf(c, 36); got != tt.want {
				t.Errorf("the key line reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
	c := setupDrawn(accountsScreen(frame3Review(), 3), 160, 37)
	if got := inkAt(c, 5, accountsAddRow); got != (ink{token: theme.TextPrimary, on: reviewChosenOn}) {
		t.Errorf("+ add an account chosen, its words are in %+v, want text.primary on bg.selection", got)
	}
}

func TestTheAccountsPageBeforeItsAccountsAreRead(t *testing.T) {
	s := accountsScreen(frame3Review(), 0)
	s.Accounts.Review, s.UnderWay = nil, true
	rows := textOf(s.Draw(160, 37, Look{}))
	if rows[4] != "ACCOUNTS  "+strings.Repeat("─", 150) || rows[6] != "" {
		t.Errorf("before the accounts are read, the page reads\n%s\nwant its heading alone", strings.Join(rows[4:8], "\n"))
	}
	if got := strings.TrimLeft(rows[36], " "); got != "q quit" {
		t.Errorf("the key line reads %q, want q quit alone", got)
	}
}

func TestAccountsThatCouldntBeReadSayWhy(t *testing.T) {
	s := accountsScreen(frame3Review(), 0)
	s.Accounts.Review, s.Accounts.Failure, s.Failed = nil, "read the config: it doesn't parse", true
	c := setupDrawn(s, 160, 37)
	if got, want := rowOf(c, 6), textsAt(160, map[int]string{2: "✗", 5: "Not read", 26: "read the config: it doesn't parse"}); got != want {
		t.Errorf("row 6 reads\n%q\nwant\n%q", got, want)
	}
}

func TestTheTableNarrowsItsPaddingThenItsBarThenItsLabels(t *testing.T) {
	long := frame3Review()
	long.Accounts[1].Label = strings.TrimSpace(strings.Repeat("Personal ", 8))
	tests := []struct {
		name   string
		review setup.Review
		width  int
		// want is the personal's row.
		want string
	}{
		{"at setup's least, the padding gone, the bar narrowed", frame3Review(), 95,
			"  2  personal  Personal  not set  none     " + strings.Repeat("█", 13) + "   expires 19 Oct 2026  ·  21 days left"},
		{"a long label, cut to what's left", long, 95,
			"  2  personal  Personal…   not set  none     " + strings.Repeat("█", 11) + "   expires 19 Oct 2026  ·  21 days left"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := setupDrawn(accountsScreen(tt.review, 0), tt.width, 25)
			if got := rowOf(c, 12); got != tt.want {
				t.Errorf("personal's row reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

func TestLongIDsAreCutBeforeAnyDaysLeftAre(t *testing.T) {
	review := frame3Review()
	review.Accounts[0].ID = strings.Repeat("w", 29)
	review.Accounts[1].ID = strings.Repeat("p", 16)
	review.Accounts[1].Token.Year = yearLeft(5)
	review.Accounts[2].ID = strings.Repeat("s", 40)
	review.Accounts[2].Token = setup.TokenStanding{State: setup.TokenWorking, Year: yearLeft(200)}
	c := setupDrawn(accountsScreen(review, 0), 95, 25)
	for i, words := range []string{"working · expiry date unknown", "  ·  5 days left", "  ·  200 days left"} {
		row := rowOf(c, accountsFirstRow+2*i)
		if !strings.HasSuffix(row, words) || !strings.Contains(row, "…") {
			t.Errorf("at setup's least, a long id's row reads\n%q\nwant its id cut and its words whole, ending %q", row, words)
		}
	}
	if row := rowOf(c, accountsFirstRow); !strings.Contains(row, "… ◆") {
		t.Errorf("the primary's row reads %q, want its id cut and ◆ kept", row)
	}
}

func TestLongIDsAreCutBeforeATokensWordsToldInPlaceOfItsBar(t *testing.T) {
	tests := []struct {
		name  string
		token setup.TokenStanding
		words string
		cut   bool
	}{
		{"every one refused", setup.TokenStanding{State: setup.TokenNotTaken}, "✗ rejected: revoked or expired", true},
		{"every one unchecked", setup.TokenStanding{State: setup.TokenUnchecked, Why: "HTTP 529"}, "unchecked", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review := frame3Review()
			for i := range review.Accounts {
				review.Accounts[i].ID = strings.Repeat(string(rune('a'+i)), 40)
				review.Accounts[i].Label = strings.Repeat("L", 30)
				review.Accounts[i].Token = tt.token
			}
			c := setupDrawn(accountsScreen(review, 0), 95, 25)
			for i, a := range review.Accounts {
				row := rowOf(c, accountsFirstRow+2*i)
				cut := !strings.Contains(row, a.ID) && strings.Contains(row, a.ID[:4]+"…")
				if !strings.HasSuffix(row, tt.words) || cut != tt.cut || a.Primary != strings.Contains(row, " ◆") {
					t.Errorf("at setup's least, a long id's row reads\n%q\nwant its id cut %v, ◆ kept on the primary, and its token's words whole, ending %q", row, tt.cut, tt.words)
				}
			}
		})
	}
}

func TestWhereAYearsWordsDontFitTheDaysLeftAreKept(t *testing.T) {
	year := yearShown{year: yearLeft(5), at: now}
	forms := year.forms()
	if got := firstFit(forms[0].width()-1, forms...).plain(); got != "5 days left" {
		t.Errorf("a cell short of the year's words, they read %q, want the days left alone", got)
	}
	if got := firstFit(forms[0].width(), forms...).plain(); !strings.HasPrefix(got, "expires ") {
		t.Errorf("with room for the year's words, they read %q, want them whole", got)
	}
}

// manyReviewed is a review of n accounts, each working, each on Pro, the
// first the primary.
func manyReviewed(n int) setup.Review {
	review := setup.Review{At: now}
	for i := range n {
		id := fmt.Sprintf("acct%02d", i+1)
		review.Accounts = append(review.Accounts, setup.Reviewed{ID: id, Label: id, Plan: "pro", Primary: i == 0, Token: setup.TokenStanding{State: setup.TokenWorking}})
	}
	return review
}

func TestTheChosenRowIsScrolledToAsLittleAsShowsIt(t *testing.T) {
	tests := []struct {
		name           string
		chosen, scroll int
		// want is the scroll that shows the row chosen.
		want int
	}{
		{"in view: where it is", 2, 0, 0},
		{"below: as little down as shows it", 10, 0, 9},
		{"above: as little up as shows it, under the heads kept", 2, 9, 7},
		{"+ add an account, last", 12, 0, 13},
		{"the first: to the top, what's over the table showing", 0, 9, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := accountsScreen(manyReviewed(12), tt.chosen)
			s.Scroll = tt.scroll
			got := s.ScrolledToChosen(95, 25)
			if got != tt.want {
				t.Errorf("scrolled to %d, want %d", got, tt.want)
			}
			s.Scroll = got
			row := fmt.Sprintf("▌ %-2d  acct%02d", tt.chosen+1, tt.chosen+1)
			if tt.chosen == 12 {
				row = "▌ +   add an account"
			}
			rows := textOf(s.Draw(95, 25, Look{}))
			if !containsPrefix(rows[6:22], row) {
				t.Errorf("scrolled to %d, the page reads\n%s\nwant %q in view", got, strings.Join(rows[6:23], "\n"), row)
			}
		})
	}
	below := accountsScreen(manyReviewed(12), 10)
	below.Scroll = 4
	if got := below.ScrolledToChosen(94, 25); got != 4 {
		t.Errorf("below setup's least, scrolled to %d, want it kept at 4", got)
	}
}

// containsPrefix reports whether a row of rows starts with prefix.
func containsPrefix(rows []string, prefix string) bool {
	for _, row := range rows {
		if strings.HasPrefix(row, prefix) {
			return true
		}
	}
	return false
}

func TestScrolledIntoTheTableItsHeadsAreKeptAndWhatsOutOfViewNamesNoKeys(t *testing.T) {
	s := accountsScreen(manyReviewed(12), 8)
	s.Scroll = 6
	rows := textOf(s.Draw(95, 25, Look{}))
	if !strings.HasPrefix(rows[6], "      ACCOUNT ") {
		t.Errorf("scrolled 6 rows, the page's first row reads %q, want the table's heads, kept", rows[6])
	}
	if got := rows[22]; got != "▲ 6 more above   ·   ▼ 7 more below" {
		t.Errorf("the line saying what's out of view reads %q, want it naming no keys, as ↑↓ choose here", got)
	}
}
