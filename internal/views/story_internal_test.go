package views

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// A line is of a request the stream told of, or another line, only by its id
// and when: an id is unique only while the router that gave it runs, so the
// same id at another time is another request.
func TestALineIsOfTheRequestWithItsIdWhileItWasInFlight(t *testing.T) {
	arrived := time.Date(2026, 10, 7, 13, 0, 0, 0, time.UTC)
	// It took 2.0009 seconds, which its line cuts to 2000 milliseconds.
	l := ledger.Line{Request: "r1", At: arrived, TotalMS: 2000}
	tests := []struct {
		name string
		id   string
		at   time.Time
		want bool
	}{
		{name: "the same line, as it arrived", id: "r1", at: arrived, want: true},
		{name: "sent as it went on", id: "r1", at: arrived.Add(time.Second), want: true},
		{name: "sent within the millisecond its line cut", id: "r1", at: arrived.Add(2000*time.Millisecond + 900*time.Microsecond), want: true},
		{name: "another id", id: "r2", at: arrived.Add(time.Second)},
		{name: "its id before it arrived, of an earlier run", id: "r1", at: arrived.Add(-time.Nanosecond)},
		{name: "its id after it ended, of a later run", id: "r1", at: arrived.Add(2001 * time.Millisecond)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameRequest(&l, tt.id, tt.at); got != tt.want {
				t.Errorf("sameRequest(%s at %v for %d ms, %s, %v) = %t, want %t", l.Request, l.At, l.TotalMS, tt.id, tt.at, got, tt.want)
			}
		})
	}
}
