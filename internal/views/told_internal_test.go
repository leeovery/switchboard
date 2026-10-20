package views

import (
	"slices"
	"testing"
	"time"
)

// A request told of may have its line filed on the day FiledOn gives, and
// on the day it was sent on where that's another: so of one sent within
// moments after midnight, the day before and the day; of one sent at any
// other time, the day it was sent on alone; none where the stream told of
// no sending.
func TestARequestsLineMayBeFiledOnTheDayItArrivedOrWasSentOn(t *testing.T) {
	midnight := time.Date(2026, 10, 7, 0, 0, 0, 0, time.Local)
	tests := []struct {
		name string
		told Told
		want []string
	}{
		{"sent in the afternoon", Told{Sent: midnight.Add(13 * time.Hour)}, []string{"2026-10-07"}},
		{"sent as the day began", Told{Sent: midnight.Add(time.Second)}, []string{"2026-10-06", "2026-10-07"}},
		{"sent a while after it began", Told{Sent: midnight.Add(2 * time.Minute)}, []string{"2026-10-07"}},
		{"sent as the day before ended", Told{Sent: midnight.Add(-time.Second)}, []string{"2026-10-06"}},
		{"never sent", Told{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, day := range tt.told.linedOn() {
				got = append(got, day.Date)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Told{Sent: %v}.linedOn() = %q, want %q", tt.told.Sent, got, tt.want)
			}
		})
	}
}
