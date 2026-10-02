package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// highlightFor is how long an event a look saw newly stays picked out on
// screen, fading back as it goes.
const highlightFor = 5 * time.Second

// news is what the watch knows of the router's events: which router told of
// them, the newest it has seen, and when each that a look saw newly was seen,
// while it's picked out.
type news struct {
	// told is set once a router's document has been read.
	told bool
	// router told of the events seen, the newest of them last, by id.
	router router.Health
	last   int
	since  map[int]time.Time
}

// looked takes in doc, read at now from the router whose health check said
// from: each of its events newer than the newest seen is news, picked out
// from now. The first router's document read is no news, there being no look
// before it; one from another router, its ids starting again from 1 as it
// started, has every event news. A document built by probing tells of none,
// and changes nothing.
func (n news) looked(doc status.Document, from router.Health, now time.Time) news {
	if !routed(doc) {
		return n
	}
	newest := 0
	for _, e := range doc.Events {
		newest = max(newest, e.ID)
	}
	since := make(map[int]time.Time)
	switch {
	case !n.told:
		return news{told: true, router: from, last: newest, since: since}
	case from.Same(n.router):
		for id, at := range n.since {
			if now.Sub(at) < highlightFor {
				since[id] = at
			}
		}
	default:
		n.last = 0
	}
	for _, e := range doc.Events {
		if e.ID > n.last {
			since[e.ID] = now
		}
	}
	return news{told: true, router: from, last: max(n.last, newest), since: since}
}

// another reports whether the router whose health check said from is another
// than the one that last told of events, as one restarted is.
func (n news) another(from router.Health) bool {
	return n.told && !from.Same(n.router)
}

// faded is how far the highlight of each event picked out at now has faded,
// by id: from 0, just seen, to 1, gone, quickening as it goes. It's nil when
// none is picked out.
func (n news) faded(now time.Time) map[int]float64 {
	var faded map[int]float64
	for id, at := range n.since {
		if p := progress(now.Sub(at), highlightFor); p < 1 {
			if faded == nil {
				faded = make(map[int]float64)
			}
			faded[id] = easeInCubic(p)
		}
	}
	return faded
}
