package capture

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
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
	h := historyFrom(autoWorld(), lastTwelveMonths)
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
	days := autoWorld().ledger().Days(dayStart(worldsNow().now, 0))
	h := historyFrom(autoWorld(), lastTwelveMonths)
	all := slices.IndexFunc(h.Plans, func(p views.PlanCost) bool { return p.All })
	if cost := h.Plans[all].Cost; cost == nil || *cost != 7200e12 {
		t.Errorf("the plans cost %v over the year, want $7,200", cost)
	}
	if n := accountTotals(t, h, "").Sessions; n != yearSessions {
		t.Errorf("the last 12 months had %d sessions, today's among them, want %d", n, yearSessions)
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
		world recordedWorld
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
			name: "three accounts", world: autoWorld(), weeks: 7,
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
			name: "ten accounts", world: tenWorld(), weeks: 6,
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
			h := historyFrom(tt.world, eightWeeks)
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
	c := historyFrom(autoWorld(), eightWeeks).Capacity
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
	if ten := historyFrom(tenWorld(), eightWeeks).Capacity; ten.Drop != "spare" {
		t.Errorf("of the ten, the one to drop is %q, want spare", ten.Drop)
	}
}

// Yesterday is as the Log's line between the days tells it, read off its
// summary as History's Days prices it: 1,912 requests, 17 sessions, 2 limits
// reached, $212 at API prices.
func TestYesterdayIsTheLogs(t *testing.T) {
	day := historyFrom(autoWorld(), today-1).Days[0]
	requests, limits, sessions := 0, 0, make(map[string]bool)
	var worth ledger.Picodollars
	for _, a := range day.Accounts {
		limits += len(a.Limits)
		for _, id := range a.SessionIDs {
			sessions[id] = true
		}
		for _, m := range a.Models {
			requests, worth = requests+m.Requests(), worth+*m.Worth
		}
	}
	if day.Day != "2026-09-30" || requests != 1912 || len(sessions) != 17 || limits != 2 || dollars(t, views.Worth{Amount: &worth}) != 212 {
		t.Errorf("%s had %d requests, %d sessions, %d limits, $%d, want 30 Sep: 1,912, 17, 2, $212", day.Day, requests, len(sessions), limits,
			dollars(t, views.Worth{Amount: &worth}))
	}
}

// Each session a day's lines name is in that day's summary, on each account
// its lines ran on: of yesterday's, b9c4's on work and on side.
func TestEachSessionADaysLinesNameIsInItsSummary(t *testing.T) {
	w, now := autoWorld(), worldsNow().now
	days := w.ledger().Days(dayStart(now, today-1))
	type named struct{ date, account, session string }
	seen := make(map[named]bool)
	for _, f := range w.records.lines {
		if f.Kind == ledger.KindCheck || f.Session == "" || f.At.After(now) {
			continue
		}
		seen[named{f.date, f.Account, f.Session}] = true
	}
	for n := range seen {
		i := slices.IndexFunc(days, func(s ledger.Summary) bool { return s.Day == n.date })
		if i < 0 {
			t.Errorf("%s's lines name %s on %s, with no summary of the day", n.date, n.session, n.account)
			continue
		}
		if a := dayOfAccount(t, days[i], n.account); !slices.Contains(a.SessionIDs, n.session) {
			t.Errorf("%s's lines name %s on %s, which its summary doesn't: %v", n.date, n.session, n.account, a.SessionIDs)
		}
	}
}

// hoursMinutes is a span of minutes as the frames draw it: 3h 10m, or 52m.
func hoursMinutes(minutes int) string {
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%dh %02dm", minutes/60, minutes%60)
}

