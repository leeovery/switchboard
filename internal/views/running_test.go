package views_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
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
	doing                    views.Doing
}

// rowsOf are RunningOf's rows, as running has them.
func rowsOf(rows []views.Running) []running {
	got := make([]running, len(rows))
	for i, r := range rows {
		got[i] = running{
			account: r.Account, session: r.Session, family: r.Family, seen: now.Sub(r.Activity.LastSeen),
			why: r.Why.String(), doing: r.Activity.Doing,
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
		{account: "work", session: asking, family: "haiku", seen: 40 * time.Minute, why: "started here 12:22"},
		{account: "personal", session: answering, family: "sonnet", why: "started here 11:12", doing: views.Answering},
		{account: "personal", session: idler, family: "opus", seen: 9 * time.Minute, why: "started here 10:12"},
		{account: "side", session: asking, family: "opus", seen: time.Second, why: "moved from work 13:01: work reached its cap", doing: views.Asking},
	}
	if got := rowsOf(views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: views.Stream{}}, views.HeldList{})); !reflect.DeepEqual(got, want) {
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
	all := views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: views.Stream{}}, views.HeldList{})
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

func TestASessionsModelsOnOneAccountAreOneRowNamedByItsModelSeenLast(t *testing.T) {
	sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{
		seatOn("claude-haiku-4-5", "haiku", "side", status.ReasonNew, 10*time.Minute, 2*time.Minute, status.Asking),
		seatOn("claude-opus-5-5", "opus", "side", status.ReasonPinned, 30*time.Minute, time.Minute, ""),
		seatOn("claude-sonnet-5-5", "sonnet", "side", status.ReasonNew, 20*time.Minute, 3*time.Minute, status.Answering),
	}}}
	want := []running{{
		account: "side", session: asking, family: "opus", seen: time.Minute, why: "pinned here 12:42", doing: views.Answering,
	}}
	if got := rowsOf(views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: views.Stream{}}, views.HeldList{})); !reflect.DeepEqual(got, want) {
		t.Errorf("RunningOf =\n  %+v\nwant\n  %+v", got, want)
	}
}

// listHeldOf is the List held of the lines given, as the watch holds it, read at
// now, of the sessions running gives, as the router lists them.
func listHeldOf(lines []ledger.Line, running []status.Session, now time.Time) views.HeldList {
	return views.HoldList(views.ReadList(fakeLedger{lines: lines, now: now}, running), ledger.Pricing, now)
}

