package watch

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// busyHour is the traffic of a busy hour's requests, n of them, d28c's on
// work, the last answered at now and each before it a second earlier, as
// the traffic keeps them, those ended within lineWithin.
func busyHour(n int, now time.Time) traffic {
	events := make([]router.StreamEvent, 0, 2*n)
	for i := range n {
		at := now.Sub(start) - time.Duration(n-1-i)*time.Second
		id := fmt.Sprintf("%08x", i)
		events = append(events, told(router.StreamSent, id, "work", at-time.Second),
			alter(told(router.StreamDone, id, "work", at), func(e *router.StreamEvent) { e.Status = 200 }))
	}
	return traffic{live: true}.took(events, heldNowhere, now)
}

// However busy the hour, the traffic keeps only the requests that ended
// within lineWithin, the views taking the rest from their lines, so what a
// tick letting go of one copies is a few minutes' requests, never the
// hour's; a tick with nothing of it to let go copies nothing; and once the
// oldest kept is due, it's let go alone.
func TestATickWithNothingToLetGoCopiesNothing(t *testing.T) {
	now := past(2 * time.Hour)
	tr := busyHour(3600, now)

	if got, want := len(tr.requests), int(lineWithin/time.Second); got != want {
		t.Errorf("an hour of a request a second, the traffic keeps %d, want %d, those ended within lineWithin", got, want)
	}
	if allocs := testing.AllocsPerRun(10, func() { tr.tidied(now.Add(pulseFor / 2)) }); allocs > 0 {
		t.Errorf("a tick with nothing to let go allocates %.0f times, want none", allocs)
	}
	first := 3600 - int(lineWithin/time.Second)
	oldest, next := fmt.Sprintf("%08x", first), fmt.Sprintf("%08x", first+1)
	due := now.Add(time.Second)
	if _, ok := toldOf(tr.tidied(due.Add(-time.Millisecond)), oldest); !ok {
		t.Error("just short of lineWithin after the oldest kept ended, it's let go, want it kept")
	}
	tidied := tr.tidied(due)
	if _, ok := toldOf(tidied, oldest); ok {
		t.Error("lineWithin after the oldest kept ended, it's kept, want it let go")
	}
	if _, ok := toldOf(tr, oldest); !ok {
		t.Error("tidied, the traffic it was tidied from lost the oldest, want its requests its own")
	}
	if _, ok := toldOf(tidied, next); !ok {
		t.Error("lineWithin after the oldest kept ended, the next is let go too, want it kept, ended a second later")
	}
}

// A request lost as the stream is forgotten, or as it opens again without
// telling of it, is let go seenFor after the stream last told of it, the
// stream telling nothing since.
func TestARequestLostIsLetGoWithNothingToldSince(t *testing.T) {
	sent := traffic{live: true}.took([]router.StreamEvent{told(router.StreamSent, "r1", "work", 0)}, heldNowhere, past(0))
	tests := []struct {
		name string
		tr   traffic
	}{
		{name: "forgotten", tr: sent.afresh()},
		{name: "unsettled by the stream opening again", tr: sent.reopened().settled()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if r, ok := toldOf(tt.tr.tidied(past(seenFor-time.Second)), "r1"); !ok || !r.Lost {
				t.Fatalf("short of seenFor after it, r1 is %+v, kept: %v, want it kept, lost", r, ok)
			}
			if _, ok := toldOf(tt.tr.tidied(past(seenFor)), "r1"); ok {
				t.Error("seenFor after it, r1 is kept, want it let go")
			}
		})
	}
}

// A tick lets go of each of a call's pulse home, a request and a move as
// each is due, keeping the rest as they were.
func TestATickLetsGoEachOfACallARequestAndAMoveAsItsDue(t *testing.T) {
	tr := traffic{}.took([]router.StreamEvent{
		told(router.StreamSent, "r1", "work", 0),
		alter(told(router.StreamDone, "r1", "work", time.Second), func(e *router.StreamEvent) { e.Status = 200 }),
		alter(pinnedToSide(2*time.Second), func(e *router.StreamEvent) { e.Request = "r2" }),
	}, heldNowhere, past(time.Second))
	if len(tr.calls) == 0 || len(tr.moves) != 1 {
		t.Fatalf("the calls are %+v and the moves %+v, want r1's answer and the move", tr.calls, tr.moves)
	}

	home := tr.tidied(past(time.Second + pulseFor))
	if _, ok := home.calls[callKey{request: "r1", account: "work"}]; ok {
		t.Errorf("r1's answer home, its call on work is kept, want it let go")
	}
	if len(home.moves) != 1 || !slices.Equal(ids(home), []string{"r1"}) {
		t.Errorf("r1's answer home, the moves are %+v and the requests %q, want the move and r1 kept", home.moves, ids(home))
	}
	ended := home.tidied(past(time.Second + lineWithin))
	if len(ended.moves) != 1 || len(ids(ended)) > 0 {
		t.Errorf("lineWithin after r1 ended, the moves are %+v and the requests %q, want the move kept and r1 let go", ended.moves, ids(ended))
	}
	if moved := ended.tidied(past(2*time.Second + seenFor)); len(moved.moves) > 0 {
		t.Errorf("seenFor after the move, the moves are %+v, want none", moved.moves)
	}
}

// ids are the ids of the requests the traffic tells the views of, sorted.
func ids(tr traffic) []string {
	var got []string
	for id := range tr.stream(start).Requests {
		got = append(got, id)
	}
	slices.Sort(got)
	return got
}

// The traffic counts each change to its requests, and those that change a
// request's row, as views.Told.SameRow compares it, for the Log to make
// again what's made of them only as they change: an answer streaming on
// changes a request but not its row; a request told of, lost or let go
// changes its row; a move, or a tick with nothing to let go, changes
// neither.
func TestTheTrafficCountsTheChangesToItsRequestsAndTheirRows(t *testing.T) {
	sent := traffic{live: true}.took([]router.StreamEvent{told(router.StreamSent, "r1", "work", 0)}, heldNowhere, past(0))
	done := sent.took([]router.StreamEvent{alter(told(router.StreamDone, "r1", "work", time.Second), func(e *router.StreamEvent) { e.Status = 200 })}, heldNowhere, past(time.Second))
	streamed := alter(told(router.StreamProgress, "r1", "work", time.Second), func(e *router.StreamEvent) { e.Chars = 4936 })
	steps := []struct {
		name          string
		from, to      traffic
		changes, rows bool
	}{
		{name: "a request told of", from: traffic{}, to: sent, changes: true, rows: true},
		{name: "its answer streaming on", from: sent, to: sent.took([]router.StreamEvent{streamed}, heldNowhere, past(time.Second)), changes: true},
		{name: "a move", from: sent, to: sent.took([]router.StreamEvent{pinnedToSide(time.Second)}, heldNowhere, past(time.Second))},
		{name: "lost as the stream is forgotten", from: sent, to: sent.afresh(), changes: true, rows: true},
		{name: "lost as the stream opens again without it", from: sent, to: sent.reopened().settled(), changes: true, rows: true},
		{name: "a tick with nothing to let go", from: done, to: done.tidied(past(2 * time.Second))},
		{name: "let go", from: done, to: done.tidied(past(time.Second + lineWithin)), changes: true, rows: true},
	}
	for _, s := range steps {
		changes, rows := s.to.changes != s.from.changes, s.to.rowChanges != s.from.rowChanges
		if changes != s.changes || rows != s.rows {
			t.Errorf("%s: the requests changed: %v, and their rows: %v, want %v and %v", s.name, changes, rows, s.changes, s.rows)
		}
	}
}
