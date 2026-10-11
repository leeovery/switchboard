package router

import (
	"net/http"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
)

// NotificationQueue is how many events can wait for notifications to deal
// with them, for tests that fill the queue.
const NotificationQueue = queueSize

// LedgerLines returns the lines the request ledger's plain files in dir hold,
// for tests that run a router.
func LedgerLines(t testing.TB, dir string) []ledger.Line {
	return ledgerLines(t, dir)
}

// Zeros reads as endless zero bytes, for tests that send a body over the
// cap.
type Zeros = zeros

// SetChooser has the router choose accounts with c, for tests that choose
// them themselves.
func (r *Router) SetChooser(c Chooser) {
	r.proxy.chooser = c
}

// HTTP returns the HTTP client c asks the router with, for tests that send it
// what Client never would.
func (c *Client) HTTP() *http.Client {
	return c.http
}

// InFlightOf returns the ids of the requests of session and model the request
// stream has in flight, for tests that wait for the router to be done with
// one whose client already has its answer.
func (r *Router) InFlightOf(session, model string) []string {
	r.stream.mu.Lock()
	defer r.stream.mu.Unlock()
	var ids []string
	for id, f := range r.stream.flying {
		if f.of == (key{session: session, model: model}) {
			ids = append(ids, id)
		}
	}
	return ids
}
