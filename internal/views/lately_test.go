package views_test

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// latelyOf are the events given, newest first, numbered from the oldest, as
// the router's document gives them.
func latelyOf(events ...status.Event) status.Document {
	for i := range events {
		events[i].ID = len(events) - i
	}
	return status.Document{Source: status.SourceRouter, Events: events}
}

// sentenceOf is the one sentence LATELY tells of e, told at now.
func sentenceOf(t *testing.T, e status.Event, now time.Time) views.Sentence {
	t.Helper()
	got := views.Lately(latelyOf(e), now, 12)
	if len(got) != 1 {
		t.Fatalf("Lately() = %+v, want e told", got)
	}
	return got[0]
}

func TestLATELYTellsEachKindOfEventInItsSentence(t *testing.T) {
	now := at(16, 0)
	limitedFive := status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2, To: "side"}
	tests := []struct {
		name            string
		event           status.Event
		full, short     string
		subject, object string
		// shortObject is the short sentence's object, where it isn't the
		// full one's.
		shortObject string
	}{
		{name: "a session starting on the router's best", event: status.Event{Kind: status.EventStarted, Account: "side", Session: "c61b2f8d-04a7", Reason: status.ReasonNew},
			full: "c61b started on side", subject: "c61b", object: "side"},
		{name: "a session starting where the global pin sends it", event: status.Event{Kind: status.EventStarted, Account: "personal", Session: "c61b2f8d", Reason: status.ReasonGlobalPin},
			full: "c61b started on personal, as pinned", subject: "c61b", object: "personal"},
		{name: "a session starting where its own pin sends it", event: status.Event{Kind: status.EventStarted, Account: "side", Session: "9e21d4a8", Reason: status.ReasonPinned},
			full: "9e21 started on side, as pinned", subject: "9e21", object: "side"},
		{name: "a session starting elsewhere than its pin, in the views' words", event: status.Event{Kind: status.EventStarted, Account: "side", Session: "9e21d4a8", Reason: "pin yields: personal has no room"},
			full: "9e21 started on side: its pin to personal yields: personal has no room", subject: "9e21", object: "side"},
		{name: "a session starting passing over an account under pressure", event: status.Event{Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: "new, personal under pressure"},
			full: "c61b started on side: personal came under pressure", subject: "c61b", object: "side"},
		{name: "a session chosen afresh, passing over an account under pressure", event: status.Event{Kind: status.EventMoved, Session: "db8a71e3", From: "personal", To: "work", Reason: status.PassingOver(status.Rescored("21h 15m"), "personal")},
			full: "db8a moved personal → work: personal came under pressure", subject: "db8a", object: "personal"},
		{name: "a session moved off an account at its cap", event: status.Event{Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", Reason: "moved: work is at its reserve"},
			full: "d28c moved work → side: work reached its cap", subject: "d28c", object: "work"},
		{name: "a session moved by pin", event: status.Event{Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", Reason: status.ReasonMovedByPin},
			full: "d28c moved work → side: pinned to side", subject: "d28c", object: "side"},
		{name: "a session rescored, its reason naming no account", event: status.Event{Kind: status.EventMoved, Session: "db8a71e3", From: "side", To: "work", Reason: status.Rescored("15h")},
			full: "db8a moved side → work: rescored after 15h idle", subject: "db8a"},
		{name: "an account under pressure", event: status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}, Until: at(17, 5)},
			full: "personal came under pressure: its 5-hour runs out ~17:05", short: "personal under pressure: out ~17:05", subject: "personal", object: "~17:05"},
		{name: "an account under pressure, running out tomorrow, within a day", event: status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"7d"}, Until: now.Add(20 * time.Hour)},
			full: "personal came under pressure: its week runs out ~Thu 12:00", short: "personal under pressure: out ~Thu 12:00", subject: "personal", object: "~Thu 12:00"},
		{name: "an account under pressure, not saying when it runs out", event: status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}},
			full: "personal came under pressure", short: "personal under pressure", subject: "personal"},
		{name: "a cap moving sessions to one account", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, Count: 3, To: "side"},
			full: "work reached its cap: 3 sessions moved to side", short: "work at its cap: 3 moved to side", subject: "work", object: "3 sessions", shortObject: "3"},
		{name: "a cap moving a session", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Count: 1, To: "side"},
			full: "work reached its cap: 1 session moved to side", short: "work at its cap: 1 moved to side", subject: "work", object: "1 session", shortObject: "1"},
		{name: "a cap moving sessions to several accounts", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Count: 2},
			full: "work reached its cap: 2 sessions moved to other accounts", short: "work at its cap: 2 moved to other accounts", subject: "work", object: "2 sessions", shortObject: "2"},
		{name: "a cap moving none", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}},
			full: "work reached its cap", short: "work at its cap", subject: "work"},
		{name: "a limit in the 5-hour window", event: limitedFive,
			full: "personal reached its 5-hour limit: 2 sessions moved to side", short: "personal at its limit: 2 moved to side", subject: "personal", object: "2 sessions", shortObject: "2"},
		{name: "a limit in two windows", event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h", "7d"}},
			full: "personal reached its 5-hour and week limits", short: "personal at its limit", subject: "personal"},
		{name: "a limit in a model's own week", event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"7d_oi"}},
			full: "personal reached its Fable week limit", short: "personal at its limit", subject: "personal"},
		{name: "a limit naming no window", event: status.Event{Kind: status.EventLimit, Account: "personal"},
			full: "personal reached its limit", short: "personal at its limit", subject: "personal"},
		{name: "a token refused, moving sessions", event: status.Event{Kind: status.EventRefused, Account: "work", Status: 401, Count: 2, To: "side"},
			full: "work refused (401): 2 sessions moved to side", short: "work refused (401): 2 moved to side", subject: "work", object: "2 sessions", shortObject: "2"},
		{name: "a model family's requests refused alone", event: status.Event{Kind: status.EventRefused, Account: "work", Status: 403, Family: "opus"},
			full: "work refused (403, opus)", subject: "work"},
		{name: "a refusal giving no answer", event: status.Event{Kind: status.EventRefused, Account: "work"},
			full: "work refused", subject: "work"},
		{name: "a prime", event: status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: at(18, 50)},
			full: "side primed: its 5-hour window started", short: "side primed", subject: "side"},
		{name: "room again as a window resets", event: status.Event{Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
			full: "work open again: its 5-hour window reset", short: "work open again", subject: "work"},
		{name: "room again, naming no window", event: status.Event{Kind: status.EventRoom, Account: "work"},
			full: "work open again", subject: "work"},
		{name: "the global pin set", event: status.Event{Kind: status.EventPin, Account: "personal", Accounts: []string{"personal"}, By: "cli"},
			full: "pin set by hand: new sessions go to personal", short: "pin set: new sessions go to personal", subject: "pin", object: "personal"},
		{name: "the global pin set on two accounts", event: status.Event{Kind: status.EventPin, Account: "personal", Accounts: []string{"personal", "side"}, By: "dashboard"},
			full: "pin set by hand: new sessions go to personal and side", short: "pin set: new sessions go to personal and side", subject: "pin", object: "personal and side"},
		{name: "the global pin set, moving the running sessions too", event: status.Event{Kind: status.EventPin, Account: "personal", Accounts: []string{"personal"}, Move: true},
			full: "pin set by hand: new and running sessions go to personal", short: "pin set: new and running sessions go to personal", subject: "pin", object: "personal"},
		{name: "a session's own pin set", event: status.Event{Kind: status.EventPin, Account: "side", Session: "c61b2f8d"},
			full: "pin set by hand: c61b goes to side", short: "pin set: c61b goes to side", subject: "pin", object: "side"},
		{name: "routing back to the router", event: status.Event{Kind: status.EventAuto, Accounts: []string{"personal"}, By: "dashboard"},
			full: "new sessions back to auto", subject: "new sessions"},
		{name: "a session's own pin cleared", event: status.Event{Kind: status.EventAuto, Account: "side", Session: "c61b2f8d"},
			full: "c61b back to auto", subject: "c61b"},
		{name: "a restart due, in the Log's words", event: status.Event{Kind: status.EventRestart, Reason: status.RestartForConfig},
			full: "restart due: the config changed", subject: "restart"},
		{name: "a restart due for a reason of a later router's", event: status.Event{Kind: status.EventRestart, Reason: "the moon rose"},
			full: "restart due: the moon rose", subject: "restart"},
		{name: "the router unhealthy", event: status.Event{Kind: status.EventHealth, Reason: "most requests failing"},
			full: "the router turned unhealthy: most requests failing", subject: "the router"},
		{name: "the router healthy again", event: status.Event{Kind: status.EventHealth},
			full: "the router is healthy again", subject: "the router"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.event.At = at(13, 0)
			short, shortObject := cmp.Or(tt.short, tt.full), cmp.Or(tt.shortObject, tt.object)

			got := sentenceOf(t, tt.event, now)
			if got.Kind != tt.event.Kind || !got.At.Equal(tt.event.At) {
				t.Errorf("told as of kind %q at %v, want %q at %v", got.Kind, got.At, tt.event.Kind, tt.event.At)
			}
			if got.Full.String() != tt.full || got.Short.String() != short {
				t.Errorf("told as %q, short %q; want %q, %q", got.Full, got.Short, tt.full, short)
			}
			if got.Full.Subject.String() != tt.subject || got.Full.Object.String() != tt.object {
				t.Errorf("%q has subject %q and object %q, want %q and %q", got.Full, got.Full.Subject, got.Full.Object, tt.subject, tt.object)
			}
			if got.Short.Subject.String() != tt.subject || got.Short.Object.String() != shortObject {
				t.Errorf("short, %q has subject %q and object %q, want %q and %q", got.Short, got.Short.Subject, got.Short.Object, tt.subject, shortObject)
			}
		})
	}
}

