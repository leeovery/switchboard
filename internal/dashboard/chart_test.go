package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestTheMidnightsAlongAnAxisAreTheDaysStartsWhereTheClocksGoForwardAtMidnight(t *testing.T) {
	santiago, err := time.LoadLocation("America/Santiago")
	if err != nil {
		t.Fatalf("load America/Santiago: %v", err)
	}
	// Chile's clocks went from 00:00 to 01:00 on Sunday 6 September 2026.
	start, now := time.Date(2026, 9, 3, 12, 0, 0, 0, santiago), time.Date(2026, 9, 5, 15, 0, 0, 0, santiago)
	hourly := func(t time.Time) int { return int(t.Sub(start) / time.Hour) }

	var got []string
	for _, m := range midnights(start, start.Add(7*day), now, hourly, 7*24, len("Mon")) {
		got = append(got, fmt.Sprintf("%s %v", m.at.Format("Mon 2 Jan 15:04 -07"), m.today))
	}
	want := []string{"Fri 4 Sep 00:00 -04 false", "Sat 5 Sep 00:00 -04 true", "Sun 6 Sep 01:00 -03 false", "Mon 7 Sep 00:00 -03 false",
		"Tue 8 Sep 00:00 -03 false", "Wed 9 Sep 00:00 -03 false", "Thu 10 Sep 00:00 -03 false"}
	if !slices.Equal(got, want) {
		t.Errorf("the midnights are\n%s\nwant each day's start, the 6th's as the clocks went forward, today's picked out\n%s",
			strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
