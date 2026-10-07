package cli

import (
	"slices"
	"strconv"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

func TestRequestsOfASessionHoldTheLinesOfOneSessionAtMost(t *testing.T) {
	// The sessions the lines are of, the first three starting 5b0e, and the
	// third its whole id.
	const first, second, whole, other = "5b0e7c1a", "5b0e9d33", "5b0e", "18bb2c41"
	tests := []struct {
		name string
		// sessions are the sessions of the lines read, in order, each line's
		// request its index.
		sessions []string
		// held are the requests whose lines are held once they're read.
		held []string
	}{
		{name: "the first session's whose id it starts, while it starts no other's", sessions: []string{first, other, first}, held: []string{"0", "2"}},
		{name: "none once it starts another's", sessions: []string{first, first, second, first, second}},
		{name: "the session it names whole, once one of its lines comes", sessions: []string{first, second, first, whole, first, second, whole}, held: []string{"3", "6"}},
		{name: "the session it names whole, from its first line", sessions: []string{whole, first, whole}, held: []string{"0", "2"}},
		{name: "none of a session it doesn't start", sessions: []string{other}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSessionMatch(whole)
			for i, session := range tt.sessions {
				s.add(ledger.Held{Request: strconv.Itoa(i), Session: session})
			}
			var held []string
			for _, h := range s.held {
				held = append(held, h.Request)
			}
			if !slices.Equal(held, tt.held) {
				t.Errorf("the lines held are of the requests %q, want %q", held, tt.held)
			}
		})
	}
}
