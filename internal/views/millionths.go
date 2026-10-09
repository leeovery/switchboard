package views

import "math"

// millionths are a share of a window, a use of it, in millionths of it, as
// the ledger rounds a window's rise: finer than any reading gives a use, so
// what's worked out from readings, summed, taken from the whole or compared,
// reads as they do, with no trace of the floating point's sums.
type millionths int64

// whole is the whole of a window.
const whole millionths = 1_000_000

// millionthsOf is a share of a window, as a reading gives it, in millionths.
func millionthsOf(share float64) millionths {
	return millionths(math.Round(share * float64(whole)))
}

// share is m as a share of the whole.
func (m millionths) share() float64 {
	return float64(m) / float64(whole)
}

// left is what a window used as far as m has left of it, none where m used
// it up, or more.
func (m millionths) left() millionths {
	return max(whole-m, 0)
}

// meanOf is the mean of n shares that sum to sum, as a share of the whole,
// as near as a float64 holds it.
func meanOf(sum millionths, n int) float64 {
	return float64(sum) / (float64(n) * float64(whole))
}
