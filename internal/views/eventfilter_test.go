package views_test

import (
	"maps"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// filtered are the events the filter's tests filter, one of each sort of
// what an event names, by their ids: a session started and moved, a cap, the
// global pin of three accounts set and cleared, a session's own pin, the
// router's health and a restart.
var filtered = []status.Event{
	{ID: 1, At: october(7, 9, 0), Kind: status.EventStarted, Account: "side", Session: older, Reason: status.ReasonNew},
	{ID: 2, At: october(7, 9, 30), Kind: status.EventMoved, Session: older, Model: opus, From: "side", To: "work", Reason: "moved: side is at its reserve"},
	{ID: 3, At: october(7, 10, 0), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05},
	{ID: 4, At: october(7, 10, 30), Kind: status.EventPin, Account: "work", Accounts: []string{"work", "side", "personal"}, By: "cli"},
	{ID: 5, At: october(7, 11, 0), Kind: status.EventPin, Account: "side", Session: mover, By: "dashboard"},
	{ID: 6, At: october(7, 11, 30), Kind: status.EventAuto, Accounts: []string{"work", "side", "personal"}, By: "dashboard"},
	{ID: 7, At: october(7, 12, 0), Kind: status.EventHealth, Reason: "most requests failing"},
	{ID: 8, At: october(7, 12, 30), Kind: status.EventRestart, Reason: status.RestartForConfig},
}

// filteredBy are the Log's Events of the filter's tests' events, today's,
// as the filter given shows them.
func filteredBy(f views.EventFilter) views.EventsTable {
	newestFirst := slices.Clone(filtered)
	slices.Reverse(newestFirst)
	read := views.ReadEvents(nil, now, now).Look(liveAt(newestFirst, now), october(7, 8, 0))
	return views.LogEvents(views.LogEventSources{From: now, Events: read, Filter: f, Now: now})
}

// Of each choice the filter makes, a choice ticked keeps the events of those
// ticked, and none ticked is every one: an event of no session is kept only
// where none is ticked, as is one naming no account.
func TestTheEventFilterKeepsThoseTickedOrEveryOneWhereNoneIs(t *testing.T) {
	tests := []struct {
		name   string
		filter views.EventFilter
		want   []int
	}{
		{name: "nothing ticked", want: []int{1, 2, 3, 4, 5, 6, 7, 8}},
		{name: "a kind", filter: views.EventFilter{Kinds: []string{status.EventCap}}, want: []int{3}},
		{name: "two kinds", filter: views.EventFilter{Kinds: []string{status.EventStarted, status.EventMoved}}, want: []int{1, 2}},
		{name: "an account a move left", filter: views.EventFilter{Accounts: []string{"side"}}, want: []int{1, 2, 4, 5, 6}},
		{name: "an account the global pin alone names", filter: views.EventFilter{Accounts: []string{"personal"}}, want: []int{4, 6}},
		{name: "two accounts", filter: views.EventFilter{Accounts: []string{"personal", "work"}}, want: []int{2, 3, 4, 6}},
		{name: "a session", filter: views.EventFilter{Sessions: []string{older}}, want: []int{1, 2}},
		{name: "a session's own pin", filter: views.EventFilter{Sessions: []string{mover}}, want: []int{5}},
		{name: "a kind and an account", filter: views.EventFilter{Kinds: []string{status.EventPin}, Accounts: []string{"side"}}, want: []int{4, 5}},
		{name: "a kind and a session", filter: views.EventFilter{Kinds: []string{status.EventMoved}, Sessions: []string{older}}, want: []int{2}},
		{name: "choices none keeps", filter: views.EventFilter{Kinds: []string{status.EventCap}, Sessions: []string{older}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, row := range filteredBy(tt.filter).Rows {
				got = append(got, row.Key.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("the filter %+v shows events %v, want %v", tt.filter, got, tt.want)
			}
		})
	}
}

// Today's events are counted every one, the filter's hidden among them: by
// kind, by each account each names, and by session, by its whole id; those
// shown the rows of today the filter keeps.
func TestTodaysEventsAreCountedByKindAccountAndSession(t *testing.T) {
	c := filteredBy(views.EventFilter{Accounts: []string{"personal"}}).Today
	want := views.EventCount{
		All: 8, Shown: 2,
		Kinds: map[string]int{
			status.EventStarted: 1, status.EventMoved: 1, status.EventCap: 1, status.EventPin: 2, status.EventAuto: 1,
			status.EventHealth: 1, status.EventRestart: 1,
		},
		Accounts: map[string]int{"work": 4, "side": 5, "personal": 2},
		Sessions: map[string]int{older: 2, mover: 1},
	}
	if c.All != want.All || c.Shown != want.Shown || c.Hidden() != 6 ||
		!maps.Equal(c.Kinds, want.Kinds) || !maps.Equal(c.Accounts, want.Accounts) || !maps.Equal(c.Sessions, want.Sessions) {
		t.Errorf("today's events count %+v, hiding %d; want %+v, hiding 6", c, c.Hidden(), want)
	}
}

// At each hard moment, today's count agrees with itself and with the rows:
// every event today is the sum of the kinds' counts, and, with nothing
// filtered, those shown, which are the rows of today; an event naming a
// session counts under it once, and every account it names once.
func TestTodaysEventsCountAsTheRowsOfTodayAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			table := eventsOf(hm, eventsRead(hm, filedIn(t, hm)))
			c, today := table.Today, views.Today(hm.Now)
			var rows, sessioned, named int
			for _, row := range table.Rows {
				if today.Holds(row.At) {
					rows, named = rows+1, named+len(row.Accounts)
					if row.Session != "" {
						sessioned++
					}
				}
			}
			if c.All != rows || c.Shown != rows || sum(c.Kinds) != rows {
				t.Errorf("today counts %d events, %d shown, its kinds %d; want the %d rows of today", c.All, c.Shown, sum(c.Kinds), rows)
			}
			if sum(c.Sessions) != sessioned || sum(c.Accounts) != named {
				t.Errorf("today's sessions count %d events and accounts %d, want %d of a session and %d named", sum(c.Sessions), sum(c.Accounts), sessioned, named)
			}
		})
	}
}

// An event-free today, its table filled from yesterday, shows yesterday's
// events and counts none today.
func TestAnEventFreeTodayCountsNoneThoughItShowsYesterdays(t *testing.T) {
	hm := momentNamed(t, "an event-free today, filled from yesterday")
	table := eventsOf(hm, eventsRead(hm, filedIn(t, hm)))
	if len(table.Rows) != 4 || table.Today.All != 0 || len(table.Today.Kinds) != 0 {
		t.Errorf("an event-free today shows %d rows and counts %+v, want yesterday's 4, and none", len(table.Rows), table.Today)
	}
}

// sum is the sum of the counts given.
func sum(counts map[string]int) int {
	n := 0
	for _, c := range counts {
		n += c
	}
	return n
}
