package views

// Waiting reports whether a request of it waits for its answer: asking, or
// throttled or refused on an account, to go out again, its answer's first
// byte yet to come.
//
// A stand-in for stage 9's own, which feat/log builds: this file goes once
// that one is merged in.
func (a Activity) Waiting() bool {
	return a.Doing == Asking || a.Doing == Throttled || a.Doing == Refused
}
