package prime_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prime"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// policy primes as Claude's windows are primed: a request starts the
// five-hour session.
var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h"}

// zone is the local time zone of every test but those of days the clocks
// change on: an hour east of UTC.
var zone = time.FixedZone("UTC+1", 60*60)

// days are the days the tests schedule over.
var (
	daytime = config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}
	// night runs past midnight.
	night = config.Day{Start: 22 * time.Hour, End: 6 * time.Hour}
	// early starts early enough that its slots fall the evening before.
	early = config.Day{Start: 2 * time.Hour, End: 20 * time.Hour}
)

func TestSlots(t *testing.T) {
	tests := []struct {
		name     string
		day      config.Day
		accounts []string
		// want are the slots, as account@HH:MM.
		want []string
	}{
		{name: "three accounts, an hour and forty minutes apart", day: daytime, accounts: []string{"work", "personal", "side"}, want: []string{"work@03:50", "personal@05:30", "side@07:10"}},
		{name: "two accounts, two and a half hours apart, each taken back to its mark", day: daytime, accounts: []string{"work", "side"}, want: []string{"work@04:10", "side@06:40"}},
		{name: "four accounts, as near an hour and a quarter apart as the marks allow", day: daytime, accounts: []string{"a", "b", "c", "d"}, want: []string{"a@03:30", "b@04:50", "c@06:00", "d@07:20"}},
		{name: "one account", day: daytime, accounts: []string{"work"}, want: []string{"work@05:30"}},
		{name: "a day past midnight", day: night, accounts: []string{"work", "side"}, want: []string{"work@18:10", "side@20:40"}},
		{name: "a day whose slots fall the evening before", day: early, accounts: []string{"work", "personal", "side"}, want: []string{"work@21:50", "personal@23:30", "side@01:10"}},
		{name: "a day whose slots fall the evening before, taken back to their marks", day: early, accounts: []string{"work", "side"}, want: []string{"work@22:10", "side@00:40"}},
		{name: "seven accounts, each taken back to its mark", day: daytime, accounts: []string{"a", "b", "c", "d", "e", "f", "g"}, want: []string{"a@03:20", "b@04:00", "c@04:40", "d@05:30", "e@06:10", "f@06:50", "g@07:30"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := prime.New(tt.day, tt.accounts, policy)
			if !ok {
				t.Fatal("New() = false, want a schedule")
			}
			if got := slotsOf(s); !slices.Equal(got, tt.want) {
				t.Errorf("slots = %q, want %q", got, tt.want)
			}
			if s.Day() != tt.day || s.Window() != "5h" {
				t.Errorf("the schedule's day and window are %+v and %q, want %+v and 5h", s.Day(), s.Window(), tt.day)
			}
		})
	}
}

func TestSlotsFallOnMarksAsEvenlyAsTheyAllowBeforeTheDayStarts(t *testing.T) {
	for n := 1; n <= 12; n++ {
		t.Run(fmt.Sprintf("%d accounts", n), func(t *testing.T) {
			accounts := make([]string, n)
			for i := range accounts {
				accounts[i] = fmt.Sprintf("account-%d", i)
			}
			s, _ := prime.New(daytime, accounts, policy)
			slots := s.Slots()
			for _, slot := range slots {
				if slot.At%(10*time.Minute) != 0 || slot.At+5*time.Second >= daytime.Start {
					t.Errorf("%s's slot is %v, want a ten-minute mark whose prime falls before the day starts", slot.Account, slot.At)
				}
			}
			// The steps between the slots, the last's to the first's next
			// window included, are as even as ten-minute marks allow.
			var steps []time.Duration
			for i := range slots {
				next := slots[0].At + 5*time.Hour
				if i+1 < len(slots) {
					next = slots[i+1].At
				}
				steps = append(steps, next-slots[i].At)
			}
			if spread := slices.Max(steps) - slices.Min(steps); spread > 10*time.Minute {
				t.Errorf("steps between slots %v, want them within ten minutes of each other", steps)
			}
		})
	}
}

func TestNoSchedule(t *testing.T) {
	tests := []struct {
		name     string
		day      config.Day
		accounts []string
		policy   score.Policy
	}{
		{name: "without a day", accounts: []string{"work"}, policy: policy},
		{name: "without an account", day: daytime, policy: policy},
		{name: "without a window a request starts", day: daytime, accounts: []string{"work"}, policy: score.Policy{Shared: policy.Shared}},
		{name: "with one whose length its key doesn't give", day: daytime, accounts: []string{"work"}, policy: score.Policy{Started: "session"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if s, ok := prime.New(tt.day, tt.accounts, tt.policy); ok {
				t.Errorf("New() = %+v, true, want no schedule", slotsOf(s))
			}
		})
	}
}

