package views_test

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
	"github.com/leeovery/switchboard/internal/views"
)

// The sessions SESSIONS' tests list.
const (
	answering = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
	asking    = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	idler     = "9c2e47b1-0d5a-4e83-b6f2-8a1c3e9d0f57"
	gone      = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
)

// seatOn is a session's model on an account, as the router lists it: put there
// made before now, for the reason given, last seen seen before now, and
// doing what its request in flight does.
func seatOn(model, family, account, reason string, made, seen time.Duration, inFlight string) status.Assignment {
	return status.Assignment{
		Model: model, Family: family, Account: account, Reason: reason, InFlight: inFlight,
		AssignedAt: now.Add(-made), LastSeen: now.Add(-seen),
	}
}

// runningDoc is the router's document of work, personal and side, new
// sessions going to side.
func runningDoc() status.Document {
	return status.Document{
		Source: status.SourceRouter, Best: "side",
		Accounts: []status.Account{{ID: "work"}, {ID: "personal"}, {ID: "side"}},
	}
}

// running is a row of RunningOf's, its words run together.
type running struct {
	account, session, family string
	seen                     time.Duration
	why                      string
	models                   []string
	doing                    views.Doing
}

// rowsOf are RunningOf's rows, as running has them.
func rowsOf(rows []views.Running) []running {
	got := make([]running, len(rows))
	for i, r := range rows {
		got[i] = running{
			account: r.Account, session: r.Session, family: r.Family, seen: now.Sub(r.Activity.LastSeen),
			why: r.Why.String(), models: r.Models, doing: r.Activity.Doing,
		}
	}
	return got
}

func TestTheSessionsRunningAreGroupedByAccountInTheConfigsOrderTheOneSeenLastFirst(t *testing.T) {
	sessions := []status.Session{
		{ID: idler, Assignments: []status.Assignment{seatOn("claude-opus-5-5", "opus", "personal", status.ReasonNew, 3*time.Hour, 9*time.Minute, "")}},
		{ID: answering, Assignments: []status.Assignment{seatOn("claude-sonnet-5-5", "sonnet", "personal", status.ReasonNew, 2*time.Hour, 0, status.Answering)}},
		{ID: asking, Assignments: []status.Assignment{
			seatOn("claude-opus-5-5", "opus", "side", "moved: work is at its reserve", 11*time.Minute, time.Second, status.Asking),
			seatOn("claude-haiku-4-5", "haiku", "work", status.ReasonNew, 50*time.Minute, 40*time.Minute, ""),
		}},
		{ID: gone, Assignments: []status.Assignment{seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, 3*time.Hour, 61*time.Minute, "")}},
	}
	want := []running{
		{account: "work", session: asking, family: "haiku", seen: 40 * time.Minute, why: "started here 12:22", models: []string{"claude-haiku-4-5"}},
		{account: "personal", session: answering, family: "sonnet", why: "started here 11:12", models: []string{"claude-sonnet-5-5"}, doing: views.Answering},
		{account: "personal", session: idler, family: "opus", seen: 9 * time.Minute, why: "started here 10:12", models: []string{"claude-opus-5-5"}},
		{account: "side", session: asking, family: "opus", seen: time.Second, why: "moved from work 13:01: work reached its cap", models: []string{"claude-opus-5-5"}, doing: views.Asking},
	}
	if got := rowsOf(views.RunningOf(runningDoc().At(now, claude.Policy), sessions, views.Stream{})); !reflect.DeepEqual(got, want) {
		t.Errorf("RunningOf =\n  %+v\nwant\n  %+v", got, want)
	}
}

