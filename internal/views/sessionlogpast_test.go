package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// A period with no session to list says it had none, but where every day
// of it is no longer kept, as the words saying so take their place.
func TestAPeriodOfNoSessionSaysSoButWhereEveryDayIsGone(t *testing.T) {
	tests := []struct {
		name   string
		unkept views.UnkeptDays
		says   bool
	}{
		{name: "every day kept", says: true},
		{name: "some days gone", unkept: views.UnkeptDays{KeptFrom: civilDate("2026-09-24")}, says: true},
		{name: "every day gone", unkept: views.UnkeptDays{KeptFrom: civilDate("2026-09-28"), Every: true}},
	}
	for _, tt := range tests {
		if got := tt.unkept.SaysNoSessions(); got != tt.says {
			t.Errorf("%s: SaysNoSessions() = %t, want %t", tt.name, got, tt.says)
		}
	}
}

// The days of a period gone no longer kept are said by the first day kept,
// its year only where it isn't now's; every day, without one; and none,
// not at all.
func TestTheDaysNoLongerKeptAreSaidByTheFirstDayKept(t *testing.T) {
	tests := []struct {
		name   string
		unkept views.UnkeptDays
		want   string
	}{
		{name: "none", want: ""},
		{name: "some", unkept: views.UnkeptDays{KeptFrom: civilDate("2026-09-14")}, want: "sessions before 14 Sep no longer kept"},
		{name: "some, of last year", unkept: views.UnkeptDays{KeptFrom: civilDate("2025-12-30")}, want: "sessions before 30 Dec 2025 no longer kept"},
		{name: "every", unkept: views.UnkeptDays{KeptFrom: civilDate("2026-09-28"), Every: true}, want: "sessions no longer kept"},
	}
	for _, tt := range tests {
		if got := tt.unkept.Said(framesNow); got != tt.want {
			t.Errorf("%s: Said() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// A session of a period gone that went on after it says so in place of its
// end; one that ended says when, by the clock, as its activity does.
func TestASessionThatWentOnSaysSoInPlaceOfItsEnd(t *testing.T) {
	ended := views.Activity{Doing: views.Ended, Since: time.Date(2026, 9, 27, 17, 40, 0, 0, time.Local)}
	tests := []struct {
		name    string
		session views.PeriodSession
		want    string
	}{
		{name: "went on", session: views.PeriodSession{ListedSession: views.ListedSession{Activity: ended}, WentOn: true}, want: "went on"},
		{name: "ended", session: views.PeriodSession{ListedSession: views.ListedSession{Activity: ended}, EndedOn: "2026-09-27"}, want: "ended 17:40"},
	}
	for _, tt := range tests {
		if got := tt.session.DayState(framesNow); got != tt.want {
			t.Errorf("%s: DayState() = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// The List's period line tells what the period's sessions come to, but
// where every day of it is no longer kept, as there's nothing to count.
func TestThePeriodLineTellsNoCountsWhereEveryDayIsGone(t *testing.T) {
	tests := []struct {
		name   string
		unkept views.UnkeptDays
		told   bool
	}{
		{name: "every day kept", told: true},
		{name: "some days gone", unkept: views.UnkeptDays{KeptFrom: civilDate("2026-09-24")}, told: true},
		{name: "every day gone", unkept: views.UnkeptDays{KeptFrom: civilDate("2026-09-28"), Every: true}},
	}
	for _, tt := range tests {
		if got := (views.SessionLog{Unkept: tt.unkept}).CountsTold(); got != tt.told {
			t.Errorf("%s: CountsTold() = %t, want %t", tt.name, got, tt.told)
		}
	}
}