func TestLATELYTellsOfTheRoutersHealthWellAgainApart(t *testing.T) {
	unwell := sentenceOf(t, status.Event{At: at(13, 0), Kind: status.EventHealth, Reason: "most requests failing"}, at(16, 0))
	well := sentenceOf(t, status.Event{At: at(13, 0), Kind: status.EventHealth}, at(16, 0))
	if unwell.Well || !well.Well {
		t.Errorf("Well is %v of the router turned unhealthy and %v of it well again, want false and true", unwell.Well, well.Well)
	}
	if room := sentenceOf(t, status.Event{At: at(13, 0), Kind: status.EventRoom, Account: "work"}, at(16, 0)); room.Well {
		t.Error("Well is set of an account with room again, want it of the router's health alone")
	}
}

func TestLATELYNamesAccountsApartAndASessionByItsFirstFourCharacters(t *testing.T) {
	cap := sentenceOf(t, status.Event{At: at(13, 0), Kind: status.EventCap, Account: "work", Count: 3, To: "side"}, at(16, 0))
	if want := (status.Words{{Text: "work", Kind: status.PartAccount}}); !slices.Equal(cap.Full.Subject, want) {
		t.Errorf("a cap's subject is %+v, want %+v", cap.Full.Subject, want)
	}
	if !slices.Contains(cap.Full.Rest, status.Part{Text: "side", Kind: status.PartAccount}) {
		t.Errorf("a cap's words after its count are %+v, want side named apart, as an account", cap.Full.Rest)
	}
	started := sentenceOf(t, status.Event{At: at(13, 0), Kind: status.EventStarted, Account: "side", Session: "c61b\x1b2f8d", Reason: status.ReasonNew}, at(16, 0))
	if want := (status.Words{{Text: "c61b"}}); !slices.Equal(started.Full.Subject, want) {
		t.Errorf("a session's start has subject %+v, want %+v: its id cleaned, cut to four characters", started.Full.Subject, want)
	}
}

