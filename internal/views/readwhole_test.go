package views_test

import (
	"encoding/json"
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// restNoted is a ledger as fakeLedger reads it that notes, of each session
// it gives, by its id, what the read asked it to rest on.
type restNoted struct {
	fakeLedger
	asked map[string]ledger.Rest
}

func (l restNoted) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return l.noted(l.fakeLedger.Sessions(ids, rest, take), rest)
}

func (l restNoted) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return l.noted(l.fakeLedger.SessionsOf(ids, rest, take), rest)
}

// noted is read, each of its sessions noted as rest says of it, by its id:
// on its state where rest is nil, as a ledger.Follower rests it.
func (l restNoted) noted(read map[string]ledger.SessionFrom, rest func(string) ledger.Rest) map[string]ledger.SessionFrom {
	for id := range read {
		l.asked[id] = restOf(rest, id)
	}
	return read
}

// restOf is what rest says the session with the given id rests on: on its
// state where rest is nil, as a ledger.Follower rests it.
func restOf(rest func(string) ledger.Rest, id string) ledger.Rest {
	if rest == nil {
		return ledger.RestOnState
	}
	return rest(id)
}

// The store reads each session resting on the state a digest holds of it,
// but the session whose page is open on its page, the newest state whose
// chain of parts reads, as its tally and its points are restored from them,
// and one it can't place a line of whole, which the same state would leave
// unplaced again; a page reads on its page each session the id given names,
// by its whole id or as much of it as is unique, as it shows one of them,
// and every other on its state.
func TestReadsOfSessionsRestThePageOnItsBaseAndReadWholeThoseTheStoreCantPlace(t *testing.T) {
	lines := []ledger.Line{sessionAsked("o1", older, "work", "new", october(4, 22, 0)), sessionAsked("e1", early, "work", "new", local(12, 0, 0))}
	behind := sessionAsked("e2", early, "work", "sticky", now.Add(-2*time.Hour))
	onState, onPage, whole := ledger.RestOnState, ledger.RestOnPage, ledger.RestWhole
	tests := []struct {
		name string
		read func(l restNoted)
		want map[string]ledger.Rest
	}{
		{name: "the store, for the List", read: func(l restNoted) { listHeld(l, nil, now) }, want: map[string]ledger.Rest{early: onState}},
		{name: "the store, for the List and a session's page", read: func(l restNoted) {
			views.HoldSessions(ledger.Pricing).Fed(l, views.Taken{}, views.SessionsWanted{List: true, Page: older}, now)
		}, want: map[string]ledger.Rest{early: onState, older: onPage}},
		{name: "the store, of a line behind its session's horizon", read: func(l restNoted) {
			fedList(listHeld(l.fakeLedger, nil, now), l, nil, now, behind)
		}, want: map[string]ledger.Rest{early: whole}},
		{name: "the store, of a line behind the horizon of the session whose page is open", read: func(l restNoted) {
			paged := views.SessionsWanted{List: true, Page: early}
			held, _ := views.HoldSessions(ledger.Pricing).Fed(l.fakeLedger, views.Taken{}, paged, now)
			held.Fed(l, views.Took(heldOf(behind), nil), paged, now)
		}, want: map[string]ledger.Rest{early: whole}},
		{name: "a page's sessions", read: func(l restNoted) { views.ReadSessions(l, nil, now, older) }, want: map[string]ledger.Rest{early: onState, older: onPage}},
		{name: "a page's sessions, one named by as much of its id as is unique", read: func(l restNoted) {
			views.ReadSessions(l, []status.Session{{ID: older}}, now, early[:11])
		}, want: map[string]ledger.Rest{early: onPage, older: onState}},
		{name: "a page's session alone", read: func(l restNoted) { views.ReadSession(l, now, older) }, want: map[string]ledger.Rest{older: onPage}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := restNoted{fakeLedger: sessionsLedger(lines, now), asked: make(map[string]ledger.Rest)}

			tt.read(l)

			if !reflect.DeepEqual(l.asked, tt.want) {
				t.Errorf("the read gave the sessions %v, each with what it asked it to rest on; want %v", l.asked, tt.want)
			}
		})
	}
}

// unrestorable is a ledger as restNoted reads it whose sessions, but those
// asked for whole, rest on a state that won't restore, as one a hand-edited
// digest holds, none of their lines after it.
type unrestorable struct {
	restNoted
}

func (l unrestorable) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return unrestorableUnless(l.restNoted.Sessions(ids, rest, take), rest)
}

func (l unrestorable) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return unrestorableUnless(l.restNoted.SessionsOf(ids, rest, take), rest)
}

// unrestorableUnless is read, each of its sessions rest doesn't ask to be
// read whole resting on a state that won't restore, afresh.
func unrestorableUnless(read map[string]ledger.SessionFrom, rest func(string) ledger.Rest) map[string]ledger.SessionFrom {
	based := maps.Clone(read)
	for id := range based {
		if restOf(rest, id) != ledger.RestWhole {
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
	l := unrestorable{restNoted{fakeLedger: sessionsLedger(lines, now), asked: make(map[string]ledger.Rest)}}

	got := listHeld(l, nil, now).List(views.ListLive{Now: now})

	if want := listHeld(sessionsLedger(lines, now), nil, now).List(views.ListLive{Now: now}); !reflect.DeepEqual(got, want) {
		t.Errorf("the List held is\n%+v\nwant, as every line read\n%+v", got, want)
	}
	if want := map[string]ledger.Rest{older: ledger.RestWhole, early: ledger.RestWhole}; !reflect.DeepEqual(l.asked, want) {
		t.Errorf("the read last gave the sessions %v, each with what it asked it to rest on; want %v", l.asked, want)
	}
}
