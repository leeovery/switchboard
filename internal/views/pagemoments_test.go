package views_test

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// momentsHeld is the page of the session with the given id at the moment hm
// held, as a session's page holds it, and what's live then: the router
// listing it, where it does, and the request stream telling what it told.
func momentsHeld(hm viewstest.SessionMoment, id string) (views.HeldPage, views.PageLive) {
	var routed *status.Session
	if i := slices.IndexFunc(hm.Sessions, func(s status.Session) bool { return s.ID == id }); i >= 0 {
		routed = &hm.Sessions[i]
	}
	read := views.ReadSessions(fakeLedger{lines: hm.Lines, now: hm.Now}, hm.Sessions, hm.Now, id)
	return views.HoldPage(id, read, ledger.Pricing, 0, hm.Now), views.PageLive{Routed: routed, Unlisted: hm.Unlisted, Stream: hm.Stream, Now: hm.Now}
}

// factsOf are the facts of a session's page p, and its story s, that the
// moments hold.
func factsOf(p views.SessionPage, s views.Story) viewstest.PageFacts {
	facts := viewstest.PageFacts{Runs: p.Runs(), Going: len(p.Turns) > 0 && p.Turns[0].Ended.IsZero(), Waiting: p.Activity.Waiting(), Marks: len(s.Moves)}
	for _, m := range p.Goes {
		facts.Goes = append(facts.Goes, viewstest.Named{Account: m.Account, Model: m.Model})
	}
	for _, a := range p.Accounts {
		facts.Accounts = append(facts.Accounts, a.Account)
	}
	for _, st := range s.Stretches {
		facts.Ribbon = append(facts.Ribbon, st.Account)
	}
	return facts
}

// At each hard moment, each session's page tells whether it runs, where its
// models go, whether its newest turn goes on, whether it waits for an
// answer, the accounts of its rows, and its story's ribbon and marks, as
// the moment has them.
func TestWhatEachSessionsPageTellsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			if !slices.Equal(slices.Sorted(maps.Keys(hm.Page)), slices.Sorted(maps.Keys(hm.Want))) {
				t.Fatalf("the moment holds the pages of %v, want those of each session in it", slices.Sorted(maps.Keys(hm.Page)))
			}
			for _, id := range slices.Sorted(maps.Keys(hm.Page)) {
				held, live := momentsHeld(hm, id)
				page, ok := held.Page(live)
				if !ok {
					t.Fatalf("%s has no page", id[:4])
				}
				if got := factsOf(page, held.Story(page, live)); !reflect.DeepEqual(got, hm.Page[id]) {
					t.Errorf("%s's page tells\n%+v\nwant\n%+v", id[:4], got, hm.Page[id])
				}
			}
		})
	}
}

// shortened is what a session's doing, as its page says it on row 6, reads
// as in SESSIONS' few cells, by rule 1's mapping: asking, throttled and
// refused alike as a request still waiting, answering by its arrow, and
// idle by its word.
func shortened(said string) string {
	switch {
	case strings.HasPrefix(said, "↑ asking"), strings.HasPrefix(said, "… "), strings.HasPrefix(said, "✕ "):
		return "↑ ask"
	case strings.HasPrefix(said, "↓ answering"):
		return "↓"
	case strings.HasPrefix(said, "idle"):
		return "idle"
	}
	return said
}

