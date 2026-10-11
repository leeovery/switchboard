package capture

import (
	"math"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// usedModel reports whether any account's requests of the day asked for the
// model given.
func usedModel(s ledger.Summary, model string) bool {
	return slices.ContainsFunc(s.Accounts, func(a ledger.AccountDay) bool {
		return slices.ContainsFunc(a.Models, func(m ledger.ModelDay) bool { return m.Model == model })
	})
}

// accountTotals are the Totals of the account with the given id, of every
// account's where it's "".
func accountTotals(t *testing.T, h views.History, id string) views.AccountTotals {
	t.Helper()
	i := slices.IndexFunc(h.Totals.Accounts, func(a views.AccountTotals) bool { return a.Account == id && a.All == (id == "") })
	if i < 0 {
		t.Fatalf("no totals of %q", id)
	}
	return h.Totals.Accounts[i]
}

func TestTheYearsMonthsAreThoseBYMONTHDraws(t *testing.T) {
	h := historyFrom(threeDays(), worldsNow().auto(), lastTwelveMonths)
	tests := []struct {
		month    string
		requests int
		worth    int
		against  float64
		// limits are the 5-hour limits the month reached, of the months
		// before the eight weeks, whose limits come from the weeks' own.
		limits int
	}{
		{"2025-10", 10148, 1180, 2.0, 23}, {"2025-11", 11524, 1340, 2.2, 26}, {"2025-12", 10406, 1210, 2.0, 23},
		{"2026-01", 13416, 1560, 2.6, 30}, {"2026-02", 14792, 1720, 2.9, 33}, {"2026-03", 17028, 1980, 3.3, 38},
		{"2026-04", 18490, 2150, 3.6, 41}, {"2026-05", 20038, 2330, 3.9, 45}, {"2026-06", 21242, 2470, 4.1, 48},
		{"2026-07", 23134, 2690, 4.5, 52}, {"2026-08", 24596, 2860, 4.8, 0}, {"2026-09", 26832, 3120, 5.2, 0},
	}
	for i, tt := range tests {
		t.Run(tt.month, func(t *testing.T) {
			m := h.Months[i]
			if m.Month != tt.month || m.Requests != tt.requests || dollars(t, m.Worth) != tt.worth || tenths(m.Ratio) != tt.against {
				t.Errorf("the month reads %s: %d requests, $%d, %.1f× its plans, want %s: %d, $%d, %.1f×",
					m.Month, m.Requests, dollars(t, m.Worth), tenths(m.Ratio), tt.month, tt.requests, tt.worth, tt.against)
			}
			if tt.limits > 0 && m.Limits[sessionKey] != tt.limits {
				t.Errorf("the month reached %d 5-hour limits, want %d", m.Limits[sessionKey], tt.limits)
			}
		})
	}
}

func TestTheYearIsTheOneYearDraws(t *testing.T) {
	days := threeDays()
	h := historyFrom(days, worldsNow().auto(), lastTwelveMonths)
	all := slices.IndexFunc(h.Plans, func(p views.PlanCost) bool { return p.All })
	if cost := h.Plans[all].Cost; cost == nil || *cost != 7200e12 {
		t.Errorf("the plans cost %v over the year, want $7,200", cost)
	}
	if n := accountTotals(t, h, "").Sessions; n != yearSessions-todaysNewSessions {
		t.Errorf("the year to yesterday had %d sessions, want %d, to be %d with today's", n, yearSessions-todaysNewSessions, yearSessions)
	}
	first := slices.IndexFunc(days, func(s ledger.Summary) bool { return usedModel(s, opus55) })
	if first < 0 || days[first].Day != "2026-06-22" {
		t.Errorf("Opus 5.5 was first used on day %d, want 22 June 2026", first)
	}
}

func TestTheEightWeeksAreThoseWeeksDraws(t *testing.T) {
	type accountPeaks struct {
		id     string
		peaks  [7]int
		limits [7]int
	}
	tests := []struct {
		name  string
		world accountWorld
		days  ledgerDays
		// weeks are how many of the eight's first the frames' peaks hold of:
		// spare's week resets on a Thursday: its week from 17 September is
		// placed under 14 September's and its week from 24 September, still
		// running, under this week's, leaving 21 September's without, and the
		// pool of the ten that week a mean of nine.
		weeks int
		// pool is every account's peak, the accounts' mean, in tenths of a
		// percent.
		pool  [7]int
		peaks []accountPeaks
	}{
		{
			name: "three accounts", world: worldsNow().auto(), days: threeDays(), weeks: 7,
			pool: [7]int{527, 643, 663, 670, 737, 727, 817},
			peaks: []accountPeaks{
				// acct3.limit_day, which the strips draw, marks a weekday's limit,
				// so a week of work's that reached more than five, as LIMITS5
				// counts them, has five: its 24 Aug's, 7 Sep's and 21 Sep's.
				{"work", [7]int{72, 88, 100, 94, 100, 81, 100}, [7]int{3, 4, 5, 5, 5, 4, 5}},
				{"personal", [7]int{64, 70, 58, 77, 69, 90, 85}, [7]int{1, 2, 0, 2, 1, 3, 2}},
				{"side", [7]int{22, 35, 41, 30, 52, 47, 60}, [7]int{0, 0, 1, 0, 1, 0, 1}},
			},
		},
		{
			name: "ten accounts", world: worldsNow().many(10), days: tenDays(), weeks: 6,
			pool: [7]int{433, 524, 528, 545, 576, 565},
			peaks: []accountPeaks{
				{"client", [7]int{40, 52, 48, 61, 55, 58}, [7]int{0, 0, 0, 1, 0, 0}},
				{"spare", [7]int{5, 8, 3, 10, 6, 4}, [7]int{}},
				{"lab", [7]int{60, 72, 70, 66, 81, 75}, [7]int{1, 2, 1, 1, 3, 2}},
				{"team", [7]int{80, 92, 100, 88, 100, 95}, [7]int{2, 3, 5, 3, 5, 4}},
				{"extra", [7]int{15, 22, 18, 30, 25, 19}, [7]int{}},
				{"ops", [7]int{30, 35, 28, 41, 33, 38}, [7]int{}},
				{"studio", [7]int{45, 50, 62, 48, 55, 58}, [7]int{0, 1, 0, 0, 1, 0}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := historyFrom(tt.days, tt.world, eightWeeks)
			if len(h.Weeks) != 8 {
				t.Fatalf("Weeks has %d weeks, want the eight", len(h.Weeks))
			}
			for i, week := range h.Weeks[:tt.weeks] {
				for _, a := range week.Accounts {
					if a.All {
						if got := int(math.Round(a.Peaks[accountKey] * 1000)); got != tt.pool[i] {
							t.Errorf("every account's week of %s peaked at %.1f%%, want %.1f%%", week.Week, float64(got)/10, float64(tt.pool[i])/10)
						}
						continue
					}
					j := slices.IndexFunc(tt.peaks, func(p accountPeaks) bool { return p.id == a.Account })
					if j < 0 {
						continue
					}
					want := tt.peaks[j]
					if got := percent(a.Peaks[accountKey]); got != want.peaks[i] {
						t.Errorf("%s's week of %s peaked at %d%%, want %d%%", a.Account, week.Week, got, want.peaks[i])
					}
					if got := a.Limits[sessionKey]; got != want.limits[i] {
						t.Errorf("%s reached %d 5-hour limits in its week of %s, want %d", a.Account, got, week.Week, want.limits[i])
					}
				}
			}
		})
	}
}

func TestTheCapacityIsTheOneWeeksDraws(t *testing.T) {
	c := historyFrom(threeDays(), worldsNow().auto(), eightWeeks).Capacity
	if c.Weeks != 7 || c.Short != 3 || c.WithOneFewer == nil || *c.WithOneFewer != 6 || c.WithOneMore != 0 || c.Verdict != "about right" {
		t.Errorf("capacity is %+v, want short on 3 of 7 weeks as now, 6 with one fewer, none with one more, about right", c)
	}
	want := map[string]struct {
		verdict string
		limits  int
		left    int
	}{
		"work":     {"hits its limit most weeks", 3, 9},
		"personal": {"ends most weeks with room", 0, 27},
		"side":     {"plenty to spare most weeks", 0, 59},
	}
	for _, a := range c.Accounts {
		w, ok := want[a.Account]
		if !ok || a.All {
			continue
		}
		if a.Verdict != w.verdict || a.Limits != w.limits || a.LeftAtReset == nil || percent(*a.LeftAtReset) != w.left {
			t.Errorf("%s's verdict is %q, %d limits, %v left, want %q, %d, %d%%", a.Account, a.Verdict, a.Limits, a.LeftAtReset, w.verdict, w.limits, w.left)
		}
	}
	if ten := historyFrom(tenDays(), worldsNow().many(10), eightWeeks).Capacity; ten.Drop != "spare" {
		t.Errorf("of the ten, the one to drop is %q, want spare", ten.Drop)
	}
}

func TestYesterdayIsTheLogs(t *testing.T) {
	all := accountTotals(t, historyFrom(threeDays(), worldsNow().auto(), today-1), "")
	if all.Requests != 1912 || dollars(t, all.Worth) != 212 || all.Sessions != 17 {
		t.Errorf("yesterday had %d requests, $%d, %d sessions, want 1,912, $212, 17", all.Requests, dollars(t, all.Worth), all.Sessions)
	}
}

// weekToYesterday is an account's week to yesterday, as Accounts counts it:
// its requests, its worth, in whole dollars, and its sessions, where they're
// checked.
type weekToYesterday struct {
	id                 string
	requests, sessions int
	worth              int
}

func TestThisWeekToYesterdayIsAccountsLessTodaysLines(t *testing.T) {
	tests := []struct {
		name  string
		world accountWorld
		days  ledgerDays
		// accounts are as Accounts draws them less what today's lines hold,
		// which the sessions' world gives: the three's sessions run on into
		// today, so only the sessions' world can count them.
		accounts []weekToYesterday
		// limits are the 5-hour limits the accounts named reached; personal's
		// at 19:12:40 yesterday, which the Log tells of, isn't among STATS'.
		limits map[string]int
	}{
		{
			name: "three accounts", world: worldsNow().auto(), days: threeDays(),
			accounts: []weekToYesterday{{"work", 1204 - 592, 0, 184 - 60}, {"personal", 2330 - 463, 0, 212 - 48}, {"side", 1876 - 185, 0, 239 - 18}},
			limits:   map[string]int{"work": 2, "side": 1},
		},
		{
			name: "ten accounts", world: worldsNow().many(10), days: tenDays(),
			accounts: []weekToYesterday{
				{"client", 1410, 5, 151}, {"spare", 96, 1, 9}, {"lab", 1102, 4, 133}, {"team", 1980, 7, 201},
				{"extra", 310, 2, 31}, {"ops", 520, 3, 52}, {"studio", 870, 4, 98},
			},
			limits: map[string]int{"client": 0, "spare": 0, "lab": 1, "team": 1, "extra": 0, "ops": 0, "studio": 0},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := historyFrom(tt.days, tt.world, thisWeek)
			for _, want := range tt.accounts {
				a := accountTotals(t, h, want.id)
				if a.Requests != want.requests || dollars(t, a.Worth) != want.worth {
					t.Errorf("%s's week to yesterday is %d requests, $%d, want %d, $%d", want.id, a.Requests, dollars(t, a.Worth), want.requests, want.worth)
				}
				if want.sessions > 0 && a.Sessions != want.sessions {
					t.Errorf("%s's week to yesterday had %d sessions, want %d", want.id, a.Sessions, want.sessions)
				}
			}
			for id, want := range tt.limits {
				if got := accountTotals(t, h, id).Limits[sessionKey]; got != want {
					t.Errorf("%s reached %d 5-hour limits this week to yesterday, want %d", id, got, want)
				}
			}
		})
	}
}

func TestTheModelVersionsShareTheWeekAndTheThirtyDaysAsAccountsDraws(t *testing.T) {
	tests := []struct {
		name   string
		days   ledgerDays
		from   int
		shares map[string][6]int
	}{
		{name: "this week", days: threeDays(), from: thisWeek, shares: map[string][6]int{
			"work": {55, 7, 28, 3, 5, 2}, "personal": {29, 0, 60, 7, 0, 4}, "side": {82, 0, 16, 0, 1, 1},
		}},
		{name: "the last 30 days", days: threeDays(), from: thirtyDays, shares: map[string][6]int{
			"work": {52, 8, 30, 3, 5, 2}, "personal": {31, 0, 57, 7, 1, 4}, "side": {79, 0, 19, 0, 1, 1},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := historyFrom(tt.days, worldsNow().auto(), tt.from)
			for id, want := range tt.shares {
				var got [6]int
				for _, m := range accountTotals(t, h, id).ByModel {
					if m.Share != nil {
						got[slices.Index(versionOrder, m.Model)] = percent(*m.Share)
					}
				}
				if got != want {
					t.Errorf("%s's worth by version is %v%%, want %v%%", id, got, want)
				}
			}
		})
	}
}
