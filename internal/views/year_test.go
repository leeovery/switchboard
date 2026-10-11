package views_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// busy is the summary of the day with the given date, of n requests of
// work's.
func busy(date string, n int) ledger.Summary {
	return day(date, account("work", sent("claude-opus-5-5", n, inputUsage)))
}

func TestYearShadesEachDayByItsRequestsInQuarters(t *testing.T) {
	tests := []struct {
		name string
		days []ledger.Summary
		want string
	}{
		{name: "the empty ledger: no cuts, and today at level 0", days: daysFrom("2026-10-07"),
			want: `{"cuts":[],"days":[{"day":"2026-10-07","requests":0,"level":0}]}`},
		{name: "one day: every cut its requests, so it's at the top", days: daysFrom("2026-10-07", busy("2026-10-07", 5)),
			want: `{"cuts":[5,5,5],"days":[{"day":"2026-10-07","requests":5,"level":4}]}`},
		{
			name: "the active days split at n × q // 4, a day with none at level 0",
			days: daysFrom("2026-10-02", busy("2026-10-02", 40), busy("2026-10-03", 10), busy("2026-10-05", 30), busy("2026-10-06", 20), busy("2026-10-07", 25)),
			want: `{"cuts":[20,25,30],"days":[{"day":"2026-10-02","requests":40,"level":4},{"day":"2026-10-03","requests":10,"level":1},` +
				`{"day":"2026-10-04","requests":0,"level":0},{"day":"2026-10-05","requests":30,"level":4},{"day":"2026-10-06","requests":20,"level":2},` +
				`{"day":"2026-10-07","requests":25,"level":3}]}`,
		},
		{
			name: "across a month's end, each level a cut more",
			days: daysFrom("2026-09-30", busy("2026-09-30", 1), busy("2026-10-01", 2), busy("2026-10-02", 3), busy("2026-10-03", 4),
				busy("2026-10-04", 5), busy("2026-10-05", 6), busy("2026-10-06", 7), busy("2026-10-07", 8)),
			want: `{"cuts":[3,5,7],"days":[{"day":"2026-09-30","requests":1,"level":1},{"day":"2026-10-01","requests":2,"level":1},` +
				`{"day":"2026-10-02","requests":3,"level":2},{"day":"2026-10-03","requests":4,"level":2},{"day":"2026-10-04","requests":5,"level":3},` +
				`{"day":"2026-10-05","requests":6,"level":3},{"day":"2026-10-06","requests":7,"level":4},{"day":"2026-10-07","requests":8,"level":4}]}`,
		},
		{
			name: "requests are messages, every account's, never checks or counts",
			days: daysFrom("2026-10-06",
				day("2026-10-06", account("work", ledger.ModelDay{Model: "claude-haiku-4-5", Checks: 4, Counts: 9})),
				day("2026-10-07", account("", ledger.ModelDay{Model: "claude-opus-5-5", Unsent: 1}),
					account("work", ledger.ModelDay{Model: "claude-opus-5-5", Upstream: 2, NoUsage: 1, Checks: 3, Counts: 5}))),
			want: `{"cuts":[3,3,3],"days":[{"day":"2026-10-06","requests":0,"level":0},{"day":"2026-10-07","requests":3,"level":4}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jsonOf(t, views.NewHistory(input(tt.days)).Year); got != tt.want {
				t.Errorf("the year is\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestYearsCutsAreTheActiveDaysQuarters(t *testing.T) {
	// Of n active days, 1 to n requests each, the cuts are the requests at
	// n × q // 4, counting from none.
	for n := 1; n <= 9; n++ {
		var days []ledger.Summary
		for i := range n {
			days = append(days, busy(now.AddDate(0, 0, i-n+1).Format(time.DateOnly), i+1))
		}
		year := views.NewHistory(input(days)).Year
		want := fmt.Sprint([]int{n*1/4 + 1, n*2/4 + 1, n*3/4 + 1})
		if got := fmt.Sprint(year.Cuts); got != want {
			t.Errorf("of %d active days, the cuts are %s, want %s", n, got, want)
		}
	}
}
