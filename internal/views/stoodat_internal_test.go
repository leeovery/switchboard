package views

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Each field the router changes after it files an event is put back as it
// stood at until: a hold's until, the windows that joined a limit, a cap or
// a refusal, a move's ForcedBy and Limit, and a held event's Count and To.
// What changed before until stays as it last stands.
func TestEventsArePutBackAsTheyStoodAtTheirEnd(t *testing.T) {
	run := time.Date(2026, 9, 20, 9, 0, 0, 0, time.Local)
	until := time.Date(2026, 9, 30, 0, 0, 0, 0, time.Local)
	before, after := until.Add(-time.Hour), until.Add(time.Hour)
	moves := func(forcedBy int, to ...string) []status.Event {
		var moved []status.Event
		for i, account := range to {
			moved = append(moved, status.Event{ID: 10 + i, At: before.Add(time.Duration(i) * time.Minute), Kind: status.EventMoved,
				Session: string(rune('a'+i)) + "c61b2f8", From: "work", To: account, ForcedBy: forcedBy})
		}
		return moved
	}
	tests := []struct {
		name   string
		events []status.Event
		want   status.Event
	}{
		{name: "a hold lifted after the end, unlifted",
			events: []status.Event{{ID: 1, At: before, Kind: status.EventHeld, Account: "work", Until: after}},
			want:   status.Event{ID: 1, At: before, Kind: status.EventHeld, Account: "work"}},
		{name: "a hold lifted at the end, unlifted",
			events: []status.Event{{ID: 1, At: before, Kind: status.EventHeld, Account: "work", Until: until}},
			want:   status.Event{ID: 1, At: before, Kind: status.EventHeld, Account: "work"}},
		{name: "a hold lifted before the end, lifted",
			events: []status.Event{{ID: 1, At: before, Kind: status.EventHeld, Account: "work", Until: before.Add(time.Minute)}},
			want:   status.Event{ID: 1, At: before, Kind: status.EventHeld, Account: "work", Until: before.Add(time.Minute)}},
		{name: "a limit's window joined after the end, gone",
			events: []status.Event{{ID: 1, At: before, Kind: status.EventLimit, Account: "work", Windows: []string{"5h", "7d", "7d_oi"},
				Joined: map[string]time.Time{"7d": before, "7d_oi": after}}},
			want: status.Event{ID: 1, At: before, Kind: status.EventLimit, Account: "work", Windows: []string{"5h", "7d"},
				Joined: map[string]time.Time{"7d": before}}},
		{name: "a cap's moves after the end, uncounted",
			events: []status.Event{{ID: 1, At: before, Kind: status.EventCap, Account: "work", Count: 2, To: "side"}},
			want:   status.Event{ID: 1, At: before, Kind: status.EventCap, Account: "work"}},
		{name: "a cap's moves before the end, counted, each session once",
			events: slices.Concat([]status.Event{{ID: 1, At: before.Add(-time.Hour), Kind: status.EventCap, Account: "work", Count: 5, To: "side"}},
				moves(1, "side", "side")),
			want: status.Event{ID: 1, At: before.Add(-time.Hour), Kind: status.EventCap, Account: "work", Count: 2, To: "side"}},
		{name: "a refusal's moves to several accounts, to none",
			events: slices.Concat([]status.Event{{ID: 1, At: before.Add(-time.Hour), Kind: status.EventRefused, Account: "work", Count: 2, To: "side"}},
				moves(1, "side", "personal")),
			want: status.Event{ID: 1, At: before.Add(-time.Hour), Kind: status.EventRefused, Account: "work", Count: 2}},
		{name: "a limit's moves by its limit alone, as before forced_by, counted",
			events: []status.Event{
				{ID: 1, At: before.Add(-time.Hour), Kind: status.EventLimit, Account: "work", Count: 3},
				{ID: 2, At: before, Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", Limit: 1},
			},
			want: status.Event{ID: 1, At: before.Add(-time.Hour), Kind: status.EventLimit, Account: "work", Count: 1, To: "side"}},
		{name: "a move forced by a cap told after the end, forced by none",
			events: []status.Event{{ID: 2, At: before, Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", ForcedBy: 3, Limit: 3}},
			want:   status.Event{ID: 2, At: before, Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side"}},
		{name: "a move forced by an event before it, as it stands",
			events: []status.Event{
				{ID: 1, At: before.Add(-time.Hour), Kind: status.EventCap, Account: "work", Count: 1, To: "side"},
				{ID: 2, At: before, Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", ForcedBy: 1},
			},
			want: status.Event{ID: 2, At: before, Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", ForcedBy: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			given := make([]Event, len(tt.events))
			for i, e := range tt.events {
				given[i] = Event{Key: EventKey{Run: run, ID: e.ID}, Event: e}
			}
			stood := stoodAt(given, until)
			i := slices.IndexFunc(stood, func(e Event) bool { return e.Key.ID == tt.want.ID })
			if i < 0 {
				t.Fatalf("the event %d isn't given back", tt.want.ID)
			}
			if !sameStood(stood[i].Event, tt.want) {
				t.Errorf("it stood as %+v, want %+v", stood[i].Event, tt.want)
			}
		})
	}
}

// sameStood reports whether two events are alike in every field stoodAt
// puts back, and when each happened.
func sameStood(a, b status.Event) bool {
	return a.ID == b.ID && a.At.Equal(b.At) && a.Kind == b.Kind && a.Until.Equal(b.Until) && slices.Equal(a.Windows, b.Windows) &&
		maps.EqualFunc(a.Joined, b.Joined, time.Time.Equal) && a.Count == b.Count && a.To == b.To && a.ForcedBy == b.ForcedBy && a.Limit == b.Limit
}

// Putting the events back leaves those read as they were, as a frame
// holding a read never sees it change.
func TestEventsPutBackLeaveTheReadAsItWas(t *testing.T) {
	until := time.Date(2026, 9, 30, 0, 0, 0, 0, time.Local)
	limit := status.Event{ID: 1, At: until.Add(-time.Hour), Kind: status.EventLimit, Windows: []string{"5h", "7d"}, Joined: map[string]time.Time{"7d": until}}
	given := []Event{{Key: EventKey{ID: 1}, Event: limit}}
	stoodAt(given, until)
	if !slices.Equal(given[0].Event.Windows, []string{"5h", "7d"}) || len(given[0].Event.Joined) != 1 {
		t.Errorf("put back, the read's limit is %+v, want it as read", given[0].Event)
	}
}
