package views

import (
	"reflect"
	"sync"
)

// Hearing holds what a request stream told, heard, for every look at a
// Stream given it, as HeardBy gives them, to share: heard once, and again
// only as what the stream told changes, whether it's live or its requests,
// rather than at every frame drawn of it. The zero Hearing has heard
// nothing. Looks may share it from more than one goroutine.
type Hearing struct {
	mu       sync.Mutex
	live     bool
	requests map[string]Told
	heard    *hearing
}

// HeardBy is s, heard through h: once for every look at it, or at another
// Stream given h, till one tells otherwise of whether it's live or of its
// requests.
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
	if h.heard == nil || h.live != s.Live || !sameRequests(h.requests, s.Requests) {
		heard := s.hear()
		h.live, h.requests, h.heard = s.Live, s.Requests, &heard
	}
	return *h.heard
}

// sameRequests reports whether a and b are the very same map, not alike
// ones: a Stream's map is its own, what changes it copying it first, so one
// that tells otherwise is another map.
func sameRequests(a, b map[string]Told) bool {
	return reflect.ValueOf(a).UnsafePointer() == reflect.ValueOf(b).UnsafePointer()
}
