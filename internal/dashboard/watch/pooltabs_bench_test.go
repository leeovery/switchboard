package watch

import (
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/ledger"
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