func TestNext(t *testing.T) {
	s, _ := prime.New(daytime, []string{"work", "side"}, policy)
	// Work's slot is 04:10, side's 06:40, and the day ends at 23:00. Each is
	// primed just after its slot, as after a reset.
	running := func(resetsAt time.Time) []quota.Window {
		return []quota.Window{
			{Key: "5h", Label: "Session", Utilization: 0.3, ResetsAt: resetsAt},
			{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: at(2, 21, 0).AddDate(0, 0, 4)},
		}
	}
	tests := []struct {
		name    string
		account string
		windows []quota.Window
		now     time.Time
		want    time.Time
		wantOK  bool
	}{
		{name: "never read, just after its slot", account: "work", now: at(1, 0, 0), want: justAfter(at(1, 4, 10)), wantOK: true},
		{name: "never read, its slot come, just after it", account: "work", now: at(1, 4, 10), want: justAfter(at(1, 4, 10)), wantOK: true},
		{name: "never read, just after its slot come", account: "work", now: justAfter(at(1, 4, 10)), want: justAfter(at(1, 4, 10)), wantOK: true},
		{name: "never read, past its slot, missed, as the day runs", account: "work", now: at(1, 9, 30), want: at(1, 9, 30), wantOK: true},
		{name: "never read, the day over, just after tomorrow's slot", account: "work", now: at(1, 23, 0), want: justAfter(at(2, 4, 10)), wantOK: true},
		{name: "the other account, just after its own slot", account: "side", now: at(1, 5, 0), want: justAfter(at(1, 6, 40)), wantOK: true},
		{name: "lapsed as the day runs, at once", account: "work", windows: running(at(1, 13, 0)), now: at(1, 13, 40), want: at(1, 13, 40), wantOK: true},
		{name: "running as the day runs, just after its reset", account: "work", windows: running(at(1, 14, 15)), now: at(1, 10, 0), want: justAfter(at(1, 14, 15)), wantOK: true},
		{name: "its reset passed a moment ago, just after it", account: "work", windows: running(at(1, 14, 15)), now: at(1, 14, 15).Add(time.Second), want: justAfter(at(1, 14, 15)), wantOK: true},
		{name: "running at its slot, from a late night, just after its reset", account: "work", windows: running(at(1, 6, 0)), now: at(1, 4, 10), want: justAfter(at(1, 6, 0)), wantOK: true},
		{name: "running until after the day ends, just after tomorrow's slot", account: "work", windows: running(at(1, 23, 50)), now: at(1, 20, 0), want: justAfter(at(2, 4, 10)), wantOK: true},
		{
			name:    "its reset six seconds before the day ends, just after it",
			account: "work",
			windows: running(at(1, 23, 0).Add(-6 * time.Second)),
			now:     at(1, 20, 0),
			want:    justAfter(at(1, 23, 0).Add(-6 * time.Second)),
			wantOK:  true,
		},
		{
			name:    "its reset five seconds before the day ends, its prime falling as the day ends, just after tomorrow's slot",
			account: "work",
			windows: running(at(1, 23, 0).Add(-5 * time.Second)),
			now:     at(1, 20, 0),
			want:    justAfter(at(2, 4, 10)),
			wantOK:  true,
		},
		{name: "running until after the day ends, its reset read in UTC, just after tomorrow's slot", account: "work", windows: running(at(1, 23, 50).UTC()), now: at(1, 20, 0), want: justAfter(at(2, 4, 10)), wantOK: true},
		{name: "lapsed overnight, just after its slot", account: "work", windows: running(at(1, 23, 50)), now: at(2, 1, 0), want: justAfter(at(2, 4, 10)), wantOK: true},
		{name: "read without the window a request starts", account: "work", windows: running(at(1, 14, 15))[1:], now: at(1, 10, 0)},
		{name: "read without a reset", account: "work", windows: []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.3}}, now: at(1, 10, 0)},
		{name: "an account it doesn't have", account: "personal", now: at(1, 4, 10)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := s.Next(tt.account, tt.windows, tt.now)
			if !got.Equal(tt.want) || ok != tt.wantOK {
				t.Errorf("Next() = %v, %v, want %v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestClear(t *testing.T) {
	s, _ := prime.New(daytime, []string{"work", "side"}, policy)
	// Work's slot is 04:10, side's 06:40, and the day ends at 23:00: a
	// window started at 01:40 resets at side's slot, before its prime goes.
	tests := []struct {
		name     string
		schedule prime.Schedule
		account  string
		at       time.Time
		want     bool
	}{
		{name: "more than five hours before its slot", schedule: s, account: "side", at: at(1, 1, 30), want: true},
		{name: "five hours before its slot, to the minute", schedule: s, account: "side", at: at(1, 1, 40), want: true},
		{name: "a minute less than five hours before its slot", schedule: s, account: "side", at: at(1, 1, 41)},
		{name: "the evening before, within five hours of its slot", schedule: s, account: "work", at: at(1, 23, 30)},
		{name: "the evening before, more than five hours before its slot", schedule: s, account: "side", at: at(1, 23, 30), want: true},
		{name: "as its day of priming runs, a prime due at once", schedule: s, account: "work", at: at(1, 12, 0)},
		{name: "an account it doesn't have", schedule: s, account: "personal", at: at(1, 1, 41), want: true},
		{name: "without a schedule", account: "side", at: at(1, 1, 41), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.schedule.Clear(tt.account, tt.at); got != tt.want {
				t.Errorf("Clear() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClearCountsTheHoursElapsedOnADayTheClocksChange(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load Europe/London: %v", err)
	}
	s, _ := prime.New(daytime, []string{"work", "side"}, policy)
	// Side's slot is 06:40 on the clock.
	tests := []struct {
		name string
		at   time.Time
		want bool
	}{
		{
			name: "the clocks going forward at 01:00, five hours and more before it on the clock, less elapsed",
			at:   time.Date(2026, time.March, 29, 0, 45, 0, 0, london),
		},
		{
			// 01:50 comes twice that night: this is the first, still summer time.
			name: "the clocks going back at 02:00, less than five hours before it on the clock, more elapsed",
			at:   time.Date(2026, time.October, 25, 0, 50, 0, 0, time.UTC).In(london),
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.Clear("side", tt.at); got != tt.want {
				t.Errorf("Clear() = %v, want %v", got, tt.want)
			}
		})
	}
}

// justAfter is a few seconds after a reset or a slot at t, when a prime is
// sent, so the upstream, its clock a little behind, takes it in after it.
func justAfter(t time.Time) time.Time {
	return t.Add(5 * time.Second)
}

func TestNextOverADayPastMidnight(t *testing.T) {
	s, _ := prime.New(night, []string{"work", "side"}, policy)
	// Work's slot is 18:10, and the day runs from 22:00 to 06:00.
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{name: "before its slot", now: at(1, 12, 0), want: justAfter(at(1, 18, 10))},
		{name: "at its slot", now: at(1, 18, 10), want: justAfter(at(1, 18, 10))},
		{name: "before midnight", now: at(1, 23, 30), want: at(1, 23, 30)},
		{name: "after midnight, the day still running", now: at(2, 5, 59), want: at(2, 5, 59)},
		{name: "once the day ends, at that evening's slot", now: at(2, 6, 0), want: justAfter(at(2, 18, 10))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := s.Next("work", nil, tt.now); !got.Equal(tt.want) || !ok {
				t.Errorf("Next() = %v, %v, want %v, true", got, ok, tt.want)
			}
		})
	}
}

func TestNextWhenTheSlotFallsTheEveningBefore(t *testing.T) {
	s, _ := prime.New(early, []string{"work", "personal", "side"}, policy)
	// Work's slot is 21:50 the evening before the day, which runs from 02:00
	// to 20:00.
	tests := []struct {
		name string
		now  time.Time
		want time.Time
	}{
		{name: "the evening before, at its slot", now: at(1, 21, 0), want: justAfter(at(1, 21, 50))},
		{name: "after midnight, missed", now: at(2, 1, 0), want: at(2, 1, 0)},
		{name: "once the day ends, at that evening's slot", now: at(2, 20, 0), want: justAfter(at(2, 21, 50))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := s.Next("work", nil, tt.now); !got.Equal(tt.want) || !ok {
				t.Errorf("Next() = %v, %v, want %v, true", got, ok, tt.want)
			}
		})
	}
}