// At each hard moment, each session's page and the List agree: its account
// and model, what it's doing, when it started and ended, and whether a move
// brought it where it is; SESSIONS shortens what the page says it's doing
// on the account the page names, by rule 1's mapping; its newest turn goes
// on while it waits for an answer, and while it runs, its last answer of its
// own conversation having called a tool; its accounts' requests, with those
// the router answered itself, are its requests, as TURNS' heading counts
// them; and its story's last 30 minutes are the session's trace, which
// leaves out those the router answered itself.
func TestASessionsPageAgreesWithTheListSessionsAndTheTracesAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			list := views.ListSessions(views.SessionSources{
				Running: hm.Sessions, Unlisted: hm.Unlisted, Stream: hm.Stream,
				Ledger: fakeLedger{lines: hm.Lines, now: hm.Now}, Prices: ledger.Pricing, Now: hm.Now,
			})
			rows := views.RunningOf(hm.Doc.At(hm.Now, claude.Policy), hm.Sessions, hm.Stream)
			for _, listed := range list.Sessions {
				held, live := momentsHeld(hm, listed.Session)
				page, _ := held.Page(live)
				id := listed.Session[:4]
				_, moved := page.MovedOnto()
				if page.Account != listed.Account || page.Model != listed.Model || said(page.Activity) != said(listed.Activity) ||
					!page.Started.Equal(listed.Started) || !page.Ended.Equal(listed.Ended) || moved != (listed.Moved != nil) {
					t.Errorf("%s's page is on %s, %s, %s, started %v, ended %v, moved %t; the List's on %s, %s, %s, started %v, ended %v, moved %t",
						id, page.Account, page.Model, said(page.Activity), page.Started, page.Ended, moved,
						listed.Account, listed.Model, said(listed.Activity), listed.Started, listed.Ended, listed.Moved != nil)
				}
				if i := slices.IndexFunc(rows, func(r views.Running) bool { return r.Session == listed.Session && r.Account == page.Account }); i >= 0 {
					if got, want := rows[i].Activity.Short(), shortened(page.Activity.Said(hm.Now)); !strings.HasPrefix(got, want) {
						t.Errorf("SESSIONS says %s is %q on %s, its page %q", id, got, page.Account, page.Activity.Said(hm.Now))
					}
				}
				checkTurnGoes(t, hm, page)
				sum := 0
				for _, a := range page.Accounts {
					sum += a.Requests
				}
				if unsent := routerAnswered(hm, listed.Session); sum+unsent != page.Totals.Requests {
					t.Errorf("%s's accounts' requests sum to %d, with %d the router answered itself, its totals %d", id, sum, unsent, page.Totals.Requests)
				}
				checkStoryTraces(t, hm, listed.Session, held.Story(page, live))
			}
		})
	}
}

// routerAnswered counts the requests of the session with the given id at
// the moment hm that the router answered itself, sending them to no account.
func routerAnswered(hm viewstest.SessionMoment, id string) int {
	n := 0
	for _, l := range hm.Lines {
		if l.Session == id && l.Counted() && l.Account == "" {
			n++
		}
	}
	return n
}

// checkTurnGoes checks the newest turn of page, a session's at the moment
// hm, goes on while it waits for an answer, and while it runs, its last line
// of its own conversation having called a tool.
func checkTurnGoes(t *testing.T, hm viewstest.SessionMoment, page views.SessionPage) {
	t.Helper()
	if len(page.Turns) == 0 {
		return
	}
	going := page.Turns[0].Ended.IsZero()
	if page.Activity.Waiting() && !going {
		t.Errorf("%s waits for an answer, its newest turn over", page.Session[:4])
	}
	var main *ledger.Line
	for i := range hm.Lines {
		if l := &hm.Lines[i]; l.Session == page.Session && l.Class == ledger.ClassMain && (main == nil || l.At.After(main.At)) {
			main = l
		}
	}
	if page.Runs() && main != nil && main.Answer.Stop == "tool_use" && !going {
		t.Errorf("%s runs, its last answer of its own conversation calling a tool, its newest turn over", page.Session[:4])
	}
}

// checkStoryTraces checks the story s of the session with the given id at
// the moment hm counts, over its last 30 minutes, what the session's trace
// counts of its lines and what the stream told.
func checkStoryTraces(t *testing.T, hm viewstest.SessionMoment, id string, s views.Story) {
	t.Helper()
	held := make([]ledger.Held, len(hm.Lines))
	for i, l := range hm.Lines {
		held[i] = ledger.Held{Line: l}
	}
	var told []views.Told
	for _, r := range hm.Stream.Requests {
		told = append(told, r)
	}
	slices.SortFunc(told, func(a, b views.Told) int { return a.Sent.Compare(b.Sent) })
	trace := views.Traces{}.Read(held, hm.Now).Told(told, hm.Now).Sessions[id]
	from := hm.Now.Add(-views.TraceSpan - time.Minute)
	recent := slices.DeleteFunc(slices.Clone(s.Sent), func(at time.Time) bool { return at.Before(from) })
	if !slices.EqualFunc(recent, trace.Sent, time.Time.Equal) {
		t.Errorf("%s's story counts %v over its last 30 minutes, its trace %v", id[:4], recent, trace.Sent)
	}
}
