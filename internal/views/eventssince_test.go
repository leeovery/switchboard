package views_test

import (
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// unheard logs nothing, for readers whose warnings a test doesn't read.
var unheard = slog.New(slog.DiscardHandler)

// filedIn files the moment's versions of its events in the router's files
// of a state directory of the test's own, each under the local day it was
// filed on, in the order filed, and returns a reader of them by a clock
// stopped at the moment's now.
func filedIn(t *testing.T, hm viewstest.EventMoment) views.Events {
	t.Helper()
	state := t.TempDir()
	files := events.Files(ledger.Dir(state), unheard)
	for _, f := range hm.Filed {
		data, err := events.Marshal(f.Line)
		if err != nil {
			t.Fatal(err)
		}
		lines := make(dayfile.Lines)
		lines.Add(f.Filed, data)
		if err := files.Append(lines); err != nil {
			t.Fatal(err)
		}
	}
	return events.NewReader(state, func() time.Time { return hm.Now }, unheard)
}

// liveAt is the moment of the router's document at now, giving the events
// live, the newest first.
func liveAt(live []status.Event, now time.Time) status.Moment {
	return status.Document{Source: status.SourceRouter, Events: live}.At(now, claude.Policy())
}

// asFiled are the events given as the router files them, a line each, by
// their runs, so two versions of one event read apart.
func asFiled(t *testing.T, given []events.Line) []string {
	t.Helper()
	lines := make([]string, len(given))
	for i, line := range given {
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		lines[i] = string(data)
	}
	return lines
}

// At each hard moment, the events since its start are the document's and
// the files' merged by run and id, each once, as it last stands, the
// document's version in place of the files', oldest first: across a router's
// restart, its ids starting again, each run's held apart; a limit joined and
// filed again, as the document holds it now; the document's newest 50 and
// the files' overlapping; none on a day nothing happened; each under the day
// it happened, not the day its last version was filed; and two at 01:30, in
// the order they happened, as the clocks go back.
func TestEventsSinceAreTheDocumentsMergedWithTheFilesEachOnce(t *testing.T) {
	for _, hm := range viewstest.EventMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			told := views.EventsSince(hm.Since, liveAt(hm.Live, hm.Now), hm.Run, filedIn(t, hm))
			got := make([]events.Line, len(told))
			for i, e := range told {
				got[i] = events.Line{Event: e.Event, Run: e.Key.Run}
				if e.Key.ID != e.Event.ID {
					t.Errorf("event %d is keyed as %d", e.Event.ID, e.Key.ID)
				}
			}
			if got, want := asFiled(t, got), asFiled(t, hm.Events); !slices.Equal(got, want) {
				t.Errorf("the events since %s are\n%s\nwant\n%s", hm.Since, strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

// Without files, as the watch's news reads them, the events since are the
// document's alone, keyed by the router's run, oldest first.
func TestEventsSinceWithoutFilesAreTheDocumentsKeyedByItsRun(t *testing.T) {
	run := at(9, 0)
	live := []status.Event{
		{ID: 3, At: at(13, 5), Kind: status.EventRoom, Account: "work"},
		{ID: 2, At: at(13, 0), Kind: status.EventRoom, Account: "side"},
		{ID: 1, At: at(10, 0), Kind: status.EventRoom, Account: "personal"},
	}
	var got []views.EventKey
	for _, e := range views.EventsSince(time.Time{}, liveAt(live, at(14, 0)), run, nil) {
		got = append(got, e.Key)
	}
	if want := []views.EventKey{{Run: run, ID: 1}, {Run: run, ID: 2}, {Run: run, ID: 3}}; !slices.Equal(got, want) {
		t.Errorf("the events since are keyed %v, want %v", got, want)
	}
}

// An event is told after another by a router started later, whatever its
// id, or by the same one with a higher id; every event after the zero key.
func TestAnEventIsToldAfterAnotherByALaterRunOrALaterID(t *testing.T) {
	first, second := at(9, 0), at(12, 0)
	tests := []struct {
		name string
		k, o views.EventKey
		want bool
	}{
		{name: "a later run, its ids started again", k: views.EventKey{Run: second, ID: 1}, o: views.EventKey{Run: first, ID: 9}, want: true},
		{name: "an earlier run", k: views.EventKey{Run: first, ID: 9}, o: views.EventKey{Run: second, ID: 1}},
		{name: "the same run, a higher id", k: views.EventKey{Run: first, ID: 3}, o: views.EventKey{Run: first, ID: 2}, want: true},
		{name: "the same run, read in another time zone", k: views.EventKey{Run: first.UTC(), ID: 2}, o: views.EventKey{Run: first, ID: 2}},
		{name: "itself", k: views.EventKey{Run: first, ID: 2}, o: views.EventKey{Run: first, ID: 2}},
		{name: "the zero key", k: views.EventKey{Run: first, ID: 1}, want: true},
	}
	for _, tt := range tests {
		if got := tt.k.After(tt.o); got != tt.want {
			t.Errorf("%s: %v.After(%v) = %v, want %v", tt.name, tt.k, tt.o, got, tt.want)
		}
	}
}
