package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Section is one of the Overview's sections, which its key shows and hides.
type Section int

// The Overview's sections, in the order the page draws them.
const (
	// SectionUsage is USAGE: every account's windows, usage's block.
	SectionUsage Section = iota
	// SectionGraphs is GRAPHS: the focused account's use so far in each
	// window, under USAGE's columns.
	SectionGraphs
	// SectionRouting is ROUTING: how each account stands with the router,
	// and its requests over the last 30 minutes.
	SectionRouting
	// SectionSessions is SESSIONS: the sessions running, by account.
	SectionSessions
	// SectionComing is COMING UP: what happens next, soonest first.
	SectionComing
	// SectionLately is LATELY: what's happened, newest first.
	SectionLately
)

// sectionWords are each section's key, its word, as the key line words it
// and its heading has it in capitals, and its name in the preferences file.
var sectionWords = [...]struct{ key, word, name string }{
	SectionUsage:    {"u", "usage", "usage"},
	SectionGraphs:   {"g", "graphs", "graphs"},
	SectionRouting:  {"r", "routing", "routing"},
	SectionSessions: {"s", "sessions", "sessions"},
	SectionComing:   {"c", "coming up", "coming"},
	SectionLately:   {"l", "lately", "lately"},
}

// EverySection are the Overview's sections, in the order the page draws
// them.
func EverySection() []Section {
	return []Section{SectionUsage, SectionGraphs, SectionRouting, SectionSessions, SectionComing, SectionLately}
}

// SectionKeyed is the section the key given shows and hides, as "u" does
// USAGE, reporting false for a key that's no section's.
func SectionKeyed(key string) (Section, bool) {
	for _, s := range EverySection() {
		if s.Key() == key {
			return s, true
		}
	}
	return 0, false
}

// Key is the key that shows and hides the section, as "u".
func (s Section) Key() string {
	return sectionWords[s].key
}

// Word is the section's word, as the key line has it after its key, as
// "coming up".
func (s Section) Word() string {
	return sectionWords[s].word
}

// heading is the section's name, as its heading has it, as "COMING UP".
func (s Section) heading() string {
	return strings.ToUpper(s.Word())
}

// Sections are which of the Overview's sections show. The zero value shows
// those shown at first: USAGE, ROUTING, COMING UP and LATELY.
type Sections struct {
	// toggled are the sections shown otherwise than at first, a bit each,
	// so the zero value shows those shown at first.
	toggled uint8
}

// shownAtFirst are the sections shown at first, a bit each.
const shownAtFirst = 1<<SectionUsage | 1<<SectionRouting | 1<<SectionComing | 1<<SectionLately

// Has reports whether the section shows.
func (s Sections) Has(section Section) bool {
	return (shownAtFirst^s.toggled)&(1<<section) != 0
}

// Toggled is s with the section hidden where it shows, and shown where it's
// hidden.
func (s Sections) Toggled(section Section) Sections {
	return Sections{toggled: s.toggled ^ 1<<section}
}

// Names are the names of the sections that show, in the order the page draws
// them, as the preferences file keeps them: never nil, so none showing is
// kept as none.
func (s Sections) Names() []string {
	names := []string{}
	for _, section := range EverySection() {
		if s.Has(section) {
			names = append(names, sectionWords[section].name)
		}
	}
	return names
}

// SectionsNamed are the sections the names given show, as the preferences
// file keeps them, a name of none of them left out, as a later
// switchboard's: or, given nil, as a file that keeps none has it, those
// shown at first.
func SectionsNamed(names []string) Sections {
	if names == nil {
		return Sections{}
	}
	var shown uint8
	for _, section := range EverySection() {
		for _, name := range names {
			if name == sectionWords[section].name {
				shown |= 1 << section
			}
		}
	}
	return Sections{toggled: shown ^ shownAtFirst}
}

// bounds are where something is drawn: from column x along row y, width cells
// wide and rows tall.
type bounds struct {
	x, y, width, rows int
}

