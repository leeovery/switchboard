package ledger

// SetLooked has a round call looked with the date of each day it summarises
// once it has looked at the day's files, before it reads them, for tests of
// what comes between the two.
func (l *Ledger) SetLooked(looked func(date string)) {
	l.looked = looked
}

// SetHistory has the reader list and read the readings history as history
// gives it, for tests of how it's asked.
func (r *Reader) SetHistory(history History) {
	r.days.history = history
}

// SetHistory has the follower list the readings history as history gives
// it, for tests of how it's asked.
func (f *Follower) SetHistory(history History) {
	f.days.history = history
}