// weekWords are how the world's week to now, today's lines and all, reads
// through the views, as Accounts' Compare draws it: each account's, and every
// account's, requests, sessions, moves off and on, 5-hour limits, time at its
// cap or a limit, the share of its week left at its week's last reset, worth,
// and that worth against its plan's week.
func weekWords(t *testing.T, w recordedWorld) []string {
	t.Helper()
	h := historyFrom(w, thisWeek)
	var words []string
	for _, a := range h.Totals.Accounts {
		name := cmp.Or(a.Account, "all")
		words = append(words, fmt.Sprintf("%s requests %d", name, a.Requests), fmt.Sprintf("%s sessions %d", name, a.Sessions),
			fmt.Sprintf("%s moved off %d", name, a.MovedOff), fmt.Sprintf("%s moved on %d", name, a.MovedOn),
			fmt.Sprintf("%s 5-hour limits %d", name, a.Limits[sessionKey]), fmt.Sprintf("%s worth $%d", name, dollars(t, a.Worth)))
		if a.MinutesAtCap != nil {
			words = append(words, fmt.Sprintf("%s at its cap or a limit %s", name, hoursMinutes(*a.MinutesAtCap+*a.MinutesAtLimit)))
		}
		if left, ok := a.LeftAtReset[accountKey]; ok {
			words = append(words, fmt.Sprintf("%s left at its last reset %d%%", name, percent(left)))
		}
	}
	for _, p := range h.Plans {
		words = append(words, fmt.Sprintf("%s against its plan %.1f×", cmp.Or(p.Account, "all"), tenths(p.Ratio)))
	}
	return words
}

// This week, Monday to now, today's lines and all, is as Accounts' Compare
// draws it, but what no world the views read can give, where the frames'
// figures disagree with the Log's (LOG-SPEC.md) or with each other; the views
// give:
//   - sessions served: 10 on personal, 14 on side, 27 in all, where the frame
//     draws 6, 11 and 19: the Log's 17 sessions yesterday and its 14 today, 4
//     of them yesterday's, are 27;
//   - moves: 2 off personal and 5 in all, where it draws 1 out and 4 moves:
//     the Log tells of 8a2c's off personal at its limit yesterday and db8a's
//     under pressure today, besides work's three;
//   - personal's 5-hour limit, and 4h 45m at it, where it draws none and —;
//     and in all 4 limits and 8h 47m at a cap or a limit, where it draws 3
//     and 4h 02m: personal's limit at 19:12:40 yesterday, which the Log tells;
//   - no share of work's or personal's week left at a reset this week, where
//     it draws 0% and 15%: their weeks reset on Sunday, before Monday's start;
//   - of the ten, in all, 11,698 requests, 53 sessions, 5 moves, 13h 31m at a
//     cap or a limit, $1,310 and 3.3× its plans' week, where it draws 12,598,
//     51, 10, 9h 57m, $1,342 and 3.4×: acct9.TEN's figures, typed, aren't the
//     sum of its accounts'.
func TestThisWeekIsAccountsTodaysLinesAndAll(t *testing.T) {
	tests := []struct {
		name  string
		world recordedWorld
		words []string
	}{
		{
			name: "three accounts", world: autoWorld(),
			words: []string{
				"work requests 1204", "work sessions 9", "work moved off 3", "work 5-hour limits 2", "work at its cap or a limit 3h 10m", "work worth $184",
				"work against its plan 4.0×",
				"personal requests 2330", "personal worth $212", "personal against its plan 4.6×",
				"side requests 1876", "side moved on 4", "side 5-hour limits 1", "side at its cap or a limit 52m", "side left at its last reset 40%",
				"side worth $239", "side against its plan 5.2×",
				"all requests 5410", "all worth $635", "all against its plan 4.6×",
			},
		},
		{
			name: "ten accounts", world: tenWorld(),
			words: []string{
				"client requests 1410", "client sessions 5", "client 5-hour limits 0", "client worth $151", "client against its plan 3.3×",
				"spare requests 96", "spare sessions 1", "spare 5-hour limits 0", "spare worth $9", "spare against its plan 0.4×",
				"lab requests 1102", "lab sessions 4", "lab 5-hour limits 1", "lab worth $133", "lab against its plan 2.9×",
				"team requests 1980", "team sessions 7", "team 5-hour limits 2", "team worth $201",
				"extra requests 310", "extra sessions 2", "extra 5-hour limits 0", "extra worth $31",
				"ops requests 520", "ops sessions 3", "ops 5-hour limits 0", "ops worth $52",
				"studio requests 870", "studio sessions 4", "studio 5-hour limits 0", "studio worth $98",
				"all 5-hour limits 7",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			words := weekWords(t, tt.world)
			for _, want := range tt.words {
				if !slices.Contains(words, want) {
					t.Errorf("this week reads\n%s\nwith no %q", strings.Join(words, "\n"), want)
				}
			}
		})
	}
}

