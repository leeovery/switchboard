package service_test

import (
	"os"
	"slices"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/service"
)

func TestInstallHoldsTheInterruptsAndSIGPIPE(t *testing.T) {
	want := []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGPIPE}
	if !slices.Equal(service.HeldSignals, want) {
		t.Errorf("Install holds %v, want %v", service.HeldSignals, want)
	}
}

func TestASignalTheProcessIgnoresIsntHeld(t *testing.T) {
	tests := []struct {
		name    string
		ignored []os.Signal
		want    []os.Signal
	}{
		{name: "none ignored: all held", want: service.HeldSignals},
		{name: "SIGHUP ignored, as under nohup", ignored: []os.Signal{syscall.SIGHUP}, want: []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE}},
		// notify given no signals would send every signal on the channel.
		{name: "all ignored: none held, notify never asked", ignored: service.HeldSignals},
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

			release := service.HoldSignals(caught, ignored, notify, stop)
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