// Detail's SESSIONS of an account are RunningOf's rows on it, in its order:
// a session on two accounts is on each, one on none is on neither.
func TestDetailsSessionsAreTheRowsOnTheAccountInTheirOrder(t *testing.T) {
	sessions := []status.Session{
		{ID: idler, Assignments: []status.Assignment{seatOn("claude-opus-5-5", "opus", "side", status.ReasonNew, 3*time.Hour, 9*time.Minute, "")}},
		{ID: answering, Assignments: []status.Assignment{seatOn("claude-sonnet-5-5", "sonnet", "side", status.ReasonNew, 2*time.Hour, 0, status.Answering)}},
		{ID: asking, Assignments: []status.Assignment{
			seatOn("claude-opus-5-5", "opus", "side", "moved: work is at its reserve", 11*time.Minute, time.Second, status.Asking),
			seatOn("claude-haiku-4-5", "haiku", "work", status.ReasonNew, 50*time.Minute, 40*time.Minute, ""),
		}},
	}
	all := views.RunningOf(runningDoc().At(now, claude.Policy), sessions, views.Stream{})
	ids := func(id string) []string {
		var got []string
		for _, r := range views.RunningOn(all, id) {
			got = append(got, r.Account+" "+status.ShownID(r.Session))
		}
		return got
	}
	tests := []struct {
		account string
		want    []string
	}{
		{account: "work", want: []string{"work d28c"}},
		{account: "personal"},
		{account: "side", want: []string{"side 41e0", "side d28c", "side 9c2e"}},
	}
	for _, tt := range tests {
		if got := ids(tt.account); !slices.Equal(got, tt.want) {
			t.Errorf("SESSIONS of %s lists %v, want %v", tt.account, got, tt.want)
		}
	}
}

func TestASessionsModelsOnOneAccountAreOneRowItsModelSeenLastLeading(t *testing.T) {
	sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{
		seatOn("claude-haiku-4-5", "haiku", "side", status.ReasonNew, 10*time.Minute, 2*time.Minute, status.Asking),
		seatOn("claude-opus-5-5", "opus", "side", status.ReasonPinned, 30*time.Minute, time.Minute, ""),
		seatOn("claude-sonnet-5-5", "sonnet", "side", status.ReasonNew, 20*time.Minute, 3*time.Minute, status.Answering),
	}}}
	want := []running{{
		account: "side", session: asking, family: "opus", seen: time.Minute, why: "pinned here 12:42",
		models: []string{"claude-opus-5-5", "claude-haiku-4-5", "claude-sonnet-5-5"}, doing: views.Answering,
	}}
	if got := rowsOf(views.RunningOf(runningDoc().At(now, claude.Policy), sessions, views.Stream{})); !reflect.DeepEqual(got, want) {
		t.Errorf("RunningOf =\n  %+v\nwant\n  %+v", got, want)
	}
}

// On an account, a session is named by the model naming it, as the request
// stream tells of its main thread, where that runs there, its reason with
// it: a title seen since doesn't name it.
func TestASessionsRowIsNamedByItsMainThreadWhereItRunsThere(t *testing.T) {
	sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{
		seatOn("claude-haiku-4-5", "haiku", "side", status.ReasonNew, 10*time.Minute, time.Minute, ""),
		seatOn("claude-opus-5-5", "opus", "side", status.ReasonPinned, 30*time.Minute, 2*time.Minute, ""),
	}}}
	main := views.Told{Request: "r1", Session: asking, Model: "claude-opus-5-5", Account: "side", Class: "main", Sent: now.Add(-2 * time.Minute)}
	main.Ended, main.Heard = main.Sent.Add(10*time.Second), main.Sent.Add(10*time.Second)
	title := views.Told{Request: "r2", Session: asking, Model: "claude-haiku-4-5", Account: "side", Class: "auxiliary", Sent: now.Add(-time.Minute)}
	title.Ended, title.Heard = title.Sent.Add(time.Second), title.Sent.Add(time.Second)
	tests := []struct {
		name        string
		told        []views.Told
		family, why string
	}{
		{name: "its main thread's, a title seen since", told: []views.Told{main, title}, family: "opus", why: "pinned here 12:42"},
		{name: "its model seen last there, no main request told of", told: []views.Told{title}, family: "haiku", why: "started here 13:02, the best"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := views.Stream{Live: true, Requests: make(map[string]views.Told)}
			for _, r := range tt.told {
				stream.Requests[r.Request] = r
			}
			rows := views.RunningOf(runningDoc().At(now, claude.Policy), sessions, stream)
			if len(rows) != 1 || rows[0].Family != tt.family || rows[0].Why.String() != tt.why {
				t.Errorf("RunningOf = %+v, want one row, %s, %q", rowsOf(rows), tt.family, tt.why)
			}
		})
	}
}