func TestLATELYTellsTheKindsItKnowsNewestFirstNAtMost(t *testing.T) {
	room := func(account string) status.Event {
		return status.Event{At: at(13, 0), Kind: status.EventRoom, Account: account}
	}
	doc := latelyOf(room("work"), status.Event{At: at(12, 0), Kind: "eclipse", Account: "work"}, room("personal"), room("side"))
	var got []int
	for _, s := range views.Lately(doc, at(16, 0), 2) {
		got = append(got, s.ID)
	}
	if want := []int{4, 2}; !slices.Equal(got, want) {
		t.Errorf("LATELY tells of the events %v, want %v: the newest two of the kinds it knows", got, want)
	}
	if got := views.Lately(doc, at(16, 0), 0); len(got) != 0 {
		t.Errorf("LATELY with no lines tells %+v, want none", got)
	}
}

// heldLine is the id of heldBackAt's cap, limit or refusal.
const heldLine = 24

// movedBy is a session's move off work, counted in the line of the event
// with the id given: as a router from before forced_by gives a limit's, by
// the limit's id alone, where old is set.
func movedBy(id int, session string, line int, old bool) status.Event {
	e := status.Event{ID: id, At: at(14, 31), Kind: status.EventMoved, Session: session, From: "work", To: "side", Reason: "moved: work is at its reserve", ForcedBy: line}
	if old {
		e.ForcedBy, e.Limit = 0, line
	}
	return e
}

