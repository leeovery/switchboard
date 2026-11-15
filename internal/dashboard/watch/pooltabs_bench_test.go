package watch

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// acmeLately is a session running at the world's now that has run three
// days, in Acme's pool, as twoPools routes it.
func (w busyWorld) acmeLately() string {
	return busySession(busySeat(busyDays-1, 4))
}

// firstShown shows the view given the first time, and delivers what that
// starts, till what landed holds as landed says.
func (h *harness) firstShown(view dashboard.View, landed func(Model) bool) {
	h.t.Helper()
	h.model.view = view
	h.deliver(h.update(nudgeMsg{})...)
	if !landed(h.model) {
		h.t.Fatalf("%v didn't read what it shows", view)
	}
}

// digestsMovedOn is views' Digests, its digests of the version given, as
// after the version of a digest's envelope, or of its sessions' states,
// moves on.
type digestsMovedOn struct {
	views.Digests
	version int
}

func (d digestsMovedOn) Version() int {
	return d.version
}

// BenchmarkThePooledTabs times the tabs the dashboard builds of the request
// ledger's digests and summaries on a busy world whose lines record their
// pools, as newPooledWorld writes it, as the owner meets them: Sessions shown
// the first time, till its List's first read lands; enter on a session in
// Accounts' Detail till its page shows its lines, of the session that ran
// every day of the four weeks, or, in Acme's pool, of one that ran the last
// three; History shown the first time, till its 12 months' read lands; and
// Graphs alike, WEEK and WHEN YOU WORK shown. Of a world of one pool, and of
// one of two, read every pool's, as they read before they read the pool
// shown's alone, then each pool's, by its id, as the dashboard reads it.
// And the first pass of a ledger opened afresh, as the router's once it
// restarts, with the digests of the four weeks' ended days due, as once
// after a digest's version moves on.
func BenchmarkThePooledTabs(b *testing.B) {
	one, two := newPooledWorld(b, start, onePool), newPooledWorld(b, start, twoPools)
	for _, on := range []struct {
		name    string
		w       busyWorld
		read    views.PoolRead
		session string
	}{
		{"one pool, every pool's", one, views.PoolRead{}, one.longest()},
		{"one pool, Default's", one, onePoolRead(), one.longest()},
		{"two pools, every pool's", two, views.PoolRead{}, two.longest()},
		{"two pools, Default's", two, twoPoolsRead("b2n8"), two.longest()},
		{"two pools, Acme's", two, twoPoolsRead("q7mt"), two.acmeLately()},
	} {
		b.Run(on.name+"/opening Sessions", func(b *testing.B) {
			h := on.w.harnessIn(b, on.read)
			if h.model.sessionsTab.listed {
				b.Fatal("the List was read before Sessions showed")
			}
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.firstShown(dashboard.Sessions, func(m Model) bool { return m.sessionsTab.listed })
			}
		})
		b.Run(on.name+"/opening a session's page", func(b *testing.B) {
			h := on.w.onDetailIn(b, on.read, on.session)
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.entered(on.session)
			}
		})
		b.Run(on.name+"/opening History", func(b *testing.B) {
			h := on.w.harnessIn(b, on.read)
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.firstShown(dashboard.History, func(m Model) bool { return m.past.last.held() })
			}
		})
		b.Run(on.name+"/opening Graphs", func(b *testing.B) {
			h := on.w.harnessIn(b, on.read)
			h.model.graphs = dashboard.GraphsSectionsNamed([]string{"week", "when"})
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.firstShown(dashboard.Graphs, func(m Model) bool { return m.past.last.held() })
			}
		})
	}
	b.Run("the first pass of a ledger opened afresh, every digest due", func(b *testing.B) {
		version := views.DigestVersion
		for b.Loop() {
			b.StopTimer()
			version++
			l := two.ledger(ledger.Digesting(digestsMovedOn{version: version}))
			b.StartTimer()
			l.DigestEnded(b.Context(), start)
		}
	})
}

// splitPools routes a busy world of two pools as twoPools does, but for the
// session resumed every day of the four weeks: in Default's for its first
// two, then in Acme's, as once its directory's fence changes, so every
// digest from then on holds its state shared between them.
func splitPools(g, d int) (account, pool string) {
	if g == busyLong && d >= busyDays/2 {
		return acmeBusy[0], "q7mt"
	}
	return twoPools(g, d)
}

// runningIn are those of the sessions running at the world's now that route
// routes in the pool with the given id today, as the router lists a pool's.
func (w busyWorld) runningIn(route busyRouting, pool string) []status.Session {
	today := busyDays - 1
	var in []status.Session
	for j, s := range w.running() {
		if _, routed := route(busySeat(today, j), today); routed == pool {
			in = append(in, s)
		}
	}
	return in
}

// BenchmarkASplitSession times, in a busy world of two pools whose longest
// session was resumed in Acme's after two weeks in Default's, as splitPools
// routes it, in each pool: Sessions shown the first time, till its List's
// first read lands, the router listing the pool's sessions running alone;
// and enter on that session in Accounts' Detail till its page shows its
// lines. And the first pass of a ledger opened afresh, every digest due, as
// once after a digest's version moves on, with the bytes its digests and
// parts files take.
func BenchmarkASplitSession(b *testing.B) {
	w := newPooledWorld(b, start, splitPools)
	for _, pool := range []struct{ name, id string }{{"Default's", "b2n8"}, {"Acme's", "q7mt"}} {
		read := twoPoolsRead(pool.id)
		b.Run(pool.name+"/opening Sessions", func(b *testing.B) {
			h := w.harnessListing(b, read, w.runningIn(splitPools, pool.id))
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.firstShown(dashboard.Sessions, func(m Model) bool { return m.sessionsTab.listed })
			}
		})
		b.Run(pool.name+"/opening its page", func(b *testing.B) {
			h := w.onDetailIn(b, read, w.longest())
			fresh := h.model
			for b.Loop() {
				h.model, h.timers, h.waits = fresh, nil, nil
				h.entered(w.longest())
			}
		})
	}
	b.Run("the first pass of a ledger opened afresh, every digest due", func(b *testing.B) {
		version := views.DigestVersion
		for b.Loop() {
			b.StopTimer()
			version++
			l := w.ledger(ledger.Digesting(digestsMovedOn{version: version}))
			b.StartTimer()
			l.DigestEnded(b.Context(), start)
		}
		digests, parts := w.builtBytes(b)
		b.ReportMetric(float64(digests), "digest-B")
		b.ReportMetric(float64(parts), "parts-B")
	})
}

// builtBytes are how many bytes the world's ledger's digests take, and its
// parts files.
func (w busyWorld) builtBytes(tb testing.TB) (digests, parts int64) {
	tb.Helper()
	err := filepath.WalkDir(ledger.Dir(w.state), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch name := d.Name(); {
		case strings.HasPrefix(name, "digest-"):
			digests += info.Size()
		case strings.HasPrefix(name, "parts-"):
			parts += info.Size()
		}
		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
	return digests, parts
}