// On an account, a session is named by the model naming it, as the List
// names it, where that runs there, its reason with it: its main thread's, as
// its lines tell of it, the request stream's taken in after them; a title
// seen since doesn't name it, whether its lines or the stream tell of it.
func TestASessionsRowIsNamedByItsMainThreadWhereItRunsThere(t *testing.T) {
	sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{
		seatOn(haiku, "haiku", "side", status.ReasonNew, 10*time.Minute, time.Minute, ""),
		seatOn(opus, "opus", "side", status.ReasonPinned, 30*time.Minute, 2*time.Minute, ""),
	}}}
	main := views.Told{Request: "r1", Session: asking, Model: opus, Account: "side", Class: "main", Sent: now.Add(-2 * time.Minute)}
	main.Ended, main.Heard = main.Sent.Add(10*time.Second), main.Sent.Add(10*time.Second)
	title := views.Told{Request: "r2", Session: asking, Model: haiku, Account: "side", Class: "auxiliary", Sent: now.Add(-time.Minute)}
	title.Ended, title.Heard = title.Sent.Add(time.Second), title.Sent.Add(time.Second)
	mainLine := filedAs("r1", took(10*time.Second, serving(p1, ledger.ClassMain, line(asking, opus, "side", status.ReasonPinned, main.Sent, hourUsage))))
	titleLine := filedAs("r2", took(time.Second, serving(p1, "auxiliary", line(asking, haiku, "side", status.ReasonNew, title.Sent, hourUsage))))
	tests := []struct {
		name               string
		told               []views.Told
		lines              []ledger.Line
		model, family, why string
	}{
		{name: "its main thread's, a title seen since, as the stream told", told: []views.Told{main, title}, model: opus, family: "opus", why: "pinned here 12:42"},
		{name: "its main thread's, a title seen since, as its lines tell, nothing told", lines: []ledger.Line{mainLine, titleLine}, model: opus, family: "opus", why: "pinned here 12:42"},
		{name: "its main thread's as its lines tell, a title told since", told: []views.Told{title}, lines: []ledger.Line{mainLine}, model: opus, family: "opus", why: "pinned here 12:42"},
		{name: "its model seen last there, no main request told of nor lined", told: []views.Told{title}, model: haiku, family: "haiku", why: "started here 13:02, the best"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := views.Stream{Live: true, Requests: make(map[string]views.Told)}
			for _, r := range tt.told {
				stream.Requests[r.Request] = r
			}
			held := listHeldOf(tt.lines, sessions, now)
			rows := views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: stream}, held)
			if len(rows) != 1 || rows[0].Model != tt.model || rows[0].Family != tt.family || rows[0].Why.String() != tt.why {
				t.Errorf("RunningOf = %+v, want one row, %s, %q", rowsOf(rows), tt.model, tt.why)
			}
			list := held.List(views.ListLive{Running: sessions, Stream: stream, Now: now})
			if len(list.Sessions) != 1 || list.Sessions[0].Model != tt.model {
				t.Errorf("the List names it %+v, want %s, as its row", list.Sessions, tt.model)
			}
		})
	}
}

// A session whose main thread has waited over the IdleHour on a subagent of
// another model is named on an account by its main thread where it goes
// there, cold as it is, as the List names it: by its model, why it's there,
// and the account it came from. SESSIONS draws a session where its models
// run, though: where its main thread goes to none they run on, its row is on
// the account its subagent runs on, named by the subagent's model, while the
// List names it by its main thread, on the account that goes to.
func TestASessionsRowIsNamedByItsMainThreadThoughItsColdThere(t *testing.T) {
	const sonnet = "claude-sonnet-5-5"
	mainAt := now.Add(-72 * time.Minute)
	subAt := []time.Time{now.Add(-70 * time.Minute), now.Add(-30 * time.Minute), now.Add(-2 * time.Minute)}
	tests := []struct {
		name                           string
		sub                            string
		account, model, family, why    string
		from, listedOn, listedAs, came string
	}{
		{
			name: "its subagent on its main thread's account", sub: "work",
			account: "work", model: opus, family: "opus", why: "moved from personal 12:00: personal reached its limit", from: "personal",
			listedOn: "work", listedAs: opus, came: "personal",
		},
		{
			name: "its subagent on another account: SESSIONS draws what runs where", sub: "side",
			account: "side", model: sonnet, family: "sonnet", why: "started here 12:02, the best",
			listedOn: "work", listedAs: opus, came: "personal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []ledger.Line{
				filedAs("m0", took(10*time.Second, serving(p1, ledger.ClassMain, line(asking, opus, "personal", status.ReasonNew, mainAt.Add(-time.Hour), hourUsage)))),
				filedAs("m1", took(10*time.Second, serving(p1, ledger.ClassMain, movedFrom("personal", line(asking, opus, "work", "moved: personal hit its limit", mainAt, hourUsage))))),
			}
			for i, at := range subAt {
				l := filedAs(fmt.Sprintf("a%d", i), took(20*time.Second, line(asking, sonnet, tt.sub, status.ReasonNew, at, hourUsage)))
				l.AgentID = "agent-1"
				lines = append(lines, l)
			}
			sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{
				{Model: opus, Family: "opus", Account: "work", Reason: "moved: personal hit its limit", AssignedAt: mainAt, LastSeen: mainAt.Add(10 * time.Second)},
				{Model: sonnet, Family: "sonnet", Account: tt.sub, Reason: status.ReasonNew, AssignedAt: subAt[0], LastSeen: subAt[2].Add(20 * time.Second)},
			}}}
			held := views.HoldList(views.ReadList(sessionsLedger(lines, now), sessions), ledger.Pricing, now)
			m := runningDoc().At(now, claude.Policy())
			calls := views.CallsOf(m, views.ListLive{Running: sessions, Stream: views.Stream{}}, held)
			if len(calls) != 1 {
				t.Fatalf("the calls are %+v, want one", calls)
			}
			c := calls[0]
			if c.Account != tt.account || c.Model != tt.model || c.Family != tt.family || c.Why.String() != tt.why || c.From != tt.from {
				t.Errorf("its row is on %s, %s, %s, %q, from %q; want on %s, %s, %s, %q, from %q",
					c.Account, c.Model, c.Family, c.Why, c.From, tt.account, tt.model, tt.family, tt.why, tt.from)
			}
			list := held.List(views.ListLive{Running: sessions, Now: now})
			if len(list.Sessions) != 1 {
				t.Fatalf("the List holds %+v, want one session", list.Sessions)
			}
			s := list.Sessions[0]
			if s.Account != tt.listedOn || s.Model != tt.listedAs || s.Moved == nil || s.Moved.From != tt.came {
				t.Errorf("the List names it on %s, %s, moved %+v; want on %s, %s, come from %s", s.Account, s.Model, s.Moved, tt.listedOn, tt.listedAs, tt.came)
			}
		})
	}
}

