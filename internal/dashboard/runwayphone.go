package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// phoneRunway is Runway on a phone, of doc at now, as its form draws it: its
// lanes from where lanesFrom starts them, after its label column, laid out
// as phoneRunwayFitted fits it, as runwayFormed draws them.
func (f Frame) phoneRunway(doc status.Document, now time.Time) runwayForm {
	return f.runwayFormed(doc, now, f.Runway.lanesFrom(f.Width), f.phoneRunwayFitted)
}

// phoneRunwayFitted is how Runway fits the rows from row top to the one over
// the key line on a phone, as runwayFittedAs fits them: its room on a row of
// its own under the pool's lane, as the label column is too narrow for it
// beside the lane, but with the account column collapsed, whose pool's label
// says it short; no blank row before the legend; and the legend as many rows
// as runwayLegend draws, its keys and what a column is on a line each at the
// least.
func (f Frame) phoneRunwayFitted(top int) runwayFit {
	return f.runwayFittedAs(top, runwayShape{roomRow: !f.Runway.Collapsed, legend: f.runwayLegendRows()})
}
