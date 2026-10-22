package dashboard

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// At each moment the views have to tell what a session is doing at, SESSIONS
// draws each session running on each account as the views have it doing
// there: when it was last seen, what it's doing, and its id bright while
// it's busy; and none the views don't have running. ROUTING counts those it
// lists on each account.
func TestSESSIONSSaysWhatEachSessionIsDoingAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			f := frameOf(160, 40)
			f.Sessions, f.Stream = hm.Sessions, hm.Stream
			m := f.Moment(hm.Doc, hm.Now)
			running := views.BusiestFirst(m, f.running(hm.Doc, hm.Now), f.Traces)
			laid := f.runningForm(f.routingColumns(m)).laid()
			want := 0
			for _, doing := range hm.Want {
				want += len(doing.On)
			}
			if len(running) != want {
				t.Fatalf("SESSIONS lists %d rows, want %d", len(running), want)
			}
			c := newCanvas(f.Width, len(running))
			f.sessionsSection(hm.Doc, hm.Now).draw(c, bounds{width: f.Width, rows: len(running)})
			for y, r := range running {
				shown := status.ShownID(r.Session)
				a, ok := hm.Want[r.Session].On[r.Account]
				if !ok {
					t.Errorf("%s runs on %s, want it not to", shown, r.Account)
					continue
				}
				row := []rune(rowOf(c, y))
				before, _, found := strings.Cut(string(row), shown)
				if !found {
					t.Fatalf("row %d is %q, want %s on it", y+1, string(row), shown)
				}
				at := utf8.RuneCountInString(before)
				seenAt := at + runningID + laid.family + familyGap
				doingAt := seenAt + seenCells + beforeDoing
				seen, doing := strings.TrimSpace(string(row[seenAt:doingAt])), strings.TrimSpace(string(row[doingAt:doingAt+doingCells]))
				wantDoing := runningDoing(a)
				if seen != runningSeen(a.LastSeen, hm.Now) || doing != wantDoing.text || c.cells[y][doingAt].ink != wantDoing.ink {
					t.Errorf("%s on %s seen %q, %q in %+v; want seen %q, %q in %+v",
						shown, r.Account, seen, doing, c.cells[y][doingAt].ink, runningSeen(a.LastSeen, hm.Now), wantDoing.text, wantDoing.ink)
				}
				busy := r
				busy.Activity = a
				if got, want := c.cells[y][at].ink, runningIDOf(busy, hm.Now).ink; got != want {
					t.Errorf("%s on %s has its id in %+v, want %+v", shown, r.Account, got, want)
				}
			}
			checkROUTINGCountsSESSIONS(t, f, hm.Doc, hm.Now, running)
		})
	}
}

// checkROUTINGCountsSESSIONS checks that ROUTING counts as many sessions on
// each account as SESSIONS lists running there, where the router lists them.
func checkROUTINGCountsSESSIONS(t *testing.T, f Frame, doc status.Document, now time.Time, running []views.Running) {
	t.Helper()
	if f.Sessions == nil {
		return
	}
	for _, a := range f.routingTableOf(doc, now).accounts {
		listed := 0
		for _, r := range running {
			if r.Account == a.id {
				listed++
			}
		}
		if a.sessions != listed {
			t.Errorf("ROUTING counts %d sessions on %s, want the %d SESSIONS lists", a.sessions, a.id, listed)
		}
	}
}

// At each moment the views have to tell what a session is doing at, a move
// while it runs among them, the routing card counts as many sessions running
// on each account as ROUTING draws there: one count, of the router's listing
// as the request stream has moved and seen it since.
func TestTheRoutingCardCountsTheSessionsROUTINGDrawsAtEachHardMoment(t *testing.T) {
	moved := false
	for _, hm := range viewstest.SessionMoments() {
		moved = moved || len(hm.Stream.Moves) > 0
		t.Run(hm.Name, func(t *testing.T) {
			if hm.Sessions == nil {
				t.Skip("no router lists the sessions: ROUTING counts the document's")
			}
			f := frameOf(160, 40)
			f.Sessions, f.Stream = hm.Sessions, hm.Stream
			drawn := f.Numbers(hm.Doc, hm.Now)
			for _, row := range RoutingRows(f.Moment(hm.Doc, hm.Now), hm.Sessions, hm.Stream)[1:] {
				counted, ok := drawn[Number{Kind: NumberSessions, Account: row.Account}]
				if want := sessionsSpan(row.Running).text; !ok || counted != want {
					t.Errorf("ROUTING draws %q on %s, drawn %t; want the card's %q", counted, row.Account, ok, want)
				}
			}
		})
	}
	if !moved {
		t.Fatal("no moment moves a session as it runs, want one, for the card to count it where it went")
	}
}
