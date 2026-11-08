package forecast

import (
	"math"
	"testing"
	"time"
)

// ratesFrom is when the rates' tests start.
var ratesFrom = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// ratesAt is d after ratesFrom.
func ratesAt(d time.Duration) time.Time { return ratesFrom.Add(d) }

func TestFastRate(t *testing.T) {
	tick := Fast{}.Rose(0.01, ratesFrom)
	tests := []struct {
		name string
		fast Fast
		at   time.Time
		want float64
	}{
		{"no rise reads 0", Fast{}, ratesFrom, 0},
		{"a point's tick reads as a point an hour as it comes", tick, ratesFrom, 0.01},
		{"a tick an hour on reads e^-1 of that", tick, ratesAt(time.Hour), 0.01 / math.E},
		{"a tick reads less as it ages", tick, ratesAt(30 * time.Minute), 0.01 * math.Exp(-0.5)},
		{"a tick decays to near nothing", tick, ratesAt(24 * time.Hour), 0.01 * math.Exp(-24)},
		{
			"rises add up, each decayed",
			tick.Rose(0.02, ratesAt(30*time.Minute)).Rose(0.01, ratesAt(time.Hour)),
			ratesAt(time.Hour), 0.01/math.E + 0.02*math.Exp(-0.5) + 0.01,
		},
		{"a rise before the last counts as at the last", Fast{}.Rose(0.01, ratesAt(time.Hour)).Rose(0.01, ratesFrom), ratesAt(time.Hour), 0.02},
		{"a time before the last rise reads as at it", Fast{}.Rose(0.01, ratesAt(time.Hour)), ratesFrom, 0.01},
		{"a fall is no rise, and never reads negative", tick.Rose(-0.05, ratesFrom), ratesFrom, 0.01},
		{"a rise of no number is no rise", tick.Rose(math.NaN(), ratesFrom), ratesFrom, 0.01},
		{"a rise without end is no rise", tick.Rose(math.Inf(1), ratesFrom), ratesFrom, 0.01},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.fast.Rate(tt.at); math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("Rate(%v) = %v, want %v", tt.at.Sub(ratesFrom), got, tt.want)
			}
		})
	}
}

