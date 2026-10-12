package watch

import (
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

// origin is where a look read its document from: the router, as its health
// check said which, or by probing. The zero origin is before any look.
type origin struct {
	read   bool
	routed bool
	router router.Health
}

// originOf is where doc came from, read from the router whose health check
// said from, or by probing.
func originOf(doc status.Document, from router.Health) origin {
	return origin{read: true, routed: routed(doc), router: from}
}

// follows reports whether a look from o follows on from one from was, what
// it finds carrying on from what that found: never the first look, there
// being none before it, nor one from elsewhere than the last, the router
// after probing or probing after it, or another router, as one restarted.
func (o origin) follows(was origin) bool {
	return was.read && o.routed == was.routed && (!o.routed || o.router.Same(was.router))
}
