package views

// Waiting reports whether a request of it waits for its answer: asking,
// gone out with nothing back yet; throttled, to go out again on its
// account; or refused on its account, before it goes out again elsewhere.
// It's the one rule of a session waiting: the Log's panel says it asking,
// and SESSIONS' short form ↑ ask.
func (a Activity) Waiting() bool {
	return a.Doing == Asking || a.Doing == Throttled || a.Doing == Refused
}
