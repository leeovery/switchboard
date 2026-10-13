package dashboard

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
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

// Toggles are the sections' keys, as the key line lists them, in the order
// the page draws them: each key and its word, its word faint where its
// section is hidden.
func (s Sections) Toggles() []Key {
	toggles := make([]Key, 0, len(sectionWords))
	for _, section := range EverySection() {
		toggles = append(toggles, Key{Key: section.Key(), Does: section.Word(), Hidden: !s.Has(section)})
	}
	return toggles
}

// none reports whether every section is hidden.
func (s Sections) none() bool {
	return !slices.ContainsFunc(EverySection(), s.Has)
}

// lists reports whether the section lists lines, as COMING UP and LATELY
// do: side by side, but on a phone, they fold together, each to its first
// line, and grow a line at a time.
func (s Section) lists() bool {
	return s == SectionComing || s == SectionLately
}

// bounds are where something is drawn: from column x along row y, width cells
// wide and rows tall.
type bounds struct {
	x, y, width, rows int
}

// holds reports whether the cell at x along row y is within the bounds.
func (b bounds) holds(x, y int) bool {
	return b.x <= x && x < b.x+b.width && b.y <= y && y < b.y+b.rows
}

// sectionHead are the rows a section takes over its body: its heading, and
// the blank row under it.
const sectionHead = 2

// section is a section shown of the Overview, as the planner lays it out and
// the page draws it, of the frame's document at a moment: which it is; the
// fewest rows it takes, and the most, sectionHead among them, or none, where
// it's drawn beside a section that leaves room for it, as aside says; its
// heading's description, where it has one; what draws its body, under its
// heading's blank row, in the bounds the planner gives it, and nothing
// outside them; what it holds, as its heading says where it's folded, the
// page adding why, or that it has more, as says has it; its click targets,
// as its body is drawn in the bounds given; and how far its body scrolls in
// them. Each is built in the section's own file.
type section struct {
	Section
	// least and most are the fewest rows the section takes and the most,
	// but for one that lists lines, whose most lines tells, as mostRows has
	// it.
	least, most int
	// aside is the column a section of no rows of its own is drawn from
	// beside this one, in its rows, as ROUTING is beside USAGE's blocks by
	// account: 0 where none can be.
	aside int
	// lines is how many lines a section that lists them holds: it takes a
	// row a line at its most, as linesRows has it, and folded, its heading
	// says where it holds more than its first. Its rows fill with what comes
	// and goes, so only a layout made afresh, or its heading folded, asks.
	lines       func() int
	description func() line
	draw        func(c *canvas, body bounds)
	folded      func() line
	targets     func(body bounds) []Target
	// scrolling is how far the body scrolls in rows rows, scrolled offset
	// accounts down, as USAGE's does where its accounts are many: Most, the
	// most accounts it scrolls out of view, and Page, how many show at once
	// from the offset. Nil where it doesn't.
	scrolling func(rows, offset int) Scrolling
}

// mostRows is the most rows the section takes: its most, or for one that
// lists lines, a row a line it holds, as linesRows has it.
func (s section) mostRows() int {
	if s.lists() {
		return linesRows(s.lines())
	}
	return s.most
}

// sectionsBuilt are the Overview's sections of a frame, as Built builds
// them, of a document at a moment: built once, the first time they're asked
// for.
type sectionsBuilt struct {
	at       *status.HeldMoment
	sections func() []section
}

// Built is the frame with the Overview's sections of doc at now built once,
// the first time a look at them asks, and shared by every look it, or a copy
// of it, takes of doc at now after: its layout, its page and its numbers, its
// click targets and how it scrolls. They're built of the frame as it stands,
// holding the moment of doc at now, as At has it, so a copy whose Layout,
// KeyLine, Brightened or Card change draws them as a frame built afresh
// would, as they draw none of those, and a copy changed otherwise draws them
// as they were built. A look at another moment, or of another document,
// builds them afresh.
func (f Frame) Built(doc status.Document, now time.Time) Frame {
	f = f.At(doc, now)
	of := f
	f.built = &sectionsBuilt{at: f.held, sections: sync.OnceValue(func() []section { return of.buildSections(doc, now) })}
	return f
}

