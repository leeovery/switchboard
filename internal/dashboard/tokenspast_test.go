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

// Over a past period, the total's row is named by the dial, as
// "30 days to 29 Aug", drawn whole at every width and split, though it's
// wider than the names otherwise are. Where the split's column has left and
// the first count's cells leave it room, the name runs on into them, a cell
// before its figure, every column standing where it does at now; else the
// names hold it, as at 160 columns, or as a past 12 months' at 73, which the
// first count's cells don't hold.
func TestTokensOverAPastPeriodNameTheTotalByTheDial(t *testing.T) {
	tests := []struct {
		length views.Length
		runsOn map[int]bool
	}{
		{length: views.LengthThirtyDays, runsOn: map[int]bool{52: true, 73: true, 104: true}},
		{length: views.LengthTwelveWeeks, runsOn: map[int]bool{52: true, 73: true}},
		{length: views.LengthTwelveMonths},
	}
	for _, tt := range tests {
		period := oneBack(t, tt.length)
		for _, width := range tokensPastWidths {
			for split := range TokensSplit(len(tokensSplitNames)) {
				t.Run(fmt.Sprintf("%s back at %d columns split %s", tt.length, width, tokensSplitNames[split]), func(t *testing.T) {
					f := accountsTokensFrame(width, 61, period, split, pastPeriodTokens())
					rows := accountsTokensRows(f)
					name := f.Dial.Name()
					if !strings.HasPrefix(rows[2], name+" ") {
						t.Fatalf("the total's row reads %q, want it named %q whole, a cell clear of what follows", rows[2], name)
					}
					if split != TokensByAccount {
						return
					}
					if runsOn := f.accountsTokens().columns().cells(tokensName) < len(name); runsOn != tt.runsOn[width] {
						t.Fatalf("the name runs on past the names' column: %t, want %t", runsOn, tt.runsOn[width])
					}
					if !tt.runsOn[width] {
						return
					}
					at := accountsTokensFrame(width, 61, tt.length.Now(), split, periodTokens())
					nowRows := accountsTokensRows(at)
					if got, want := figuresOf(rows[2], name), figuresOf(nowRows[2], at.Dial.Name()); got != want {
						t.Errorf("past its name, the total's row reads %q, want %q, where it stands at now", got, want)
					}
					if got, want := rows[3], nowRows[3]; got != want {
						t.Errorf("the first row under the total reads %q, want %q, as at now", got, want)
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
