package views_test

import (
	"cmp"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// liveOf is what the look gives the List beside its lines.
func liveOf(look viewstest.SessionsLook) views.ListLive {
	return views.ListLive{Running: look.Sessions, Unlisted: look.Unlisted, Stream: look.Stream, Now: look.Now}
}

// walked is a look as the watch takes it, after the looks before it: its
// moment; the List held, as its first look read it, fed since as the
// ledger's tail grows; the sessions read whole at the look; its calls; and
// the board kept.
type walked struct {
	look  viewstest.SessionsLook
	m     status.Moment
	held  views.HeldList
	found []string
	calls []views.Call
	board views.Patched
}

// walk takes the moment's looks in turn, as the watch does: the List held as
// its first look reads the ledger, then fed what the ledger gained by each
// look, each day's lines taken at its date, today's last, as the tail gives
// them, and the sessions it lists but hasn't read whole read so; the calls
// built of the List held; and the board patched from the last look's.
func walk(t *testing.T, sm viewstest.SessionsMoment) []walked {
	t.Helper()
	first := sm.Looks[0]
	held := views.HoldList(views.ReadList(sessionsLedger(first.Lines, first.Now), first.Sessions), ledger.Pricing, first.Now)
	var board views.Patched
	var walks []walked
	taken := first.Lines
	for _, look := range sm.Looks {
		if len(look.Lines) < len(taken) || !reflect.DeepEqual(look.Lines[:len(taken)], taken) {
			t.Fatalf("a look's lines lose some of the last look's: the ledger only grows")
		}
		held = tailed(held, look.Lines[len(taken):], look.Now)
		taken = look.Lines
		found := held.Unfound(liveOf(look))
		if len(found) > 0 {
			held = held.Found(views.ReadListOf(sessionsLedger(look.Lines, look.Now), found...))
		}
		m := look.Doc.At(look.Now, claude.Policy)
		calls := views.CallsOf(m, look.Sessions, look.Stream, held)
		board = views.Patch(board, calls, views.StubsOf(m))
		walks = append(walks, walked{look: look, m: m, held: held, found: found, calls: calls, board: board})
	}
	return walks
}

// tailed is held fed the lines gained, as a read of the ledger's tail at
// now feeds it: those of each day before now's at its date, oldest first,
// then today's, at today's, which rolls it on to today.
func tailed(held views.HeldList, gained []ledger.Line, now time.Time) views.HeldList {
	today := views.Today(now).Date
	byDate := make(map[string][]ledger.Held)
	for _, l := range gained {
		date := dayfile.DateOf(l.At)
		byDate[date] = append(byDate[date], ledger.Held{Line: l})
	}
	for _, date := range slices.Sorted(maps.Keys(byDate)) {
		if date < today {
			held = held.Take(date, byDate[date])
		}
	}
	return held.Take(today, byDate[today])
}

// boardSaid says an account's line as SessionsLook's Board says it.
func boardSaid(l views.PatchedLine) string {
	var said []string
	for _, c := range l.Calls {
		said = append(said, status.ShownID(c.Session)+"@"+strconv.Itoa(c.Jack))
	}
	for _, jack := range l.Stubs {
		said = append(said, "╌"+strconv.Itoa(jack))
	}
	return strings.Join(said, " ")
}

// callSaid says a call at now as SessionsLook's Calls say it.
func callSaid(c views.Call, now time.Time) string {
	said := c.Activity.Said(now)
	if c.From != "" {
		said += " ▸ from " + c.From
	}
	works := "rests"
	if c.Activity.Works() {
		works = "works"
	}
	return said + " · " + cmp.Or(c.Dir, "no dir") + " · " + works
}

// callsSaid say each call at now, by its session's shown id and its
// account.
func callsSaid(calls []views.Call, now time.Time) map[string]string {
	said := make(map[string]string, len(calls))
	for _, c := range calls {
		said[status.ShownID(c.Session)+" "+c.Account] = callSaid(c, now)
	}
	return said
}

// listSaid says the List's sessions at now as SessionsLook's List says
// them.
func listSaid(list views.SessionList, now time.Time) []string {
	said := []string{}
	for _, s := range list.Sessions {
		row := status.ShownID(s.Session) + " " + s.Activity.State(now)
		if !s.Started.IsZero() {
			row += " · started " + prose.Past(now, s.Started)
		}
		said = append(said, row+" · "+strconv.Itoa(s.Requests)+" requests")
	}
	return said
}

// todaySaid says the List's today line as SessionsLook's Today says it.
func todaySaid(today views.SessionsToday) string {
	return fmt.Sprintf("%d sessions · %d requests", today.Sessions, today.Requests)
}

// eachLook runs check at each look of each of Sessions' hard moments, as
// walk takes them, a subtest a look.
func eachLook(t *testing.T, check func(t *testing.T, w walked)) {
	for _, sm := range viewstest.SessionsMoments() {
		t.Run(sm.Name, func(t *testing.T) {
			for i, w := range walk(t, sm) {
				t.Run(fmt.Sprintf("look %d at %s", i+1, w.look.Now.Format(time.TimeOnly)), func(t *testing.T) {
					check(t, w)
				})
			}
		})
	}
}

func TestTheBoardAtEachOfSessionsHardMoments(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		for _, a := range w.look.Doc.Accounts {
			if got, want := boardSaid(w.board.Line(a.ID)), w.look.Board[a.ID]; got != want {
				t.Errorf("%s's line is %q, want %q", a.ID, got, want)
			}
		}
	})
}

