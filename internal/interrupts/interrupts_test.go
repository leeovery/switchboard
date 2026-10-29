package interrupts_test

import (
	"os"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/interrupts"
)

func TestTheInterruptsAndSIGPIPEAreHeld(t *testing.T) {
	want := []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE}
	if !slices.Equal(interrupts.Signals, want) {
		t.Errorf("a hold holds %v, want %v", interrupts.Signals, want)
	}
}

func TestASignalTheProcessIgnoresIsntHeld(t *testing.T) {
	tests := []struct {
		name    string
		ignored []os.Signal
		want    []os.Signal
	}{
		{name: "none ignored: all held", want: interrupts.Signals},
		{name: "SIGHUP ignored, as under nohup", ignored: []os.Signal{syscall.SIGHUP}, want: []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE}},
		// notify given no signals would send every signal on the channel.
		{name: "all ignored: none held, notify never asked", ignored: interrupts.Signals},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caught := make(chan os.Signal, 1)
			var notified []os.Signal
			var notifies, stops int
			notify := func(c chan<- os.Signal, sigs ...os.Signal) {
				notifies++
				if c != caught {
					t.Error("notify was given another channel, want the one held on")
				}
				notified = sigs
			}
			stop := func(c chan<- os.Signal) {
				stops++
				if c != caught {
					t.Error("stop was given another channel, want the one held on")
				}
			}
			ignored := func(sig os.Signal) bool { return slices.Contains(tt.ignored, sig) }

			release := interrupts.HoldSignals(caught, ignored, notify, stop)
			if !slices.Equal(notified, tt.want) {
				t.Errorf("held %v, want %v", notified, tt.want)
			}
			if stops != 0 {
				t.Error("stopped notifying before the release, want the signals held till then")
			}
			release()
			if want := min(len(tt.want), 1); notifies != want || stops != want {
				t.Errorf("notified %d times and stopped %d, want %d each", notifies, stops, want)
			}
		})
	}
}

// fakeProcess stands in for the process's signals, held as Hold holds them:
// one sent reaches the hold only while it's held, and none is ever a real
// signal. atRelease, where it's set, is sent as they're released.
type fakeProcess struct {
	caught    chan<- os.Signal
	atRelease os.Signal
	holds     int
}

func (f *fakeProcess) hold(caught chan<- os.Signal) func() {
	f.caught = caught
	f.holds++
	return func() {
		if f.atRelease != nil {
			f.send(f.atRelease)
		}
		f.caught = nil
	}
}

// send sends sig, which reaches the hold only while it's held.
func (f *fakeProcess) send(sig os.Signal) {
	if f.caught != nil {
		f.caught <- sig
	}
}

func TestAnInterruptHeldBeforeAnyWatchIsCaught(t *testing.T) {
	tests := []struct {
		name string
		sent []os.Signal
		want bool
	}{
		{name: "none", want: false},
		{name: "an interrupt", sent: []os.Signal{os.Interrupt}, want: true},
		{name: "a SIGTERM", sent: []os.Signal{syscall.SIGTERM}, want: true},
		{name: "a SIGHUP after a SIGPIPE", sent: []os.Signal{syscall.SIGPIPE, syscall.SIGHUP}, want: true},
		{name: "a SIGPIPE, which isn't one", sent: []os.Signal{syscall.SIGPIPE}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			process := &fakeProcess{}
			h := interrupts.Hold(process.hold)
			for _, sig := range tt.sent {
				process.send(sig)
			}
			if got := h.Caught(); got != tt.want {
				t.Errorf("Caught() = %t, want %t", got, tt.want)
			}
			h.End()
			if process.holds != 1 || process.caught != nil {
				t.Errorf("held %d times, held still: %t; want held once, then released", process.holds, process.caught != nil)
			}
		})
	}
}

// A watch tells of the first interrupt, past a SIGPIPE, and ends there,
// leaving a second, as a second Ctrl-C, held as the first, waiting for End.
func TestAWatchTellsOfTheFirstInterruptAlone(t *testing.T) {
	process := &fakeProcess{}
	h := interrupts.Hold(process.hold)
	process.send(syscall.SIGPIPE)
	process.send(os.Interrupt)
	process.send(os.Interrupt)
	var told int
	h.Watch(func() { told++ })

	select {
	case <-h.WatchEnded():
	case <-time.After(time.Minute):
		t.Fatal("a minute on, the watch watched on past an interrupt, want it ended at the first")
	}
	if told != 1 {
		t.Errorf("the watch told of %d interrupts, want the first alone", told)
	}
	if !h.End() {
		t.Error("End() reports no interrupt, want the one watched for")
	}
}

func TestEndCatchesAnInterruptAsTheHoldIsLetGo(t *testing.T) {
	tests := []struct {
		name      string
		atRelease os.Signal
		want      bool
	}{
		{name: "an interrupt", atRelease: os.Interrupt, want: true},
		{name: "a SIGPIPE, which isn't one", atRelease: syscall.SIGPIPE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			process := &fakeProcess{atRelease: tt.atRelease}
			h := interrupts.Hold(process.hold)
			var told int
			h.Watch(func() { told++ })

			if got := h.End(); got != tt.want || told != 0 {
				t.Errorf("End() = %t, told %d times; want %t, none told, as the watch had stopped", got, told, tt.want)
			}
		})
	}
}