// A session's row on an account does what its models there do, as the List
// has the whole session do: those the router lists there, and those the
// request stream told of there that it doesn't list yet, as a model new to
// the session; but not those on another account.
func TestASessionsRowDoesWhatItsModelsTheStreamToldOfThereDo(t *testing.T) {
	sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{
		seatOn(opus, "opus", "side", status.ReasonNew, 30*time.Minute, 2*time.Minute, ""),
	}}}
	newModel := func(account string) views.Told {
		return views.Told{Request: "r1", Session: asking, Model: haiku, Account: account, Class: "auxiliary", Sent: now.Add(-time.Second), Heard: now.Add(-time.Second)}
	}
	tests := []struct {
		name      string
		told      views.Told
		row, list views.Doing
	}{
		{name: "a new model asking on its account, not listed yet", told: newModel("side"), row: views.Asking, list: views.Asking},
		{name: "a new model asking on another account, not listed yet", told: newModel("work"), row: views.Idle, list: views.Asking},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := views.Stream{Live: true, Requests: map[string]views.Told{tt.told.Request: tt.told}}
			rows := views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: stream}, views.HeldList{})
			list := listHeldOf(nil, sessions, now).List(views.ListLive{Running: sessions, Stream: stream, Now: now})
			if len(rows) != 1 || rows[0].Account != "side" || rows[0].Activity.Doing != tt.row {
				t.Errorf("RunningOf = %+v, want one row, on side, doing %d", rowsOf(rows), tt.row)
			}
			if len(list.Sessions) != 1 || list.Sessions[0].Activity.Doing != tt.list {
				t.Errorf("the List has it doing %+v, want %d", list.Sessions, tt.list)
			}
		})
	}
}

