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
// for one that upgrades its connection.
func (f *inFlight) count(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !upgrades(r) {
			f.begin()
			defer f.end()
		}
		h.ServeHTTP(w, r)
	})
}

// upgrades reports whether r upgrades its connection, such as to a
// WebSocket, which can stay open for as long as its session runs, and which
// closing the server leaves open: so it never counts as in flight.
func upgrades(r *http.Request) bool {
	return r.Header.Get("Upgrade") != ""
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

// requests counts the requests in flight.
func (f *inFlight) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

// quiet returns what's closed once no request is in flight: at once, while
// none is.
func (f *inFlight) quiet() <-chan struct{} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.none
}
