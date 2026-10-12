package views_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// daysOf is Days' page of the summaries given, as the views' tests' input
// gives them, now Wednesday 7 October.
func daysOf(days []ledger.Summary) views.DaysPage {
	return views.NewPast(input(days), days).Days()
}

// stripDays are a strip's 56 days, those at the indexes given at their
// levels, and those at the indexes limited given a 5-hour limit, the rest
// at level 0.
func stripDays(levels map[int]int, limited ...int) []views.StripDay {
	days := make([]views.StripDay, 56)
	for i, level := range levels {
		days[i].Level = level
	}
	for _, i := range limited {
		days[i].Limit = true
	}
	return days
}

// rose is an account's day on which its windows rose as given, by key, and
// it hit the limits given.
func rose(id string, rise map[string]float64, limits ...ledger.Limit) ledger.AccountDay {
	return ledger.AccountDay{Account: id, Rise: rise, Limits: limits}
}

func TestDaysIsTheWeekSoFarAndTheSevenBeforeIt(t *testing.T) {
	tests := []struct {
		name   string
		starts time.Weekday
		weeks  []string
		toCome int
	}{
		{
			name: "weeks from Monday: Thursday to Sunday to come", starts: time.Monday, toCome: 4,
			weeks: []string{"2026-08-17", "2026-08-24", "2026-08-31", "2026-09-07", "2026-09-14", "2026-09-21", "2026-09-28", "2026-10-05"},
		},
		{
			name: "weeks from Sunday: Thursday to Saturday to come", starts: time.Sunday, toCome: 3,
			weeks: []string{"2026-08-16", "2026-08-23", "2026-08-30", "2026-09-06", "2026-09-13", "2026-09-20", "2026-09-27", "2026-10-04"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := daysFrom("2026-01-01")
			in := input(days)
			in.WeekStarts = tt.starts
			got := views.NewPast(in, days).Days()
			if !slices.Equal(got.Weeks, tt.weeks) || got.ToCome != tt.toCome {
				t.Errorf("the weeks are %q, %d days to come; want %q, %d", got.Weeks, got.ToCome, tt.weeks, tt.toCome)
			}
			for _, s := range got.ByAccount {
				if len(s.Days) != 56 {
					t.Errorf("%s's strip has %d days, want the eight weeks' 56", s.Of, len(s.Days))
				}
			}
		})
	}
}

// BY MODEL is a strip a family that had requests in the eight weeks, in the
// version table's order, each day the family's worth, every account's, the
// strips on one scale: the active days' quarters, over every family's. Each
// has its share of the eight weeks' worth.
func TestDaysByModelIsAStripAFamilyOnOneScaleOfWorth(t *testing.T) {
	checked := ledger.ModelDay{Model: "claude-haiku-4-5", Checks: 1, Usage: []byte(inputUsage)}
	days := daysFrom("2026-08-10",
		day("2026-08-16", account("work", sent("claude-opus-5-5", 1, cacheHourUsage))),
		day("2026-08-17", account("work", sent("claude-opus-5-5", 1, opusUsage))),
		day("2026-09-01", account("side", sent("claude-sonnet-5-5", 1, inputUsage))),
		day("2026-10-06", account("work", sent("claude-unknown-1", 2, inputUsage))),
		day("2026-10-07", account("side", sent("claude-opus-5-5", 1, inputUsage)), account("work", checked)),
	)
	opus, sonnet := 11.6/13.6, 2/13.6
	want := []views.DaysStrip{
		{Of: "opus", Days: stripDays(map[int]int{0: 4, 51: 3}), Share: &opus},
		{Of: "sonnet", Days: stripDays(map[int]int{15: 2}), Share: &sonnet},
		{Of: "unknown", Days: stripDays(nil)},
	}
	got := daysOf(days).ByModel
	if len(got) != len(want) {
		t.Fatalf("BY MODEL has %d strips, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Of != w.Of || !reflect.DeepEqual(g.Days, w.Days) || !sharesNear(g.Share, w.Share) || g.Limits != 0 {
			t.Errorf("strip %d is %+v (share %v), want %+v (share %v)", i, g, deref(g.Share), w, deref(w.Share))
		}
	}
}

func TestDaysByAccountIsAStripAnAccountOnOneScaleOfItsWeek(t *testing.T) {
	days := daysFrom("2026-08-10",
		day("2026-08-16", rose("work", map[string]float64{"7d": 0.9}, limit("5h", "2026-08-16", "10:00"))),
		day("2026-08-17", rose("work", map[string]float64{"7d": 0.1, "5h": 0.9})),
		day("2026-10-06",
			rose("old", map[string]float64{"7d": 0.5}),
			rose("work", map[string]float64{"7d": 0.3}, limit("5h", "2026-10-06", "10:00"), limit("5h", "2026-10-06", "15:00"), limit("7d", "2026-10-06", "16:00"))),
		day("2026-10-07", rose("side", map[string]float64{"7d": 0.2}, limit("5h", "2026-10-07", "11:00"))),
	)
	want := []views.DaysStrip{
		{Of: "work", Days: stripDays(map[int]int{0: 2, 50: 4}, 50), Limits: 2},
		{Of: "side", Days: stripDays(map[int]int{51: 3}, 51), Limits: 1},
		{Of: "spare", Days: stripDays(nil)},
	}
	if got := daysOf(days).ByAccount; !reflect.DeepEqual(got, want) {
		t.Errorf("BY ACCOUNT is\n%+v\nwant\n%+v", got, want)
	}
}

func TestDaysSaysWhetherAnythingIsRecordedInItsWeeks(t *testing.T) {
	tests := []struct {
		name string
		days []ledger.Summary
		want bool
	}{
		{name: "the empty ledger", days: daysFrom("2026-10-07")},
		{name: "a request before the eight weeks", days: daysFrom("2026-08-01", day("2026-08-16", account("work", sent("claude-opus-5-5", 1, inputUsage))))},
		{name: "a request", days: daysFrom("2026-10-07", day("2026-10-07", account("work", sent("claude-opus-5-5", 1, inputUsage)))), want: true},
		{name: "an account's week rising", days: daysFrom("2026-10-07", day("2026-10-07", rose("work", map[string]float64{"7d": 0.1}))), want: true},
		{name: "a 5-hour limit", days: daysFrom("2026-10-07", day("2026-10-07", rose("work", nil, limit("5h", "2026-10-07", "10:00")))), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := daysOf(tt.days).Recorded(); got != tt.want {
				t.Errorf("Recorded() = %t, want %t", got, tt.want)
			}
		})
	}
}

// sharesNear reports whether two shares are both none, or within a
// millionth of each other.
func sharesNear(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a-*b < 1e-6 && *b-*a < 1e-6
}

// deref is a share, or nil.
func deref(share *float64) any {
	if share == nil {
		return nil
	}
	return *share
}
