package prose_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/prose"
)

// Each unit is 1,024 of the one before; a count moves to the next unit where
// it would round to a thousand of its own, and from tenths to whole units
// where it would round to ten, so none takes more than the six cells the
// Log's size has.
func TestBytesAreCountedInTheLeastUnitThatHoldsThemInSixCells(t *testing.T) {
	const kb, gb = 1 << 10, 1 << 30
	tests := []struct {
		name string
		n    int
		want string
	}{
		{name: "none", n: 0, want: "0 B"},
		{name: "the most as they are", n: 999, want: "999 B"},
		{name: "a thousand, in kilobytes", n: 1000, want: "1.0 KB"},
		{name: "the most kilobytes to a tenth", n: 10188, want: "9.9 KB"},
		{name: "the least whole kilobytes", n: 10189, want: "10 KB"},
		{name: "a request's body", n: 482 * kb, want: "482 KB"},
		{name: "the most kilobytes", n: 1023487, want: "999 KB"},
		{name: "a thousand kilobytes, in megabytes", n: 1023488, want: "1.0 MB"},
		{name: "megabytes to a tenth", n: 1258291, want: "1.2 MB"},
		{name: "the most megabytes to a tenth", n: 10433331, want: "9.9 MB"},
		{name: "the least whole megabytes", n: 10433332, want: "10 MB"},
		{name: "the most megabytes", n: 1048051711, want: "999 MB"},
		{name: "a thousand megabytes, in gigabytes", n: 1048051712, want: "1.0 GB"},
		{name: "gigabytes past a thousand", n: 1500 * gb, want: "1500 GB"},
		{name: "a half rounded up", n: 1280, want: "1.3 KB"},
		{name: "under a half rounded down", n: 1279, want: "1.2 KB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.Bytes(tt.n); got != tt.want {
				t.Errorf("Bytes(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
