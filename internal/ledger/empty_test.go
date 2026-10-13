package ledger_test

import (
	"errors"
	"io/fs"
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
	if got, want := empty.Day("2026-10-06"), follower.Day("2026-10-06"); len(got) != len(want) {
		t.Errorf("Day() = %+v, want %+v, as a Follower of an empty state directory gives", got, want)
	}
	if _, err := empty.FirstLinesDay(clock.at); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FirstLinesDay() fails with %v, want fs.ErrNotExist", err)
	}
	if _, err := follower.FirstLinesDay(clock.at); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a Follower of an empty state directory's FirstLinesDay() fails with %v, want fs.ErrNotExist, as Empty's does", err)
	}
	if got, want := slices.Collect(empty.Session("5b0e7c1a")), slices.Collect(follower.Session("5b0e7c1a")); len(got) != len(want) {
		t.Errorf("Session() = %+v, want %+v, as a Follower of an empty state directory gives", got, want)
	}
	if got, want := empty.Sessions([]string{"5b0e7c1a"}, nil), follower.Sessions([]string{"5b0e7c1a"}, nil); len(got) != len(want) {
		t.Errorf("Sessions() = %+v, want %+v, as a Follower of an empty state directory gives", got, want)
	}
}
