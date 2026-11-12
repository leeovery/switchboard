package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// GraphsSection is one of Graphs' sections, which its key shows and hides.
type GraphsSection int

// Graphs' sections, in the order the page draws them.
const (
	// GraphsWeek is WEEK: a burn-down card an account, over its week.
	GraphsWeek GraphsSection = iota
	// GraphsPool is POOL: the pool's room over Runway's week, each account's
	// stacked.
	GraphsPool
	// GraphsGlance is GLANCE: a row an account and the pool, each window's
	// room as a sparkline, and usage's words.
	GraphsGlance
	// GraphsWhen is WHEN YOU WORK: requests by the hour of the day and the
	// day of the week, over the last four weeks.
	GraphsWhen
)

// graphsSections are Graphs' sections' words, and those shown at first:
// WEEK.
var graphsSections = sectionList{
	words: []sectionWording{
		GraphsWeek:   {key: "w", word: "week", heading: "WEEK", name: "week"},
		GraphsPool:   {key: "o", word: "pool", heading: "POOL", name: "pool"},
		GraphsGlance: {key: "g", word: "glance", heading: "GLANCE", name: "glance"},
		GraphsWhen:   {key: "h", word: "when", heading: "WHEN YOU WORK", name: "when"},
	},
	first: 1 << GraphsWeek,
}

// list is Graphs' sections' words.
func (GraphsSection) list() sectionList { return graphsSections }

// EveryGraphsSection are Graphs' sections, in the order the page draws them.
func EveryGraphsSection() []GraphsSection {
	return everyOf[GraphsSection]()
}

// GraphsSectionKeyed is Graphs' section the key given shows and hides, as
// "w" does WEEK, reporting false for a key that's none of its sections'.
func GraphsSectionKeyed(key string) (GraphsSection, bool) {
	return keyedOf[GraphsSection](key)
}

// Key is the key that shows and hides the section, as "w".
func (s GraphsSection) Key() string {
	return wordingOf(s).key
}

// Word is the section's word, as the key line has it after its key, as
// "when".
func (s GraphsSection) Word() string {
	return wordingOf(s).word
}

// heading is the section's name, as its heading has it, as "WHEN YOU WORK".
func (s GraphsSection) heading() string {
	return wordingOf(s).heading
}

// GraphsSections are which of Graphs' sections show. The zero value shows
// those shown at first.
type GraphsSections = shown[GraphsSection]

// GraphsSectionsNamed are Graphs' sections the names given show, as
// shownNamed reads them.
func GraphsSectionsNamed(names []string) GraphsSections {
	return shownNamed[GraphsSection](names)
}

// GraphsWindow is the window Graphs charts, WEEK's cards and GLANCE's
// sparklines alike: the week, over Runway's live view of it.
const GraphsWindow = views.RunwayWeek

// GraphsWindows are the windows whose lanes Graphs reads with the sections
// given shown: the week's, while WEEK or GLANCE shows, as GLANCE places its
// ✕ as a card's; none else.
func GraphsWindows(sections GraphsSections) []views.RunwayWindow {
	if sections.Has(GraphsWeek) || sections.Has(GraphsGlance) {
		return []views.RunwayWindow{GraphsWindow}
	}
	return nil
}

// GraphsShown is what the Graphs tab shows: its sections shown, as w, o, g
// and h show and hide them, and the preferences keep; the configured
// accounts, in the config's order, which alone, with the frame's size, its
// look, which words POOL's key, and the sections shown, lay the page out, as
// layGraphs has it; WHEN YOU WORK's page, whose weeks its heading says, as
// the watch built it at the last look on Graphs, nil where the ledger's days
// weren't read then, or WHEN YOU WORK was hidden; and Runway's lanes over its
// live view of the week while WEEK or GLANCE shows, as GraphsWindows has
// them, by the window, as the watch built them at the look, which its cards
// say what Runway says of each account by, and GLANCE places its ✕ by, none
// till it's built them; POOL's stack, as the watch built it at the last look
// on Graphs with POOL shown, held while it hides, none stacked till it's
// built it; and the pool's room over the week's span, by the window, as
// GLANCE draws it beside Σ pool, as the watch built it at the last look on
// Graphs with GLANCE shown, held while it hides, none till it's built it.
type GraphsShown struct {
	Sections GraphsSections
	Accounts config.Accounts
	Hours    *views.HoursPage
	Lanes    map[views.RunwayWindow]WindowLanes
	Pool     views.PoolStack
	Spans    map[views.RunwayWindow]views.PoolSpan
}

// graphsNamed are the configured accounts given, in the config's order,
// each as the moment's Configured has it, and named as Graphs names them,
// as Runway's lanes are.
func graphsNamed(m status.Moment, configured config.Accounts) ([]status.AccountNow, []accountName) {
	accounts := m.Configured(configured)
	return accounts, namesAt(accounts)
}

