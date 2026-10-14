package views_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// named is the account and model naming a session.
type named struct{ account, model string }

// A session is named by its main thread: the model of its latest main
// request, as its lines and the request stream tell, and the account that
// model goes to now, as the router lists it and the stream has moved it
// since, else the one that request went to; where none of its requests is
// its main thread's, by the model the router saw last, and its account,
// else by its latest request. The List and a session's page name it alike.
func TestASessionIsNamedByItsMainThread(t *testing.T) {
	conversation := func(account string, at time.Time) ledger.Line {
		return serving(p1, ledger.ClassMain, line(paged, opus, account, "sticky", at, hourUsage))
	}
	title := func(account string, at time.Time) ledger.Line {
		return serving(p1, "auxiliary", line(paged, haiku, account, "sticky", at, smallUsage))
	}
	listing := func(models ...status.Assignment) *status.Session {
		return &status.Session{ID: paged, Assignments: models}
	}
	goes := func(model, account string, seen time.Duration) status.Assignment {
		return status.Assignment{Model: model, Account: account, Reason: "sticky", LastSeen: now.Add(-seen)}
	}
	moved := views.Stream{Live: true, Moves: []views.Moved{{Session: paged, Model: opus, From: "work", To: "personal", Reason: "moved: work hit its limit", At: now}}}
	tests := []struct {
		name   string
		lines  []ledger.Line
		routed *status.Session
		stream views.Stream
		want   named
	}{
		{
			name:   "running, its main thread's account as the router routes it, a side request on another account seen since",
			lines:  []ledger.Line{conversation("work", local(12, 0, 0)), title("side", local(12, 1, 0))},
			routed: listing(goes(haiku, "side", time.Minute), goes(opus, "work", 2*time.Minute)),
			want:   named{"work", opus},
		},
		{
			name:  "ended, its latest main request's account, a side request on another after it",
			lines: []ledger.Line{conversation("work", local(12, 0, 0)), title("side", local(12, 1, 0))},
			want:  named{"work", opus},
		},
		{
			name:   "moved, with no request since, where the router has its model go now",
			lines:  []ledger.Line{conversation("work", local(12, 0, 0))},
			routed: listing(goes(opus, "side", time.Hour)),
			want:   named{"side", opus},
		},
		{
			name:   "moved as the stream told, before the router lists it moved",
			lines:  []ledger.Line{conversation("work", local(12, 0, 0))},
			routed: listing(goes(opus, "work", time.Minute)),
			stream: moved,
			want:   named{"personal", opus},
		},
		{
			name:   "its latest main request's account, where the router has its model go nowhere",
			lines:  []ledger.Line{conversation("work", local(12, 0, 0)), title("side", local(12, 1, 0))},
			routed: listing(goes(haiku, "side", time.Minute)),
			want:   named{"work", opus},
		},
		{
			name:   "its main request in flight, newer than its lines, of another model",
			lines:  []ledger.Line{conversation("work", local(12, 0, 0))},
			routed: listing(goes("claude-sonnet-5-5", "side", 0), goes(opus, "work", time.Hour)),
			stream: heard(toldOf("r9", p2, ledger.ClassMain, "claude-sonnet-5-5", "side", now)),
			want:   named{"side", "claude-sonnet-5-5"},
		},
		{
			name:   "its first main request in flight, its title alone read",
			lines:  []ledger.Line{title("side", local(13, 11, 0))},
			routed: listing(goes(haiku, "side", 0), goes(opus, "work", time.Minute)),
			stream: heard(toldOf("r9", p1, ledger.ClassMain, opus, "work", local(13, 11, 0))),
			want:   named{"work", opus},
		},
		{
			name:   "its title alone read, the stream unheard, as the verbs read it: the model the router saw last",
			lines:  []ledger.Line{title("side", local(13, 11, 0))},
			routed: listing(goes(opus, "work", time.Minute), goes(haiku, "side", 0)),
			want:   named{"side", haiku},
		},
		{
			name:   "running, its lines from before requests said their class: the model the router saw last",
			lines:  []ledger.Line{line(paged, haiku, "side", "sticky", local(12, 1, 0), smallUsage), line(paged, opus, "work", "sticky", local(12, 2, 0), hourUsage)},
			routed: listing(goes(haiku, "side", time.Minute), goes(opus, "work", 2*time.Minute)),
			want:   named{"side", haiku},
		},
		{
			name:  "ended, its lines from before requests said their class: its latest request's",
			lines: []ledger.Line{line(paged, opus, "work", "sticky", local(12, 1, 0), hourUsage), line(paged, haiku, "side", "sticky", local(12, 2, 0), smallUsage)},
			want:  named{"side", haiku},
		},
		{
			name:   "running with no request read or told of: the model the router saw last",
			routed: listing(goes(opus, "work", time.Minute), goes(haiku, "side", 0)),
			want:   named{"side", haiku},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var running []status.Session
			if tt.routed != nil {
				running = []status.Session{*tt.routed}
			}
			list := views.ListSessions(views.SessionSources{Running: running, Stream: tt.stream, Ledger: fakeLedger{now: now, lines: tt.lines}, Prices: ledger.Pricing, Now: now})
			if len(list.Sessions) != 1 || (named{list.Sessions[0].Account, list.Sessions[0].Model}) != tt.want {
				t.Errorf("the List holds %+v, want one session, named %+v", list.Sessions, tt.want)
			}
			if page := pageHearing(t, tt.lines, tt.routed, tt.stream); (named{page.Account, page.Model}) != tt.want {
				t.Errorf("its page names it %s, %s, want %+v", page.Account, page.Model, tt.want)
			}
		})
	}
}

