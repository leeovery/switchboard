package swipe_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/leeovery/switchboard/internal/swipe"
)

// errExited is defaults exiting non-zero, as it does for a setting not set.
var errExited = errors.New("exit status 1")

func TestParseIsTraditionalOnlyForZero(t *testing.T) {
	tests := []struct {
		name string
		out  string
		err  error
		want swipe.Direction
	}{
		{name: "0", out: "0\n", want: swipe.Traditional},
		{name: "1", out: "1\n", want: swipe.Natural},
		{name: "the key missing", out: "", err: errExited, want: swipe.Natural},
		{name: "0 with an error", out: "0\n", err: errExited, want: swipe.Natural},
		{name: "an error", err: context.DeadlineExceeded, want: swipe.Natural},
		{name: "nothing printed", out: "", want: swipe.Natural},
		{name: "unexpected output", out: "true\n", want: swipe.Natural},
		{name: "0 among more", out: "0 0\n", want: swipe.Natural},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := swipe.Parse([]byte(tt.out), tt.err); got != tt.want {
				t.Errorf("Parse(%q, %v) = %v, want %v", tt.out, tt.err, got, tt.want)
			}
		})
	}
}

func TestReadParsesWhatTheRunnerPrints(t *testing.T) {
	tests := []struct {
		out  string
		want swipe.Direction
	}{
		{out: "0\n", want: swipe.Traditional},
		{out: "1\n", want: swipe.Natural},
	}
	for _, tt := range tests {
		t.Run(tt.want.String(), func(t *testing.T) {
			ran := 0
			got := swipe.Read(t.Context(), func(context.Context) ([]byte, error) {
				ran++
				return []byte(tt.out), nil
			})
			if ran != 1 || got != tt.want {
				t.Errorf("Read of %q = %v, run %d times; want %v, run once", tt.out, got, ran, tt.want)
			}
		})
	}
}

func TestReadWithoutARunnerIsNatural(t *testing.T) {
	if got := swipe.Read(t.Context(), nil); got != swipe.Natural {
		t.Errorf("Read(nil) = %v, want natural", got)
	}
}

func TestTheZeroDirectionIsNatural(t *testing.T) {
	var d swipe.Direction
	if d != swipe.Natural {
		t.Errorf("the zero Direction is %v, want natural, macOS's default", d)
	}
}

func TestDirectionsAreNamedAsTheMacNamesThem(t *testing.T) {
	for d, want := range map[swipe.Direction]string{swipe.Natural: "natural", swipe.Traditional: "traditional"} {
		if got := d.String(); got != want {
			t.Errorf("Direction(%d).String() = %q, want %q", int(d), got, want)
		}
	}
}

// Under natural scrolling, fingers down reach the terminal as the wheel
// turned up; under traditional, as the wheel turned down.
func TestFingersAreTheWheelReversedOnlyUnderNatural(t *testing.T) {
	tests := []struct {
		d          swipe.Direction
		wheel, got int
	}{
		{d: swipe.Natural, wheel: -1, got: 1},
		{d: swipe.Natural, wheel: 1, got: -1},
		{d: swipe.Traditional, wheel: -1, got: -1},
		{d: swipe.Traditional, wheel: 1, got: 1},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v wheel %d", tt.d, tt.wheel), func(t *testing.T) {
			if got := tt.d.Fingers(tt.wheel); got != tt.got {
				t.Errorf("%v: Fingers(%d) = %d, want %d", tt.d, tt.wheel, got, tt.got)
			}
		})
	}
}
