package views

import (
	"reflect"
	"sync"
	"time"
)

// Hearing holds what a request stream told, heard, for every look at a
// Stream given it, as HeardBy gives them, to share: heard once, and again
// only as what the stream told changes, whether it's live, its requests or
// when its seats were seen, rather than at every frame drawn of it. The zero
// Hearing has heard nothing. Looks may share it from more than one
// goroutine.
type Hearing struct {
	mu       sync.Mutex
	live     bool
	requests map[string]Told
	seen     map[Seat]time.Time
	heard    *hearing
}

// HeardBy is s, heard through h: once for every look at it, or at another
// Stream given h, till one tells otherwise of whether it's live, of its
// requests or of when its seats were seen.
func (s Stream) HeardBy(h *Hearing) Stream {
	s.heard = h
	return s
}

// hearing is what s told, as hear has it: as its Hearing last heard it,
// where it has one and what s told hasn't changed since, else heard afresh.
func (s Stream) hearing() hearing {
	if s.heard == nil {
		return s.hear()
	}
	return s.heard.of(s)
}

// of is what s told, as hear has it: as h last heard it, where s tells what
// that did, else heard afresh and held for the looks after.
func (h *Hearing) of(s Stream) hearing {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.heard == nil || h.live != s.Live || !sameMap(h.requests, s.Requests) || !sameMap(h.seen, s.Seen) {
		heard := s.hear()
		h.live, h.requests, h.seen, h.heard = s.Live, s.Requests, s.Seen, &heard
	}
	return *h.heard
}

// sameMap reports whether a and b are the very same map, not alike ones: a
// Stream's maps are its own, what changes one copying it first, so one that
// tells otherwise is another map.
func sameMap[K comparable, V any](a, b map[K]V) bool {
	return reflect.ValueOf(a).UnsafePointer() == reflect.ValueOf(b).UnsafePointer()
}
