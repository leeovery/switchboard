package views_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"testing"
	"time"

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

// The store reads each session resting on the state a digest holds of it,
// asking for none whole, but the session whose page is open, whose tally
// and points take in every one of its days' answers, and one it can't place
// a line of, which the same state would leave unplaced again; a page reads
// its sessions each whole.
func TestReadsOfSessionsReadWholeThePagesAndThoseTheStoreCantPlace(t *testing.T) {
	lines := []ledger.Line{sessionAsked("o1", older, "work", "new", october(4, 22, 0)), sessionAsked("e1", early, "work", "new", local(12, 0, 0))}
	behind := sessionAsked("e2", early, "work", "sticky", now.Add(-2*time.Hour))
	tests := []struct {
		name string
		read func(l wholeNoted)
		want map[string]bool
	}{
		{name: "the store, for the List", read: func(l wholeNoted) { listHeld(l, nil, now) }, want: map[string]bool{early: false}},
		{name: "the store, for the List and a session's page", read: func(l wholeNoted) {
			views.HoldSessions(ledger.Pricing).Fed(l, views.Taken{}, views.SessionsWanted{List: true, Page: older}, now)
		}, want: map[string]bool{early: false, older: true}},
		{name: "the store, of a line behind its session's horizon", read: func(l wholeNoted) {
			fedList(listHeld(l.fakeLedger, nil, now), l, nil, now, behind)
		}, want: map[string]bool{early: true}},
		{name: "a page's sessions", read: func(l wholeNoted) { views.ReadSessions(l, nil, now, older) }, want: map[string]bool{early: true, older: true}},
		{name: "a page's session alone", read: func(l wholeNoted) { views.ReadSession(l, now, older) }, want: map[string]bool{older: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := wholeNoted{fakeLedger: sessionsLedger(lines, now), asked: make(map[string]bool)}

			tt.read(l)

			if !reflect.DeepEqual(l.asked, tt.want) {
				t.Errorf("the read gave the sessions %v, each true where it asked for it whole; want %v", l.asked, tt.want)
			}
		})
	}
}

// unrestorable is a ledger as wholeNoted reads it whose sessions, but those
// asked for whole, rest on a state that won't restore, as one a hand-edited
// digest holds, none of their lines after it.
type unrestorable struct {
	wholeNoted
}

func (l unrestorable) Sessions(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return unrestorableUnless(l.wholeNoted.Sessions(ids, whole, take), whole)
}

func (l unrestorable) SessionsOf(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return unrestorableUnless(l.wholeNoted.SessionsOf(ids, whole, take), whole)
}

// unrestorableUnless is read, each of its sessions whole doesn't report
// true of resting on a state that won't restore, afresh.
func unrestorableUnless(read map[string]ledger.SessionFrom, whole func(string) bool) map[string]ledger.SessionFrom {
	based := maps.Clone(read)
	for id := range based {
		if !whole(id) {
			based[id] = ledger.SessionFrom{Base: &ledger.SessionBase{Date: "2026-10-06", State: json.RawMessage(`{"d":`)}}
		}
	}
	return based
}

// The store reads again whole a session resting on a state that won't
// restore, holding it as it holds one read whole from the first: the List
// it holds is the List of every line, each session asked for whole once its
// state wouldn't restore.
func TestTheStoreReadsWholeASessionWhoseStateWontRestore(t *testing.T) {
	lines := []ledger.Line{
		sessionAsked("o1", older, "work", "new", october(6, 22, 0)), sessionAsked("o2", older, "work", "sticky", local(9, 0, 0)),
		sessionAsked("e1", early, "work", "new", local(12, 0, 0)),
	}
	l := unrestorable{wholeNoted{fakeLedger: sessionsLedger(lines, now), asked: make(map[string]bool)}}

	got := listHeld(l, nil, now).List(views.ListLive{Now: now})

	if want := listHeld(sessionsLedger(lines, now), nil, now).List(views.ListLive{Now: now}); !reflect.DeepEqual(got, want) {
		t.Errorf("the List held is\n%+v\nwant, as every line read\n%+v", got, want)
	}
	if want := map[string]bool{older: true, early: true}; !reflect.DeepEqual(l.asked, want) {
		t.Errorf("the read last gave the sessions %v, each true where it asked for it whole; want %v", l.asked, want)
	}
}
