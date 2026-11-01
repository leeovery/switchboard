package cli

import (
	"sync"

	"github.com/leeovery/switchboard/internal/interrupts"
)

// keptFrom holds the process's interrupts with hold from when it's first
// kept till it's let go, as a token made, which setup's hold through its
// save no longer covers, is held till it's shown: from whichever goroutine
// keeps it, once.
type keptFrom struct {
	hold interrupts.Holder
	mu   sync.Mutex
	held *interrupts.Held
}

// keep holds the interrupts, where they aren't held already, telling told,
// where it's set, of the first.
func (k *keptFrom) keep(told func()) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.held == nil {
		k.held = interrupts.Hold(k.hold)
		k.held.Watch(told)
	}
}

// let lets the interrupts go, once, where they're held, reporting whether
// one came while they were.
func (k *keptFrom) let() bool {
	k.mu.Lock()
	held := k.held
	k.mu.Unlock()
	return held != nil && held.End()
}