// At each moment the views have to get right, the List, a session's page
// and the Log's panel name each session alike, as the moment has it; and on
// the account the List names it on, where it runs there, cold as its main
// thread may be there, SESSIONS and Detail name it by the model the List
// names, and its family, and say why that model is there, as the List gives
// its reason and since when, as its call on the Switchboard does, come from
// where the List has it come from.
func TestTheListAPageAndSESSIONSNameEachSessionAlikeAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			list := views.ListSessions(views.SessionSources{
				Running: hm.Sessions, Unlisted: hm.Unlisted, Stream: hm.Stream, Ledger: fakeLedger{lines: hm.Lines, now: hm.Now}, Now: hm.Now,
			})
			listed := make(map[string]views.ListedSession)
			for _, s := range list.Sessions {
				listed[s.Session] = s
			}
			panel := make(map[string]views.LogSession)
			for _, s := range logPanelOf(hm.Lines, hm.Sessions, hm.Unlisted, hm.Stream, hm.Now) {
				panel[s.ID] = s
			}
			m := hm.Doc.At(hm.Now, claude.Policy)
			held := listHeldOf(hm.Lines, hm.Sessions, hm.Now)
			rows, calls := views.RunningOf(m, hm.Sessions, hm.Stream, held), views.CallsOf(m, hm.Sessions, hm.Stream, held)
			for _, id := range slices.Sorted(maps.Keys(hm.Want)) {
				want := named{hm.Named[id].Account, hm.Named[id].Model}
				s := listed[id]
				if got := (named{s.Account, s.Model}); got != want {
					t.Errorf("the List names %s %+v, want %+v", id[:4], got, want)
				}
				if page := momentsPage(t, hm, id); (named{page.Account, page.Model}) != want {
					t.Errorf("its page names %s %s, %s, want %+v", id[:4], page.Account, page.Model, want)
				}
				if logged, ok := panel[id]; ok && logged.Account != want.account {
					t.Errorf("the Log's panel has %s on %s, want %s", id[:4], logged.Account, want.account)
				}
				detail := views.RunningOn(rows, want.account)
				i := slices.IndexFunc(detail, func(r views.Running) bool { return r.Session == id })
				j := slices.IndexFunc(calls, func(c views.Call) bool { return c.Session == id && c.Account == want.account })
				if i < 0 || j < 0 {
					continue
				}
				checkRowNamed(t, m, s, detail[i], calls[j], hm.Stream)
			}
		})
	}
}

// checkRowNamed checks the session s, as the List names it, is named alike
// at the moment m by r, its row on that account, as SESSIONS and Detail list
// it, and by c, its call there, as the Switchboard draws it, the stream
// given telling what it told: by its model and family; why that model is
// there, as the List gives its reason and since when; and the account it
// came from, as the List has it, else none, or one the stream alone told of.
func checkRowNamed(t *testing.T, m status.Moment, s views.ListedSession, r views.Running, c views.Call, stream views.Stream) {
	t.Helper()
	id := s.Session[:4]
	if !reflect.DeepEqual(c.Running, r) {
		t.Errorf("%s's call on %s is\n%+v\nwant SESSIONS' row\n%+v", id, r.Account, c.Running, r)
	}
	if family := (claude.Provider{}).Family(s.Model); r.Model != s.Model || r.Family != family {
		t.Errorf("SESSIONS names %s on %s by %s, %s; want %s, its family %s, as the List", id, r.Account, r.Model, r.Family, s.Model, family)
	}
	if j := slices.IndexFunc(s.Models, func(sm views.SessionModel) bool { return sm.Model == s.Model && sm.Account == s.Account }); j >= 0 {
		if why := views.WhyRunning(m, s.Session, s.Models[j]); r.Why.String() != why.String() {
			t.Errorf("SESSIONS says %s is on %s as %q, want %q, as the List gives %s's reason and since when", id, r.Account, r.Why, why, s.Model)
		}
	}
	switch {
	case s.Moved != nil && c.From != s.Moved.From:
		t.Errorf("%s's call came from %q, want %q, as the List has it", id, c.From, s.Moved.From)
	case s.Moved == nil && c.From != "" && !toldMoved(stream, s.Session, c.From, c.Account):
		t.Errorf("%s's call came from %q, but neither its lines nor the stream tell of the move", id, c.From)
	}
}
