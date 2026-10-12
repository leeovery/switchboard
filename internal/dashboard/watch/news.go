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

// looked takes in look, a look at a document read from the router whose
// health check said from: each of its events newer than the newest seen is
// news, picked out from the look's moment. The first router's document read
// is no news, there being no look before it; one from another router, its
// ids starting again from 1 as it started, has every event news. A document
// built by probing tells of none, and changes nothing.
func (n news) looked(look status.Moment, from router.Health) news {
	if !look.Routed {
		return n
	}
	newest := 0
	for _, e := range look.Events {
		newest = max(newest, e.ID)
	}
	since := make(map[int]time.Time)
	switch {
	case !n.told:
		return news{told: true, router: from, last: newest, since: since}
	case from.Same(n.router):
		since = lasting(n.since, look.Now, highlightFor)
	default:
		n.last = 0
	}
	for _, e := range look.Events {
		if e.ID > n.last {
			since[e.ID] = look.Now
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
// by id, as fading says over highlightFor.
func (n news) faded(now time.Time) map[int]float64 {
	return fading(n.since, now, highlightFor)
}

// lasting are the things picked out at the time since gives that are picked
// out still at now, by their key: those picked out less than lasts before.
func lasting[K comparable](since map[K]time.Time, now time.Time, lasts time.Duration) map[K]time.Time {
	still := make(map[K]time.Time)
	for key, at := range since {
		if now.Sub(at) < lasts {
			still[key] = at
		}
	}
	return still
}

// fading is how far the highlight of each thing picked out at the time
// since gives has faded at now, by its key: from 0, just seen, to 1, gone,
// quickening as it goes, lasts after it was seen. It's nil when none is
// picked out still.
func fading[K comparable](since map[K]time.Time, now time.Time, lasts time.Duration) map[K]float64 {
	var faded map[K]float64
	for key, at := range since {
		if p := progress(now.Sub(at), lasts); p < 1 {
			if faded == nil {
				faded = make(map[K]float64)
			}
			faded[key] = easeInCubic(p)
		}
	}
	return faded
}
