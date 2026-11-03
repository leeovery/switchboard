package export

// WithLooked returns e, calling looked with each day's date once its files of
// lines are looked at, before they're read, for tests of a day pruned
// between the two.
func WithLooked(e Exporter, looked func(date string)) Exporter {
	e.looked = looked
	return e
}
