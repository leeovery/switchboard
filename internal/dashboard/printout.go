package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// Snapshot is what the dashboard is drawn from, read once, as usage reads it
// to print the dashboard: the document; the sessions the router lists, for
// the cards' dots; the windows' history, for the charts; and whether the
// router is from before it gave its history and told of events.
type Snapshot struct {
	Doc      status.Document
	Sessions []status.Session
	History  History
	Outdated bool
}

// Printout is the dashboard as usage prints it into the scrollback, once: the
// Accounts view, as wide as the terminal, every card at its fullest, without
// the footer.
type Printout struct {
	// Width is the terminal's, in cells.
	Width int
	Look  Look
	// Featured is which window every card features, and Chart the style
	// every card draws it in, as the preferences keep them.
	Featured Feature
	Chart    Chart
	// Policy is the provider's say in which windows every model shares, which
	// a session's 5-hour window is, and which its week.
	Policy score.Policy
}

// Draw draws the printout of snap at now: a string for each of its rows.
// Countdowns run from now, and times show in now's time zone.
func (p Printout) Draw(snap Snapshot, now time.Time) []string {
	return Frame{
		Width: p.Width, Look: p.Look, View: Accounts,
		Outdated: snap.Outdated, History: snap.History, Featured: p.Featured, Chart: p.Chart,
		Sessions: snap.Sessions, Policy: p.Policy,
	}.Draw(snap.Doc, now)
}
