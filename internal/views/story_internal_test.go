package views

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// A line is of a request the stream told of, or another line, only by its id
// and when: an id is unique only while the router that gave it runs, so the
// same id at another time is another request. A held request's flight is
// the same request as its line.
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
			if got := flightOf(&l).is(tt.id, tt.at); got != tt.want {
				t.Errorf("the flight of %s at %v for %d ms is of %s at %v: %t, want %t", l.Request, l.At, l.TotalMS, tt.id, tt.at, got, tt.want)
			}
		})
	}
}

// A line is its own request whenever it says it arrived, as a hand-edited
// file may give one: at no time at all, or in a year nanoseconds since the
// epoch can't hold. Its flight arrives and ends when its line does, and its
// story tells it's the first request and the move it made.
func TestALineIsItsOwnRequestWheneverItArrived(t *testing.T) {
	for _, at := range []time.Time{
		{}, time.Date(1600, 3, 1, 9, 0, 0, 0, time.UTC), time.Date(3000, 3, 1, 9, 0, 0, 1, time.UTC), time.Date(2026, 10, 7, 13, 0, 0, 0, time.UTC),
	} {
		t.Run(at.String(), func(t *testing.T) {
			l := ledger.Line{
				Request: "r1", At: at, TotalMS: 2000, Kind: ledger.KindMessage, Session: "s", Model: "claude-opus-5-5",
				Account: "side", From: "work", Reason: "moved: work hit its limit", Class: ledger.ClassMain, Attempts: 1,
			}
			f := flightOf(&l)
			if !f.arrived().Equal(l.At) || !f.ended().Equal(ended(&l)) || !f.is(l.Request, l.At) {
				t.Errorf("the flight arrived %v and ended %v, of its line: %t; want %v and %v, of it", f.arrived(), f.ended(), f.is(l.Request, l.At), l.At, ended(&l))
			}
			s := storyOf(heldRequestsOf([]ledger.Line{l}), ledger.Pricing, Today(at))
			if first, made, _ := s.toldOf(&l); !first || made == nil || !s.first.arrived().Equal(l.At) {
				t.Errorf("its story begins at %v, and tells it's the first %t, and made a move %t; want it to begin at %v, the first, its move made",
					s.first.arrived(), first, made != nil, l.At)
			}
		})
	}
}
