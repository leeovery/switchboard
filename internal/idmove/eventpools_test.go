package idmove_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/status"
)

// kindOfEvent is an event of one kind, as the router tells it, and whether
// its kind names the pool it's of, as the design has it.
type kindOfEvent struct {
	event     status.Event
	namesPool bool
}

// everyKind is an event of every kind the router tells of, naming the two
// accounts given: a session's start and move, and a pin set and cleared,
// name their pool; what befalls an account, and the router's own, none.
func everyKind(one, other string) []kindOfEvent {
	session, model := "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", "claude-opus-5-5"
	return []kindOfEvent{
		{event: status.Event{Kind: status.EventStarted, Account: one, Session: session, Model: model}, namesPool: true},
		{event: status.Event{Kind: status.EventPressure, Account: one, Windows: []string{"five_hour"}}},
		{event: status.Event{Kind: status.EventCap, Account: one, Windows: []string{"seven_day"}}},
		{event: status.Event{Kind: status.EventLimit, Account: one, Windows: []string{"five_hour"}, To: other}},
		{event: status.Event{Kind: status.EventMoved, Session: session, Model: model, From: one, To: other, Reason: status.ReasonMovedOff + one + " " + status.WhyLimit}, namesPool: true},
		{event: status.Event{Kind: status.EventRefused, Account: one}},
		{event: status.Event{Kind: status.EventForeign, Account: one}},
		{event: status.Event{Kind: status.EventPrimed, Account: other, Windows: []string{"five_hour"}}},
		{event: status.Event{Kind: status.EventRoom, Account: one, Windows: []string{"five_hour"}}},
		{event: status.Event{Kind: status.EventPin, Account: other, Accounts: []string{other, one}, By: "cli"}, namesPool: true},
		{event: status.Event{Kind: status.EventAuto, Accounts: []string{other, one}, By: "dashboard"}, namesPool: true},
		{event: status.Event{Kind: status.EventRestart, Reason: status.RestartForConfig}},
		{event: status.Event{Kind: status.EventHealth, Reason: "most requests failing"}},
	}
}

// eventFiles are the router's events' files in the state directory.
func eventFiles(state string) *dayfile.Files {
	return events.Files(ledger.Dir(state), logs.For("test"))
}

// putEveryKind writes the events of everyKind, naming the accounts given, as
// the router writes them before pools, as the day with the given date's,
// plain.
func putEveryKind(t *testing.T, state, date, one, other string) {
	t.Helper()
	var lines []string
	for i, k := range everyKind(one, other) {
		lines = append(lines, event(date, i+1, k.event))
	}
	putDay(t, eventFiles(state), date, false, lines...)
}

// wantEveryKindIn fails the test where the events of the day with the given
// date in the state directory don't read as everyKind, naming the accounts
// given, as the router writes them in the pool with the given id: each of a
// kind that names its pool naming that one, every other as it was.
func wantEveryKindIn(t *testing.T, state, date, one, other, pool string) {
	t.Helper()
	var want []string
	for i, k := range everyKind(one, other) {
		if k.namesPool {
			k.event.Pool = pool
		}
		want = append(want, event(date, i+1, k.event))
	}
	var got []string
	if _, _, err := dayfile.ReadDay(eventFiles(state), date, func(l []byte) ([]byte, bool) { return l, true }, func(l []byte) bool {
		got = append(got, string(l))
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("the day holds %d events, want %d:\n%q", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("the %s event reads\n%s\nwant\n%s", everyKind(one, other)[i].event.Kind, got[i], want[i])
		}
	}
}

func TestTheMoveToPoolsGivesEveryEventThatNamesItsPoolTheFirst(t *testing.T) {
	f := newIDsFixture(t)
	putEveryKind(t, f.state, yesterday, workID, personalID)
	first := f.pooled(t)

	wantEveryKindIn(t, f.state, yesterday, workID, personalID, first)
}

// An event moved to pools reads as the first pool's by the pool it names,
// so it stays the first's once the config's first is another pool, as after
// the first is emptied and removed.
func TestAnEventMovedToPoolsReadsInTheFirstPool(t *testing.T) {
	f := newIDsFixture(t)
	putEveryKind(t, f.state, yesterday, workID, personalID)
	first := f.pooled(t)

	const client = "c1nt"
	placesNone := func(string) (string, bool) { return "", false }
	read := 0
	for e := range events.NewReader(f.state, clock, logs.For("test")).Between(at(yesterday), at(today)) {
		if !status.KindNamesItsPool(e.Kind) {
			continue
		}
		read++
		if !e.InPool(first, client, placesNone) {
			t.Errorf("the %s event, naming pool %q, isn't in the first, %q, the config's first being %q", e.Kind, e.Pool, first, client)
		}
		if e.InPool(client, client, placesNone) {
			t.Errorf("the %s event, naming pool %q, is in %q, the config's first, want the first moved, %q, alone", e.Kind, e.Pool, client, first)
		}
	}
	if read != 4 {
		t.Errorf("read %d events of the kinds that name their pool, want the day's four", read)
	}
}

// A folder exported before pools has every event that names its pool given
// this Mac's first pool as it's taken in, its accounts renamed as ever, and
// every other as it was but for its accounts.
func TestAFolderFromBeforePoolsGivesEveryEventThatNamesItsPoolTheFirst(t *testing.T) {
	f := newFixture(t)
	putEveryKind(t, f.state, yesterday, work, personal)
	asFolder(t, f.state, export.Manifest{Install: thisInstall, Version: 1})
	record := thisMacPooled()
	f.joined(t, &record)

	wantEveryKindIn(t, f.state, yesterday, workID, personalID, thisFirst)
}
