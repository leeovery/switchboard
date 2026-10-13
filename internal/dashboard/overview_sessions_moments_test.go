package dashboard

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// At each moment the views have to tell what a session is doing at, SESSIONS
// draws each session running on each account as the views have it doing
// there: when it was last seen, what it's doing, and its id bright while
// it's busy; and none the views don't have running.
func TestSESSIONSSaysWhatEachSessionIsDoingAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.SessionMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			f := frameOf(160, 40)
			f.Sessions, f.Stream = hm.Sessions, hm.Stream
			running := f.running(hm.Doc, hm.Now)
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
				seenAt := at + runningID + familyCells
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
		})
	}
}