func TestAnIdleDayResetsAsTheDesignSays(t *testing.T) {
	tests := []struct {
		accounts []string
		// primes and resets are when each is due, and when each window it
		// starts resets within the day, as HH:MM.
		primes, resets []string
	}{
		{
			accounts: []string{"work", "personal", "side"},
			primes:   []string{"03:50", "05:30", "07:10", "08:50", "10:30", "12:10", "13:50", "15:30", "17:10", "18:50", "20:30", "22:10"},
			resets:   []string{"08:50", "10:30", "12:10", "13:50", "15:30", "17:10", "18:50", "20:30", "22:10"},
		},
		{
			accounts: []string{"work", "side"},
			primes:   []string{"04:10", "06:40", "09:10", "11:40", "14:10", "16:40", "19:10", "21:40"},
			resets:   []string{"09:10", "11:40", "14:10", "16:40", "19:10", "21:40"},
		},
		{
			accounts: []string{"a", "b", "c", "d"},
			primes: []string{
				"03:30", "04:50", "06:00", "07:20", "08:30", "09:50", "11:00", "12:20",
				"13:30", "14:50", "16:00", "17:20", "18:30", "19:50", "21:00", "22:20",
			},
			resets: []string{"08:30", "09:50", "11:00", "12:20", "13:30", "14:50", "16:00", "17:20", "18:30", "19:50", "21:00", "22:20"},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d accounts", len(tt.accounts)), func(t *testing.T) {
			s, _ := prime.New(daytime, tt.accounts, policy)
			primes, resets := idleDay(t, s, tt.accounts, at(1, 0, 0), at(2, 0, 0))
			if !slices.Equal(primes, tt.primes) {
				t.Errorf("primed at %q, want %q", primes, tt.primes)
			}
			if !slices.Equal(resets, tt.resets) {
				t.Errorf("reset at %q within the day, want %q", resets, tt.resets)
			}
		})
	}
}

