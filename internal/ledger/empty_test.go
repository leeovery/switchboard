package ledger_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

func TestAnEmptyLedgerReadsAsAFollowerOfAnEmptyStateDirectory(t *testing.T) {
	clock := &testClock{at: time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local)}
	follower := followerOf(t.TempDir(), clock, reserving)
	empty := ledger.NewEmpty(clock.now, reserving)

	for _, from := range []time.Time{clock.at, clock.at.AddDate(0, 0, -30)} {
		if got, want := empty.Days(from), follower.Days(from); !reflect.DeepEqual(got, want) {
			t.Errorf("Days(%v) = %+v, want %+v, as a Follower of an empty state directory gives", from, got, want)
		}
	}
	gotMark, wantMark := ledger.Mark{}, ledger.Mark{}
	for _, call := range []string{"first", "second"} {
		var gotLines, wantLines []ledger.Held
		var gotAfresh, wantAfresh bool
		gotLines, gotMark, gotAfresh = empty.Today(gotMark)
		wantLines, wantMark, wantAfresh = follower.Today(wantMark)
		if len(gotLines) != len(wantLines) || gotAfresh != wantAfresh {
			t.Errorf("the %s Today() = %+v, afresh %v, want %+v, afresh %v, as a Follower of an empty state directory gives",
				call, gotLines, gotAfresh, wantLines, wantAfresh)
		}
	}
	if got, want := slices.Collect(empty.Session("5b0e7c1a")), slices.Collect(follower.Session("5b0e7c1a")); len(got) != len(want) {
		t.Errorf("Session() = %+v, want %+v, as a Follower of an empty state directory gives", got, want)
	}
	if got, want := slices.Collect(empty.DayLines("2026-10-07")), slices.Collect(follower.DayLines("2026-10-07")); len(got) != len(want) {
		t.Errorf("DayLines() = %+v, want %+v, as a Follower of an empty state directory gives", got, want)
	}
}
