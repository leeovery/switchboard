package dashboard

import "github.com/leeovery/switchboard/internal/views"

// GraphsSection is one of Graphs' sections, which its key shows and hides.
type GraphsSection int

// Graphs' sections, in the order the page draws them.
const (
	// GraphsWeek is WEEK: a burn-down card an account, over its week.
	GraphsWeek GraphsSection = iota
	// GraphsFiveHour is 5-HOUR: a burn-down card an account, over its
	// 5-hour window.
	GraphsFiveHour
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
// WEEK and 5-HOUR.
var graphsSections = sectionList{
	words: []sectionWording{
		GraphsWeek:     {key: "w", word: "week", heading: "WEEK", name: "week"},
		GraphsFiveHour: {key: "f", word: "5-hour", heading: "5-HOUR", name: "5-hour"},
		GraphsPool:     {key: "o", word: "pool", heading: "POOL", name: "pool"},
		GraphsGlance:   {key: "g", word: "glance", heading: "GLANCE", name: "glance"},
		GraphsWhen:     {key: "h", word: "when", heading: "WHEN YOU WORK", name: "when"},
	},
	first: 1<<GraphsWeek | 1<<GraphsFiveHour,
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

// GraphsShown is what the Graphs tab shows: its sections shown, as w, f, o, g
// and h show and hide them; and WHEN YOU WORK's page, whose weeks its
// heading says, nil while the ledger's days aren't read, or it doesn't show.
type GraphsShown struct {
	Sections GraphsSections
	Hours    *views.HoursPage
}

// graphsSectionRows are the rows a Graphs section takes, its body empty: its
// heading, the blank row under it, and the blank row before the next.
const graphsSectionRows = sectionHead + 1

// graphsHead is a Graphs section's heading as the page lays it out: the
// section, and the row and columns it's drawn along.
type graphsHead struct {
	section GraphsSection
	head    bounds
}

// graphsHeads are the headings of Graphs' sections shown, from row top, in
// the order the page draws them, each across the width, graphsSectionRows
// under the one before.
func (f Frame) graphsHeads(top int) []graphsHead {
	var heads []graphsHead
	for _, s := range EveryGraphsSection() {
		if f.Graphs.Sections.Has(s) {
			heads = append(heads, graphsHead{section: s, head: bounds{y: top, width: f.Width, rows: 1}})
			top += graphsSectionRows
		}
	}
	return heads
}

// graphsTab draws Graphs in the view's rows, from row top: where every
// section is hidden, how to bring them back, as everythingHidden has it;
// else each section shown, its heading, as headSection draws it, saying
// what graphsSays has it say.
func (f Frame) graphsTab(c *canvas, top int) {
	view := newCanvas(f.Width, f.viewRows())
	if f.Graphs.Sections.none() {
		everythingHidden(view, f.Width, f.Graphs.Sections.Toggles())
	}
	for i, h := range f.graphsHeads(0) {
		says := f.graphsSays(h.section, i == 0)
		headSection(view, h.head, h.section.heading(), h.section.Key(), func(cells int) line { return says.fit(cells) })
	}
	c.paste(view, 0, top, view.height())
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

// graphsTargets are Graphs' click targets: each section's heading, a click
// on which is a press of its key, which hides it; but none past the view's
// rows.
func (f Frame) graphsTargets() []Target {
	top := f.top()
	end := top + f.viewRows()
	var targets []Target
	for _, h := range f.graphsHeads(top) {
		if h.head.y < end {
			targets = append(targets, Target{X: h.head.x, Y: h.head.y, Width: h.head.width, On: OnPress, Key: h.section.Key()})
		}
	}
	return targets
}