// heldBackAt are a cap, a limit or a refusal, of the kind given, that moved
// three sessions to side, each move counted in its line, as movedBy has
// them, between two sessions starting.
func heldBackAt(kind string, old bool) status.Document {
	return status.Document{Source: status.SourceRouter, Events: []status.Event{
		{ID: 28, At: at(14, 41), Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: status.ReasonNew},
		movedBy(27, "db8a71e3", heldLine, old), movedBy(26, "7f3a5c90", heldLine, old), movedBy(25, "d28c0b1e", heldLine, old),
		{ID: heldLine, At: at(14, 31), Kind: kind, Account: "work", Windows: []string{"5h"}, Status: 401, Count: 3, To: "side"},
		{ID: 23, At: at(14, 12), Kind: status.EventStarted, Account: "work", Session: "7f3a5c90", Reason: status.ReasonNew},
	}}
}

func TestLATELYFoldsTheMovesALimitACapOrARefusalForcedIntoItsLine(t *testing.T) {
	tests := []struct {
		name string
		doc  status.Document
		want []string
	}{
		{name: "a cap's", doc: heldBackAt(status.EventCap, false),
			want: []string{"c61b started on side", "work reached its cap: 3 sessions moved to side", "7f3a started on work"}},
		{name: "a limit's", doc: heldBackAt(status.EventLimit, false),
			want: []string{"c61b started on side", "work reached its 5-hour limit: 3 sessions moved to side", "7f3a started on work"}},
		{name: "a refusal's", doc: heldBackAt(status.EventRefused, false),
			want: []string{"c61b started on side", "work refused (401): 3 sessions moved to side", "7f3a started on work"}},
		{name: "a limit's, from a router from before forced_by", doc: heldBackAt(status.EventLimit, true),
			want: []string{"c61b started on side", "work reached its 5-hour limit: 3 sessions moved to side", "7f3a started on work"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, s := range views.Lately(tt.doc, at(16, 0), 3) {
				got = append(got, s.Full.String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("LATELY tells %q, want %q: the moves folded into their line, which counts them", got, tt.want)
			}
		})
	}
}

func TestLATELYShowsAMoveAloneOnlyWhereItsLineIsntAmongThoseShown(t *testing.T) {
	room := func(id int) status.Event {
		return status.Event{ID: id, At: at(14, 35), Kind: status.EventRoom, Account: "personal"}
	}
	outOfReach := status.Document{Source: status.SourceRouter, Events: []status.Event{
		movedBy(27, "db8a71e3", heldLine, false), room(26), room(25),
		{ID: heldLine, At: at(14, 31), Kind: status.EventCap, Account: "work", Count: 1, To: "side"},
	}}
	unkept := heldBackAt(status.EventCap, false)
	unkept.Events = unkept.Events[:4]
	tests := []struct {
		name string
		doc  status.Document
		n    int
		want []int
	}{
		{name: "the line shown, its moves folded into it", doc: heldBackAt(status.EventCap, false), n: 3, want: []int{28, heldLine, 23}},
		{name: "the line out of reach, its move alone", doc: outOfReach, n: 2, want: []int{27, 26}},
		{name: "the line's event no longer kept, its moves alone", doc: unkept, n: 4, want: []int{28, 27, 26, 25}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []int
			for _, s := range views.Lately(tt.doc, at(16, 0), tt.n) {
				got = append(got, s.ID)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("LATELY tells of the events %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLATELYSaysWhenAnAccountPassedOverRunsOutAsItsLatestPressureEventSaid(t *testing.T) {
	pressure := func(account string, until time.Time) status.Event {
		return status.Event{At: at(13, 0), Kind: status.EventPressure, Account: account, Windows: []string{"5h"}, Until: until}
	}
	started := status.Event{At: at(14, 0), Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: "new, personal under pressure"}
	tests := []struct {
		name   string
		events []status.Event
		want   string
	}{
		{name: "no pressure told of it", events: []status.Event{started},
			want: "c61b started on side: personal came under pressure"},
		{name: "its latest pressure before, another's after that", events: []status.Event{started, pressure("work", at(17, 0)), pressure("personal", at(15, 10)), pressure("personal", at(14, 50))},
			want: "c61b started on side: personal came under pressure, its 5-hour to run out at 15:10"},
		{name: "its pressure told after", events: []status.Event{pressure("personal", at(15, 10)), started},
			want: "c61b started on side: personal came under pressure"},
		{name: "its run-out come by the time it was passed over", events: []status.Event{started, pressure("personal", at(14, 0))},
			want: "c61b started on side: personal came under pressure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := latelyOf(tt.events...)
			got := views.Lately(doc, at(16, 0), 12)
			i := slices.IndexFunc(got, func(s views.Sentence) bool { return s.Kind == status.EventStarted })
			if i < 0 || got[i].Full.String() != tt.want {
				t.Errorf("LATELY tells %+v, want c61b's start as %q", got, tt.want)
			}
		})
	}
}
