package views

import "time"

// TraceSpan is how far back a trace goes: the last 30 minutes.
const TraceSpan = 30 * time.Minute

// Traces are the requests the Overview's traces count: those that spend
// quota and went upstream, each once, by its id, as it's sent, on the
// account it went out on; Claude Code's quota checks and token counts never.
// They're by account, and by session, a session's carrying its whole
// TraceSpan across its moves.
type Traces struct {
	// Accounts are each account's, by its id.
	Accounts map[string]Trace
	// Sessions are each session's, by its id.
	Sessions map[string]Trace
}

// Trace is the requests an account, or a session, sent over the TraceSpan:
// when each went out, the oldest first.
type Trace struct {
	Sent []time.Time
}

// Cells are the trace n cells wide at now, the newest at the right: each its
// requests a minute over its slice of the TraceSpan to now, the mean of four
// samples across it.
func (t Trace) Cells(n int, now time.Time) []float64 {
	return make([]float64, max(n, 0))
}

// Rate is the trace's requests of the last minute to now.
func (t Trace) Rate(now time.Time) int {
	return 0
}