// sectionHead are the rows a section takes over its body: its heading, and
// the blank row under it.
const sectionHead = 2

// section is a section shown of the Overview, as the planner lays it out and
// the page draws it, of the frame's document at a moment: which it is; the
// fewest rows it takes, and the most, sectionHead among them; its heading's
// description, where it has one; what draws its body, under its heading's
// blank row, in the bounds the planner gives it; what its heading says it
// holds where it's folded; and its click targets, as its body is drawn in the
// bounds given. Each is built in the section's own file.
type section struct {
	Section
	least, most int
	description func() line
	draw        func(c *canvas, body bounds)
	folded      func() line
	targets     func(body bounds) []Target
}

// sectionsOf are the Overview's sections the frame shows, of doc at now, in
// the order the page draws them.
func (f Frame) sectionsOf(doc status.Document, now time.Time) []section {
	builds := [...]func(status.Document, time.Time) section{
		SectionUsage:    f.usageSection,
		SectionGraphs:   f.graphsSection,
		SectionRouting:  f.routingSection,
		SectionSessions: f.sessionsSection,
		SectionComing:   f.comingSection,
		SectionLately:   f.latelySection,
	}
	var shown []section
	for _, s := range EverySection() {
		if f.Sections.Has(s) {
			shown = append(shown, builds[s](doc, now))
		}
	}
	return shown
}

// planned is a section as the planner places it: the row its heading is on;
// whether it folds to its heading, which then says what it holds; and, where
// it doesn't, its body's bounds, under the blank row after its heading.
type planned struct {
	section
	heading int
	fold    bool
	body    bounds
}

// plan lays the sections out in rows rows from row top, in their order, each
// at its least rows, a blank row between two, but for one that takes no rows
// of its own; and one whose least rows don't fit in the rows left folds to its
// heading, where it folds.
func (f Frame) plan(sections []section, top, rows int) []planned {
	var laid []planned
	y := top
	for _, s := range sections {
		if s.least == 0 {
			continue
		}
		p := planned{section: s, heading: y, body: bounds{y: y + sectionHead, width: f.Width, rows: s.least - sectionHead}}
		p.fold = y+s.least > top+rows && s.folded != nil
		laid = append(laid, p)
		if p.fold {
			y += sectionHead
		} else {
			y += s.least + 1
		}
	}
	return laid
}

// overview draws the Overview of doc at now in the view's rows, from row top:
// each section shown, as plan places it, its heading its name, then three
// spaces and what it holds, where it folds, else its description; and under
// a section that doesn't fold, its body.
func (f Frame) overview(c *canvas, doc status.Document, now time.Time, top int) {
	view := newCanvas(f.Width, f.viewRows())
	for _, p := range f.plan(f.sectionsOf(doc, now), 0, view.height()) {
		heading := line{{p.Section.heading(), dimInk}}
		switch {
		case p.fold:
			heading = slices.Concat(heading, line{spaces(3)}, p.folded())
		case p.description != nil:
			heading = slices.Concat(heading, line{spaces(3)}, p.description())
		}
		view.line(0, p.heading, heading.fit(f.Width))
		if !p.fold {
			p.draw(view, p.body)
		}
	}
	c.paste(view, 0, top, view.height())
}

// overviewTargets are the Overview's click targets of doc at now: those of
// each section that has any, as plan places it, but for one that folds.
func (f Frame) overviewTargets(doc status.Document, now time.Time) []Target {
	var targets []Target
	for _, p := range f.plan(f.sectionsOf(doc, now), f.top(), f.viewRows()) {
		if p.targets != nil && !p.fold {
			targets = append(targets, p.targets(p.body)...)
		}
	}
	return targets
}

// overviewScrolling is how far the Overview's USAGE scrolls within its
// section, of doc at now: nowhere.
func (Frame) overviewScrolling(status.Document, time.Time) Scrolling {
	return Scrolling{}
}
