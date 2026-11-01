package interrupts

import "sync"

// Kept is the process's interrupts held with Holder, Process where it's nil,
// from when Keep first holds them till Let lets them go, as a token made is
// held till it's saved or shown: from whichever goroutines call them, each
// as often as they like. Once let go, they're never held again, so a Keep
// racing a Let can't leave them held with nothing to let them go.
type Kept struct {
	Holder Holder
	mu     sync.Mutex
	held   *Held
	let    bool
}

// Keep holds the interrupts, where they aren't held already, nor let go,
// telling told, where it's set, of the first caught, as Held's Watch tells
// it.
func (k *Kept) Keep(told func()) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.held == nil && !k.let {
		k.held = Hold(k.Holder)
		k.held.Watch(told)
	}
}

// Let lets the interrupts go, where they're held, reporting whether one was
// caught while they were, as Held's End has it: false where they were never
// held, or were let go before.
func (k *Kept) Let() bool {
	k.mu.Lock()
	held := k.held
	k.held, k.let = nil, true
	k.mu.Unlock()
	return held != nil && held.End()
}