// The week and the 30 days share their worth by model version as Accounts
// draws them, today's lines and all, to a tenth of a percent, as the days
// before today are made to leave them whole.
func TestTheModelVersionsShareTheWeekAndTheThirtyDaysAsAccountsDraws(t *testing.T) {
	tests := []struct {
		name   string
		from   int
		shares map[string][6]float64
	}{
		{name: "this week", from: thisWeek, shares: map[string][6]float64{
			"work": {55, 7, 28, 3, 5, 2}, "personal": {29, 0, 60, 7, 0, 4}, "side": {82, 0, 16, 0, 1, 1},
		}},
		{name: "the last 30 days", from: thirtyDays, shares: map[string][6]float64{
			"work": {52, 8, 30, 3, 5, 2}, "personal": {31, 0, 57, 7, 1, 4}, "side": {79, 0, 19, 0, 1, 1},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := historyFrom(autoWorld(), tt.from)
			for id, want := range tt.shares {
				var got [6]float64
				for _, m := range accountTotals(t, h, id).ByModel {
					if m.Share != nil {
						got[slices.Index(versionOrder, m.Model)] = math.Round(*m.Share*1000) / 10
					}
				}
				if got != want {
					t.Errorf("%s's worth by version is %v%%, to a tenth, want %v%%", id, got, want)
				}
			}
		})
	}
}

// This week's Monday and Tuesday have their 5-hour windows read by the
// hundredth, each day's own, however many each has: none on a day an account
// used none of its week.
func TestThisWeeksDaysBeforeYesterdayReadEachOfTheirOwnWindowsByTheHundredth(t *testing.T) {
	y := year{now: localMoment(), days: make(map[string][]accountDay)}
	for _, id := range three {
		y.days[id] = make([]accountDay, today)
		y.days[id][thisWeek].rise, y.days[id][thisWeek+1].rise = 0.1, 0.1
	}
	y.days["personal"][thisWeek].rise = 0

	y.limitThisWeek()
	if n := len(y.days["personal"][thisWeek].windows); n != 0 {
		t.Errorf("personal has %d 5-hour windows on Monday, want none, as it used none of its week", n)
	}
	for _, id := range three {
		for d := thisWeek; d < thisWeek+2; d++ {
			for _, s := range y.days[id][d].windows {
				if s.step != 0.01 {
					t.Errorf("%s's 5-hour window from %s is read each %v of it, want each hundredth", id, s.start, s.step)
				}
			}
		}
	}
}

// Each account's week to now, today's lines and all, is worth through the
// views what Accounts draws, to the cent, as no day of the year is worth
// anything without a request, a day the ledger keeps no summary of.
func TestEachAccountsWeekIsWorthWhatAccountsDraws(t *testing.T) {
	y, h := worldsNow().year, historyFrom(tenWorld(), thisWeek)
	for _, a := range tenAccounts {
		for d, day := range y.days[a.ID][:today] {
			if day.requests == 0 && day.worth != 0 {
				t.Errorf("%s's day of %s is worth $%.4f with no request", a.ID, dateOf(d), day.worth)
			}
		}
		worth := accountTotals(t, h, a.ID).Worth
		if worth.Amount == nil {
			t.Fatalf("%s's week is unpriced", a.ID)
		}
		if got := float64(*worth.Amount) / 1e12; math.Abs(got-weekUses[a.ID].worth) >= 0.005 {
			t.Errorf("%s's week is worth $%.4f, want $%.2f, as Accounts draws it", a.ID, got, weekUses[a.ID].worth)
		}
	}
}

