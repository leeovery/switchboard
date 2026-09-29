package router

import (
	"net/http"
	"sync"
)

// inFlight counts the proxy's requests in flight, and says when there are
// none. It's safe for concurrent use.
type inFlight struct {
	mu sync.Mutex
	n  int
	// none is closed while no request is in flight.
	none chan struct{}
}

func newInFlight() *inFlight {
	none := make(chan struct{})
	close(none)
	return &inFlight{none: none}
}

// count returns h, counting each request it serves while it's in flight, but
// for one that upgrades its connection, such as to a WebSocket, which can
// stay open for as long as its session runs.
func (f *inFlight) count(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") == "" {
			f.begin()
			defer f.end()
		}
		h.ServeHTTP(w, r)
	})
}

func (f *inFlight) begin() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.n == 0 {
		f.none = make(chan struct{})
	}
	f.n++
}

func (f *inFlight) end() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n--
	if f.n == 0 {
		close(f.none)
	}
}

// quiet returns what's closed once no request is in flight: at once, while
// none is.
func (f *inFlight) quiet() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.none
}
