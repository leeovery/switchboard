package setuptoken

import "time"

// Waiting returns m giving setup-token grace to exit once it says it has made
// the token, and handOver to hand its sign-in address over, in place of
// exitGrace and handOverWithin, so a test of either needn't wait it out.
func Waiting(m Maker, grace, handOver time.Duration) Maker {
	m.waits = waits{grace: grace, handOver: handOver}
	return m
}
