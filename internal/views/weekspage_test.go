package views_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// weeksPageOf is Weeks' page of the days given, the accounts configured as
// given.
func weeksPageOf(days []ledger.Summary, configured config.Accounts) views.WeeksPage {
	in := input(days)
	in.Accounts = configured
	return views.NewPast(in, days).Weeks()
}

// peaksText reads a row's peaks as Weeks' table draws them: a percentage,
// this week's with … after it, or — where it isn't known.
func peaksText(row views.WeeksRow) string {
	var peaks []string
	for _, p := range row.Peaks {
		switch {
		case p.Share == nil:
			peaks = append(peaks, "—")
		case p.Open:
			peaks = append(peaks, prose.Percent(*p.Share)+"…")
		default:
			peaks = append(peaks, prose.Percent(*p.Share))
		}
	}
	return strings.Join(peaks, " ")
}

func TestWeeksPageIsTheEightWeeksToThisOneEachAccountsPeakInEach(t *testing.T) {
	// Work's and side's weeks reset each Monday at 00:30, the last 5 Oct's,
	// which began this week: work's use since has reached 34%, side's isn't
	// known. Side's first week began on 24 Aug, the one before the ledger
	// began, of the eight.
	days := peaking(map[string][]float64{"work": {1, 0.6, 0.5, 0.5, 0.4, 0.7, 0.4}, "side": {0.5, 0.6, 0.3, 0.3, 0.2, 0.5}})
	days = append(days[:len(days)-1], day("2026-10-07", windowsDay{highest: map[string]float64{"7d": 0.34}}.of("work")))
	page := weeksPageOf(days, config.Accounts{{ID: "work", Label: "Work", Primary: true}, {ID: "side"}})

	if want := []string{"2026-08-17", "2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28", "2026-10-05"}; !slices.Equal(page.Weeks, want) {
		t.Errorf("the weeks are %v, want %v", page.Weeks, want)
	}
	tests := []struct {
		name string
		row  views.WeeksRow
		want string
	}{
		{name: "work", row: page.Accounts[0], want: "100% 60% 50% 50% 40% 70% 40% 34%…"},
		{name: "side", row: page.Accounts[1], want: "— 50% 60% 30% 30% 20% 50% —"},
		{name: "all accounts, the mean of those known", row: page.All, want: "100% 55% 55% 40% 35% 45% 45% 34%…"},
	}
	for _, tt := range tests {
		if got := peaksText(tt.row); got != tt.want {
			t.Errorf("%s's peaks are %s, want %s", tt.name, got, tt.want)
		}
	}
	work := page.Accounts[0]
	if work.Account != "work" || work.Label != "Work" || !work.Primary || page.Accounts[1].Primary || !page.All.All || page.Window != "7d" {
		t.Errorf("the rows are %+v and %+v, all %+v, of %q: want work, labelled and primary, then side, then all accounts, of 7d", work, page.Accounts[1], page.All, page.Window)
	}
	if work.Used == nil || *work.Used != 0.5857142857142857 {
		t.Errorf("work used %v of its weeks on average, want their peaks' mean, 4.1 of 7", work.Used)
	}
	// Work hit its limit in the week side's peak isn't known in.
	if c := page.Capacity; c.Weeks != 6 || c.Short != 0 || work.Limits != 1 || work.Verdict != views.PlentyToSpare {
		t.Errorf("the capacity is over %d weeks, %d short, work's limits %d, its verdict %q: want the 6 whole weeks both accounts' peaks are known in, none short, work's own 7 weeks with 1 limit, %q",
			c.Weeks, c.Short, work.Limits, work.Verdict, views.PlentyToSpare)
	}
}

// An account that could go is judged on what it used, as the figure beside
// the verdict says it, a whole percentage: 10.4% used is said 10%, so it
// could go.
func TestWeeksPageJudgesAnAccountThatCouldGoOnTheFigureBesideIt(t *testing.T) {
	days := peaking(map[string][]float64{"work": slices.Repeat([]float64{0.5}, 7), "spare": slices.Repeat([]float64{0.104}, 7)})
	page := weeksPageOf(days, config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "spare", Plan: "pro"}})
	spare := page.Accounts[1]
	if spare.Used == nil {
		t.Fatalf("spare's use isn't given, its verdict %q", spare.Verdict)
	}
	if used := prose.Percent(*spare.Used); spare.Verdict != views.CouldGo || used != "10%" {
		t.Errorf("spare's verdict = %q, having used %s; want %q, having used 10%%", spare.Verdict, used, views.CouldGo)
	}
}

func TestWeeksPageJudgesTheLast7WholeWeeksAlone(t *testing.T) {
	// Ten weeks, the three oldest short: of the eight, the last 7 whole
	// weeks have none.
	days := peaking(map[string][]float64{"work": {1, 1, 1, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}, "side": slices.Repeat([]float64{0.2}, 10)})
	page := weeksPageOf(days, workAndSide)

	if c := page.Capacity; c.Weeks != 7 || c.Short != 0 || page.Accounts[0].Limits != 0 {
		t.Errorf("the capacity is over %d weeks, %d short, work's limits %d, want the 7 whole weeks of the eight, none short", c.Weeks, c.Short, page.Accounts[0].Limits)
	}
}

