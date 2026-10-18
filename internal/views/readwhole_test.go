package views_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// wholeNoted is a ledger as fakeLedger reads it that notes, of each session
// it gives, by its id, whether the read asked for it whole.
type wholeNoted struct {
	fakeLedger
	asked map[string]bool
}

func (l wholeNoted) Sessions(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return l.noted(l.fakeLedger.Sessions(ids, whole, take), whole)
}

func (l wholeNoted) SessionsOf(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return l.noted(l.fakeLedger.SessionsOf(ids, whole, take), whole)
}

// noted is read, each of its sessions noted, whole as whole reports of it.
func (l wholeNoted) noted(read map[string]ledger.SessionFrom, whole func(string) bool) map[string]ledger.SessionFrom {
	for id := range read {
		l.asked[id] = whole != nil && whole(id)
	}
	return read
}

// Every read of the ledger's sessions the views make reads each whole, every
// line of it, resting on no state a digest holds: the store's, for the List
// and for a session's page, and a page's own.
func TestEveryReadOfSessionsReadsEachWhole(t *testing.T) {
	lines := []ledger.Line{sessionAsked("o1", older, "work", "new", october(4, 22, 0)), sessionAsked("e1", early, "work", "new", local(12, 0, 0))}
	tests := []struct {
		name string
		read func(l wholeNoted)
	}{
		{name: "the store, for the List", read: func(l wholeNoted) { listHeld(l, nil, now) }},
		{name: "the store, for a session's page", read: func(l wholeNoted) {
			views.HoldSessions(ledger.Pricing).Fed(l, views.Taken{}, views.SessionsWanted{Page: older}, now)
		}},
		{name: "a page's sessions", read: func(l wholeNoted) { views.ReadSessions(l, nil, now, older) }},
		{name: "a page's session alone", read: func(l wholeNoted) { views.ReadSession(l, now, older) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := wholeNoted{fakeLedger: sessionsLedger(lines, now), asked: make(map[string]bool)}

			tt.read(l)

			if len(l.asked) == 0 || slices.Contains(slices.Collect(maps.Values(l.asked)), false) {
				t.Errorf("the read gave the sessions %v, each true where it asked for it whole; want some, each asked for whole", l.asked)
			}
		})
	}
}