func TestTheCallsAtEachOfSessionsHardMoments(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		if got, want := callsSaid(w.calls, w.look.Now), w.look.Calls; !maps.Equal(got, want) {
			t.Errorf("the calls say\n%s\nwant\n%s", byKey(got), byKey(want))
		}
	})
}

// byKey says what each key says, a line each, in the order of the keys.
func byKey(said map[string]string) string {
	var lines []string
	for _, key := range slices.Sorted(maps.Keys(said)) {
		lines = append(lines, key+": "+said[key])
	}
	return strings.Join(lines, "\n")
}

func TestTheListAtEachOfSessionsHardMoments(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		list := w.held.List(liveOf(w.look))
		if w.look.List != nil {
			if got := listSaid(list, w.look.Now); !slices.Equal(got, w.look.List) {
				t.Errorf("the List is\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(w.look.List, "\n"))
			}
		}
		if w.look.Today != "" {
			if got := todaySaid(list.Today); got != w.look.Today {
				t.Errorf("today is %q, want %q", got, w.look.Today)
			}
		}
	})
}

// The List held, fed each look's gain as the tail gives it, is the List
// sessions builds afresh of the same lines at every look: midnight, a
// session started yesterday with no line today, and probing among them.
func TestTheListHeldIsTheListBuiltAfreshOfTheSameLinesAtEveryLook(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		got := w.held.List(liveOf(w.look))
		want := views.ListSessions(views.SessionSources{
			Running: w.look.Sessions, Unlisted: w.look.Unlisted, Stream: w.look.Stream,
			Ledger: sessionsLedger(w.look.Lines, w.look.Now), Prices: ledger.Pricing, Now: w.look.Now,
		})
		assertList(t, got, want)
	})
}

// The calls are RunningOf's rows, each session's on each account its models
// go to, in its order, so SESSIONS, Detail and the Switchboard never
// disagree; and each line holds a call for each row ROUTING counts.
func TestTheCallsAreRunningOfsRowsAndTheBoardROUTINGsCounts(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		running := views.RunningOf(w.m, w.look.Sessions, w.look.Stream)
		var rows []views.Running
		for _, c := range w.calls {
			rows = append(rows, c.Running)
		}
		if !reflect.DeepEqual(rows, running) {
			t.Errorf("the calls' rows are\n%+v\nwant RunningOf's\n%+v", rows, running)
		}
		counts := views.RunningCounts(running)
		for _, a := range w.look.Doc.Accounts {
			if got := len(w.board.Line(a.ID).Calls); got != counts[a.ID] {
				t.Errorf("%s's line holds %d calls, want ROUTING's count, %d", a.ID, got, counts[a.ID])
			}
		}
	})
}

