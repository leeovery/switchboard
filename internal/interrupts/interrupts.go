// Package interrupts holds the process's interrupts through work that must
// run to its end once begun, as the service's swap and a made token's save:
// SIGINT, SIGTERM and SIGHUP, and SIGPIPE, which isn't one, so a write to an
// output whose reader has gone fails rather than ending the process.
package interrupts

import (
	"os"
	"os/signal"
	"slices"
	"syscall"
)

// signals are the signals a hold holds.
var signals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE}

// Holder holds the process's signals, as Process does: it sends each on
// caught in place of what it would do, until the release it returns is
// called.
type Holder func(caught chan<- os.Signal) (release func())

// Process holds the signals for the process, through os/signal, as hold has
// it.
func Process(caught chan<- os.Signal) (release func()) {
	return hold(caught, signal.Ignored, signal.Notify, signal.Stop)
}

// hold holds the signals but those the process ignores, as ignored says,
// sending each on caught in place of what it would do, until the release it
// returns is called: it notifies caught, and stops notifying it, alone,
// leaving any other handler of them as it was. One ignored stays ignored, as
// holding it would have it handled from then on, as a nohup's SIGHUP.
func hold(caught chan<- os.Signal, ignored func(os.Signal) bool, notify func(chan<- os.Signal, ...os.Signal), stop func(chan<- os.Signal)) (release func()) {
	held := slices.DeleteFunc(slices.Clone(signals), ignored)
	if len(held) == 0 {
		// notify with no signals would send every signal on caught.
		return func() {}
	}
	notify(caught, held...)
	return func() { stop(caught) }
}

// Held is the process's interrupts, held from Hold till End.
type Held struct {
	signals chan os.Signal
	release func()
	stop    chan struct{}
	// watched is made as Watch begins, and closed once it has ended, got set
	// where it caught an interrupt.
	watched chan struct{}
	got     bool
}

// Hold holds the process's interrupts with holder, Process where it's nil,
// till End.
func Hold(holder Holder) *Held {
	// Room for a SIGPIPE or two beside an interrupt, which os/signal drops
	// where the channel is full.
	h := &Held{signals: make(chan os.Signal, 4), stop: make(chan struct{})}
	if holder == nil {
		holder = Process
	}
	h.release = holder(h.signals)
	return h
}

// Caught reports whether an interrupt waits, while nothing watches for one,
// taking what waits.
func (h *Held) Caught() bool {
	for {
		select {
		case sig := <-h.signals:
			if interrupt(sig) {
				return true
			}
		default:
			return false
		}
	}
}

// Watch tells told, where it's set, of the first interrupt caught from now,
// as it comes, till End.
func (h *Held) Watch(told func()) {
	h.watched = make(chan struct{})
	go func() {
		defer close(h.watched)
		for {
			select {
			case sig := <-h.signals:
				if !interrupt(sig) {
					continue
				}
				h.got = true
				if told != nil {
					told()
				}
				return
			case <-h.stop:
				return
			}
		}
	}()
}

// End stops watching, where Watch began, and releases the interrupts, then
// reports whether one was caught since Watch began: one coming once watching
// has stopped, or as they're released, is caught all the same.
func (h *Held) End() bool {
	close(h.stop)
	if h.watched != nil {
		<-h.watched
	}
	h.release()
	return h.got || h.Caught()
}

// interrupt reports whether sig is an interrupt: any held but SIGPIPE.
func interrupt(sig os.Signal) bool {
	return sig != syscall.SIGPIPE
}