func TestDayRate(t *testing.T) {
	opened := Day{}.Open(true, ratesFrom)
	tests := []struct {
		name     string
		day      Day
		at       time.Time
		want     float64
		wantOver time.Duration
		wantOK   bool
	}{
		{name: "no room has no rate", day: Day{}.Rose(1, ratesAt(time.Hour)), at: ratesAt(3 * time.Hour)},
		{name: "under 2 hours of room has no rate", day: opened.Rose(1, ratesAt(time.Hour)), at: ratesAt(119 * time.Minute)},
		{
			name: "2 hours of room has one", day: opened.Rose(1, ratesAt(time.Hour)), at: ratesAt(2 * time.Hour),
			want: 0.5, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "hours without room don't count, but rises in them do",
			day: opened.Rose(1, ratesAt(time.Hour)).Open(false, ratesAt(2*time.Hour)).
				Rose(1, ratesAt(5*time.Hour)).Open(true, ratesAt(6*time.Hour)),
			at:   ratesAt(8 * time.Hour),
			want: 0.5, wantOver: 4 * time.Hour, wantOK: true,
		},
		{
			name: "hours closed now don't count, but rises in them do",
			day:  opened.Rose(1, ratesAt(time.Hour)).Open(false, ratesAt(2*time.Hour)).Rose(1, ratesAt(5*time.Hour)),
			at:   ratesAt(8 * time.Hour),
			want: 1, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "over the last 24 hours with room at the most",
			day:  opened.Rose(1, ratesAt(5*time.Hour)).Rose(1, ratesAt(10*time.Hour)).Rose(2, ratesAt(30*time.Hour)),
			at:   ratesAt(30 * time.Hour),
			want: 3.0 / 24, wantOver: 24 * time.Hour, wantOK: true,
		},
		{
			name: "the span reaches back across a closed stretch",
			day: opened.Rose(1, ratesAt(3*time.Hour)).Open(false, ratesAt(10*time.Hour)).
				Rose(1, ratesAt(20*time.Hour)).Open(true, ratesAt(30*time.Hour)).Rose(1, ratesAt(40*time.Hour)),
			at:   ratesAt(44 * time.Hour),
			want: 3.0 / 24, wantOver: 24 * time.Hour, wantOK: true,
		},
		{
			name: "a rise at the span's start is of use before it",
			day:  opened.Rose(1, ratesAt(time.Hour)).Rose(1, ratesAt(2*time.Hour)),
			at:   ratesAt(25 * time.Hour),
			want: 1.0 / 24, wantOver: 24 * time.Hour, wantOK: true,
		},
		{
			name: "a rise just after the span's start counts",
			day:  opened.Rose(1, ratesAt(time.Hour+time.Minute)),
			at:   ratesAt(25 * time.Hour),
			want: 1.0 / 24, wantOver: 24 * time.Hour, wantOK: true,
		},
		{
			name: "a rise as the room first opens is of use before it",
			day:  Day{}.Rose(1, ratesFrom).Open(true, ratesFrom),
			at:   ratesAt(2 * time.Hour),
			want: 0, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "a closed stretch inside the span counts its rises",
			day: opened.Open(false, ratesAt(time.Hour)).Rose(1, ratesAt(2*time.Hour)).
				Open(true, ratesAt(3*time.Hour)),
			at:   ratesAt(26*time.Hour + 30*time.Minute),
			want: 1.0 / 24, wantOver: 24 * time.Hour, wantOK: true,
		},
		{
			name: "a closed stretch just before the span's start leaves its rises out",
			day: opened.Open(false, ratesAt(time.Hour)).Rose(1, ratesAt(2*time.Hour)).
				Open(true, ratesAt(3*time.Hour)),
			at:   ratesAt(27 * time.Hour),
			want: 0, wantOver: 24 * time.Hour, wantOK: true,
		},
		{
			name: "room that closes as it opens adds none",
			day:  Day{}.Open(true, ratesAt(time.Hour)).Open(false, ratesAt(time.Hour)).Rose(1, ratesAt(2*time.Hour)),
			at:   ratesAt(5 * time.Hour),
		},
		{
			name: "a rise before the latest time counts as at it",
			day:  opened.Rose(1, ratesAt(4*time.Hour)).Rose(1, ratesAt(-time.Hour)),
			at:   ratesAt(4 * time.Hour),
			want: 0.5, wantOver: 4 * time.Hour, wantOK: true,
		},
		{
			name: "a fall is no rise",
			day:  opened.Rose(1, ratesAt(time.Hour)).Rose(-1, ratesAt(2*time.Hour)),
			at:   ratesAt(2 * time.Hour),
			want: 0.5, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "a rise of no number is no rise",
			day:  opened.Rose(1, ratesAt(time.Hour)).Rose(math.NaN(), ratesAt(2*time.Hour)),
			at:   ratesAt(2 * time.Hour),
			want: 0.5, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "a rise without end is no rise",
			day:  opened.Rose(1, ratesAt(time.Hour)).Rose(math.Inf(1), ratesAt(2*time.Hour)),
			at:   ratesAt(2 * time.Hour),
			want: 0.5, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "room opened again while open stays one stretch",
			day:  opened.Open(true, ratesAt(time.Hour)).Rose(1, ratesAt(time.Hour)),
			at:   ratesAt(2 * time.Hour),
			want: 0.5, wantOver: 2 * time.Hour, wantOK: true,
		},
		{
			name: "room closed while it never opened stays none",
			day:  Day{}.Open(false, ratesAt(time.Hour)).Rose(1, ratesAt(2*time.Hour)),
			at:   ratesAt(5 * time.Hour),
		},
		{
			name: "room turned before the latest time turns at it",
			day:  opened.Rose(1, ratesAt(4*time.Hour)).Open(false, ratesAt(time.Hour)),
			at:   ratesAt(6 * time.Hour),
			want: 0.25, wantOver: 4 * time.Hour, wantOK: true,
		},
		{
			name: "a rate asked before the latest time is as at it",
			day:  opened.Rose(1, ratesAt(4*time.Hour)),
			at:   ratesAt(time.Hour),
			want: 0.25, wantOver: 4 * time.Hour, wantOK: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, over, ok := tt.day.Rate(tt.at)
			if math.Abs(got-tt.want) > 1e-12 || over != tt.wantOver || ok != tt.wantOK {
				t.Errorf("Rate(%v) = %v, %v, %v, want %v, %v, %v", tt.at.Sub(ratesFrom), got, over, ok, tt.want, tt.wantOver, tt.wantOK)
			}
		})
	}
}

