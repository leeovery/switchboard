package views

// Waiting reports whether a request of it waits for its answer: asking,
// gone out with nothing back yet; throttled, to go out again on its
// account; or refused on its account, before it goes out again elsewhere;
// never as last known, as Stale has it. It's the one rule of a session
// waiting: the Log's panel says it asking, and a session's page ends its
// newest turn's row in it.
func (a Activity) Waiting() bool {
	return !a.Stale && (a.Doing == Asking || a.Doing == Throttled || a.Doing == Refused)
}