func TestASessionRunsWhereItWasRoutedWithinTheHour(t *testing.T) {
	tests := []struct {
		name       string
		made, seen time.Duration
		runs       bool
	}{
		{name: "seen within the hour", made: 3 * time.Hour, seen: 59 * time.Minute, runs: true},
		{name: "seen an hour ago, to the second", made: 3 * time.Hour, seen: time.Hour, runs: true},
		{name: "seen longer ago", made: 3 * time.Hour, seen: time.Hour + time.Second},
		{name: "seen longer ago, but put there since, as a move puts it", made: 10 * time.Minute, seen: 2 * time.Hour, runs: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sessions := []status.Session{{ID: idler, Assignments: []status.Assignment{seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, tt.made, tt.seen, "")}}}
			if got := len(views.RunningOf(runningDoc().At(now, claude.Policy), sessions, views.Stream{})) == 1; got != tt.runs {
				t.Errorf("runs = %t, want %t", got, tt.runs)
			}
		})
	}
}

func TestWhyASessionRunsWhereItDoes(t *testing.T) {
	movedByPin := seatOn("claude-opus-5-5", "opus", "side", status.ReasonMovedByPin, 3*time.Minute, time.Minute, "")
	pinnedAsItRan := seatOn("claude-opus-5-5", "opus", "side", status.ReasonPinned, 50*time.Minute, time.Minute, "")
	pinnedAsItRan.Pinned, pinnedAsItRan.PinnedAt = true, now.Add(-5*time.Minute)
	moves := []status.Event{
		{ID: 3, Kind: status.EventMoved, At: now.Add(-3 * time.Minute), Session: asking, Model: "claude-opus-5-5", From: "personal", To: "side", Reason: status.ReasonMovedByPin},
		{ID: 2, Kind: status.EventMoved, At: now.Add(-20 * time.Minute), Session: asking, Model: "claude-opus-5-5", From: "work", To: "side", Reason: status.ReasonMovedByPin},
		{ID: 1, Kind: status.EventMoved, At: now.Add(-25 * time.Minute), Session: asking, Model: "claude-haiku-4-5", From: "work", To: "side", Reason: status.ReasonMovedByPin},
	}
	tests := []struct {
		name   string
		seat   status.Assignment
		events []status.Event
		want   string
	}{
		{name: "started on the account new sessions go to", seat: seatOn("claude-opus-5-5", "opus", "side", status.ReasonNew, 3*time.Minute, 0, ""), want: "started here 13:09, the best"},
		{name: "started on another", seat: seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, 3*time.Minute, 0, ""), want: "started here 13:09"},
		{name: "moved by the pin, from the account its newest move there tells", seat: movedByPin, events: moves, want: "moved from personal 13:09: pinned to side"},
		{name: "moved by the pin, no move told", seat: movedByPin, want: "here since 13:09: pinned to side"},
		{name: "pinned as it ran: since its pin", seat: pinnedAsItRan, want: "pinned here 13:07"},
		{name: "pinned as it started: since it came", seat: seatOn("claude-opus-5-5", "opus", "side", status.ReasonPinned, 50*time.Minute, time.Minute, ""), want: "pinned here 12:22"},
		{name: "its pin yielding", seat: seatOn("claude-opus-5-5", "opus", "side", "pin yields: personal has no room", 50*time.Minute, time.Minute, ""), want: "its pin to personal yields: personal has no room"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := runningDoc()
			doc.Events = tt.events
			rows := views.RunningOf(doc.At(now, claude.Policy), []status.Session{{ID: asking, Assignments: []status.Assignment{tt.seat}}}, views.Stream{})
			if len(rows) != 1 {
				t.Fatalf("RunningOf gave %d rows, want 1", len(rows))
			}
			if got := rows[0].Why.String(); got != tt.want {
				t.Errorf("Why = %q, want %q", got, tt.want)
			}
		})
	}
}

