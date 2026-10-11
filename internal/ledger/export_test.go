package ledger

// SetLooked has a round call looked with the date of each day it summarises
// once it has looked at the day's files, before it reads them, for tests of
// what comes between the two.
func (l *Ledger) SetLooked(looked func(date string)) {
	l.looked = looked
}
