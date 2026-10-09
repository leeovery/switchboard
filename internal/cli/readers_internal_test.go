package cli

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

func TestNoLedgerReadsAsAFollowerOfAnEmptyStateDirectory(t *testing.T) {
	now := func() time.Time { return time.Date(2026, 10, 7, 13, 12, 0, 0, time.Local) }
	caps := ledger.CapsOf([]config.Account{{ID: "work", Reserve: 0.1}, {ID: "side"}}, claude.SharedWindows)
	follower := ledger.NewFollower(t.TempDir(), now, caps, logger)
	none := noLedger{now: now, caps: caps}

	for _, from := range []time.Time{now(), now().AddDate(0, 0, -30)} {
		if got, want := none.Days(from), follower.Days(from); !reflect.DeepEqual(got, want) {
			t.Errorf("Days(%v) = %+v, want %+v, as a Follower of an empty state directory gives", from, got, want)
		}
	}
	gotLines, gotMark, gotAfresh := none.Today(ledger.Mark{})
	wantLines, _, wantAfresh := follower.Today(ledger.Mark{})
	if len(gotLines) != len(wantLines) || gotAfresh != wantAfresh {
		t.Errorf("Today() = %+v, afresh %v, want %+v, afresh %v, as a Follower of an empty state directory gives", gotLines, gotAfresh, wantLines, wantAfresh)
	}
	if again, _, _ := none.Today(gotMark); len(again) > 0 {
		t.Errorf("Today() from its mark = %+v, want no line", again)
	}
	if got, want := slices.Collect(none.Session("5b0e7c1a")), slices.Collect(follower.Session("5b0e7c1a")); len(got) != len(want) {
		t.Errorf("Session() = %+v, want %+v, as a Follower of an empty state directory gives", got, want)
	}
}
