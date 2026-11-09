package main

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// begins is when the scoring tests' history begins: a Monday, on the hour.
var begins = time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)

// hoursIn is n hours after begins.
func hoursIn(n float64) time.Time {
	return begins.Add(time.Duration(n * float64(time.Hour)))
}

// weekRead is a reading of the account's week at utilization, resetting at
// resets, read at at.
func weekRead(id string, at time.Time, utilization float64, resets time.Time) readings.Reading {
	return readings.Reading{At: at, Account: id, Key: "7d", Utilization: utilization, ResetsAt: resets, Source: readings.FromAnswer}
}

func TestAWeekReachesItsFloorAtItsReserveElseItsLimitOrARejection(t *testing.T) {
	// Work keeps a tenth back; side keeps nothing back. Each week resets 50
	// hours in, and the history ends 60 hours in.
	configured := config.Accounts{{ID: "work", Reserve: 0.1}, {ID: "side"}}
	resets := hoursIn(50)
	tests := []struct {
		name string
		read []readings.Reading
		// account is the week's, and reached when it reached its floor: zero
		// where it didn't.
		account string
		reached time.Time
	}{
		{name: "at its reserve", account: "work", read: []readings.Reading{weekRead("work", hoursIn(1), 0.89, resets), weekRead("work", hoursIn(2), 0.9, resets)}, reached: hoursIn(2)},
		{name: "short of its reserve", account: "work", read: []readings.Reading{weekRead("work", hoursIn(1), 0.89, resets)}},
		{name: "at its limit, without a reserve", account: "side", read: []readings.Reading{weekRead("side", hoursIn(1), 0.95, resets), weekRead("side", hoursIn(3), 1, resets)}, reached: hoursIn(3)},
		{name: "past its reserve without a reserve, short of its limit", account: "side", read: []readings.Reading{weekRead("side", hoursIn(1), 0.95, resets)}},
		{
			name: "rejected short of its limit", account: "side",
			read:    []readings.Reading{{At: hoursIn(4), Account: "side", Key: "7d", Utilization: 0.5, ResetsAt: resets, Status: quota.StatusRejected}},
			reached: hoursIn(4),
		},
		{name: "read at its limit first, and again", account: "side", read: []readings.Reading{weekRead("side", hoursIn(1), 1, resets), weekRead("side", hoursIn(5), 1, resets)}, reached: hoursIn(1)},
		{name: "read at its limit only as it resets", account: "side", read: []readings.Reading{weekRead("side", resets, 1, resets)}},
		{name: "another window at its limit", account: "side", read: []readings.Reading{{At: hoursIn(1), Account: "side", Key: "5h", Utilization: 1, ResetsAt: hoursIn(4)}}},
		{name: "an account not configured, at its limit", account: "gone", read: []readings.Reading{weekRead("gone", hoursIn(1), 1, resets)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := slices.Concat(tt.read, []readings.Reading{weekRead("side", hoursIn(60), 0.1, hoursIn(200))})
			got := outcomesOf(configured, "7d", read).of(forecast{week: weekOf(tt.account, resets), at: hoursIn(0.5)})
			want := tt.reached
			if want.IsZero() {
				want = resets
			}
			if !got.known || !got.ended.Equal(want) {
				t.Errorf("the week ended at %v (%t), want %v", got.ended, got.known, want)
			}
		})
	}
}

func TestTheHistoryDoesntSayHowAWeekEndsWhereItEndsFirst(t *testing.T) {
	// Work's week, read 30% used, resets 50 hours in; the history ends 40
	// hours in.
	read := []readings.Reading{weekRead("work", hoursIn(1), 0.3, hoursIn(50)), weekRead("work", hoursIn(40), 0.35, hoursIn(50))}

	if got := outcomesOf(config.Accounts{{ID: "work"}}, "7d", read).of(forecast{week: weekOf("work", hoursIn(50)), at: hoursIn(1)}); got.known {
		t.Errorf("the week ended at %v, want no end said: the history ends before it resets", got.ended)
	}
}