// sectionsOf are the Overview's sections the frame shows, of doc at now, in
// the order the page draws them: as Built built them, where it built them of
// doc at now, else afresh.
func (f Frame) sectionsOf(doc status.Document, now time.Time) []section {
	if f.built != nil && f.built.at.Of(doc, now, f.Policy) {
		return f.built.sections()
	}
	return f.buildSections(doc, now)
}

// buildSections builds the Overview's sections the frame shows, of doc at
// now, in the order the page draws them, each of the one moment of doc at
// now, as At holds it.
func (f Frame) buildSections(doc status.Document, now time.Time) []section {
	f = f.At(doc, now)
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

// planned is a section as the planner places it: its heading's row, and the
// columns it spans; whether it folds to its heading, which then says what it
// holds; and, where it doesn't, its body's bounds, under the blank row after
// its heading.
type planned struct {
	section
	head bounds
	fold bool
	body bounds
}

// foldOrder are the sections in the order they fold to their headings, where
// rows run short: SESSIONS; then COMING UP and LATELY together; then GRAPHS;
// then ROUTING. USAGE never folds.
var foldOrder = [][]Section{{SectionSessions}, {SectionComing, SectionLately}, {SectionGraphs}, {SectionRouting}}

// growth is the order the rows to spare go in, where nothing folds, each
// section to its most: to GRAPHS, a row at a time; to USAGE's big bars, then
// to ROUTING's two rows of trace, each where it fits whole; then to COMING UP
// and LATELY, a line at a time.
var growth = []struct {
	sections []Section
	grow     func(units []*unit, spare int) int
}{
	{[]Section{SectionGraphs}, rowByRow},
	{[]Section{SectionUsage}, whole},
	{[]Section{SectionRouting}, whole},
	{[]Section{SectionComing, SectionLately}, rowByRow},
}

// unit is sections that share rows: a section alone; USAGE with a section
// beside it; or COMING UP and LATELY side by side. It takes rows rows, its
// headings' among them, from least to most, as its sections' most rows; or
// where it folds, its headings' row alone.
type unit struct {
	sections    []section
	least, rows int
	fold        bool
}

// join has s share the unit's rows, beside its sections.
func (u *unit) join(s section) {
	u.sections = append(u.sections, s)
	u.least = max(u.least, s.least)
}

// most is the most rows the unit takes: the most its sections take, as
// mostRows has each.
func (u unit) most() int {
	most := 0
	for _, s := range u.sections {
		most = max(most, s.mostRows())
	}
	return most
}

// ledBy reports whether the unit's first section is among those given: a
// unit folds and grows as the section it's led by does.
func (u unit) ledBy(sections ...Section) bool {
	return slices.Contains(sections, u.sections[0].Section)
}

// taken is how many rows the unit takes: its rows, or its headings' row
// alone where it folds.
func (u unit) taken() int {
	if u.fold {
		return 1
	}
	return u.rows
}

// blankBetween reports whether a blank row separates two units, one after
// the other: always, but between two that fold.
func blankBetween(before, after unit) bool {
	return !before.fold || !after.fold
}

// rowsTaken is how many rows the units take, laid out in turn, a blank row
// between two, as blankBetween says.
func rowsTaken(units []unit) int {
	n := 0
	for i, u := range units {
		n += u.taken()
		if i > 0 && blankBetween(units[i-1], u) {
			n++
		}
	}
	return n
}

// unitsOf are the units the sections are laid out in, in their order: a
// section of no rows of its own beside the one that leaves room for it, as
// aside says, and where none does, nowhere; LATELY beside COMING UP, but on
// a phone, where it's under it; and each other section alone.
func (f Frame) unitsOf(sections []section) []unit {
	var units []unit
	for _, s := range sections {
		i, joins := f.joins(units, s)
		switch {
		case joins:
			units[i].join(s)
		case s.least > 0:
			units = append(units, unit{sections: []section{s}, least: s.least})
		}
	}
	return units
}

// joins is which of the units laid out so far the section shares rows with,
// reporting false where it takes rows of its own: for a section of no rows of
// its own, the one that leaves room beside it; for one that lists lines, the
// last, where it lists them too, but on a phone.
func (f Frame) joins(units []unit, s section) (int, bool) {
	if s.least == 0 {
		i := slices.IndexFunc(units, func(u unit) bool { return u.sections[0].aside > 0 })
		return i, i >= 0
	}
	last := len(units) - 1
	return last, s.lists() && !f.phone() && last >= 0 && units[last].sections[0].lists()
}

// allot gives each unit its rows in room rows, and returns them: its least,
// to start with. Where they don't fit, units fold to their headings, as
// foldOrder has them, until they do; and where they still don't, USAGE
// scrolls within the rows left to it. Where they fit with none folded, the
// rows to spare go as growth has them, and the rest stay empty at the foot.
func allot(units []unit, room int) []unit {
	for i := range units {
		units[i].rows = units[i].least
	}
	for _, folds := range foldOrder {
		if rowsTaken(units) <= room {
			break
		}
		for i := range units {
			units[i].fold = units[i].fold || units[i].ledBy(folds...)
		}
	}
	switch over := rowsTaken(units) - room; {
	case over > 0:
		squeeze(units, over)
	case !slices.ContainsFunc(units, func(u unit) bool { return u.fold }):
		grow(units, -over)
	}
	return units
}

// squeeze takes over rows from USAGE, which scrolls within the rest, its
// heading kept.
func squeeze(units []unit, over int) {
	if i := slices.IndexFunc(units, func(u unit) bool { return u.ledBy(SectionUsage) }); i >= 0 {
		units[i].rows = max(units[i].rows-over, sectionHead)
	}
}

// grow gives the units spare rows, as growth has them.
func grow(units []unit, spare int) {
	for _, g := range growth {
		var grown []*unit
		for i := range units {
			if units[i].ledBy(g.sections...) {
				grown = append(grown, &units[i])
			}
		}
		spare = g.grow(grown, spare)
	}
}

// rowByRow grows the units a row at a time, in turn, each to its most, while
// there are rows to spare, and returns how many are left.
func rowByRow(units []*unit, spare int) int {
	for grew := true; grew && spare > 0; {
		grew = false
		for _, u := range units {
			if u.rows < u.most() && spare > 0 {
				u.rows, spare, grew = u.rows+1, spare-1, true
			}
		}
	}
	return spare
}

// whole grows each unit to its most where that fits whole in the rows to
// spare, and returns how many are left.
func whole(units []*unit, spare int) int {
	for _, u := range units {
		if need := u.most() - u.rows; need > 0 && need <= spare {
			u.rows, spare = u.rows+need, spare-need
		}
	}
	return spare
}

// place lays the units out in turn from row top, a blank row between two, as
// blankBetween says: each section's heading on its unit's first row, in the
// columns columns gives it, and where it doesn't fold, its body under its
// heading's blank row, in the rest of its unit's rows.
func (f Frame) place(units []unit, top int) []planned {
	var laid []planned
	y := top
	for i, u := range units {
		if i > 0 && blankBetween(units[i-1], u) {
			y++
		}
		for j, s := range u.sections {
			head, body := f.columns(u, j)
			head.y, head.rows = y, 1
			p := planned{section: s, head: head, fold: u.fold}
			if !u.fold {
				body.y, body.rows = y+sectionHead, u.rows-sectionHead
				p.body = body
			}
			laid = append(laid, p)
		}
		y += u.taken()
	}
	return laid
}

// columns are the columns section j of the unit takes, its heading's and its
// body's. A section alone takes the frame's width. One beside USAGE takes
// those from USAGE's aside to the edge, USAGE's heading ending headingGap
// short of them, and its body the whole width. COMING UP and LATELY side by
// side take comingWidth from the first column, and from latelyAt to the
// edge.
func (f Frame) columns(u unit, j int) (head, body bounds) {
	whole := bounds{width: f.Width}
	aside := u.sections[0].aside
	switch {
	case len(u.sections) == 1:
		return whole, whole
	case aside > 0 && j == 0:
		return bounds{width: aside - headingGap}, whole
	case aside > 0:
		beside := bounds{x: aside, width: f.Width - aside}
		return beside, beside
	case j == 0:
		coming := bounds{width: comingWidth(f.Width)}
		return coming, coming
	}
	at := latelyAt(f.Width)
	lately := bounds{x: at, width: f.Width - at}
	return lately, lately
}

// comingWidth is how wide COMING UP is beside LATELY, on a page width cells
// wide: round(w × 0.45).
func comingWidth(width int) int {
	return int(math.Round(float64(width) * 0.45))
}

// latelyAt is the column LATELY starts at beside COMING UP, on a page width
// cells wide: ⌊w / 2⌋ + 2.
func latelyAt(width int) int {
	return width/2 + 2
}

// Layout is the Overview's layout as it was made: the rows each section shown
// was given, its heading's among them, and which fold; and what it was made
// for, as layoutFor says. The watch keeps it from look to look, as LayoutOf
// gives it, so the layout changes only on a key or a resize, or a section's
// shape changing, never as sessions start and lines come: SESSIONS keeps the
// rows it was given however many sessions run, and COMING UP and LATELY
// theirs however many lines they have. A frame lays the Overview out afresh
// where its Layout is the zero Layout, or one made for another frame or
// shape.
type Layout struct {
	made     layoutFor
	allotted [len(sectionWords)]allotment
}

// layoutFor is what a layout is made for: a frame of a size, showing the
// sections it shows, grouped as it groups them; and the shape of each section
// shown whose rows don't fill with what comes and goes, so a layout isn't
// kept once one changes shape, as USAGE's block grows as its accounts'
// windows are first read.
type layoutFor struct {
	width, height int
	sections      Sections
	by            By
	shapes        [len(sectionWords)]shape
}

// shape is the fewest rows a section takes, and the most.
type shape struct {
	least, most int
}

// allotment is the rows a layout gives a section, and whether it folds.
type allotment struct {
	rows int
	fold bool
}

// fills reports whether the section's rows fill with what comes and goes, as
// SESSIONS' do with the sessions running, and COMING UP's and LATELY's with
// their lines: a layout kept keeps the rows it gave them, however many they
// hold.
func (s Section) fills() bool {
	return s == SectionSessions || s.lists()
}

// LayoutOf is the Overview's layout as the frame lays it out of doc at now,
// for the watch to keep for the next look: the zero Layout off the Overview.
func (f Frame) LayoutOf(doc status.Document, now time.Time) Layout {
	if f.View != Overview {
		return Layout{}
	}
	f = f.At(doc, now)
	units := f.unitsOf(f.sectionsOf(doc, now))
	made := f.layoutFor(units)
	if f.keeps(made) {
		return f.Layout
	}
	l := Layout{made: made}
	for _, u := range allot(units, f.viewRows()) {
		l.allotted[u.sections[0].Section] = allotment{rows: u.rows, fold: u.fold}
	}
	return l
}

// layoutFor is what a layout of the units, as the frame lays them out, is
// made for.
func (f Frame) layoutFor(units []unit) layoutFor {
	made := layoutFor{width: f.Width, height: f.Height, sections: f.Sections, by: f.By}
	for _, u := range units {
		for _, s := range u.sections {
			if !s.fills() {
				made.shapes[s.Section] = shape{least: s.least, most: s.mostRows()}
			}
		}
	}
	return made
}

// keeps reports whether the frame keeps its Layout for a layout made for
// made: one made for it, which isn't the zero Layout.
func (f Frame) keeps(made layoutFor) bool {
	return f.Layout != (Layout{}) && f.Layout.made == made
}

// allotted are the units of the Overview's sections of doc at now, each
// given its rows: as the frame's Layout gives them, where it keeps it; else
// afresh, as allot gives them in the view's rows.
func (f Frame) allotted(doc status.Document, now time.Time) []unit {
	units := f.unitsOf(f.sectionsOf(doc, now))
	if f.keeps(f.layoutFor(units)) {
		return f.Layout.give(units)
	}
	return allot(units, f.viewRows())
}

// give gives each unit the rows, and the fold, the layout gave the section
// it's led by, and returns them.
func (l Layout) give(units []unit) []unit {
	for i := range units {
		a := l.allotted[units[i].sections[0].Section]
		units[i].rows, units[i].fold = a.rows, a.fold
	}
	return units
}

// laidOut are the Overview's sections of doc at now as the page lays them
// out from row top, given their rows as allotted gives them.
func (f Frame) laidOut(doc status.Document, now time.Time, top int) []planned {
	return f.place(f.allotted(doc, now), top)
}

// overview draws the Overview of doc at now in the view's rows, from row
// top, as overviewDrawn draws it.
func (f Frame) overview(c *canvas, doc status.Document, now time.Time, top int) {
	view := f.overviewDrawn(doc, now)
	c.paste(view, 0, top, view.height())
}

// overviewDrawn is the Overview of doc at now drawn in the view's rows: where
// every section is hidden, how to bring them back; else each section shown,
// as laidOut lays it out, its heading, and under one that doesn't fold, its
// body; its traces painted once they're all drawn, at the scale they share,
// as paintTraces paints them.
func (f Frame) overviewDrawn(doc status.Document, now time.Time) *canvas {
	view := holdingTraces(newCanvas(f.Width, f.viewRows()))
	if f.Sections.none() {
		f.everythingHidden(view)
	}
	f.drawSections(view, f.laidOut(doc, now, 0))
	f.paintTraces(view)
	return view
}

// drawSections draws the sections as they're laid out: each its heading, and
// under one that doesn't fold, its body, within its body's bounds.
func (f Frame) drawSections(c *canvas, laid []planned) {
	for _, p := range laid {
		f.sectionHeading(c, p)
		if !p.fold {
			p.draw(c.within(p.body), p.body)
		}
	}
}

// leastRule is the fewest cells of rule a section's heading keeps between
// what it says and its key.
const leastRule = 1

// sectionHeading draws the section's heading along its row, in the columns
// it spans: its name in text.subtle; three cells on, what it says, as says
// has it, where it says anything, cut to keep leastRule cells of rule; a
// cell after that, or two after the name, a rule in border; and a cell on,
// at its last column, the key that hides it, in bold accent.key.
func (f Frame) sectionHeading(c *canvas, p planned) {
	h := p.head
	key := h.x + h.width - 1
	x := c.line(h.x, h.y, line{{p.heading(), dimInk}})
	ruled := x + ruleGap
	if says := f.says(p); len(says) > 0 {
		at := x + headingGap
		ruled = c.line(at, h.y, says.fit(key-at-leastRule-2)) + 1
	}
	c.text(ruled, h.y, rule(key-1-ruled), borderInk)
	c.text(key, h.y, p.Key(), keyInk)
}

// says is what the section's heading says after its name: where it folds,
// what it holds, as its folded has it, then, for a section that lists lines,
// where it holds more than that, two cells on, an ellipsis, or for any other,
// three on, that there's no room for it at the frame's height, in
// text.subtle; else its description, where it has one.
func (f Frame) says(p planned) line {
	if !p.fold {
		return saying(p.description)
	}
	holds := saying(p.folded)
	switch {
	case p.lists() && p.lines() > 1:
		return slices.Concat(holds, line{{"  " + prose.Ellipsis, dimInk}})
	case p.lists():
		return holds
	}
	noRoom := line{{"no room at " + strconv.Itoa(f.Height) + " rows", dimInk}}
	if len(holds) == 0 {
		return noRoom
	}
	return slices.Concat(holds, line{spaces(headingGap)}, noRoom)
}

// saying is the line words gives, or none where it's nil.
func saying(words func() line) line {
	if words == nil {
		return nil
	}
	return words()
}

// everythingHidden draws, from the first row, how to bring the sections back
// where every one is hidden: "every section is hidden:" in text.faint, then
// the toggles, as the key line words them, keysInGroup apart, as many to a
// row as fit, the rest on the rows under it.
func (f Frame) everythingHidden(c *canvas) {
	x, y := c.line(0, 0, line{{"every section is hidden:", faintInk}}), 0
	for _, k := range f.Sections.Toggles() {
		words, gap := k.words(), keysInGroup
		if x+gap+words.width() > f.Width {
			x, y, gap = 0, y+1, 0
		}
		x = c.line(x+gap, y, words)
	}
}

// overviewTargets are the Overview's click targets of doc at now, as
// targetsOf has them of its sections as laidOut lays them out on the page.
func (f Frame) overviewTargets(doc status.Document, now time.Time) []Target {
	return f.targetsOf(f.laidOut(doc, now, f.top()))
}

// targetsOf are the click targets of the sections as they're laid out on the
// page: each section's heading, which hides it, and the targets of the body
// of each that doesn't fold; but none past the view's rows.
func (f Frame) targetsOf(laid []planned) []Target {
	var targets []Target
	for _, p := range laid {
		targets = append(targets, Target{X: p.head.x, Y: p.head.y, Width: p.head.width, On: OnSection, Section: p.Section})
		if p.targets != nil && !p.fold {
			targets = append(targets, p.targets(p.body)...)
		}
	}
	end := f.top() + f.viewRows()
	return slices.DeleteFunc(targets, func(t Target) bool { return t.Y >= end })
}

// overviewScrolling is how far the Overview's USAGE of doc at now scrolls
// within its section, an account at a time, scrolled to each offset given, as
// scrollingOf has it of its sections as laidOut lays them out on the page,
// once for every offset.
func (f Frame) overviewScrolling(doc status.Document, now time.Time) func(offset int) Scrolling {
	laid := f.laidOut(doc, now, f.top())
	return func(offset int) Scrolling { return scrollingOf(laid, offset) }
}

// scrollingOf is how far USAGE, of the sections as they're laid out, scrolls
// within its section, scrolled offset accounts down, as its scrolling says
// in the rows of body it's given: nowhere where it's hidden, or doesn't
// scroll.
func scrollingOf(laid []planned, offset int) Scrolling {
	usage := usageOf(laid)
	if usage.scrolling == nil {
		return Scrolling{}
	}
	return usage.scrolling(usage.body.rows, offset)
}

// overviewScrolls is what of the Overview of doc at now scrolls, from column
// x along row y, width cells wide and rows tall: USAGE's body, as laidOut
// lays it out on the page; nothing where USAGE is hidden.
func (f Frame) overviewScrolls(doc status.Document, now time.Time) (x, y, width, rows int) {
	body := usageOf(f.laidOut(doc, now, f.top())).body
	return body.x, body.y, body.width, body.rows
}

// usageOf is USAGE of the sections as they're laid out: the zero planned
// where it's hidden.
func usageOf(laid []planned) planned {
	for _, p := range laid {
		if p.Section == SectionUsage {
			return p
		}
	}
	return planned{}
}