func TestWeeksPageSaysWhoWasShortEachTimeWhereTheSameWere(t *testing.T) {
	tests := []struct {
		name  string
		peaks map[string][]float64
		want  []string
	}{
		{name: "work alone each time", peaks: map[string][]float64{"work": {1, 0.6, 1, 0.5, 1, 0.7, 0.4}, "side": {0.5, 0.5, 0.6, 0.3, 0.3, 0.2, 0.5}}, want: []string{"work"}},
		{name: "both each time", peaks: map[string][]float64{"work": {1, 0.6, 1, 0.5, 0.4, 0.7, 0.4}, "side": {1, 0.5, 1, 0.3, 0.3, 0.2, 0.5}}, want: []string{"work", "side"}},
		{name: "one, then the other", peaks: map[string][]float64{"work": {1, 0.6, 0.5, 0.5, 0.4, 0.7, 0.4}, "side": {0.5, 1, 0.6, 0.3, 0.3, 0.2, 0.5}}},
		{name: "one, then both", peaks: map[string][]float64{"work": {1, 1, 0.5, 0.5, 0.4, 0.7, 0.4}, "side": {0.5, 1, 0.6, 0.3, 0.3, 0.2, 0.5}}},
		{name: "none short", peaks: map[string][]float64{"work": {0.5, 0.6, 0.5, 0.5, 0.4, 0.7, 0.4}, "side": {0.5, 0.5, 0.6, 0.3, 0.3, 0.2, 0.5}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := weeksPageOf(peaking(tt.peaks), workAndSide).ShortEach; !slices.Equal(got, tt.want) {
				t.Errorf("short each time are %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeeksPageComparesTheWeeksShortWithoutTheOneToDropWithNows(t *testing.T) {
	pro, _ := ledger.Pricing.Plan("pro")
	price := pro.Price("2026-10-07")
	tests := []struct {
		name                        string
		accounts                    config.Accounts
		peaks                       map[string][]float64
		drop                        string
		price                       *ledger.Picodollars
		sameWeeks, noneShortWithout bool
	}{
		{
			// By their plans, the rest cover spare: none is short with or
			// without it, which isn't the same weeks short, as there are none.
			name:     "none short either way",
			accounts: config.Accounts{{ID: "work", Plan: "max20x"}, {ID: "side", Plan: "max5x"}, {ID: "spare", Plan: "pro"}},
			peaks: map[string][]float64{"work": {0.5, 0.6, 0.4, 0.5, 0.6, 0.5, 0.4}, "side": {0.2, 0.2, 0.3, 0.2, 0.2, 0.3, 0.2},
				"spare": {0.05, 0.1, 0, 0.05, 0.1, 0.05, 0}},
			drop: "spare", price: &price, noneShortWithout: true,
		},
		{
			// Without side, on no plan, work is short on the weeks it was.
			name: "the same weeks short", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 0.6, 1, 0.5, 1, 0.7, 0.4}, "side": {0.2, 0.3, 0.1, 0.2, 0.3, 0.1, 0.2}},
			drop:  "side", sameWeeks: true, noneShortWithout: true,
		},
		{
			// Work, on Pro, hit its limit the first week, which side, on Max
			// 20x, would have covered; but without work, side would have hit
			// its own the second.
			name: "as many weeks short, but others", accounts: config.Accounts{{ID: "work", Plan: "pro"}, {ID: "side", Plan: "max20x"}},
			peaks: map[string][]float64{"work": {1, 0.5}, "side": {0, 0.98}},
			drop:  "work", price: &price,
		},
		{
			// Side, on Max 20x, would have covered work's limit, on Pro.
			name: "fewer weeks short", accounts: config.Accounts{{ID: "work", Plan: "pro"}, {ID: "side", Plan: "max20x"}},
			peaks: map[string][]float64{"work": {1, 0.5}, "side": {0, 0.5}},
			drop:  "work", price: &price, noneShortWithout: true,
		},
		{
			name: "none to drop", accounts: workAndSide,
			peaks: map[string][]float64{"work": {1, 0.6, 0.5, 0.5, 0.4, 0.7, 0.4}, "side": {0.5, 0.5, 0.6, 0.3, 0.3, 0.2, 0.5}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := weeksPageOf(peaking(tt.peaks), tt.accounts)
			if page.Capacity.Drop != tt.drop || page.SameWeeks != tt.sameWeeks || page.NoneShortWithout != tt.noneShortWithout {
				t.Errorf("drop %q, the same weeks %t, none short without it %t; want %q, %t, %t",
					page.Capacity.Drop, page.SameWeeks, page.NoneShortWithout, tt.drop, tt.sameWeeks, tt.noneShortWithout)
			}
			if got := page.DropPrice; (got == nil) != (tt.price == nil) || got != nil && *got != *tt.price {
				t.Errorf("the plan of the one to drop costs %v a month, want %v", got, tt.price)
			}
		})
	}
}

func TestWeeksPageOfAnEmptyLedgerJudgesNoWeek(t *testing.T) {
	page := weeksPageOf(nil, workAndSide)

	if page.Capacity.Weeks != 0 || len(page.Weeks) != 8 || peaksText(page.Accounts[0]) != "— — — — — — — —" || page.Accounts[0].Used != nil {
		t.Errorf("the empty ledger's page judges %d weeks, its weeks %v, work's peaks %s, used %v: want none judged, eight weeks, none known",
			page.Capacity.Weeks, page.Weeks, peaksText(page.Accounts[0]), page.Accounts[0].Used)
	}
}
