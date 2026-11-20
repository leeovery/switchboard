package dashboard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// pastPeriodTokens is Accounts' Tokens' page over a past period, as the
// views give it: periodTokens' rows and total, none so far.
func pastPeriodTokens() *views.TokensPage {
	page := periodTokens()
	for i := range page.Periods {
		page.Periods[i].SoFar = false
	}
	total := *page.Total
	total.SoFar = false
	page.Total = &total
	return page
}

// oneBack is the period of the length given before now's, as < steps back
// to it.
func oneBack(t *testing.T, length views.Length) views.Period {
	t.Helper()
	back, ok := length.Now().Back(now, time.Monday, time.Time{})
	if !ok {
		t.Fatalf("%s can't step back from now", length)
	}
	return back
}

// The widths Tokens over a past period is held at: a phone's, the phone
// frames', an iPad's and a laptop's, its columns leaving as it narrows.
var tokensPastWidths = []int{52, 73, 104, 160}

// Over a past period, no figure of Tokens brightens as a look changes it,
// neither a row's, the total's nor a row either is split into, at any
// width or split: the period holds no today.
func TestTokensOverAPastPeriodBrightenNothing(t *testing.T) {
	period := oneBack(t, views.LengthThirtyDays)
	for _, width := range tokensPastWidths {
		for split := range TokensSplit(len(tokensSplitNames)) {
			t.Run(fmt.Sprintf("%d columns split %s", width, tokensSplitNames[split]), func(t *testing.T) {
				f := accountsTokensFrame(width, 61, period, split, pastPeriodTokens())
				for n, v := range f.Numbers(threeRouted(), now) {
					if tokensNumber(n) {
						t.Errorf("%+v reads %q, want no figure of a past period numbered", n, v)
					}
				}
			})
		}
	}
	at := accountsTokensFrame(160, 61, views.LengthThirtyDays.Now(), TokensByAccount, periodTokens()).Numbers(threeRouted(), now)
	if got := at[Number{Kind: NumberPeriod, Of: "tokens total"}]; got != "660M" {
		t.Errorf("at now, the total's total reads %q as a number, want 660M", got)
	}
}

// tokensNumber reports whether the number is one of Tokens' figures: a row's
// still running, a row it's split into, or the total's.
func tokensNumber(n Number) bool {
	return n.Kind == NumberTokens || n.Kind == NumberTokensSplit ||
		n.Kind == NumberPeriod && strings.HasPrefix(n.Of, "tokens ")
}

// Over a past period, Tokens' columns stand where they do at now, at every
// width and split: none leaves and none moves for the dial. The total's row
// is named by the dial, as "30 days to 29 Aug", where it fits: in the names'
// column, or where the split's column has left, run on into the first
// count's cells, a cell before its figure. Where it doesn't, as at 160
// columns, or a past 12 months' at 73, it's named by the period's length, as
// at now, never cut.
func TestTokensOverAPastPeriodNameTheTotalByTheDial(t *testing.T) {
	tests := []struct {
		length views.Length
		dialed map[int]bool
	}{
		{length: views.LengthThirtyDays, dialed: map[int]bool{52: true, 73: true, 104: true}},
		{length: views.LengthTwelveWeeks, dialed: map[int]bool{52: true, 73: true}},
		{length: views.LengthTwelveMonths},
	}
	for _, tt := range tests {
		period := oneBack(t, tt.length)
		for _, width := range tokensPastWidths {
			for split := range TokensSplit(len(tokensSplitNames)) {
				t.Run(fmt.Sprintf("%s back at %d columns split %s", tt.length, width, tokensSplitNames[split]), func(t *testing.T) {
					f := accountsTokensFrame(width, 61, period, split, pastPeriodTokens())
					rows := accountsTokensRows(f)
					name := tt.length.Name()
					if strings.HasPrefix(rows[2], f.Dial.Name()+" ") {
						name = f.Dial.Name()
					}
					if !strings.HasPrefix(rows[2], name+" ") {
						t.Fatalf("the total's row reads %q, want it named %q or %q whole, a cell clear of what follows", rows[2], f.Dial.Name(), tt.length.Name())
					}
					if dialed := name == f.Dial.Name(); split == TokensByAccount && dialed != tt.dialed[width] {
						t.Errorf("the total's row reads %q, named by the dial: %t, want %t", rows[2], dialed, tt.dialed[width])
					}
					at := accountsTokensFrame(width, 61, tt.length.Now(), split, periodTokens())
					nowRows := accountsTokensRows(at)
					if got, want := figuresOf(rows[2], name), figuresOf(nowRows[2], at.Dial.Name()); got != want {
						t.Errorf("past its name, the total's row reads %q, want %q, where it stands at now", got, want)
					}
					for _, y := range []int{0, 3} {
						if rows[y] != nowRows[y] {
							t.Errorf("row %d reads %q, want %q, as at now", y, rows[y], nowRows[y])
						}
					}
				})
			}
		}
	}
}

// figuresOf is the row past its name, its figures where they stand.
func figuresOf(row, name string) string {
	return strings.Repeat(" ", len(name)) + strings.TrimPrefix(row, name)
}
