package dashboard

// runwayLegend draws the legend, as laid out: nothing yet.
func (f Frame) runwayLegend(*canvas, runwayLaid) {}

// runwayLegendRows is how many rows the legend takes, as runwayLegend draws
// it, for Runway's compaction to count: one yet.
func (f Frame) runwayLegendRows() int {
	return 1
}
