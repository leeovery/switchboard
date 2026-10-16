package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

func TestEachLineIsFoundByItsOwnIdentityAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		table := views.LogRows(logSourcesOf(hm))
		for i, l := range table.Lines() {
			if found, ok := table.Find(l.Key()); !ok || found != i {
				t.Errorf("%s: line %d, %s, is found at %d, %t", hm.Name, i, lineSaid(l), found, ok)
			}
		}
	}
}

// lineSaid says a line of a table, as the moments want it, for a failure
// to show.
func lineSaid(l views.LogLine) string {
	return logSaid([]viewstest.LogLine{logLineOf(l)})
}

// pausedAndNow are the Log's tables at a paused moment: as it paused, of
// the read of the moment then at once, and the same of every line; and now, of that read grown by the
// moment's lines now, as a read of the tail gives each day's whole again.
func pausedAndNow(hm viewstest.LogMoment) (then, all, table views.LogTable) {
	l := ledgerHolding(hm.Now, hm.Lines...)
	reading := readingAtOnce(*hm.Then, ledgerHolding(hm.Then.Now, hm.Then.Lines...))
	src := logSourcesOf(*hm.Then)
	src.Read = reading.Read
	then = views.LogRows(src)
	src.Filter = views.LogFilter{}
	all = views.LogRows(src)
	for _, day := range spannedDays(hm) {
		reading = reading.Took(l, day, true, hm.Stream, hm.Now)
	}
	src = logSourcesOf(hm)
	src.Read = reading.Read
	return then, all, views.LogRows(src)
}

// TestAChosenLineStaysChosenAndNewRowsAreCountedAtEachPausedMoment holds
// the table at each hard moment paused, to the table as it paused, its read
// grown since: the line chosen then is found where it is now, every line
// then is found now, and the rows new since, as the mark it paused at
// counts them, are those of requests the table then didn't hold, as the
// table then counts them itself.
func TestAChosenLineStaysChosenAndNewRowsAreCountedAtEachPausedMoment(t *testing.T) {
	paused := 0
	for _, hm := range viewstest.LogMoments() {
		if hm.Then == nil {
			continue
		}
		paused++
		t.Run(hm.Name, func(t *testing.T) {
			then, all, table := pausedAndNow(hm)
			for i, l := range then.Lines() {
				if _, ok := table.Find(l.Key()); !ok {
					t.Errorf("line %d then, %s, isn't found now", i, lineSaid(l))
				}
			}
			if found, ok := table.Find(then.Lines()[hm.Chosen].Key()); !ok || found != hm.ChosenNow {
				t.Errorf("the line chosen, %s, is found at %d, %t, want %d", then.Lines()[hm.Chosen].Row.Request, found, ok, hm.ChosenNow)
			}
			if got, held := table.NewSince(then.Mark()), views.NewSinceHeld(table, all); got != hm.New || got != held {
				t.Errorf("%d rows are new since the table paused, want %d, as the table then counts them, %d", got, hm.New, held)
			}
		})
	}
	if paused < 4 {
		t.Errorf("%d hard moments are paused, want 4 at least", paused)
	}
}

func TestARowAFilterShowsAgainWhilePausedIsntNew(t *testing.T) {
	r0 := withID("r0", line(mover, opus, "work", "new", october(7, 12, 0), hourUsage))
	o0 := withID("o0", line(older, opus, "work", "sticky", october(7, 12, 1), hourUsage))
	r1 := withID("r1", line(mover, opus, "work", "sticky", october(7, 12, 5), hourUsage))
	reading := logReading(now, views.Stream{}, r0, o0)
	then := views.LogRows(views.LogSources{From: now, Now: now, Read: reading.Read, Filter: views.LogFilter{Sessions: []string{mover}}})
	grown := reading.Took(ledgerHolding(now, r0, o0, r1), heldOf(r1), false, views.Stream{}, now)
	table := views.LogRows(views.LogSources{From: now, Now: now, Read: grown.Read})
	if got := table.NewSince(then.Mark()); got != 1 {
		t.Errorf("%d rows are new since the table paused, older's shown again, want r1's alone", got)
	}
}

// TestAMomentsFirstRequestIsTheOneThatCameFirstAtOrAfterItAtEachHardMoment
// asks, at each hard moment, for the first request at or after each row's
// own time, and after the last: each row's is itself, or one before it
// that came together with it, and after the last there's none.
func TestAMomentsFirstRequestIsTheOneThatCameFirstAtOrAfterItAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		table := views.LogRows(logSourcesOf(hm))
		var last time.Time
		for i, l := range table.Lines() {
			if !l.IsRow() {
				continue
			}
			if found, ok := table.At(l.Row.At); !ok || found > i || !table.Lines()[found].Row.At.Equal(l.Row.At) {
				t.Errorf("%s: the first request at or after %s's time is at %d, %t, want %d", hm.Name, l.Row.Request, found, ok, i)
			}
			if l.Row.At.After(last) {
				last = l.Row.At
			}
		}
		if found, ok := table.At(last.Add(time.Nanosecond)); ok {
			t.Errorf("%s: after the last request, %s's comes", hm.Name, table.Lines()[found].Row.Request)
		}
	}
}

func TestTheLogNotLiveIsToTheLatestEndOfTheLinesReadAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		var want time.Time
		for _, day := range spannedDays(hm) {
			for _, h := range day {
				if end := h.At.Add(time.Duration(h.TotalMS) * time.Millisecond); end.After(want) {
					want = end
				}
			}
		}
		if got := readAtOnce(hm, ledgerHolding(hm.Now, hm.Lines...)).WrittenTo(); !got.Equal(want) {
			t.Errorf("%s: the ledger was last written at %s, want %s", hm.Name, got, want)
		}
	}
}
