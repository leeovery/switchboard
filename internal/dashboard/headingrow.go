package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// The blank cells the heading row sets its parts apart by: before the
// sub-tabs and before the state; between the rule and what's either side of
// it, but the state, which the rule starts a cell after; and on a phone,
// before the sub-tabs.
const (
	headingGap = 3
	ruleGap    = 2
	compactGap = 2
)

// shownSubTabInk picks out the sub-tab shown.
var shownSubTabInk = ink{token: theme.TextPrimary, on: hue{token: theme.BgSubtle}, bold: true}

// heading is a tab's heading row, as the tab gives it: its name; its
// sub-tabs, where it has them, and which of them is shown; the state
// the screen can't otherwise show, in text.faint, a key within it in bold
// accent.key, where there's any, and its shorter forms, each shorter, for
// less room; the widest its least form takes as the keys that change what it
// names step it, whether it shows yet or not; and at its right, its period or
// window, which < and > step, or another line, as the Log's count and its f,
// and that line where the heading is compact, where it has one of its own;
// the widest form its period takes as < and > step it, the period itself
// where it's not given; and whether it says its page is loading.
type heading struct {
	name         string
	subTabs      []string
	shown        int
	state        line
	shorter      []line
	least        string
	reading      bool
	period       string
	widest       string
	right        line
	compactRight line
}

// piece is a part of the heading row as it's laid out: a line, from x along
// row y, and what a click on it does, as a target has it but for where it
// is: nothing where its On is zero.
type piece struct {
	x, y  int
	line  line
	click Target
}

// target is the piece as a click's target, over the cells it's drawn on.
func (p piece) target() Target {
	t := p.click
	t.X, t.Y, t.Width = p.x, p.y, p.line.width()
	return t
}

// loadingWord is what a heading says while what its page shows is being
// read.
const loadingWord = "loading" + prose.Ellipsis

// heading is the heading row of the tab shown, as tabHeading has it, saying
// it's loading, as loading has it, while the frame says so.
func (f Frame) heading() heading {
	h := f.tabHeading()
	if f.Loading {
		h = h.loading()
	}
	return h
}

// loading is h saying its page is loading, as stateForms words it.
func (h heading) loading() heading {
	h.reading = true
	return h
}

// quietForms are the forms the heading's state is said in while its page
// isn't loading, the fullest first: the state, then its shorter forms.
func (h heading) quietForms() []line {
	if h.state == nil {
		return nil
	}
	return slices.Concat([]line{h.state}, h.shorter)
}

// stateForms are the forms the heading's state is said in, the fullest
// first, as firstFit tries them: its quiet forms; and while its page is
// loading, loading…, in text.subtle, leading each, three cells before it,
// then alone, for the least room.
func (h heading) stateForms() []line {
	forms := h.quietForms()
	if !h.reading {
		return forms
	}
	said := line{{loadingWord, dimInk}}
	loading := make([]line, 0, len(forms)+1)
	for _, form := range forms {
		loading = append(loading, slices.Concat(said, line{spaces(headingGap)}, form))
	}
	return append(loading, said)
}

// leastState is the fewest cells the state must have whole beside a compact
// heading's period, so neither a read nor a step moves the heading between
// one row and two: loading… whole, whether it shows or not, as a read can
// start at any moment; or the widest the state's least form takes as it
// steps, or the least form shown, where that's wider.
func (h heading) leastState() int {
	least := max(ansi.StringWidth(loadingWord), ansi.StringWidth(h.least))
	if forms := h.quietForms(); forms != nil {
		least = max(least, forms[len(forms)-1].width())
	}
	return least
}

// tabHeading is the heading row of the tab shown: Accounts', as
// accountsHeading has it; History's, Sessions', Runway's and the Log's, as
// theirs have them; else its name alone.
func (f Frame) tabHeading() heading {
	switch f.Shown() {
	case Accounts:
		return f.accountsHeading()
	case History:
		return f.History.heading()
	case Sessions:
		return f.SessionsTab.heading()
	case Runway:
		return f.Runway.heading()
	case Log:
		return f.Log.heading(f.Width, func(id string) string { return named(f.heldMoment(), id) })
	}
	return heading{name: strings.ToUpper(f.Shown().Title())}
}

// headingRow draws the tab's heading row, as headingPieces lays it out.
func (f Frame) headingRow(c *canvas) {
	for _, p := range f.headingPieces() {
		c.line(p.x, p.y, p.line)
	}
}

// headingPieces are the heading row of the tab shown, laid out under the
// status line: none on the Overview, whose sections head themselves, nor
// under a session's page, which heads itself, nor on a page too short for
// its view, as cramped says.
func (f Frame) headingPieces() []piece {
	if shown := f.Shown(); shown == Overview || shown == SessionPage || f.cramped() {
		return nil
	}
	return f.heading().pieces(f.headingY(), f.Width, f.phone())
}

// rows is how many rows the heading takes in a frame width cells wide, on a
// phone where phone says: two where its period takes the row under it, as
// ownPeriod has it; else one.
func (h heading) rows(width int, phone bool) int {
	if h.ownPeriod(width, phone) {
		return 2
	}
	return 1
}

// compact reports whether the heading is drawn as a phone draws a tab's
// with sub-tabs: without [ ] or a rule.
func (h heading) compact(phone bool) bool {
	return phone && len(h.subTabs) > 0
}