// panicked is the message f panicked with, failing the test where it didn't
// panic, or panicked with other than a message, as a runtime error.
func panicked(t *testing.T, f func()) (message string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("it didn't panic")
		}
		s, ok := r.(string)
		if !ok {
			t.Fatalf("it panicked with %v, want a message", r)
		}
		message = s
	}()
	f()
	return ""
}

// A year whose data can't give what building it needs panics, naming the
// account, the day and what's missing: never an index out of range, nor a
// NaN carried on.
func TestAYearItsDataCantBuildPanicsSayingWhatsMissing(t *testing.T) {
	// thisWeeks are how far each account's week rose on Monday, Tuesday and
	// Wednesday, Wednesday's as yesterday's requests were shared.
	thisWeeks := map[string][3]float64{"work": {.1, .1, .02}, "personal": {.1, .1, .07}, "side": {.1, .1, .09}, "spare": {.1, .1, .1}}
	tests := []struct {
		name string
		// rose are the accounts whose weeks rose otherwise than thisWeeks.
		rose  map[string][3]float64
		build func(y year)
		want  []string
	}{
		{
			name: "work's Tuesday, with no 5-hour window", rose: map[string][3]float64{"work": {.1, 0, .02}}, build: year.limitThisWeek,
			want: []string{"work's week rose none on 2026-09-29", "no 5-hour window"},
		},
		{
			name: "side's Tuesday, with no 5-hour window", rose: map[string][3]float64{"side": {.1, 0, .09}}, build: year.limitThisWeek,
			want: []string{"side's week rose none on 2026-09-29", "no 5-hour window"},
		},
		{
			name: "side's Monday and Tuesday, with no rise to spend its week over", rose: map[string][3]float64{"side": {0, 0, .09}},
			build: year.spendThisWeek,
			want:  []string{"over side's Monday and Tuesday, 2026-09-28 and 2026-09-29", "rose its week"},
		},
		{
			name: "work's Monday and Tuesday, after more of yesterday's than its week holds", rose: map[string][3]float64{"personal": {.1, .1, 0}, "side": {.1, .1, 0}},
			build: year.spendThisWeek,
			want:  []string{"-1300 requests", "over work's Monday and Tuesday, 2026-09-28 and 2026-09-29", "less than none"},
		},
		{
			name: "yesterday, with no account risen to share it", rose: map[string][3]float64{"work": {.1, .1, 0}, "personal": {.1, .1, 0}, "side": {.1, .1, 0}},
			build: year.spendThisWeek,
			want:  []string{"1912 requests and $212.00", "over work, personal, side on 2026-09-30", "rose its week"},
		},
		{
			name: "spare's week, with no rise to spend its days by", rose: map[string][3]float64{"spare": {}},
			build: func(y year) { y.spendAsThisWeek("spare", weekUses["spare"]) },
			want:  []string{"96 requests and $9.00", "over spare's days from 2026-09-28 to 2026-09-30", "rose its week"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			y := year{now: localMoment(), days: make(map[string][]accountDay)}
			for id, rises := range thisWeeks {
				if rose, ok := tt.rose[id]; ok {
					rises = rose
				}
				y.days[id] = make([]accountDay, today)
				for i, rise := range rises {
					y.days[id][thisWeek+i].rise = rise
				}
			}

			message := panicked(t, func() { tt.build(y) })
			for _, want := range tt.want {
				if !strings.Contains(message, want) {
					t.Errorf("it panicked with %q, which doesn't say %q", message, want)
				}
			}
		})
	}
}