func TestDayLeavesTheDayItCameFromAsItWas(t *testing.T) {
	open := Day{}.Open(true, ratesFrom).Rose(1, ratesAt(time.Hour))
	closed := open.Open(false, ratesAt(2*time.Hour)).Rose(1, ratesAt(3*time.Hour))
	at := ratesAt(4 * time.Hour)
	wantOpen, wantClosed := 1.0/4, 2.0/2

	_ = open.Open(false, ratesAt(3*time.Hour))
	_ = closed.Rose(5, ratesAt(4*time.Hour))
	_ = closed.Open(true, ratesAt(4*time.Hour)).Rose(5, ratesAt(4*time.Hour))

	if got, _, _ := open.Rate(at); got != wantOpen {
		t.Errorf("open.Rate(4h) = %v after another closed it, want %v", got, wantOpen)
	}
	if got, _, _ := closed.Rate(at); got != wantClosed {
		t.Errorf("closed.Rate(4h) = %v after others rose from it, want %v", got, wantClosed)
	}
}

func TestDayStaysSmallAcrossAWeek(t *testing.T) {
	const step = 10 * time.Minute
	// Room for 6 hours, then none for 6, from ratesFrom, rising a point
	// every 10 minutes throughout.
	day := Day{}
	for at := time.Duration(0); at <= 7*24*time.Hour; at += step {
		day = day.Open(at/(6*time.Hour)%2 == 0, ratesAt(at))
		day = day.Rose(0.01, ratesAt(at))
	}

	// 24 hours of room back from the week's end span 48 hours, a rise every
	// 10 minutes of them: 2.88 Pro weeks.
	got, over, ok := day.Rate(ratesAt(7 * 24 * time.Hour))
	if math.Abs(got-2.88/24) > 1e-9 || over != 24*time.Hour || !ok {
		t.Errorf("Rate(a week) = %v, %v, %v, want %v, 24h, true", got, over, ok, 2.88/24)
	}
	if most := int(48 * time.Hour / step); len(day.rises) > most || len(day.rooms) > 5 {
		t.Errorf("after a week: %d rises and %d rooms kept, want %d and 5 at the most", len(day.rises), len(day.rooms), most)
	}
}

func TestDayWithoutRoomKeepsItsRisesAsOne(t *testing.T) {
	day := Day{}.Open(true, ratesFrom).Open(false, ratesAt(3*time.Hour))
	for at := 4 * time.Hour; at <= 7*24*time.Hour; at += 10 * time.Minute {
		day = day.Rose(0.01, ratesAt(at))
	}

	if len(day.rises) != 1 {
		t.Errorf("after a week without room: %d rises kept, want 1", len(day.rises))
	}
	if got, over, ok := day.Rate(ratesAt(7 * 24 * time.Hour)); math.Abs(got-9.85/3) > 1e-9 || over != 3*time.Hour || !ok {
		t.Errorf("Rate(a week) = %v, %v, %v, want %v, 3h, true", got, over, ok, 9.85/3)
	}
}
