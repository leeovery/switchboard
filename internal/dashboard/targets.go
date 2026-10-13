package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Target is a zone of the page a click acts on, from X along row Y, Width
// cells wide, and what's there, as On says: a tab, Tab; a sub-tab, by its
// place among them, SubTab; < or > by a period or a window, Step, -1 or 1; a
// key of the key line, Key, as the line names it; a row of the view, Row, by
// its index; an Overview's section, Section; or an account, by its id,
// Account.
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
	// OnRow is a row of the view.
	OnRow
	// OnSection is a section's heading on the Overview, or its key there,
	// which hides it.
	OnSection
	// OnAccount is an account's row on the Overview, in USAGE or ROUTING,
	// which moves the focus to it.
	OnAccount
)

// Targets are the zones of the frame's page of doc at now a click acts on,
// each over the cells it's drawn on, laid out as the page draws them: its
// tabs; the heading row's sub-tabs, and < and > by its period or window; the
// Overview's, as overviewTargets has them; and the key line's keys, as
// keyTargets has them.
func (f Frame) Targets(doc status.Document, now time.Time) []Target {
	targets := f.tabTargets()
	for _, p := range f.headingPieces() {
		if p.click.On != 0 {
			targets = append(targets, p.target())
		}
	}
	if f.View == Overview && !f.cramped() {
		targets = append(targets, f.At(doc, now).overviewTargets(doc, now)...)
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
