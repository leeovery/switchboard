package views_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// filedAs returns l, the line of the request the router knows by the id
// given.
func filedAs(id string, l ledger.Line) ledger.Line {
	l.Request = id
	return l
}

// toldOf is a request of paged's the stream told of, by the id given: of the
// class given, serving prompt, of model on account, sent at sent, and in
// flight since.
func toldOf(id, prompt, class, model, account string, sent time.Time) views.Told {
	return views.Told{Request: id, Session: paged, Model: model, Account: account, Prompt: prompt, Class: class, Sent: sent, Heard: sent}
}

// endedAt returns t, ended at the time given, as the stream told.
func endedAt(at time.Time, t views.Told) views.Told {
	t.Ended, t.Heard, t.Status = at, at, 200
	return t
}

// lostAt returns t, lost as the stream last told of it at the time given.
func lostAt(at time.Time, t views.Told) views.Told {
	t.Lost, t.Heard = true, at
	return t
}

// heard is the request stream, live, telling of the requests given.
func heard(requests ...views.Told) views.Stream {
	s := views.Stream{Live: true, Requests: make(map[string]views.Told, len(requests))}
	for _, r := range requests {
		s.Requests[r.Request] = r
	}
	return s
}

// A request counts in its turn as it's sent, by what the stream told of it
// till its line comes, by its line after: a turn goes on while a request of
// its own conversation is in flight, or its answer is yet to be read from
// its line, and following its own conversation, on the accounts that went
// to; a request lost, with no line, counts, ended untold, keeping it going
// no longer.
func TestARequestCountsInItsTurnAsItsSent(t *testing.T) {
	sideOnly, workOnly := []string{"side"}, []string{"work"}
	first := filedAs("r1", mainOn(p1, local(10, 0, 0), "end_turn"))
	calling := filedAs("r1", mainOn(p1, local(10, 0, 0), "tool_use"))
	title := filedAs("r3", took(time.Minute, serving(p2, "auxiliary", line(paged, haiku, "work", "sticky", local(10, 5, 1), smallUsage))))
	check, count, others := toldOf("c1", p1, ledger.ClassMain, opus, "side", local(10, 2, 0)), toldOf("c2", p1, ledger.ClassMain, opus, "side", local(10, 2, 0)),
		toldOf("o1", p1, ledger.ClassMain, opus, "side", local(10, 2, 0))
	check.Check, count.Count, others.Session = true, true, other
	tests := []struct {
		name    string
		lines   []ledger.Line
		told    []views.Told
		running bool
		want    []turnSaid
	}{
		{
			name:  "one in flight, joining the prompt running, keeping it going, its session ended or not",
			lines: []ledger.Line{first},
			told:  []views.Told{toldOf("r2", p1, ledger.ClassMain, opus, "side", local(10, 2, 0))},
			want:  []turnSaid{{1, local(10, 0, 0), time.Time{}, 2, sideOnly}},
		},
		{
			name:  "one in flight, a new prompt's first, a turn of its own, going",
			lines: []ledger.Line{first},
			told:  []views.Told{toldOf("r2", p2, ledger.ClassMain, opus, "work", local(10, 5, 0))},
			want:  []turnSaid{{2, local(10, 5, 0), time.Time{}, 1, workOnly}, {1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name:    "its side requests alone come, its main in flight, going, on its main's account",
			lines:   []ledger.Line{first, title},
			told:    []views.Told{toldOf("r2", p2, ledger.ClassMain, opus, "side", local(10, 5, 0)), endedAt(local(10, 6, 1), toldOf("r3", p2, "auxiliary", haiku, "work", local(10, 5, 1)))},
			running: true,
			want:    []turnSaid{{2, local(10, 5, 0), time.Time{}, 2, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name:    "its side requests alone come, the stream unheard, as the verbs read it: ended with them, on theirs",
			lines:   []ledger.Line{first, title},
			running: true,
			want:    []turnSaid{{2, local(10, 5, 1), local(10, 6, 1), 1, workOnly}, {1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name:  "its line come, counted once, by its line",
			lines: []ledger.Line{first},
			told:  []views.Told{endedAt(local(10, 1, 0), toldOf("r1", p1, ledger.ClassMain, opus, "side", local(10, 0, 0)))},
			want:  []turnSaid{{1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
		{
			name:  "ended, its line yet to come, going till it does, its session ended or not",
			lines: []ledger.Line{calling},
			told:  []views.Told{endedAt(local(10, 2, 30), toldOf("r2", p1, ledger.ClassMain, opus, "side", local(10, 2, 0)))},
			want:  []turnSaid{{1, local(10, 0, 0), time.Time{}, 2, sideOnly}},
		},
		{
			name:    "lost with no line, counted, ended as the stream last told of it, its answer before calling a tool",
			lines:   []ledger.Line{calling},
			told:    []views.Told{lostAt(local(10, 2, 20), toldOf("r2", p1, ledger.ClassMain, opus, "work", local(10, 2, 0)))},
			running: true,
			want:    []turnSaid{{1, local(10, 0, 0), local(10, 2, 20), 2, []string{"side", "work"}}},
		},
		{
			name:  "a subagent's in flight counting in it without keeping it going",
			lines: []ledger.Line{first},
			told:  []views.Told{toldOf("r2", p1, "subagent", haiku, "work", local(10, 0, 30))},
			want:  []turnSaid{{1, local(10, 0, 0), local(10, 1, 0), 2, sideOnly}},
		},
		{
			name:  "a subagent's in flight counting in the turn of the prompt its first request named, whatever prompt it names",
			lines: []ledger.Line{calling, subagent("a1", "", p1, local(10, 1, 0)), mainOn(p2, local(10, 5, 0), "end_turn")},
			told:  []views.Told{{Request: "r9", Session: paged, Model: haiku, Account: "work", Prompt: p2, Class: "subagent", AgentID: "a1", Sent: local(10, 6, 0)}},
			want:  []turnSaid{{2, local(10, 5, 0), local(10, 6, 0), 1, sideOnly}, {1, local(10, 0, 0), local(10, 1, 0), 3, sideOnly}},
		},
		{
			name:    "of side requests alone, going while one is in flight, following them",
			told:    []views.Told{toldOf("r2", p1, "auxiliary", haiku, "work", local(10, 0, 0))},
			running: true,
			want:    []turnSaid{{1, local(10, 0, 0), time.Time{}, 1, workOnly}},
		},
		{
			name:  "none of a quota check, a count of tokens, another session's, or one that names no prompt",
			lines: []ledger.Line{first},
			told:  []views.Told{check, count, others, toldOf("r2", "", ledger.ClassMain, opus, "side", local(10, 2, 0))},
			want:  []turnSaid{{1, local(10, 0, 0), local(10, 1, 0), 1, sideOnly}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var routed *status.Session
			if tt.running {
				routed = runningOn("side")
			}
			for i := range tt.want {
				tt.want[i].started, tt.want[i].ended = utc(tt.want[i].started), zeroOrUTC(tt.want[i].ended)
			}
			if got := saidOf(pageHearing(t, tt.lines, routed, heard(tt.told...)).Turns); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("the turns are\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestEachSessionsTurnsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			for _, id := range slices.Sorted(maps.Keys(hm.Want)) {
				page := momentsPage(t, hm, id)
				got := make([]viewstest.Turn, len(page.Turns))
				for i, turn := range page.Turns {
					got[i] = viewstest.Turn{Turn: turn.Turn, Started: turn.Started, Ended: turn.Ended, Requests: turn.Requests, Accounts: turn.Accounts}
				}
				want := make([]viewstest.Turn, len(hm.Turns[id]))
				for i, turn := range hm.Turns[id] {
					turn.Started, turn.Ended = utc(turn.Started), zeroOrUTC(turn.Ended)
					want[i] = turn
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("%s's turns are\n%+v\nwant\n%+v", id[:4], got, want)
				}
			}
		})
	}
}

// momentsPage is the page of the session with the given id at the moment hm,
// as the router lists it, where it does, and its lines and the request stream
// tell.
func momentsPage(t *testing.T, hm viewstest.SessionMoment, id string) views.SessionPage {
	t.Helper()
	var routed *status.Session
	if i := slices.IndexFunc(hm.Sessions, func(s status.Session) bool { return s.ID == id }); i >= 0 {
		routed = &hm.Sessions[i]
	}
	read := views.ReadSessions(fakeLedger{lines: hm.Lines, now: hm.Now}, hm.Sessions, hm.Now, id)
	page, ok := views.SessionPageOf(id, views.PageSources{
		Routed: routed, Unlisted: hm.Unlisted, Read: read, Stream: hm.Stream, Prices: ledger.Pricing, Now: hm.Now,
	})
	if !ok {
		t.Fatalf("SessionPageOf(%s) found no session", id[:4])
	}
	return page
}

// pageHearing builds paged's page as pageOf does, the request stream telling
// what s told.
func pageHearing(t *testing.T, lines []ledger.Line, routed *status.Session, s views.Stream) views.SessionPage {
	t.Helper()
	page, ok := views.SessionPageOf(paged, views.PageSources{Routed: routed, Read: readOf(summarisedLedger(lines), routed), Stream: s, Prices: ledger.Pricing,
		Keep: 90 * 24 * time.Hour, Now: now})
	if !ok {
		t.Fatalf("SessionPageOf(%s) found no session in %+v", paged, lines)
	}
	return page
}