// Spending what can't be spent panics naming what's missing: requests with no
// part risen to put one on, worth with no request to put it on, or less than
// none, even by a residue, which worthLeft snaps where the difference is made.
func TestSpendingWhatCantBeSpentPanicsNamingWhatsMissing(t *testing.T) {
	week, monday, tuesday := 0.3, 0.1, 0.2
	tests := []struct {
		name     string
		worth    float64
		requests int
		weights  []float64
		want     string
	}{
		{
			name: "requests, with no part risen", worth: 0, requests: 3, weights: []float64{0, 0},
			want: "capture: 3 requests and $0.00 to spend over the parts, none of which rose its week to put a request on",
		},
		{
			name: "worth, with no request", worth: 2.5, requests: 0, weights: []float64{.1, .2},
			want: "capture: $2.50 to spend over the parts, with no request to put it on",
		},
		{
			name: "less than none", worth: -0.01, requests: 3, weights: []float64{.1, .2},
			want: "capture: 3 requests and $-0.01 to spend over the parts, less than none",
		},
		{
			name: "a residue below none", worth: week - (monday + tuesday), requests: 3, weights: []float64{.1, .2},
			want: "capture: 3 requests and $-0.00 to spend over the parts, less than none",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if message := panicked(t, func() { spent(tt.worth, tt.requests, tt.weights, "the parts") }); message != tt.want {
				t.Errorf("it panicked with %q, want %q", message, tt.want)
			}
		})
	}
}

// What's left of a worth once another is taken from it is none within a
// hundredth of a cent of none, either side of it, as $0.30 less the $0.10 and
// $0.20 of its days is below none in floating point; and the difference
// otherwise, less than none included.
func TestWhatsLeftOfAWorthWithinAHundredthOfACentOfNoneIsNone(t *testing.T) {
	week, monday, tuesday := 0.3, 0.1, 0.2
	tests := []struct {
		name        string
		worth, less float64
		want        float64
	}{
		{name: "a residue below none", worth: week, less: monday + tuesday, want: 0},
		{name: "a residue above none", worth: monday + tuesday, less: week, want: 0},
		{name: "under a hundredth of a cent", worth: 0.00009, less: 0, want: 0},
		{name: "a hundredth of a cent", worth: aHundredthOfACent, less: 0, want: aHundredthOfACent},
		{name: "more than none", worth: 9, less: 2.5, want: 6.5},
		{name: "less than none", worth: 0.25, less: 0.5, want: -0.25},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worthLeft(tt.worth, tt.less); got != tt.want {
				t.Errorf("worthLeft(%g, %g) = %g, want %g", tt.worth, tt.less, got, tt.want)
			}
		})
	}
}

// An account whose week before today is worth a residue of none, as its week
// less today's lines can leave, spends none on any day, before this week as
// on this week's: never less than none.
func TestAnAccountSpendingAResidueOfNoneSpendsNoneOnAnyDay(t *testing.T) {
	week, monday, tuesday := 0.3, 0.1, 0.2
	y := year{now: localMoment(), days: map[string][]accountDay{"spare": make([]accountDay, today)}}
	days := y.days["spare"]
	days[eightWeeks].rise = 0.2
	days[thisWeek].rise, days[thisWeek+1].rise, days[thisWeek+2].rise = 0.1, 0.1, 0.1

	y.spendAsThisWeek("spare", weekUse{requests: 5, worth: week, today: weekToday{worths: map[string]float64{opus55: monday, haiku45: tuesday}}})
	if days[eightWeeks].requests == 0 {
		t.Fatalf("spare's %s has no request to put a worth on, want some", dateOf(eightWeeks))
	}
	for d, day := range days[firstDay("spare"):today] {
		if day.worth != 0 {
			t.Errorf("spare's %s, with %d requests, is worth $%g, want none", dateOf(firstDay("spare")+d), day.requests, day.worth)
		}
	}
}

// An account that spent nothing this week spends nothing on any of its days,
// though its week rose none this week and rose before it.
func TestAnIdleAccountSpendsNothing(t *testing.T) {
	y := year{now: localMoment(), days: map[string][]accountDay{"idle": make([]accountDay, today)}}
	days := y.days["idle"]
	days[eightWeeks].rise = 0.2

	y.spendAsThisWeek("idle", weekUses["idle"])
	for d, day := range days[firstDay("idle"):today] {
		if day.requests != 0 || day.worth != 0 {
			t.Errorf("idle's %s has %d requests worth $%.4f, want none", dateOf(firstDay("idle")+d), day.requests, day.worth)
		}
	}
}

