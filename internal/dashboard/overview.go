package dashboard

import (
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
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
// do: side by side, where listsBeside says, they fold together, each to its
// first line, and grow a line at a time.
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

// and is where the bounds and o overlap: none where they don't.
func (b bounds) and(o bounds) bounds {
	x, y := max(b.x, o.x), max(b.y, o.y)
	return bounds{x: x, y: y, width: max(min(b.x+b.width, o.x+o.width)-x, 0), rows: max(min(b.y+b.rows, o.y+o.rows)-y, 0)}
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
	// it; between are the rows past its least and short of its most it may
	// grow to whole, as whole grows it, the fewest first.
	least, most int
	between     []int
	// leaner are the rows it takes short of its least, a step at a time, as
	// USAGE's block gives way to rows short, as it never folds.
	leaner []int
	// accounts is how many accounts it shows at the step given, in rows
	// rows, its heading's among them, scrolled to its top: nil where it
	// shows none.
	accounts func(given giving, rows int) int
	// leaned is the section given way as far as the step given.
	leaned func(given giving) section
	// heads are the rows its heading takes over its body, its own among
	// them, as USAGE's gives way its blank row: 0 for sectionHead.
	heads int
	// aside is the column a section of no rows of its own is drawn from
	// beside this one, in its rows, as ROUTING is beside USAGE's blocks by
	// account: 0 where none can be.
	aside int
	// lines is how many lines a section that lists them holds: it takes a
	// row a line at its most, as linesRows has it, and folded, its heading
	// says where it holds more than its first. Its rows fill with what comes
	// and goes, so only a layout made afresh, or its heading folded, asks.
	lines func() int
	// description and folded are what its heading says, unfolded and
	// folded, in the cells its heading has for them, as says gives them:
	// a section of several forms says the fullest that fits; one of a
	// single form, as oneForm has it, says it at any width.
	description headingWords
	// keyed reports whether its description is items blank spans apart, as
	// a key's, which give way whole, as fitItems cuts them, where its
	// heading has no room for them all.
	keyed   bool
	draw    func(c *canvas, body bounds)
	folded  headingWords
	targets func(body bounds) []Target
	// scrolling is how far the body scrolls in rows rows, scrolled offset
	// down, a row at a time, as USAGE's does where its accounts are many:
	// Most, the furthest, and Page, how many show at once from the offset;
	// and revealing, how far it's scrolled from offset to show the account
	// with the given id. Nil where it doesn't scroll.
	scrolling func(rows, offset int) Scrolling
	revealing func(rows, offset int, id string) int
	// scrollForm is the form its body's rows are laid out in, in rows rows,
	// which an offset into them is read in, as USAGE's usageForm has it.
	// Nil where it doesn't scroll.
	scrollForm func(rows int) usageForm
	// order is the order of the rows SESSIONS lists, which a layout made
	// keeps; and steady is SESSIONS listing them in the order a layout kept,
	// as give gives it. Nil for any other section.
	order  []views.RunningRow
	steady func(kept []views.RunningRow) section
}

// mostRows is the most rows the section takes: its most, or for one that
// lists lines, a row a line it holds, as linesRows has it.
func (s section) mostRows() int {
	if s.lists() {
		return linesRows(s.lines())
	}
	return s.most
}

// viewsBuilt are the Overview's sections of a frame and Accounts' form, as
// Built builds them, of a document at a moment: each built once, the first
// time it's asked for.
type viewsBuilt struct {
	at       *status.HeldMoment
	sections func() []section
	accounts func() accountsForm
}

// Built is the frame with the Overview's sections and Accounts' form of doc
// at now each built once, the first time a look at them asks, and shared by
// every look it, or a copy of it, takes of doc at now after: its layout, its
// page and its numbers, its click targets, how it scrolls and its bars
// filled. They're built of the frame as it stands, holding the moment of doc
// at now, as At has it, so a copy whose Layout, KeyLine, Brightened, Card or
// View change draws them as a frame built afresh would, as they draw none of
// those, and a copy changed otherwise draws them as they were built. A look
// at another moment, or of another document, builds them afresh.
func (f Frame) Built(doc status.Document, now time.Time) Frame {
	f = f.At(doc, now)
	of := f
	f.built = &viewsBuilt{
		at:       f.held,
		sections: sync.OnceValue(func() []section { return of.buildSections(doc, now) }),
		accounts: sync.OnceValue(func() accountsForm { return of.buildAccountsForm(doc, now) }),
	}
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
// now, as At holds it, and of the one list of the sessions running then, as
// holdRunning holds it.
func (f Frame) buildSections(doc status.Document, now time.Time) []section {
	f = f.At(doc, now).holdRunning(doc, now)
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

// foldOrder are the sections in the order they give way, where rows run
// short: USAGE first, which never folds, but gives way the blank rows between
// its accounts, its first step short of its least; then, each folding to its
// headings, SESSIONS; then COMING UP and LATELY together; then GRAPHS; then
// ROUTING.
var foldOrder = [][]Section{{SectionUsage}, {SectionSessions}, {SectionComing, SectionLately}, {SectionGraphs}, {SectionRouting}}

// growth is the order the rows to spare go in, to the units not folded, each
// to its most: to SESSIONS, a whole session at a time; to GRAPHS, a row at a
// time; to USAGE's big bars, then to ROUTING's two rows of trace, each where
// it fits whole; then to COMING UP and LATELY, a line at a time.
var growth = []struct {
	sections []Section
	grow     func(units []*unit, spare int) int
}{
	{[]Section{SectionSessions}, whole},
	{[]Section{SectionGraphs}, rowByRow},
	{[]Section{SectionUsage}, whole},
	{[]Section{SectionRouting}, whole},
	{[]Section{SectionComing, SectionLately}, rowByRow},
}

// unit is sections that share rows: a section alone; USAGE with a section
// beside it; or COMING UP and LATELY side by side. It takes rows rows, its
// headings' among them, from least to most, as its sections' most rows; or
// where it folds, its headings' row alone; or led by USAGE, short of its
// least, as far as lean has its block give way.
type unit struct {
	sections    []section
	least, rows int
	fold        bool
	lean        giving
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

// giveWay has the unit give way where rows run short: USAGE, which never
// folds, its blank rows between accounts, the first step short of its least
// its leaner has, where it has one; any other, folded to its headings.
func (u *unit) giveWay() {
	if !u.ledBy(SectionUsage) {
		u.fold = true
		return
	}
	u.leanTo(givenApart)
}

// leanTo has USAGE's unit give way as far as the step given, taking the
// rows its section takes there, as rowsAt has them, where its leaner has the
// step; or, given none, its least.
func (u *unit) leanTo(given giving) {
	switch s := u.sections[0]; {
	case given == givenNone:
		u.rows, u.lean = u.least, givenNone
	case int(given) <= len(s.leaner):
		u.rows, u.lean = s.rowsAt(given), given
	}
}

// showsMost is the step USAGE's unit shows the most accounts at in rows
// rows, as its accounts counts them, the least far of those that tie whose
// key shows, as keyShows has it: a key scrolled out of view says nothing, so
// the step that gives it way, showing as many, shows what that one would
// over it, the pool's rows among them. Where none before it does, it's the
// last step, which shows as many as any before it, its key given way, or
// with none to give.
func (u unit) showsMost(rows int) giving {
	s := u.sections[0]
	if s.accounts == nil {
		return givenNone
	}
	last, most := giving(len(s.leaner)), 0
	for given := givenNone; given <= last; given++ {
		most = max(most, s.accounts(given, rows))
	}
	for given := range last {
		if s.keyShows(given, rows) && s.accounts(given, rows) == most {
			return given
		}
	}
	return last
}

// rowsAt are the rows USAGE's section takes whole at the step given: its
// least as it is, else the rows its leaner has for the step.
func (s section) rowsAt(given giving) int {
	if given == givenNone {
		return s.least
	}
	return s.leaner[given-1]
}

// keyShows reports whether USAGE's key shows in rows rows at the step given:
// where it's given way, or can't be; or the step fits whole, the key being
// the last of the rows it scrolls.
func (s section) keyShows(given giving, rows int) bool {
	if given >= givenKey || len(s.leaner) < int(givenKey) {
		return true
	}
	return s.rowsAt(given) <= rows
}

// gaveWay reports whether the unit has given way: folded, or led by USAGE,
// its block given way.
func (u unit) gaveWay() bool {
	return u.fold || u.lean > givenNone
}

// regain has the unit take back what it gave way: unfolded, at its least.
func (u *unit) regain() {
	u.fold = false
	u.leanTo(givenNone)
}

// formed are the unit's sections in the form it has them: USAGE as far as
// its block has given way, as leaned has it.
func (u unit) formed() []section {
	formed := slices.Clone(u.sections)
	for i, s := range formed {
		if s.leaned != nil && u.lean > givenNone {
			formed[i] = s.leaned(u.lean)
		}
	}
	return formed
}

// headRows are the rows the section's heading takes over its body, its own
// among them: as its heads has them, else sectionHead.
func (s section) headRows() int {
	if s.heads > 0 {
		return s.heads
	}
	return sectionHead
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

// layUnits lays the units out by flex down room rows, in turn, each the rows
// it takes, a blank row between two as blankBetween says: where each lies,
// the rows they take, and whether they fit in room.
func layUnits(units []unit, room int) (spans []flex.Span, taken int, fits bool) {
	parts := make([]flex.Part, len(units))
	for i, u := range units {
		parts[i] = flex.Part{Least: u.taken(), Natural: u.taken()}
		if i > 0 && blankBetween(units[i-1], u) {
			parts[i].Gap = 1
		}
	}
	spans, fits = flex.Lay(room, parts...)
	if len(spans) > 0 {
		last := spans[len(spans)-1]
		taken = last.At + last.Size
	}
	return spans, taken, fits
}

// unitsFit reports whether the units fit in room rows, as layUnits lays them out.
func unitsFit(units []unit, room int) bool {
	_, _, fits := layUnits(units, room)
	return fits
}

// unitsOf are the units the sections are laid out in, in their order: a
// section of no rows of its own beside the one that leaves room for it, as
// aside says, and where none does, nowhere; LATELY beside COMING UP, where
// listsBeside says, else under it; and each other section alone.
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
// last, where it lists them too, and listsBeside says they sit side by side.
func (f Frame) joins(units []unit, s section) (int, bool) {
	if s.least == 0 {
		i := slices.IndexFunc(units, func(u unit) bool { return u.sections[0].aside > 0 })
		return i, i >= 0
	}
	last := len(units) - 1
	return last, s.lists() && f.listsBeside() && last >= 0 && units[last].sections[0].lists()
}

// listsBeside reports whether COMING UP and LATELY sit side by side: where
// each one's least fits in the columns columns gives it beside the other,
// COMING UP comingWidth wide, and LATELY from latelyAt to the edge.
func (f Frame) listsBeside() bool {
	return comingLeast() <= comingWidth(f.Width) && latelyLeast() <= f.Width-latelyAt(f.Width)
}

// allot gives each unit its rows in room rows, and returns them: its least,
// to start with. Where they don't fit, units give way, USAGE's blank rows
// between its accounts first, then sections folding to their headings, as
// fold has them, and what then fits takes back what it gave, as unfold has
// it; where they still don't, USAGE scrolls or gives way further, as
// squeeze has it. The rows to spare go to the units not folded, as growth
// has them, and the rest stay empty at the foot.
func allot(units []unit, room int) []unit {
	for i := range units {
		units[i].rows = units[i].least
	}
	unfold(units, room, fold(units, room))
	if _, taken, fits := layUnits(units, room); fits {
		grow(units, room-taken)
	} else {
		squeeze(units, taken-room)
	}
	return units
}

// fold has the units give way, as giveWay has them, a group of foldOrder's
// at a time, until they fit in room rows, and returns the groups that gave
// way, in turn.
func fold(units []unit, room int) [][]Section {
	var folded [][]Section
	for _, group := range foldOrder {
		if unitsFit(units, room) {
			break
		}
		folded = append(folded, group)
		for i := range units {
			if units[i].ledBy(group...) {
				units[i].giveWay()
			}
		}
	}
	return folded
}

// unfold has each of the units that gave way take it back, as regain has it,
// where it then keeps it in room rows at its least, as keeps has it: the
// group that gave way last first, and within a group, unit by unit, in the
// page's order, so where LATELY is under COMING UP, COMING UP unfolds alone
// where LATELY can't; and USAGE's blank rows between its accounts last, so
// they stay given way only where a section open would fold for them.
func unfold(units []unit, room int, folded [][]Section) {
	for _, group := range slices.Backward(folded) {
		for i := range units {
			if units[i].gaveWay() && units[i].ledBy(group...) {
				units[i].regain()
				if !keeps(units, i, room) {
					units[i].giveWay()
				}
			}
		}
	}
}

// keeps reports whether unit i keeps what it took back in room rows: where
// the units fit; and USAGE where no other unit is open, as no section is
// open for its blank rows between accounts to give way to, though it doesn't
// fit, which squeeze then settles.
func keeps(units []unit, i, room int) bool {
	if unitsFit(units, room) {
		return true
	}
	return units[i].ledBy(SectionUsage) && !slices.ContainsFunc(units, func(u unit) bool { return !u.fold && !u.ledBy(SectionUsage) })
}

// squeeze takes over rows from USAGE, every other section folded, its
// heading kept: in the rest, it takes the step, as it is or given way as far
// as its blank rows between accounts, those under its headings, or its key,
// that shows the most accounts, as showsMost has it, keeping its blank rows
// where they cost none and its key where it shows; the rows a step leaves go
// blank at the foot, and where it doesn't fit, it scrolls within them.
func squeeze(units []unit, over int) {
	i := slices.IndexFunc(units, func(u unit) bool { return u.ledBy(SectionUsage) })
	if i < 0 {
		return
	}
	u := &units[i]
	left := max(u.rows-over, sectionHead)
	u.leanTo(u.showsMost(left))
	u.rows = min(u.rows, left)
}

// grow gives the units not folded spare rows, as growth has them: a unit
// folded never grows.
func grow(units []unit, spare int) {
	for _, g := range growth {
		var grown []*unit
		for i := range units {
			if !units[i].fold && units[i].ledBy(g.sections...) {
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
// spare, else to the most of its sections' steps between that does, and
// returns how many are left.
func whole(units []*unit, spare int) int {
	for _, u := range units {
		to := u.rows
		for _, s := range u.sections {
			for _, step := range slices.Concat(s.between, []int{u.most()}) {
				if step > to && step-u.rows <= spare {
					to = step
				}
			}
		}
		u.rows, spare = to, spare-(to-u.rows)
	}
	return spare
}

// place lays the units out from row top, as layUnits lays them: each
// section, in the form its unit has it, as formed has it, its heading on its
// unit's first row, in the columns columns gives it, and where it doesn't
// fold, its body under its first section's heading rows, as headRows has
// them, in the rest of its unit's rows.
func (f Frame) place(units []unit, top int) []planned {
	var laid []planned
	spans, _, _ := layUnits(units, f.viewRows())
	for i, u := range units {
		y := top + spans[i].At
		formed := u.formed()
		heads := formed[0].headRows()
		for j, s := range formed {
			head, body := f.columns(u, j)
			head.y, head.rows = y, 1
			p := planned{section: s, head: head, fold: u.fold}
			if !u.fold {
				body.y, body.rows = y+heads, u.rows-heads
				p.body = body
			}
			laid = append(laid, p)
		}
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
// was given, its heading's among them, and which fold; the rows SESSIONS
// listed, in their order; the form USAGE's rows were laid out in, in the
// rows it was given, as usageFormOf has it; and what it was made for, as
// layoutFor says. The watch keeps it from look to look, as LayoutOf gives
// it, so the layout changes only on a key or a resize, or a section's shape
// changing, never as sessions start and lines come: SESSIONS keeps the rows
// it was given however many sessions run, and the order it listed them in,
// as views.Steady keeps it, and COMING UP and LATELY theirs however many
// lines they have. A frame lays the Overview out afresh where its Layout is
// the zero Layout, or one made for another frame or shape.
type Layout struct {
	made     layoutFor
	allotted [len(sectionWords)]allotment
	running  []views.RunningRow
	usage    usageForm
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

// allotment is the rows a layout gives a section, whether it folds, and for
// USAGE, how far its block gives way.
type allotment struct {
	rows int
	fold bool
	lean giving
}

// fills reports whether the section's rows fill with what comes and goes, as
// SESSIONS' do with the sessions running, and COMING UP's and LATELY's with
// their lines: a layout kept keeps the rows it gave them, however many they
// hold.
func (s Section) fills() bool {
	return s == SectionSessions || s.lists()
}

// LayoutOf is the Overview's layout as the frame lays it out of doc at now,
// for the watch to keep for the next look: the zero Layout off the Overview,
// and under a session's page, which hides it.
func (f Frame) LayoutOf(doc status.Document, now time.Time) Layout {
	if f.Shown() != Overview {
		return Layout{}
	}
	f = f.At(doc, now)
	units := f.unitsOf(f.sectionsOf(doc, now))
	made := f.layoutFor(units)
	if f.keeps(made) {
		return f.Layout
	}
	units = allot(units, f.viewRows())
	l := Layout{made: made, usage: f.usageFormOf(units)}
	for _, u := range units {
		l.allotted[u.sections[0].Section] = allotment{rows: u.rows, fold: u.fold, lean: u.lean}
		for _, s := range u.sections {
			if s.steady != nil {
				l.running = s.order
			}
		}
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
	return f.Layout.made != (layoutFor{}) && f.Layout.made == made
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

// give gives each unit the rows, the fold and the lean the layout gave the
// section it's led by, SESSIONS listing in the order the layout kept, as its
// steady lists them, and returns them.
func (l Layout) give(units []unit) []unit {
	for i := range units {
		a := l.allotted[units[i].sections[0].Section]
		units[i].rows, units[i].fold, units[i].lean = a.rows, a.fold, a.lean
		for j, s := range units[i].sections {
			if s.steady != nil {
				units[i].sections[j] = s.steady(l.running)
			}
		}
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
// has it, where it says anything, cut to keep leastRule cells of rule, as
// fitted cuts it; a cell after that, or two after the name, a rule in
// border; and a cell on, at its last column, the key that hides it, in bold
// accent.key.
func (f Frame) sectionHeading(c *canvas, p planned) {
	h := p.head
	key := h.x + h.width - 1
	x := c.line(h.x, h.y, line{{p.heading(), dimInk}})
	ruled, at := x+ruleGap, x+headingGap
	room := key - at - leastRule - 2
	if says := f.says(p, room); len(says) > 0 {
		ruled = c.line(at, h.y, p.fitted(says, room)) + 1
	}
	c.text(ruled, h.y, rule(key-1-ruled), borderInk)
	c.text(key, h.y, p.Key(), keyInk)
}

// fitted is what the section's heading says, as given, cut to width cells:
// unfolded, of a section keyed, by whole items, as fitItems cuts them; else
// word by word, as fit cuts it.
func (p planned) fitted(says line, width int) line {
	if p.keyed && !p.fold {
		return says.fitItems(width)
	}
	return says.fit(width)
}

// says is what the section's heading says after its name, in cells cells:
// where it folds, what it holds, as its folded has it, then, for a section
// that lists lines, where it holds more than that, two cells on, an
// ellipsis, or for any other, three on, that there's no room for it at the
// frame's height, in text.subtle, what it holds in the cells those leave;
// else its description, where it has one.
func (f Frame) says(p planned, cells int) line {
	if !p.fold {
		return saying(p.description, cells)
	}
	switch {
	case p.lists() && p.lines() > 1:
		more := line{{"  " + prose.Ellipsis, dimInk}}
		return slices.Concat(saying(p.folded, cells-more.width()), more)
	case p.lists():
		return saying(p.folded, cells)
	}
	noRoom := f.noRoom()
	holds := saying(p.folded, cells-headingGap-noRoom.width())
	if len(holds) == 0 {
		return noRoom
	}
	return slices.Concat(holds, line{spaces(headingGap)}, noRoom)
}

// noRoom says a folded heading has no room for what it heads at the frame's
// height, in text.subtle: "no room at 24 rows".
func (f Frame) noRoom() line {
	return line{{"no room at " + strconv.Itoa(f.Height) + " rows", dimInk}}
}

// headingWords are what a section's heading says in the cells given.
type headingWords func(cells int) line

// oneForm is what a heading of a single form says, as words gives it, at any
// width.
func oneForm(words func() line) headingWords {
	return func(int) line { return words() }
}

// saying is what words says in cells cells, or none where it's nil.
func saying(words headingWords, cells int) line {
	if words == nil {
		return nil
	}
	return words(cells)
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
// of each that doesn't fold; but none past the view's rows, nor an account's
// row, which moves the focus, where the Overview has none, as Focuses says.
func (f Frame) targetsOf(laid []planned) []Target {
	var targets []Target
	for _, p := range laid {
		targets = append(targets, Target{X: p.head.x, Y: p.head.y, Width: p.head.width, On: OnSection, Section: p.Section})
		if p.targets != nil && !p.fold {
			targets = append(targets, p.targets(p.body)...)
		}
	}
	end, focuses := f.top()+f.viewRows(), f.Focuses()
	return slices.DeleteFunc(targets, func(t Target) bool { return t.Y >= end || t.On == OnAccount && !focuses })
}

// overviewScrolling is how far the Overview's USAGE of doc at now scrolls
// within its section, scrolled to each offset given, as scrollingOf has it of
// its sections as laidOut lays them out on the page, once for every offset.
func (f Frame) overviewScrolling(doc status.Document, now time.Time) func(offset int) Scrolling {
	laid := f.laidOut(doc, now, f.top())
	return func(offset int) Scrolling { return scrollingOf(laid, offset) }
}

// scrollingOf is how far USAGE, of the sections as they're laid out, scrolls
// within its section, scrolled offset down, as its scrolling says in the rows
// of body it's given: nowhere where it's hidden, or doesn't scroll.
func scrollingOf(laid []planned, offset int) Scrolling {
	usage := usageOf(laid)
	if usage.scrolling == nil {
		return Scrolling{}
	}
	return usage.scrolling(usage.body.rows, offset)
}

// ScrollForm is what an offset into the view the frame shows of doc at now
// is read in: rows or things, as ScrollsRows says; and on the Overview, the
// form USAGE's rows are laid out in, as the frame's Layout has it. An offset
// read in one form means nothing in another.
type ScrollForm struct {
	rows  bool
	usage usageForm
}

// ScrollForm is the form an offset into the view the frame shows of doc at
// now is read in, its Overview's as LayoutOf made the frame's Layout, which
// the frame lays it out by.
func (f Frame) ScrollForm(doc status.Document, now time.Time) ScrollForm {
	return ScrollForm{rows: f.ScrollsRows(doc, now), usage: f.Layout.usage}
}

// usageFormOf is the form USAGE's rows are laid out in, as its scrollForm
// has it, of the units as they're given their rows: none where it's hidden,
// or the frame is too short for the view.
func (f Frame) usageFormOf(units []unit) usageForm {
	usage := usageOf(f.place(units, 0))
	if f.cramped() || usage.scrollForm == nil {
		return usageForm{}
	}
	return usage.scrollForm(usage.body.rows)
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
