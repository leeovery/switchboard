package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

// The columns of a trace a row an account, or a session, has from 110
// columns: from traceFrom to traceTail cells short of the frame's edge, its
// rate right-aligned in them.
const (
	traceFrom = 92
	traceTail = 8
)

// leastScale is the fewest requests a minute that fill a trace's height, so
// a quiet page isn't magnified.
const leastScale = 8

// traceScale is the requests a minute that fill a trace's height, of every
// trace the page draws at now, each cells wide: the frame's TraceScale, where
// it pins one; else the busiest column of them all, accounts' and sessions'
// alike, never less than leastScale.
func (f Frame) traceScale(now time.Time, cells int) float64 {
	if f.TraceScale > 0 {
		return f.TraceScale
	}
	return leastScale
}

// drawTrace draws the trace t at now in the bounds given, a column a cell,
// the newest at the right, scale requests a minute filling its rows; faded
// where its account is out of the pool.
func (f Frame) drawTrace(c *canvas, t views.Trace, now time.Time, at bounds, scale float64, faded bool) {
}

// traceRate is the trace t's rate at now, its requests of the last minute,
// as in "15/min"; faint where its account is out of the pool.
func traceRate(t views.Trace, now time.Time, faded bool) span {
	return span{}
}
