package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Target is a zone of the page a click acts on, from X along row Y, Width
// cells wide, and what's there, as On says: a tab, Tab; a sub-tab, by its
// place among them, SubTab; < or > by a period or a window, Step, -1 or 1; a
// key of the key line, Key, as the line names it; something on the page
// that presses a key, Key, as a Graphs section's heading presses its own; a row of the view, Row, by its index; an Overview's section, Section;
// an account, by its id, Account, or Compare's column of one, "" for all
// accounts; a session, by its id, Session, and on Sessions' Switchboard, the
// account its call's row is on, Account; a turn of a session's page, by its
// number, Turn; or a request of its turn open, by its index among them, Row.
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
	// OnKey is a key of the key line, Key.
	OnKey
	// OnPress is something on the page a click on which presses a key, Key,
	// as a Graphs section's heading, its key among it, presses its key, which
	// hides it, and the Log's ▼ N new presses end. Unlike the key line's, it
	// lies on the page, under any card open.
	OnPress
	// OnRow is a row of the view: of the Log's table shown, a line, by its
	// place among the table's lines.
	OnRow
	// OnSection is a section's heading on the Overview, or its key there,
	// which hides it.
	OnSection
	// OnAccount is an account's row on the Overview, in USAGE or ROUTING,
	// which moves the focus to it; or in Detail's list, which chooses it.
	OnAccount
	// OnSession is a session's row, which opens its page; in Detail's
	// SESSIONS, it selects it, and opens the one selected; and on Sessions,
	// a call or a row of the List, it chooses it, and opens the one chosen.
	OnSession
	// OnTurn is a turn's row on a session's page, which chooses it.
	OnTurn
	// OnRequest is a request's row in a session's turn open, which chooses
	// it.
	OnRequest
	// OnColumn is a column's heads in Compare, by its account's id, "" for
	// all accounts, which moves the focus to it.
	OnColumn
)

// Targets are the zones of the frame's page of doc at now a click acts on,
// each over the cells it's drawn on, laid out as the page draws them: its
// tabs; the heading row's sub-tabs, and < and > by its period or window; a
// session's page's, where one is open, as sessionTargets has them, else the
// Overview's, as overviewTargets has them, Accounts', as accountsTargets
// has them, Sessions', as sessionsTargets has them, Graphs', as
// graphsTargets has them, or the Log's, as logTargets has them; and the key
// line's keys, as keyTargets has them.
func (f Frame) Targets(doc status.Document, now time.Time) []Target {
	targets := f.tabTargets()
	for _, p := range f.headingPieces() {
		if p.click.On != 0 {
			targets = append(targets, p.target())
		}
	}
	if !f.cramped() {
		targets = append(targets, f.viewTargets(doc, now)...)
	}
	return append(targets, f.keyTargets()...)
}

// viewTargets are the zones of what the frame shows of doc at now a click
// acts on, as Targets lays them out: a session's page's, the Overview's,
// Accounts', Sessions', Graphs' and the Log's.
func (f Frame) viewTargets(doc status.Document, now time.Time) []Target {
	switch f.Shown() {
	case SessionPage:
		return f.At(doc, now).sessionTargets(doc, now)
	case Overview:
		return f.At(doc, now).overviewTargets(doc, now)
	case Accounts:
		return f.At(doc, now).accountsTargets(doc, now)
	case Sessions:
		return f.At(doc, now).sessionsTargets(doc, now)
	case Graphs:
		return f.graphsTargets()
	case Log:
		return f.logTargets(now)
	}
	return nil
}

// tabTargets are the tabs, as tabs draws them.
func (f Frame) tabTargets() []Target {
	tabs := f.tabsAt()
	targets := make([]Target, 0, len(tabs))
	for _, t := range tabs {
		targets = append(targets, Target{X: t.x, Y: t.y, Width: f.tab(t.view).width(), On: OnTab, Tab: t.view})
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
