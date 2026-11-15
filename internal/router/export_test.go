package router

import (
	"context"
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

// DefaultHold is the hold curve the status document gives before a day of
// answers, for tests that read a router's whole document.
var DefaultHold = defaultHold

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

// RunRouter runs r as Run runs the router it builds, until ctx ends, for
// tests that ask r itself as it runs.
func RunRouter(ctx context.Context, r *Router) error {
	return r.run(ctx)
}

// TendHistory has the readings history's writer make its round where its
// files are due their prune, as its tendNow does, returning once it has, so
// after the round the writer makes as it starts, which the router may answer
// before: for tests that hold what they ask of the router to come after that
// round, as it has for a router running a while.
func (r *Router) TendHistory(ctx context.Context) error {
	return r.history.tendNow(ctx)
}
