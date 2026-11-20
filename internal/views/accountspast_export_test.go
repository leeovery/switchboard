package views

// WeeksOver is the Past's WeeksPage over the n weeks to this one, as
// weeksOver has them at its Now, for the tests to hold a period gone's weeks
// to the trailing window of the weeks at now.
func (p Past) WeeksOver(n int) WeeksPage {
	return p.weeksOver(n, 0)
}