// pooled reports whether the configured accounts given make a pool: two or
// more, as Graphs lays out what it keeps for the pool, whatever a pin makes
// the pool's.
func pooled(accounts config.Accounts) bool {
	return len(accounts) > 1
}

// graphsTab draws Graphs of doc at now in the view's rows, from row top:
// where every section is hidden, how to bring them back, as everythingHidden
// has it; else the sections shown laid out whole, as graphsLaid lays them,
// in the rows over the legend, scrolled as the frame has it, a row at a
// time, as paged pages them; and the legend on the view's last rows.
func (f Frame) graphsTab(c *canvas, doc status.Document, now time.Time, top int) {
	if f.Graphs.Sections.none() {
		view := newCanvas(f.Width, f.viewRows())
		everythingHidden(view, f.Width, f.Graphs.Sections.Toggles())
		c.paste(view, 0, top, view.height())
		return
	}
	laid := f.graphsLaid()
	whole := newCanvas(f.Width, laid.rows)
	f.graphsDrawn(whole, f.Moment(doc, now), laid)
	laid.paged().paste(c, whole, top, f.Scroll, f.Width)
	for i, l := range laid.legend {
		c.line(0, top+laid.room+i, l)
	}
}

// graphsDrawn draws the sections of the moment m as laid out, whole, on c:
// each heading across its columns, as headSection draws it, saying what
// graphsSays has it say, under WEEK's, its cards, as burnCards draws them,
// and under POOL's, GLANCE's and WHEN YOU WORK's, their bodies, as
// poolDrawn, glanceDrawn and whenDrawn draw them; and the pool's line, where
// there's one, as poolLine words it.
func (f Frame) graphsDrawn(c *canvas, m status.Moment, laid graphsLaid) {
	for i, s := range laid.sections {
		says := f.graphsSays(s.section, i == 0)
		headSection(c, s.head(), s.section.heading(), s.section.Key(), func(cells int) line { return says.fit(cells) })
		switch s.section {
		case GraphsWeek:
			f.burnCards(c, m, laid, s)
		case GraphsPool:
			f.poolDrawn(c, m.Now, laid, s)
		case GraphsGlance:
			f.glanceDrawn(c, m, laid, s)
		case GraphsWhen:
			f.whenDrawn(c, s)
		}
	}
	if laid.poolLine >= 0 {
		c.line(0, laid.poolLine, f.poolLine(m, f.Width))
	}
}

// graphsSays is what the section's heading says after its name: on the first
// heading, while the frame says the page is loading, loading…, in
// text.subtle, as SESSIONS' says it on the Overview; else WHEN YOU WORK's
// weeks, as its page words them, in text.faint; and nothing for the rest.
func (f Frame) graphsSays(s GraphsSection, first bool) line {
	switch {
	case first && f.Loading:
		return line{{loadingWord, dimInk}}
	case s == GraphsWhen && f.Graphs.Hours != nil && f.Graphs.Hours.Description != "":
		return line{{f.Graphs.Hours.Description, faintInk}}
	}
	return nil
}

// graphsOut is the form of the line saying how many of Graphs' rows are out
// of view, above and below, all in text.subtle: "▲ 3 more above · ▼ 12 more
// below".
var graphsOut = outForm{above: " above", below: " below", gap: " · ", inks: subtleOut}

// graphsState draws on the state row, where Graphs' sections are scrolled
// past what shows, how many of their rows are out of view, as outOfView says
// it in graphsOut's form.
func (f Frame) graphsState(c *canvas) {
	if f.Graphs.Sections.none() {
		return
	}
	paged := f.graphsLaid().paged()
	from, to := paged.shown(f.Scroll)
	c.line(0, f.stateRow(), outOfView(from, paged.content-to, f.Width, graphsOut))
}

// graphsScrolling is how far Graphs' sections scroll, a row at a time,
// scrolled to each offset given, as their laying out pages them: none where
// every one is hidden.
func (f Frame) graphsScrolling() func(offset int) Scrolling {
	if f.Graphs.Sections.none() {
		return func(int) Scrolling { return Scrolling{} }
	}
	return f.graphsLaid().paged().scrolling
}

// graphsTargets are Graphs' click targets: each section's heading the page
// shows, across its columns, scrolled as the frame has it, a click on which
// is a press of its key, which hides it.
func (f Frame) graphsTargets() []Target {
	if f.Graphs.Sections.none() {
		return nil
	}
	laid := f.graphsLaid()
	paged := laid.paged()
	from, _ := paged.shown(f.Scroll)
	var targets []Target
	for _, s := range laid.sections {
		if paged.shows(f.Scroll, s.at) {
			head := s.head()
			targets = append(targets, Target{X: head.x, Y: f.top() + head.y - from, Width: head.width, On: OnPress, Key: s.section.Key()})
		}
	}
	return targets
}