// An account spent as it spent this week has no worth on a day of this week
// too little used for a request, its week's worth all on the days that had
// one, as on the days before this week.
func TestADayOfThisWeekWithNoRequestHasNoWorth(t *testing.T) {
	y := year{now: localMoment(), days: map[string][]accountDay{"spare": make([]accountDay, today)}}
	days := y.days["spare"]
	days[thisWeek].rise, days[thisWeek+1].rise, days[thisWeek+2].rise = 0.3, 0.3, 0.0001

	y.spendAsThisWeek("spare", weekUses["spare"])
	if day := days[thisWeek+2]; day.requests != 0 || day.worth != 0 {
		t.Errorf("spare's %s, too little used for a request, has %d requests worth $%.4f, want none", dateOf(thisWeek+2), day.requests, day.worth)
	}
	requests, worth := weekUses["spare"].beforeToday()
	spent, made := 0.0, 0
	for _, day := range days[thisWeek:today] {
		spent, made = spent+day.worth, made+day.requests
	}
	if math.Abs(spent-worth) > 1e-9 || made != requests {
		t.Errorf("spare's week before today has %d requests worth $%.4f, want %d worth $%.2f", made, spent, requests, worth)
	}
}

// Each account's day of the year is worth, through the views, what the year
// spent on it, its versions' worths summed: none lost to a version too little
// used for a request of its own, whose worth goes to the version used most.
func TestEachDaysVersionsSumToItsWorth(t *testing.T) {
	y, days := worldsNow().year, historyFrom(tenWorld(), 0).Days
	for _, id := range tenAccounts.IDs() {
		for d, made := range y.days[id][:today] {
			var worth ledger.Picodollars
			if i := slices.IndexFunc(days[d].Accounts, func(a ledger.PricedAccount) bool { return a.Account == id }); i >= 0 {
				for _, m := range days[d].Accounts[i].Models {
					if m.Worth == nil {
						t.Fatalf("%s's %s on %s is unpriced", id, m.Model, days[d].Day)
					}
					worth += *m.Worth
				}
			}
			// Within what rounding each version's tokens to a whole one loses.
			if got := float64(worth) / 1e12; math.Abs(got-made.worth) > 0.001 {
				t.Errorf("%s's day of %s is worth $%.4f by its versions, want $%.4f, the year's", id, days[d].Day, got, made.worth)
			}
		}
	}
}

// A year built in any zone, whatever the machine's, has each of its days on
// its own date, one after another, each starting at its date's first instant
// there, though the clocks go forward over its midnight, as Santiago's do on
// 6 September 2026 and Havana's on 8 March: its 5-hour windows and its weeks'
// readings of the day falling on it there, and its sessions' ids made from its
// date. Its summaries aren't checked here: ledger.Summarise splits their days
// in the machine's zone, so their contents are checked in it alone, by
// TestEachDayOfTheYearReadsAsTheYearMadeIt.
func TestEachDayOfTheYearHasItsOwnDateFromItsFirstInstant(t *testing.T) {
	for _, zone := range []string{"America/Santiago", "America/Havana", "Australia/Sydney", "Europe/London"} {
		t.Run(zone, func(t *testing.T) {
			loc, err := time.LoadLocation(zone)
			if err != nil {
				t.Fatal(err)
			}
			y := newYear(time.Date(2026, time.October, 1, 14, 42, 7, 0, loc))
			h := y.readings().history()
			dates := make([]string, 0, today+1)
			before := "2025-09-28"
			for d := range today + 1 {
				date := nextDate(t, before)
				start := dayStart(y.now, d)
				if on, was := start.In(loc).Format(time.DateOnly), start.Add(-time.Nanosecond).In(loc).Format(time.DateOnly); on != date || was != before {
					t.Errorf("%s starts at %s, on %s, the instant before on %s, want its first instant", date, start.In(loc), on, was)
				}
				dates = append(dates, date)
				before = date
			}
			for _, a := range tenAccounts {
				windowsFallOnTheirDays(t, y, a.ID, dates)
				risesAreReadOnTheirDays(t, y, h, a.ID, dates)
			}
			sessionsAreMadeFromTheirDates(t, y, dates)
		})
	}
}