// A session started on the account new sessions go to says so, as ▲ marks
// it: a lone account always, whatever its room, and of several, the router's
// best alone.
func TestASessionStartedWhereNewSessionsGoIsTheBest(t *testing.T) {
	started := seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, 3*time.Minute, 0, "")
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{
			name: "the router's, of several, its best",
			doc:  status.Document{Source: status.SourceRouter, Best: "work", Accounts: []status.Account{{ID: "work"}, {ID: "side"}}},
			want: "started here 13:09, the best",
		},
		{
			name: "the router's, of several, another its best",
			doc:  status.Document{Source: status.SourceRouter, Best: "side", Accounts: []status.Account{{ID: "work"}, {ID: "side"}}},
			want: "started here 13:09",
		},
		{
			name: "the router's, of one, though the router names no best, as with no room",
			doc:  status.Document{Source: status.SourceRouter, Accounts: []status.Account{{ID: "work"}}},
			want: "started here 13:09, the best",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := views.RunningOf(tt.doc.At(now, claude.Policy), []status.Session{{ID: asking, Assignments: []status.Assignment{started}}}, views.Stream{})
			if len(rows) != 1 {
				t.Fatalf("RunningOf gave %d rows, want 1", len(rows))
			}
			if got := rows[0].Why.String(); got != tt.want {
				t.Errorf("Why = %q, want %q", got, tt.want)
			}
		})
	}
}

// At each hard moment, a session started on an account says it's the best
// where ▲ marks the account, as the moment has it, and nowhere else.
func TestASessionIsTheBestWhereTheMomentMarksItsAccountAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claude.Policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, claude.Policy)
			var sessions []status.Session
			for _, a := range m.Accounts {
				sessions = append(sessions, status.Session{ID: a.ID + "-session", Assignments: []status.Assignment{{
					Model: "claude-opus-5-5", Family: "opus", Account: a.ID, Reason: status.ReasonNew,
					AssignedAt: hm.Now.Add(-time.Minute), LastSeen: hm.Now,
				}}})
			}
			rows := views.RunningOf(m, sessions, views.Stream{})
			if len(rows) != len(m.Accounts) {
				t.Fatalf("RunningOf gave %d rows, want one an account, %d", len(rows), len(m.Accounts))
			}
			for i, a := range m.Accounts {
				if best := strings.HasSuffix(rows[i].Why.String(), ", the best"); best != a.Next {
					t.Errorf("%s: a session started there says it's the best: %v; ▲ marks it: %v", a.ID, best, a.Next)
				}
			}
		})
	}
}

// The stream's moves put each model the newest move of it took where it
// went, till a listing of the router's sessions shows it done: the session
// runs there, and nowhere it ran only by that model; one the router didn't
// list that a move brought on runs too, named by the family the router
// gives its model; and RunningCounts counts them as RunningOf lists them.
func TestTheStreamsMovesPutTheSessionsWhereTheyWent(t *testing.T) {
	sessions := []status.Session{
		{ID: asking, Assignments: []status.Assignment{seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, time.Hour, time.Minute, status.Asking)}},
		{ID: idler, Assignments: []status.Assignment{seatOn("claude-haiku-4-5", "haiku", "work", status.ReasonNew, time.Hour, 9*time.Minute, "")}},
	}
	moved := func(session, model, from, to string, ago time.Duration) views.Moved {
		return views.Moved{Session: session, Model: model, From: from, To: to, Reason: status.ReasonMovedOff + from + " " + status.WhyLimit, At: now.Add(-ago)}
	}
	stream := views.Stream{Live: true, Moves: []views.Moved{
		moved(asking, "claude-opus-5-5", "work", "personal", 3*time.Second),
		moved(gone, "claude-haiku-4-5", "work", "side", 2*time.Second),
		moved(asking, "claude-opus-5-5", "personal", "side", time.Second),
	}}
	m := runningDoc().At(now, claude.Policy)
	rows := views.RunningOf(m, sessions, stream)
	var got []string
	for _, r := range rows {
		got = append(got, r.Account+" "+r.Session[:4]+" "+r.Family)
	}
	if want := []string{"work 9c2e haiku", "side d28c opus", "side 7f3a haiku"}; !reflect.DeepEqual(got, want) {
		t.Errorf("RunningOf lists %q, want %q", got, want)
	}
	if got, want := views.RunningCounts(rows), map[string]int{"work": 1, "side": 2}; !maps.Equal(got, want) {
		t.Errorf("RunningCounts = %v, want %v", got, want)
	}
	if sessions[0].Assignments[0].Account != "work" || len(sessions) != 2 {
		t.Error("the router's listing was changed")
	}
}
