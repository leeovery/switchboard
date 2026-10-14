package dashboard

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// A row's BY MODEL bar is split by what its use of each version the period
// used was worth, in the period's order: nothing of one it didn't use.
func TestBYMODELsBarIsSplitByEachVersionsWorth(t *testing.T) {
	got := shareAmounts(comparePersonal.row(), comparedVersions(t))
	want := []ledger.Picodollars{*usd(29), 0, *usd(60), *usd(7), 0, *usd(4)}
	if !slices.Equal(got, want) {
		t.Errorf("personal's amounts are %v, want %v", got, want)
	}
	hues := versionHues(comparedVersions(t))
	for i, v := range comparedVersions(t) {
		if hues[i] != versionHue(v) {
			t.Errorf("%s's part is in %+v, want its shade, %+v", v.Name, hues[i], versionHue(v))
		}
	}
}

// Detail's BY MODEL names each version the row used, in the period's order,
// as "● Opus 5.5 55%", a share too small for a whole percent "<1%", one not
// known a dash; where they don't all fit, the smallest are left out, one not
// known first, as many as it takes for the rest and how many more there are
// to fit.
func TestDetailsBYMODELNamesTheVersionsThatFit(t *testing.T) {
	unpriced := compareWork
	unpriced.shares = []int{90, 0, 10, -1, -1, -1}
	row := unpriced.row()
	row.Totals.ByModel[1].Share, row.Totals.ByModel[1].Amount = nil, nil
	tiny := compareWork.row()
	tiny.Totals.ByModel = tiny.Totals.ByModel[:1]
	tiny.Totals.ByModel[0].Share = new(0.004)
	tests := []struct {
		name  string
		row   compareFramed
		width int
		want  string
	}{
		{
			name: "every one", row: compareWork, width: 104,
			want: "● Opus 5.5 55%  ● Opus 5 7%  ● Sonnet 5.5 28%  ● Sonnet 5 3%  ● Fable 5.1 5%  ● Haiku 4.5 2%",
		},
		{name: "the smallest left out", row: compareWork, width: 70, want: "● Opus 5.5 55%  ● Opus 5 7%  ● Sonnet 5.5 28%  ● Fable 5.1 5%  +2 more"},
		{name: "the next smallest left out", row: compareWork, width: 60, want: "● Opus 5.5 55%  ● Opus 5 7%  ● Sonnet 5.5 28%  +3 more"},
		{name: "none that fits", row: compareWork, width: 6, want: ""},
		{name: "those it used alone", row: compareSide, width: 104, want: "● Opus 5.5 82%  ● Sonnet 5.5 16%  ● Fable 5.1 1%  ● Haiku 4.5 1%"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionShares(tt.row.row(), comparedVersions(t), tt.width).plain(); got != tt.want {
				t.Errorf("BY MODEL names %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("under a whole percent", func(t *testing.T) {
		if got, want := versionShares(tiny, comparedVersions(t), 40).plain(), "● Opus 5.5 <1%"; got != want {
			t.Errorf("BY MODEL names %q, want %q", got, want)
		}
	})
	t.Run("a share not known", func(t *testing.T) {
		if got, want := versionShares(row, comparedVersions(t), 41).plain(), "● Opus 5.5 90%  ● Sonnet 5.5 10%  +1 more"; got != want {
			t.Errorf("BY MODEL names %q, want %q", got, want)
		}
		if got, want := versionShares(row, comparedVersions(t), 104).plain(), "● Opus 5.5 90%  ● Opus 5 —  ● Sonnet 5.5 10%"; got != want {
			t.Errorf("BY MODEL names %q, want %q", got, want)
		}
	})
}