// windowsFallOnTheirDays checks the account with the given id's 5-hour
// windows each start, and reach their caps and limits, on the date of the
// day of the year that holds them, in the year's zone.
func windowsFallOnTheirDays(t *testing.T, y year, id string, dates []string) {
	t.Helper()
	for d := firstDay(id); d < today; d++ {
		for _, s := range y.days[id][d].windows {
			for _, at := range []time.Time{s.start, s.capped, s.limited} {
				if on := at.In(y.now.Location()).Format(time.DateOnly); !at.IsZero() && on != dates[d] {
					t.Errorf("%s's 5-hour window of %s, from %s, reads at %s, on %s", id, dates[d], s.start, at, on)
				}
			}
		}
	}
}

// risesAreReadOnTheirDays checks the readings history reads each day's rise
// of the account with the given id's own weeks on that day, in the year's
// zone: as each of a week's days starts and ends there, the week reads what it
// had risen to by then.
func risesAreReadOnTheirDays(t *testing.T, y year, h history, id string, dates []string) {
	t.Helper()
	weeks := make(map[int64][]readings.Reading)
	for _, r := range h {
		if r.Account == id && r.Key == accountKey {
			weeks[r.ResetsAt.Unix()] = append(weeks[r.ResetsAt.Unix()], r)
		}
	}
	for _, w := range y.weeksOf(id) {
		read := weeks[w.end.Unix()]
		readBy := func(at time.Time) float64 {
			i, _ := slices.BinarySearchFunc(read, at, func(r readings.Reading, at time.Time) int { return r.At.Compare(at) })
			if i == 0 {
				return 0
			}
			return read[i-1].Utilization
		}
		used := 0.0
		for _, d := range w.days {
			if d < firstDay(id) || d >= today {
				continue
			}
			if start := dayStart(y.now, d); readBy(start) != hundredths(used) {
				t.Errorf("%s's week to %s reads %v as %s starts, at %s, want %v", id, w.end, readBy(start), dates[d], start, hundredths(used))
			}
			used += y.days[id][d].rise
			if end := dayStart(y.now, d+1); readBy(end) != hundredths(used) {
				t.Errorf("%s's week to %s reads %v as %s ends, at %s, want %v", id, w.end, readBy(end), dates[d], end, hundredths(used))
			}
		}
	}
}

// sessionsAreMadeFromTheirDates checks each account's sessions of each day
// before this week are made from that day's date: each one of those
// sessionIDs makes of it for an account, as many as the account had that day,
// as one moved onto an account is another's. This week's run on from
// Monday's or yesterday's.
func sessionsAreMadeFromTheirDates(t *testing.T, y year, dates []string) {
	t.Helper()
	for d := range thisWeek {
		made := make(map[string]bool)
		for _, a := range tenAccounts {
			for _, id := range sessionIDs(dates[d], a.ID, len(y.days[a.ID][d].sessions)) {
				made[id] = true
			}
		}
		for _, a := range tenAccounts {
			for _, id := range y.days[a.ID][d].sessions {
				if !made[id] {
					t.Errorf("%s's session %s of %s isn't made from its date", a.ID, id, dates[d])
				}
			}
		}
	}
}

// nextDate is the date after the one given.
func nextDate(t *testing.T, date string) string {
	t.Helper()
	day, err := time.Parse(time.DateOnly, date)
	if err != nil {
		t.Fatal(err)
	}
	return day.AddDate(0, 0, 1).Format(time.DateOnly)
}

// dayOfYear is an account's day of the year: its requests, its sessions and
// those moved onto it and off it; when it reached each 5-hour limit; and when
// each of its 5-hour windows and its own weeks reset, each list in order.
type dayOfYear struct {
	requests, sessions, movedOn, movedOff int
	limits, resets                        string
}

