package dashboard

// runwayFit is how Runway fits the rows the page gives it, as its compaction
// takes the first layout that fits, and its lanes scroll where none does:
// nothing yet.
type runwayFit struct{}

// runwayFitted is how Runway fits the rows from row top to the one over the
// key line: nothing yet.
func (f Frame) runwayFitted(int) runwayFit {
	return runwayFit{}
}

// scrolling is how far Runway's lanes scroll, scrolled to each offset, as
// fitted: nowhere yet.
func (r runwayFit) scrolling(int) Scrolling {
	return Scrolling{}
}