// idleDay primes the accounts from from until until, as each falls due, with
// nothing else using them, and returns when they were primed, and when the
// windows the primes started reset within the day, each as HH:MM, in order.
// Each window starts, as the upstream starts it, at the ten-minute mark its
// prime falls in.
func idleDay(t *testing.T, s prime.Schedule, accounts []string, from, until time.Time) (primes, resets []string) {
	t.Helper()
	windows := make(map[string][]quota.Window)
	for now := from; ; {
		account, due, ok := soonest(s, accounts, windows, now)
		if !ok || !due.Before(until) {
			return primes, resets
		}
		now = due
		reset := now.Truncate(10 * time.Minute).Add(5 * time.Hour)
		windows[account] = []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.01, ResetsAt: reset}}
		primes = append(primes, now.Format("15:04"))
		if reset.Before(at(1, 23, 0)) {
			resets = append(resets, reset.Format("15:04"))
		}
		if len(primes) > 100 {
			t.Fatalf("still priming at %v", now)
		}
	}
}

// soonest returns the account next due a prime at now or after, and when.
func soonest(s prime.Schedule, accounts []string, windows map[string][]quota.Window, now time.Time) (string, time.Time, bool) {
	var first string
	var due time.Time
	for _, account := range accounts {
		if next, ok := s.Next(account, windows[account], now); ok && (first == "" || next.Before(due)) {
			first, due = account, next
		}
	}
	return first, due, first != ""
}

func TestPrimesKeepTheirTimesOfDayOnADayTheClocksChange(t *testing.T) {
	london, err := time.LoadLocation("Europe/London")
	if err != nil {
		t.Fatalf("load Europe/London: %v", err)
	}
	s, _ := prime.New(daytime, []string{"work", "side"}, policy)
	on := func(y int, m time.Month, d, hour, minute int) time.Time {
		return time.Date(y, m, d, hour, minute, 0, 0, london)
	}
	tests := []struct {
		name string
		now  time.Time
		// want is when work is next due, just after 04:10 on the clock, and
		// until is when it's due until, the day ending at 23:00 on the clock.
		want, until time.Time
	}{
		{
			name:  "the clocks going forward at 01:00",
			now:   on(2026, time.March, 29, 0, 30),
			want:  justAfter(time.Date(2026, time.March, 29, 3, 10, 0, 0, time.UTC)),
			until: time.Date(2026, time.March, 29, 22, 0, 0, 0, time.UTC),
		},
		{
			name:  "the clocks going back at 02:00",
			now:   on(2026, time.October, 25, 0, 30),
			want:  justAfter(time.Date(2026, time.October, 25, 4, 10, 0, 0, time.UTC)),
			until: time.Date(2026, time.October, 25, 23, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := s.Next("work", nil, tt.now); !got.Equal(tt.want) || !ok {
				t.Errorf("Next() = %v, %v, want %v, just after 04:10 on the clock", got.UTC(), ok, tt.want)
			}
			until := tt.until.In(london)
			last := until.Add(-time.Minute)
			if got, _ := s.Next("work", nil, last); !got.Equal(last) {
				t.Errorf("a minute before 23:00 on the clock, Next() = %v, want then: the day still runs", got.UTC())
			}
			if got, _ := s.Next("work", nil, until); !got.After(until.Add(time.Hour)) {
				t.Errorf("at 23:00 on the clock, Next() = %v, want the next day's slot: the day has ended", got.UTC())
			}
		})
	}
}

// at is a time on the given day of September 2026, an hour east of UTC.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, time.September, 27+day, hour, minute, 0, 0, zone)
}

// slotsOf shows the schedule's slots as account@HH:MM.
func slotsOf(s prime.Schedule) []string {
	var slots []string
	for _, slot := range s.Slots() {
		slots = append(slots, slot.Account+"@"+clock(slot.At))
	}
	return slots
}

// clock shows a time of day, as the time since midnight, as HH:MM.
func clock(d time.Duration) string {
	return time.Time{}.Add(d).Format("15:04")
}