// The List's running are the sessions the calls name, each once; and of a
// session on one account, the List and its call agree: the account it's
// on, what it's doing, and the move that brought it there, where the List's
// lines tell of it, or else the stream alone told of it, its line yet to
// come.
func TestTheListAndTheCallsAgreeOnTheSessionsRunning(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		list := w.held.List(liveOf(w.look))
		var running []string
		for _, s := range list.Sessions {
			if s.Running {
				running = append(running, s.Session)
			}
		}
		named := make(map[string][]views.Call)
		for _, c := range w.calls {
			named[c.Session] = append(named[c.Session], c)
		}
		if got := slices.Sorted(maps.Keys(named)); !slices.Equal(slices.Sorted(slices.Values(running)), got) {
			t.Errorf("the List's running are %v, want the sessions the calls name, %v", running, got)
		}
		for _, s := range list.Sessions {
			calls := named[s.Session]
			if len(calls) != 1 {
				continue
			}
			c, shown := calls[0], status.ShownID(s.Session)
			if s.Account != c.Account || s.Activity.Doing != c.Activity.Doing {
				t.Errorf("%s is on %s doing %d on the List, want it on %s doing %d, as its call", shown, s.Account, s.Activity.Doing, c.Account, c.Activity.Doing)
			}
			switch {
			case s.Moved != nil && c.From != s.Moved.From:
				t.Errorf("%s's call came from %q, want %q, as its routing on the List", shown, c.From, s.Moved.From)
			case s.Moved == nil && c.From != "" && !toldMoved(w.look.Stream, s.Session, c.From, c.Account):
				t.Errorf("%s's call came from %q, but neither its lines nor the stream tell of the move", shown, c.From)
			}
		}
	})
}

// Each session the List lists runs on its page as it does on the List, and
// as the Switchboard has a call of it, and is doing what the List says,
// whether the router lists it, or, while it doesn't answer, its last
// listing did, however long ago.
func TestASessionsPageRunsAsTheListAndTheSwitchboardHaveIt(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		for _, s := range w.held.List(liveOf(w.look)).Sessions {
			read := views.ReadSessions(sessionsLedger(w.look.Lines, w.look.Now), w.look.Sessions, w.look.Now, s.Session)
			live := views.PageLive{Routed: views.Routed(w.look.Sessions, s.Session), Unlisted: w.look.Unlisted, Stream: w.look.Stream, Now: w.look.Now}
			page, ok := views.HoldPage(s.Session, read, ledger.Pricing, 0, w.look.Now).Page(live)
			called := slices.ContainsFunc(w.calls, func(c views.Call) bool { return c.Session == s.Session })
			if !ok || page.Running != s.Running || page.Running != called || page.Activity.Doing != s.Activity.Doing {
				t.Errorf("%s's page (%t) runs %t, doing %d; want it running %t, as the List has it, and %t, as the Switchboard calls it, doing %d",
					status.ShownID(s.Session), ok, page.Running, page.Activity.Doing, s.Running, called, s.Activity.Doing)
			}
		}
	})
}