// A session's row is seen as late as the stream last told of a request of
// its model there, and idle since, whether the stream holds the request
// still or it has gone, its line read, as Seen keeps its seat's sight: an
// answer sent seven minutes ago, as the router lists it seen, and ended three
// minutes ago, is idle three minutes either way. A seat seen of another
// account's says nothing of this one.
func TestASessionsRowIsSeenAsLateAsItsSeatWasWhetherItsRequestIsHeldOrGone(t *testing.T) {
	sessions := []status.Session{{ID: idler, Assignments: []status.Assignment{
		seatOn(opus, "opus", "work", status.ReasonNew, 30*time.Minute, 7*time.Minute, ""),
	}}}
	ended := views.Told{Request: "r1", Session: idler, Model: opus, Account: "work", Sent: now.Add(-7 * time.Minute), Ended: now.Add(-3 * time.Minute), Status: 200, Heard: now.Add(-3 * time.Minute)}
	seat, _ := ended.Seat()
	tests := []struct {
		name   string
		stream views.Stream
		seen   time.Duration
	}{
		{name: "its request held", stream: views.Stream{Live: true, Requests: map[string]views.Told{"r1": ended}}, seen: 3 * time.Minute},
		{name: "its request gone, its seat seen", stream: views.Stream{Live: true, Seen: map[views.Seat]time.Time{seat: ended.Heard}}, seen: 3 * time.Minute},
		{name: "another account's seat seen", stream: views.Stream{Live: true, Seen: map[views.Seat]time.Time{{Session: idler, Model: opus, Account: "side"}: ended.Heard}}, seen: 7 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: tt.stream}, views.HeldList{})
			i := slices.IndexFunc(rows, func(r views.Running) bool { return r.Account == "work" })
			if i < 0 {
				t.Fatalf("RunningOf = %+v, want a row on work", rowsOf(rows))
			}
			if got := rowsOf(rows)[i]; got.seen != tt.seen || got.doing != views.Idle || !rows[i].Activity.Since.Equal(now.Add(-tt.seen)) {
				t.Errorf("the row on work is %+v, idle since %s, want it seen %s ago, idle since then", got, rows[i].Activity.Since, tt.seen)
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
			if got := len(views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: views.Stream{}}, views.HeldList{})) == 1; got != tt.runs {
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
			rows := views.RunningOf(doc.At(now, claude.Policy()), views.ListLive{Running: []status.Session{{ID: asking, Assignments: []status.Assignment{tt.seat}}}}, views.HeldList{})
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
			rows := views.RunningOf(tt.doc.At(now, claude.Policy()), views.ListLive{Running: []status.Session{{ID: asking, Assignments: []status.Assignment{started}}}}, views.HeldList{})
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
	for _, hm := range statustest.HardMoments(claude.Policy()) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, claude.Policy())
			var sessions []status.Session
			for _, a := range m.Accounts {
				sessions = append(sessions, status.Session{ID: a.ID + "-session", Assignments: []status.Assignment{{
					Model: "claude-opus-5-5", Family: "opus", Account: a.ID, Reason: status.ReasonNew,
					AssignedAt: hm.Now.Add(-time.Minute), LastSeen: hm.Now,
				}}})
			}
			rows := views.RunningOf(m, views.ListLive{Running: sessions, Stream: views.Stream{}}, views.HeldList{})
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

// A session the router didn't list that a move brought on is named by its
// model's family, as the one owner of a model's family reads it, though
// the router lists no other session of that model.
func TestASessionAMoveBroughtOnIsNamedByItsModelsFamily(t *testing.T) {
	sessions := []status.Session{
		{ID: asking, Assignments: []status.Assignment{seatOn("claude-opus-5-5", "opus", "work", status.ReasonNew, time.Hour, time.Minute, status.Asking)}},
	}
	stream := views.Stream{Live: true, Moves: []views.Moved{
		{Session: gone, Model: "claude-haiku-4-5", From: "work", To: "side", Reason: status.ReasonMovedOff + "work " + status.WhyLimit, At: now.Add(-2 * time.Second)},
	}}
	for _, r := range views.RunningOf(runningDoc().At(now, claude.Policy()), views.ListLive{Running: sessions, Stream: stream}, views.HeldList{}) {
		if r.Session == gone && r.Family != "haiku" {
			t.Errorf("the session a move brought on is named by family %q, want haiku", r.Family)
		}
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
	m := runningDoc().At(now, claude.Policy())
	rows := views.RunningOf(m, views.ListLive{Running: sessions, Stream: stream}, views.HeldList{})
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