// instants are the times given, in order, as the machine's clock reads them.
func instants(times []time.Time) string {
	slices.SortFunc(times, time.Time.Compare)
	var read []string
	for _, t := range times {
		read = append(read, t.In(time.Local).Format("Mon 2 Jan 15:04:05"))
	}
	return strings.Join(read, ", ")
}

// madeDays are the account with the given id's days of the year to today's
// as the year makes them, each holding the limits and resets that fall in it
// as the year places it: of its 5-hour windows that rose, and the ends of its
// own weeks.
func (y year) madeDays(id string) []dayOfYear {
	limits, resets := make([][]time.Time, today), make([][]time.Time, today)
	onDay := func(held [][]time.Time, t time.Time) {
		if d := dayOf(t.In(y.now.Location())); d >= 0 && d < today {
			held[d] = append(held[d], t)
		}
	}
	made := make([]dayOfYear, today)
	for d, day := range y.days[id][:today] {
		made[d] = dayOfYear{requests: day.requests, sessions: len(day.sessions), movedOn: day.movedOn, movedOff: day.movedOff}
		for _, s := range day.windows {
			if !s.limited.IsZero() {
				onDay(limits, s.limited)
			}
			if s.use > 0 || !s.limited.IsZero() {
				onDay(resets, s.resets())
			}
		}
	}
	for _, w := range y.weeksOf(id) {
		onDay(resets, w.end)
	}
	for d := range made {
		made[d].limits, made[d].resets = instants(limits[d]), instants(resets[d])
	}
	return made
}

// readDay is the account with the given id's day as the views read it, of
// its 5-hour windows and its weeks.
func readDay(day views.Day, id string) dayOfYear {
	i := slices.IndexFunc(day.Accounts, func(a ledger.PricedAccount) bool { return a.Account == id })
	if i < 0 {
		return dayOfYear{}
	}
	a := day.Accounts[i]
	read := dayOfYear{sessions: a.Sessions, movedOn: a.MovedOn, movedOff: a.MovedOff}
	for _, m := range a.Models {
		read.requests += m.Requests()
	}
	var limits, resets []time.Time
	for _, l := range a.Limits {
		if l.Window == sessionKey {
			limits = append(limits, l.At)
		}
	}
	for _, r := range a.Resets {
		if r.Window == sessionKey || r.Window == accountKey {
			resets = append(resets, r.At)
		}
	}
	read.limits, read.resets = instants(limits), instants(resets)
	return read
}

// Each day of the year reads through the views as the year made it, on its
// own date, wherever the machine is: each account's requests, its sessions and
// those moved onto it and off it, when it reached each 5-hour limit, and when
// its 5-hour windows and its weeks reset. The ledger and the views split days
// in the machine's zone, and the year is placed in it, so run under TZ, this
// shows the worlds read the same in each zone.
func TestEachDayOfTheYearReadsAsTheYearMadeIt(t *testing.T) {
	y := worldsNow().year
	tests := []struct {
		name  string
		world recordedWorld
	}{
		{name: "three accounts", world: autoWorld()},
		{name: "ten accounts", world: tenWorld()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := historyFrom(tt.world, 0).Days
			if len(days) != today+1 {
				t.Fatalf("History reads %d days, want the year's %d", len(days), today+1)
			}
			for d, day := range days[:today] {
				if want := time.Date(2025, time.September, 29+d, 12, 0, 0, 0, time.UTC).Format(time.DateOnly); day.Day != want {
					t.Fatalf("day %d of the year is dated %s, want %s", d, day.Day, want)
				}
			}
			for _, id := range tt.world.accounts.IDs() {
				for d, want := range y.madeDays(id) {
					if got := readDay(days[d], id); got != want {
						t.Errorf("%s's day of %s reads\n%+v, want\n%+v", id, days[d].Day, got, want)
					}
				}
			}
		})
	}
}