func TestAWeekResetByHandBeginsARunOfItsOwn(t *testing.T) {
	// Work's week, resetting 100 hours in, is half used an hour in, used up
	// 9h12m in, reset by hand 9h30m in, and used up again 60 hours in. Side's,
	// resetting 100 hours in too, is half used an hour in, reset by hand 5
	// hours in, before it reached its floor, and used up 50 hours in; then it
	// dips 2 points 51 hours in, short of a reset by hand. The history ends 70
	// hours in.
	read := []readings.Reading{
		weekRead("work", hoursIn(1), 0.5, hoursIn(100)),
		weekRead("side", hoursIn(1), 0.5, hoursIn(100)),
		weekRead("side", hoursIn(5), 0.3, hoursIn(100)),
		weekRead("work", hoursIn(9.2), 1, hoursIn(100)),
		weekRead("work", hoursIn(9.5), 0.05, hoursIn(100)),
		weekRead("side", hoursIn(50), 1, hoursIn(100)),
		weekRead("side", hoursIn(51), 0.98, hoursIn(100)),
		weekRead("work", hoursIn(60), 1, hoursIn(100)),
		weekRead("personal", hoursIn(70), 0.1, hoursIn(200)),
	}
	o := outcomesOf(config.Accounts{{ID: "work"}, {ID: "side"}, {ID: "personal"}}, "7d", read)
	tests := []struct {
		name    string
		account string
		at      time.Time
		want    outcome
	}{
		{name: "before the first run reached its floor", account: "work", at: hoursIn(9), want: outcome{ended: hoursIn(9.2), known: true}},
		{name: "after the first run reached its floor, before its reset by hand", account: "work", at: hoursIn(9.25), want: outcome{ended: hoursIn(9.2), known: true}},
		{name: "after the reset by hand", account: "work", at: hoursIn(10), want: outcome{began: hoursIn(9.5), ended: hoursIn(60), known: true}},
		{name: "in a run cut short by a reset by hand", account: "side", at: hoursIn(3)},
		{name: "after that reset by hand", account: "side", at: hoursIn(6), want: outcome{began: hoursIn(5), ended: hoursIn(50), known: true}},
		{name: "after a dip short of a reset by hand", account: "side", at: hoursIn(52), want: outcome{began: hoursIn(5), ended: hoursIn(50), known: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := o.of(forecast{week: weekOf(tt.account, hoursIn(100)), at: tt.at}); got != tt.want {
				t.Errorf("of() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAForecastMoreThan12HoursEitherWayIsEarlyOrLate(t *testing.T) {
	tests := []struct {
		name        string
		off         time.Duration
		early, late int
	}{
		{name: "on time", off: 0},
		{name: "12 hours early", off: -12 * time.Hour},
		{name: "past 12 hours early", off: -12*time.Hour - time.Second, early: 1},
		{name: "12 hours late", off: 12 * time.Hour},
		{name: "past 12 hours late", off: 12*time.Hour + time.Second, late: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tl tally
			tl.add(tt.off)
			if tl.early != tt.early || tl.late != tt.late || tl.off != tt.off.Abs() {
				t.Errorf("off by %v, tallied early %d, late %d, off %v; want early %d, late %d, off %v", tt.off, tl.early, tl.late, tl.off, tt.early, tt.late, tt.off.Abs())
			}
		})
	}
}

func TestAForecastIsBucketedByHowMuchOfItsWeekWasUsed(t *testing.T) {
	tests := []struct {
		used float64
		want int
	}{
		{used: 0, want: 0}, {used: 0.399, want: 0}, {used: 0.4, want: 1}, {used: 0.699, want: 1}, {used: 0.7, want: 2}, {used: 1, want: 2},
	}
	for _, tt := range tests {
		t.Run(strconv.FormatFloat(tt.used, 'g', -1, 64), func(t *testing.T) {
			if got := bucketOf(tt.used); got != tt.want {
				t.Errorf("bucketOf(%v) = %d, want %d", tt.used, got, tt.want)
			}
		})
	}
}

func TestAWeekIsForecastWhereItRunsOutElseAtItsResetWhereItHeadsShort(t *testing.T) {
	// The week, a third used, resets 100 hours in; the forecast is made an
	// hour in.
	resets := hoursIn(100)
	read := status.AsRead{Used: 1.0 / 3, ResetsAt: resets, At: begins}
	onPace := status.Heading{Kind: score.OnPace, AtReset: 0.8}
	runsOut := status.RunOut{At: hoursIn(40), Kind: score.RunsOut}
	tests := []struct {
		name string
		w    status.WindowNow
		// ends is when it's forecast to end: none where it isn't forecast.
		ends time.Time
	}{
		{name: "running out", w: status.WindowNow{Kind: status.WindowRunsOut, Out: runsOut, Read: read}, ends: hoursIn(40)},
		{name: "heading short", w: status.WindowNow{Kind: status.WindowOpen, Out: status.RunOut{Heading: onPace}, Read: read}, ends: resets},
		{name: "heading nowhere", w: status.WindowNow{Kind: status.WindowOpen, Read: read}},
		{name: "at its cap", w: status.WindowNow{Kind: status.WindowCapped, Out: status.RunOut{Heading: onPace}, Read: read}},
		{name: "held at its limit", w: status.WindowNow{Kind: status.WindowLimited, Read: read}},
		{name: "reset since it was read, its next week heading short", w: status.WindowNow{Kind: status.WindowOpen, Out: status.RunOut{Heading: onPace}, Read: read, ResetSinceRead: true}},
		{name: "its reset unknown", w: status.WindowNow{Kind: status.WindowOpen, Out: status.RunOut{Heading: onPace}, Read: status.AsRead{Used: 1.0 / 3}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := forecastOf("work", tt.w, hoursIn(1))
			if ok == tt.ends.IsZero() || ok && (!f.ends.Equal(tt.ends) || f.week != weekOf("work", resets) || !f.at.Equal(hoursIn(1))) {
				t.Errorf("forecastOf() = %+v, %t; want it to end at %v, none where that's zero", f, ok, tt.ends)
			}
		})
	}
}

func TestForecastsScoreAgainstWhatBecameOfTheirWeeks(t *testing.T) {
	// Work's week, resetting 100 hours in, ran out 50 hours in, then dipped
	// back below its floor an hour on, short of a reset by hand; side's,
	// resetting 60 hours in, didn't run out; personal's resets after the
	// history ends, 80 hours in, short of its floor.
	read := []readings.Reading{
		weekRead("work", hoursIn(50), 1, hoursIn(100)),
		weekRead("work", hoursIn(51), 0.95, hoursIn(100)),
		weekRead("side", hoursIn(1), 0.1, hoursIn(60)),
		weekRead("personal", hoursIn(80), 0.2, hoursIn(120)),
	}
	o := outcomesOf(config.Accounts{{ID: "work"}, {ID: "side"}, {ID: "personal"}}, "7d", read)
	worksWeek, sidesWeek := weekOf("work", hoursIn(100)), weekOf("side", hoursIn(60))
	forecasts := []forecast{
		// 13 hours late, then 13 early, an hour on: it moved 26 hours.
		{week: worksWeek, at: hoursIn(10), used: 0.3, ends: hoursIn(63)},
		{week: worksWeek, at: hoursIn(11), used: 0.45, ends: hoursIn(37)},
		// Two hours on, not an hour, then an hour after: on time, then an
		// hour late, having moved an hour.
		{week: worksWeek, at: hoursIn(13), used: 0.75, ends: hoursIn(50)},
		{week: worksWeek, at: hoursIn(14), used: 0.8, ends: hoursIn(51)},
		// Made once it had run out, after its dip, and as it did.
		{week: worksWeek, at: hoursIn(50), used: 1, ends: hoursIn(50)},
		{week: worksWeek, at: hoursIn(52), used: 0.95, ends: hoursIn(60)},
		{week: worksWeek, at: hoursIn(53), used: 0.95, ends: hoursIn(61)},
		// Run out 20 hours before a reset it reached short.
		{week: sidesWeek, at: hoursIn(10), used: 0.1, ends: hoursIn(40)},
		// Of weeks the history doesn't say the end of.
		{week: weekOf("side", hoursIn(228)), at: hoursIn(70), used: 0.05, ends: hoursIn(228)},
		{week: weekOf("personal", hoursIn(120)), at: hoursIn(10), used: 0.2, ends: hoursIn(120)},
	}

	got := scoresOf(forecasts, o)
	want := scores{
		byUse: [3]tally{
			{forecasts: 2, early: 1, late: 1, off: 33 * time.Hour},
			{forecasts: 1, early: 1, off: 13 * time.Hour, pairs: 1, moved: 26 * time.Hour},
			{forecasts: 2, off: time.Hour, pairs: 1, moved: time.Hour},
		},
		all: tally{forecasts: 5, early: 2, late: 1, off: 47 * time.Hour, pairs: 2, moved: 27 * time.Hour},
	}
	if got != want {
		t.Errorf("the forecasts scored\n%+v\nwant\n%+v", got, want)
	}
}

func TestAForecastPairsOnlyWithTheHourBeforesOfItsRun(t *testing.T) {
	// Work's week, resetting 100 hours in, is half used an hour in, used up
	// 9h12m in, reset by hand 9h30m in, and used up again 60 hours in; the
	// history ends 70 hours in. It's forecast on time 9 hours in, and on time
	// an hour on, after the reset by hand, then two hours late an hour after
	// that, having moved two hours.
	read := []readings.Reading{
		weekRead("work", hoursIn(1), 0.5, hoursIn(100)),
		weekRead("work", hoursIn(9.2), 1, hoursIn(100)),
		weekRead("work", hoursIn(9.5), 0.05, hoursIn(100)),
		weekRead("work", hoursIn(60), 1, hoursIn(100)),
		weekRead("work", hoursIn(70), 0.1, hoursIn(200)),
	}
	o := outcomesOf(config.Accounts{{ID: "work"}}, "7d", read)
	worksWeek := weekOf("work", hoursIn(100))
	forecasts := []forecast{
		{week: worksWeek, at: hoursIn(9), used: 0.9, ends: hoursIn(9.2)},
		{week: worksWeek, at: hoursIn(10), used: 0.1, ends: hoursIn(60)},
		{week: worksWeek, at: hoursIn(11), used: 0.12, ends: hoursIn(62)},
	}

	got := scoresOf(forecasts, o)
	want := scores{
		byUse: [3]tally{{forecasts: 2, off: 2 * time.Hour, pairs: 1, moved: 2 * time.Hour}, {}, {forecasts: 1}},
		all:   tally{forecasts: 3, off: 2 * time.Hour, pairs: 1, moved: 2 * time.Hour},
	}
	if got != want {
		t.Errorf("the forecasts scored\n%+v\nwant\n%+v", got, want)
	}
}
