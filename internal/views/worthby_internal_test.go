package views

import (
	"testing"
	"time"
)

// Every period's coarsest chart has no more than BucketsMost buckets, on
// any day of the year, weeks starting on any day, today's with its hours
// given or not, so a drawing that fits so many fits every period's; and it
// reaches BucketsMost, as the constant says.
func TestEveryPeriodsCoarsestChartHoldsBucketsMostAtMost(t *testing.T) {
	var hours [24]Worth
	most := 0
	for day := range 366 {
		now := time.Date(2026, time.January, 1, 13, 0, 0, 0, time.Local).AddDate(0, 0, day)
		for starts := time.Sunday; starts <= time.Saturday; starts++ {
			for _, period := range Periods() {
				for _, given := range []*[24]Worth{nil, &hours} {
					n := len(worthCharts(period, now, starts, dayWorths{}, given).Coarsest().Buckets)
					if n > BucketsMost {
						t.Fatalf("on %s, weeks from %s, %s's coarsest chart has %d buckets, want %d at most", now.Format(time.DateOnly), starts, period.Name(), n, BucketsMost)
					}
					most = max(most, n)
				}
			}
		}
	}
	if most != BucketsMost {
		t.Errorf("the coarsest charts have %d buckets at most, want BucketsMost, %d", most, BucketsMost)
	}
}