// The heading's parts as a compact heading lays them out, deciding where its
// period goes: the name and the sub-tabs, whole; three cells on, the state at
// its least; and two cells on, the period at its widest between its keys,
// which gives way to a row of its own.
const (
	periodNamed = iota
	periodState
	periodBeside
)

// ownPeriod reports whether the heading's period takes the row under it, as
// a compact heading has it where the name, the sub-tabs, the state at its
// least and the period at its widest don't fit a frame width cells wide, so
// stepping the period never moves the heading between one row and two. A tab
// without sub-tabs, as Runway, keeps its rule and its window on its row.
func (h heading) ownPeriod(width int, phone bool) bool {
	if !h.compact(phone) || h.period == "" {
		return false
	}
	var laid laying
	named, widest := h.named(&laid, 0, phone), ends(periodPieces(cmp.Or(h.widest, h.period)))
	state := h.leastState()
	spans, fits := flex.Lay(width, []flex.Part{
		periodNamed:  {Least: named, Natural: named},
		periodState:  {Least: state, Natural: state, Gap: headingGap},
		periodBeside: {Least: widest, Natural: widest, Leaves: 1, Gap: ruleGap},
	}...)
	return !fits || spans[periodBeside].Left
}

// pieces are the heading's parts, laid out along row y of a frame width
// cells wide, on a phone where phone says: its name and sub-tabs, as named
// lays them out; then three cells on, the state, or the first of its shorter
// forms that fits where the rest leaves it short of room, cut where none
// does. Ending at the last column, its period between < and >, or
// its line, on the row under it where ownPeriod says, and left out where it
// doesn't fit beside the name and the sub-tabs. Between them, the rule, from
// two cells after them, or one after the state, to two cells short of what
// sits at the right, or to the last column where nothing does. A compact
// heading has no rule.
func (h heading) pieces(y, width int, phone bool) []piece {
	var laid laying
	x := h.named(&laid, y, phone)
	right, rightY, end := h.rightPieces(h.compact(phone)), y, width
	switch {
	case h.ownPeriod(width, phone):
		rightY++
	case width-ends(right) < x+ruleGap:
		right = nil
	case right != nil:
		end = width - ends(right) - ruleGap
	}
	ruled := x + ruleGap
	if forms := h.stateForms(); forms != nil {
		ruled = laid.put(x+headingGap, y, firstFit(end-x-headingGap, forms...), Target{}) + 1
	}
	if !h.compact(phone) {
		laid.put(ruled, y, line{{rule(end - ruled), borderInk}}, Target{})
	}
	from := width - ends(right)
	for _, p := range right {
		from = laid.put(from, rightY, p.line, p.click)
	}
	return laid
}

// named lays the heading's name out from the first column of row y, then,
// where it has them, its sub-tabs, touching, three cells on, or two where
// it's compact, and a cell on, [ ], but where it's compact, or has one
// sub-tab alone, with none to turn to; and returns the column after them.
func (h heading) named(laid *laying, y int, phone bool) int {
	x := laid.put(0, y, line{{h.name, dimInk}}, Target{})
	if len(h.subTabs) == 0 {
		return x
	}
	gap := headingGap
	if h.compact(phone) {
		gap = compactGap
	}
	x += gap
	for i := range h.subTabs {
		x = laid.put(x, y, h.subTab(i), Target{On: OnSubTab, SubTab: i})
	}
	if h.compact(phone) || len(h.subTabs) == 1 {
		return x
	}
	return laid.put(x+1, y, line{{"[ ]", faintInk}}, Target{})
}

// laying is a heading row as it's being laid out: its pieces so far.
type laying []piece

// put lays l out from x along row y, a click on it doing what click says,
// where it takes any cells, and returns the column after it.
func (l *laying) put(x, y int, ln line, click Target) int {
	if ln.width() > 0 {
		*l = append(*l, piece{x: x, y: y, line: ln, click: click})
	}
	return x + ln.width()
}

// subTab is the heading's sub-tab i, its name with a space either side,
// picked out, the spaces with it, where it's the one shown.
func (h heading) subTab(i int) line {
	name := " " + h.subTabs[i] + " "
	if i == h.shown {
		return line{{name, shownSubTabInk}}
	}
	return line{{name, tabInk}}
}

// rightPieces are what sits at the heading's right, in order, where they
// are yet to be placed: its period, with < before it and > after, each a
// step back and on; else, where compact says, its compact line, where it has
// one; else its line; else nothing.
func (h heading) rightPieces(compact bool) []piece {
	switch {
	case h.period != "":
		return periodPieces(h.period)
	case compact && h.compactRight != nil:
		return []piece{{line: h.compactRight}}
	case h.right != nil:
		return []piece{{line: h.right}}
	default:
		return nil
	}
}

// widestOf is the form that takes the most cells, the first of those that
// tie; "" where there are none.
func widestOf(forms []string) string {
	widest := ""
	for _, form := range forms {
		if ansi.StringWidth(form) > ansi.StringWidth(widest) {
			widest = form
		}
	}
	return widest
}

// periodPieces are the period given between < and >, each a step back and
// on.
func periodPieces(period string) []piece {
	return []piece{
		{line: line{{"<", keyInk}}, click: Target{On: OnStep, Step: -1}},
		{line: line{{" " + period + " ", mutedInk}}},
		{line: line{{">", keyInk}}, click: Target{On: OnStep, Step: 1}},
	}
}

// ends is how many cells the pieces take, laid end to end.
func ends(pieces []piece) int {
	n := 0
	for _, p := range pieces {
		n += p.line.width()
	}
	return n
}
