package interrupts

// Signals are the signals a hold holds.
var Signals = signals

// HoldSignals holds Signals but those ignored says the process ignores, as
// Process does, through notify and stop in place of os/signal's.
var HoldSignals = hold

// WatchEnded is closed once Watch has ended, as it does at the first
// interrupt.
func (h *Held) WatchEnded() <-chan struct{} {
	return h.watched
}
