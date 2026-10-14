package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Target is a zone of the page a click acts on, from X along row Y, Width
// cells wide, and what's there, as On says: a tab, Tab; a sub-tab, by its
// place among them, SubTab; < or > by a period or a window, Step, -1 or 1; a
// key of the key line, Key, as the line names it; a row of the view, Row, by
// its index; an Overview's section, Section; an account, by its id, Account;
// a session, by its id, Session; a turn of a session's page, by its number,
// Turn; or a request of its turn open, by its index among them, Row.
type Target struct {
	X, Y, Width int
	On          Clicked
	Tab         View
	SubTab      int
	Step        int
	Key         string
	Row         int
	Section     Section
	Account     string
	Session     string
	Turn        int
}

// Clicked is what a click lands on.
type Clicked int

const (
	// OnTab is a tab in the title row.
	OnTab Clicked = iota + 1
	// OnSubTab is a sub-tab in the heading row.
	OnSubTab
	// OnStep is < or > by a period or a window.
	OnStep
	// OnKey is a key of the key line.
	OnKey
	// OnRow is a row of the view: of the Log's table shown, a line, by its
	// place among the table's lines.
	OnRow
	// OnSection is a section's heading on the Overview, or its key there,
	// which hides it.
	OnSection
	// OnAccount is an account's row on the Overview, in USAGE or ROUTING,
	// which moves the focus to it.
	OnAccount
	// OnSession is a session's row, which opens its page.
	OnSession
	// OnTurn is a turn's row on a session's page, which chooses it.
	OnTurn
	// OnRequest is a request's row in a session's turn open, which chooses
	// it.
	OnRequest
)

// Targets are the zones of the frame's page of doc at now a click acts on,
// each over the cells it's drawn on, laid out as the page draws them: its
// tabs; the heading row's sub-tabs, and < and > by its period or window; a
// session's page's, where one is open, as sessionTargets has them, else the
// Overview's, as overviewTargets has them, and the Log's, as logTargets has
// them; and the key line's keys, as keyTargets has them.
func (f Frame) Targets(doc status.Document, now time.Time) []Target {
	targets := f.tabTargets()
	for _, p := range f.headingPieces() {
		if p.click.On != 0 {
			targets = append(targets, p.target())
		}
	}
	switch {
	case f.cramped():
	case f.Session != nil:
		targets = append(targets, f.At(doc, now).sessionTargets(doc, now)...)
	case f.View == Overview:
		targets = append(targets, f.At(doc, now).overviewTargets(doc, now)...)
	case f.View == Log:
		targets = append(targets, f.logTargets(now)...)
	}
	return append(targets, f.keyTargets()...)
}

// tabTargets are the tabs, as tabs draws them.
func (f Frame) tabTargets() []Target {
	x, y, gap := f.tabsAt()
	targets := make([]Target, 0, len(Views()))
	for _, v := range Views() {
		width := f.tab(v).width()
		targets = append(targets, Target{X: x, Y: y, Width: width, On: OnTab, Tab: v})
		x += width + gap
	}
	return targets
}

// keyTargets are the key line's keys, as keyLine draws them, but for p pin,
// as no click opens the routing card, and q quit: none while a note takes
// the line's place, or where it has no row of its own.
func (f Frame) keyTargets() []Target {
	y, keys, _ := f.keyLineRow()
	var targets []Target
	for _, k := range keys {
		if k.key.Key == "p" || k.key.Key == "q" {
			continue
		}
		targets = append(targets, Target{X: k.x, Y: y, Width: k.key.words().width(), On: OnKey, Key: k.key.Key})
	}
	return targets
}
