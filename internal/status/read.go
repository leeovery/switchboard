package status

import (
	"errors"
	"time"
)

// FreshFor is how lately the router must have read an account for a Read to
// leave it be: the least the router waits between probes of one.
const FreshFor = time.Minute

// ErrNoRouter is what a read that doesn't probe fails with when the router
// doesn't answer.
var ErrNoRouter = errors.New("the router isn't answering")

// A Read is what a read of the status document asks for.
type Read struct {
	// Refresh, when the router answers, has it first probe the accounts it
	// hasn't read for this long; zero takes its document as it stands.
	Refresh time.Duration
	// Probe, when the router doesn't answer, builds the document by probing
	// every account instead. Without it, such a read fails with ErrNoRouter.
	Probe bool
}

// Fresh is the read the dashboard's r asks for, which usage --refresh asks
// for too: the router first refreshes every account it hasn't read in the
// last minute, the least it waits between probes of one, and every one that
// can take no request anyway, or, without the router, every account is
// probed.
func Fresh() Read {
	return Read{Refresh: FreshFor, Probe: true}
}