// A session running by its listing names every model the listing gives it,
// cold ones too, on the List and its page, as the router lists a session:
// whether the router answers, or the listing is the last it gave.
func TestARunningSessionKeepsEveryModelItsListingGives(t *testing.T) {
	modelsOf := func(models []views.SessionModel) []string {
		var named []string
		for _, m := range models {
			named = append(named, m.Model)
		}
		return slices.Sorted(slices.Values(named))
	}
	eachLook(t, func(t *testing.T, w walked) {
		for _, s := range w.held.List(liveOf(w.look)).Sessions {
			routed := views.Routed(w.look.Sessions, s.Session)
			if !s.Running || routed == nil {
				continue
			}
			var want []string
			for _, a := range routed.Assignments {
				want = append(want, a.Model)
			}
			slices.Sort(want)
			read := views.ReadSessions(sessionsLedger(w.look.Lines, w.look.Now), w.look.Sessions, w.look.Now, s.Session)
			live := views.PageLive{Routed: routed, Unlisted: w.look.Unlisted, Stream: w.look.Stream, Now: w.look.Now}
			page, _ := views.HoldPage(s.Session, read, ledger.Pricing, 0, w.look.Now).Page(live)
			if got, goes := modelsOf(s.Models), modelsOf(page.Goes); !slices.Equal(got, want) || !slices.Equal(goes, want) {
				t.Errorf("%s names the models %v on the List and %v on its page, want %v, every one its listing gives", status.ShownID(s.Session), got, goes, want)
			}
		}
	})
}

// toldMoved reports whether the stream s told of the session with the
// given id moving from one account to another.
func toldMoved(s views.Stream, session, from, to string) bool {
	return slices.ContainsFunc(s.Moves, func(m views.Moved) bool { return m.Session == session && m.From == from && m.To == to })
}

// The Log's panel lists today's sessions as the List does: each it lists is
// the List's, and the List's others run with no request today.
func TestTheLogsSessionsAreTheListsOfToday(t *testing.T) {
	eachLook(t, func(t *testing.T, w walked) {
		l := sessionsLedger(w.look.Lines, w.look.Now)
		today := views.Today(w.look.Now)
		var todays []ledger.Held
		for _, line := range w.look.Lines {
			if today.Holds(line.At) {
				todays = append(todays, ledger.Held{Line: line})
			}
		}
		read := views.ReadLogHeld(l, todays, ledger.Pricing, w.look.Now)
		counts := views.LogRows(views.LogSources{From: w.look.Now, Read: read, Stream: w.look.Stream, Now: w.look.Now}).Count(today.Date)
		logged := views.LogSessions(views.LogSessionSources{
			Read: read, Lines: counts.Sessions, Running: w.look.Sessions, Unlisted: w.look.Unlisted, Stream: w.look.Stream, Now: w.look.Now,
		})
		listed := make(map[string]views.ListedSession)
		for _, s := range w.held.List(liveOf(w.look)).Sessions {
			listed[s.Session] = s
		}
		for _, s := range logged {
			if _, ok := listed[s.ID]; !ok {
				t.Errorf("the Log lists %s today, the List doesn't", status.ShownID(s.ID))
			}
		}
		for id, s := range listed {
			if !slices.ContainsFunc(logged, func(l views.LogSession) bool { return l.ID == id }) && (!s.Running || s.Requests > 0) {
				t.Errorf("the List lists %s, running %t with %d requests today, the Log doesn't", status.ShownID(id), s.Running, s.Requests)
			}
		}
	})
}

// Each session is read whole once, as the List first lists it unread, its
// start found then, and never again; and once read, none the List lists is
// left unread.
func TestEachSessionTheListHoldsIsReadWholeOnce(t *testing.T) {
	for _, sm := range viewstest.SessionsMoments() {
		t.Run(sm.Name, func(t *testing.T) {
			read := make(map[string]int)
			for i, w := range walk(t, sm) {
				for _, id := range w.found {
					read[id]++
				}
				if left := w.held.Unfound(liveOf(w.look)); len(left) > 0 {
					t.Errorf("at look %d, %v are left unread once read", i+1, left)
				}
			}
			for id, n := range read {
				if n > 1 {
					t.Errorf("%s was read whole %d times, want once", status.ShownID(id), n)
				}
			}
		})
	}
}
