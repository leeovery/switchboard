package watch

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// onePool routes a busy world of one pool, Default, as unpooled does, each
// line recording Default, as since the move to pools.
func onePool(g, d int) (account, pool string) {
	return busyOn(g, d), "b2n8"
}

// onePoolRead is the read of a busy world of one pool's pool, Default, each
// of its accounts in it.
func onePoolRead() views.PoolRead {
	return views.PoolRead{ID: "b2n8", First: "b2n8", PoolOf: func(account string) (string, bool) {
		return "b2n8", slices.Contains(busyAccounts, account)
	}}
}

// The accounts of a busy world of two pools: Acme's, a client's, and
// Default's, the first pool's.
var (
	acmeBusy    = []string{"client"}
	defaultBusy = []string{"work", "personal", "side"}
)

// twoPools routes a busy world of two pools, each session in one for good:
// every fourth of those that run three days in Acme's, a fifth of the work
// or so, the rest in Default; each on one of its pool's accounts, a day on
// another.
func twoPools(g, d int) (account, pool string) {
	if g != busyLong && g%4 == 0 {
		return acmeBusy[0], "q7mt"
	}
	n := len(defaultBusy)
	return defaultBusy[((g+d)%n+n)%n], "b2n8"
}

// twoPoolsRead is the read of a busy world of two pools' pool with the
// given id, its accounts placed as twoPools routes them.
func twoPoolsRead(id string) views.PoolRead {
	return views.PoolRead{ID: id, First: "b2n8", PoolOf: func(account string) (string, bool) {
		switch {
		case slices.Contains(acmeBusy, account):
			return "q7mt", true
		case slices.Contains(defaultBusy, account):
			return "b2n8", true
		}
		return "", false
	}}
}

// newPooledWorld writes a busy world whose today ends at now, its sessions
// routed as route routes them, as the router leaves it: every day that has
// ended summarised, and its digest and head built.
func newPooledWorld(tb testing.TB, now time.Time, route busyRouting) busyWorld {
	tb.Helper()
	w := writeBusyWorld(tb, now, route)
	l := w.ledger(ledger.Digesting(views.Digests{}))
	l.SummariseEnded(now)
	l.DigestEnded(tb.Context(), now)
	follower, _, _ := w.readers(w.clock)
	if _, ok := follower.DayRows(views.Today(dayfile.DayStart(now, -1)).Date, views.RowsVersion); !ok {
		tb.Fatal("the day before today's rows don't stand, its head built")
	}
	return w
}

// pooledHarness is the world's harness, its Log read through the pool read
// given, as the dashboard showing the pool reads it.
func (w busyWorld) pooledHarness(tb testing.TB, read views.PoolRead) *harness {
	tb.Helper()
	return w.harnessIn(tb, read)
}

// readBackADay reads the Log back a day, as its tables reaching back read
// it, and delivers what the read finds, returning the date of the day it
// read, "" where it read none.
func (h *harness) readBackADay() string {
	h.t.Helper()
	m, cmd := h.model.readBack(h.clock.now)
	h.model = m
	var from string
	found := h.run(cmd)
	for _, msg := range found {
		if read, ok := msg.(logReadMsg); ok {
			from = read.grown.from.Date
		}
	}
	h.deliver(found...)
	return from
}

// rowsMovedOn is views' Digests, its heads and rows of the version given,
// as after RowsVersion moves on.
type rowsMovedOn struct {
	views.Digests
	version int
}

func (r rowsMovedOn) RowsVersion() int {
	return r.version
}

// BenchmarkThePooledLog times the Log on a busy world whose lines record
// their pools, as newPooledWorld writes it, as the owner meets it: shown the
// first time, till its first read lands, today's lines read; and the day
// before today read back from its rows, as its tables reach back a day. Of
// a world of one pool, and of one of two, read every pool's, as the Log
// read before it read the pool shown's alone, then each pool's, by its id,
// as the dashboard reads it. And the first pass of a ledger opened afresh, as the router's
// once it restarts, with nothing due, and with the heads of the four weeks'
// ended days due, as once after RowsVersion moves on.
func BenchmarkThePooledLog(b *testing.B) {
	one, two := newPooledWorld(b, start, onePool), newPooledWorld(b, start, twoPools)
	for _, on := range []struct {
		name string
		w    busyWorld
		read views.PoolRead
	}{
		{"one pool, every pool's", one, views.PoolRead{}},
		{"one pool, Default's", one, onePoolRead()},
		{"two pools, every pool's", two, views.PoolRead{}},
		{"two pools, Default's", two, twoPoolsRead("b2n8")},
		{"two pools, Acme's", two, twoPoolsRead("q7mt")},
	} {
		b.Run(on.name+"/opening the Log", func(b *testing.B) {
			h := on.w.pooledHarness(b, on.read)
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.openedLog()
			}
		})
		b.Run(on.name+"/a day read back", func(b *testing.B) {
			h := on.w.pooledHarness(b, on.read)
			h.openedLog()
			opened, read := h.model, ""
			for b.Loop() {
				h.model, h.timers, h.waits = opened, nil, nil
				read = h.readBackADay()
			}
			if want := views.Today(dayfile.DayStart(start, -1)).Date; read != want {
				b.Fatalf("the Log read back %q, want %s", read, want)
			}
		})
	}
	for _, due := range []struct {
		name  string
		moves int
	}{{"nothing due", 0}, {"every head due", 1}} {
		b.Run("the first pass of a ledger opened afresh, "+due.name, func(b *testing.B) {
			version := views.RowsVersion
			for b.Loop() {
				b.StopTimer()
				version += due.moves
				l := two.ledger(ledger.Digesting(rowsMovedOn{version: version}))
				b.StartTimer()
				l.DigestEnded(b.Context(), start)
			}
		})
	}
}
