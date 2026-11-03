package export

import "io"

// WithLooked returns e, calling looked with each day's date once its files of
// lines are looked at, before they're read, for tests of a day pruned
// between the two.
func WithLooked(e Exporter, looked func(date string)) Exporter {
	e.looked = looked
	return e
}

// WithSnapshotsTo returns e, writing each snapshot to what wrap makes of what
// it would write it to, for tests of writes that fail.
func WithSnapshotsTo(e Exporter, wrap func(io.Writer) io.Writer) Exporter {
	e.wrap = wrap
	return e
}
