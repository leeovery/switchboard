package dashboard

import (
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/theme"
)

// The blank cells the heading row sets its parts apart by: before the
// sub-tabs and before the state; between the rule and what's either side of
// it, but the state, which the rule starts a cell after, and the period's
// readout, which it ends three short of, so a faint - stands clear of it;
// and on a phone, before the sub-tabs.
const (
	headingGap = 3
	ruleGap    = 2
	readoutGap = 3
	compactGap = 2
)

// shownSubTabInk picks out the sub-tab shown.
var shownSubTabInk = ink{token: theme.TextPrimary, on: hue{token: theme.BgSubtle}, bold: true}

// heading is a tab's heading row, as the tab gives it: its name; its
// sub-tabs, where it has them, and which of them is shown; the state
// the screen can't otherwise show, in text.faint, a key within it in bold
// accent.key, where there's any, and its shorter forms, each shorter, for
// less room; the widest its least form takes as the keys that change what it
// names step it, whether it shows yet or not; whether it's free text, its
// length unbounded, as the Log's filter, cut to the room it's left; and at
// its right, the dial's period's readout, as dialed has it, or another
// line, as the Log's count and its f, and that line where the heading is
// compact, where it has one of its own; and whether it says its page is
// loading.
type heading struct {
	name         string
	subTabs      []string
	shown        int
	state        line
	shorter      []line
	least        string
	free         bool
	reading      bool
	period       dialed
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
// heading's name, so neither loading nor a step moves the heading between
// one row and two: loading… whole, whether it shows or not, as a read can
// start at any moment; or the widest the state's least form takes as it
// steps, or the least form shown, where that's wider. A free state's length
// is its data's, so it holds none of it whole, or the heading's rows would
// hang on it.
func (h heading) leastState() int {
	least := max(ansi.StringWidth(loadingWord), ansi.StringWidth(h.least))
	if forms := h.quietForms(); forms != nil && !h.free {
		least = max(least, forms[len(forms)-1].width())
	}
	return least
}

// tabHeading is the heading row of the tab shown: Accounts', as
// accountsHeading has it; Projects', Sessions', Runway's and the Log's, as
// theirs have them; else its name alone.
func (f Frame) tabHeading() heading {
	switch f.Shown() {
	case Accounts:
		return f.accountsHeading()
	case Projects:
		return f.Projects.heading(f.Dial)
	case Sessions:
		return f.SessionsTab.heading(f.Dial)
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
// status line: none on a tab whose sections head themselves, as headsItself
// has it, nor under a session's page, which heads itself, nor on a page too
// short for its view, as cramped says.
func (f Frame) headingPieces() []piece {
	if shown := f.Shown(); shown.headsItself() || shown == SessionPage || f.cramped() {
		return nil
	}
	return f.heading().pieces(f.headingY(), f.Width, f.phone())
}

// rows is how many rows the heading takes in a frame width cells wide, on a
// phone where phone says: two where its state or what's at its right takes
// the row under it, as under has it; else one.
func (h heading) rows(width int, phone bool) int {
	if state, right := h.under(width, phone); state || right {
		return 2
	}
	return 1
}

// compact reports whether the heading is drawn as a phone draws a tab's
// with sub-tabs: without [ ] or a rule.
func (h heading) compact(phone bool) bool {
	return phone && len(h.subTabs) > 0
}

// The heading's parts as a compact heading lays them out, deciding what
// takes the row under: the name and the sub-tabs, whole; three cells on, the
// state at its least, which is never cut, so gives way to the row under
// where even it doesn't fit beside them; and two cells on, what's at the
// right at its widest, the period's readout or the line, which gives way to
// the row under first.
const (
	periodNamed = iota
	periodState
	periodBeside
)

// under reports what of a heading takes the row under its name and sub-tabs
// in a frame width cells wide, on a phone where phone says. Of a compact
// heading, as its parts above lay out there: its state; and what's at its
// right, which goes where the state does. Of another, its period's readout
// alone, where it doesn't fit beside them as ruledBeside has it, its state
// and its rule keeping their row. So neither loading, nor a step of the
// period or of what the state names, moves the heading between one row and
// two, but a step between now and a past period, whose widest readouts
// differ. A tab without sub-tabs or a period, as Runway, keeps its rule and
// what's at its right on its row.
func (h heading) under(width int, phone bool) (state, right bool) {
	var laid laying
	named := h.named(&laid, 0, phone)
	if !h.compact(phone) {
		return false, h.period.shown() && !h.ruledBeside(named, width)
	}
	least := h.leastState()
	parts := []flex.Part{
		periodNamed: {Least: named, Natural: named},
		periodState: {Least: least, Natural: least, Gap: headingGap, Leaves: 2},
	}
	beside := ends(h.widestRight())
	if beside > 0 {
		parts = append(parts, flex.Part{Least: beside, Natural: beside, Leaves: 1, Gap: ruleGap})
	}
	spans, fits := flex.Lay(width, parts...)
	state = !fits || spans[periodState].Left
	return state, beside > 0 && (state || spans[periodBeside].Left)
}

// ruledBeside reports whether a heading with a rule, its name and sub-tabs
// ending at column x, has room on its row for its period's readout at the
// widest it keeps room for: the rule's gap, a cell of rule at least, so the
// readout never floats on blanks, and the readout's gap before it.
func (h heading) ruledBeside(x, width int) bool {
	return x+ruleGap+1+readoutGap+ends(h.widestRight()) <= width
}

// widestRight are what sits at a compact heading's right at its widest: its
// period's readout at the widest it keeps room for, as atWidest has it;
// else its line.
func (h heading) widestRight() []piece {
	if h.period.shown() {
		return periodPieces(h.period.atWidest())
	}
	return h.rightPieces(true)
}

// pieces are the heading's parts, laid out along row y of a frame width
// cells wide, on a phone where phone says: its name and sub-tabs, as named
// lays them out; then three cells on, the state, or the first of its shorter
// forms that fits where the rest leaves it short of room, cut where none
// does. Ending at the last column, on the row under has it on, its period's
// readout; or its line, left out where it doesn't fit beside the name and
// the sub-tabs, as far from them as the rule ends short of it. Between them, the rule, from two cells
// after them, or one after the state, to as many short of what sits at the
// right as rightGap has it, or to the last column where nothing does. A
// compact heading has no rule; and where under says, its state starts the
// row under, and what's at its right ends it, left out where it doesn't fit
// after the state's least; but the period's readout, which the state gives
// way to there, left out where even its least doesn't fit before it. A
// readout wider than the frame is left out whole, never cut.
func (h heading) pieces(y, width int, phone bool) []piece {
	var laid laying
	x := h.named(&laid, y, phone)
	stateUnder, rightUnder := h.under(width, phone)
	right, rightY, gap := h.rightPieces(h.compact(phone)), y, h.rightGap(phone)
	switch {
	case rightUnder:
		rightY++
	case width-ends(right) < x+gap:
		right = nil
	}
	if h.period.shown() && ends(right) > width {
		right = nil
	}
	stateX, stateY := x+headingGap, y
	if stateUnder {
		stateX, stateY = 0, y+1
	}
	end := width
	if right != nil && rightY == stateY {
		end = width - ends(right) - gap
	}
	forms := h.stateForms()
	if stateUnder && end-stateX < h.leastState() {
		if h.period.shown() {
			forms = nil
		} else {
			right, end = nil, width
		}
	}
	ruled := x + ruleGap
	if forms != nil {
		ruled = laid.put(stateX, stateY, firstFit(end-stateX, forms...), Target{}) + 1
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

// rightGap is how many cells short of what sits at the heading's right what's
// before it ends, on a phone where phone says: the rule three short of the
// period's readout, as readoutGap has it; else two.
func (h heading) rightGap(phone bool) int {
	if h.period.shown() && !h.compact(phone) {
		return readoutGap
	}
	return ruleGap
}

// named lays the heading's name out from the first column of row y, then,
// where it has them, its sub-tabs, touching, three cells on, or two where
// it's compact, and a cell on, [ ], but where it's compact, or has one
// sub-tab alone, with none to turn to; and returns the column after them.
func (h heading) named(laid *laying, y int, phone bool) int {
	x := laid.put(0, y, line{{h.name, headingInk}}, Target{})
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
// are yet to be placed: its period's readout, as periodPieces has it; else,
// where compact says, its compact line, where it has one; else its line;
// else nothing.
func (h heading) rightPieces(compact bool) []piece {
	switch {
	case h.period.shown():
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

// stepOffInk is a sign of the readout's where its step does nothing: faint,
// and still bold, as the keys beside it are.
var stepOffInk = ink{token: theme.TextFaint, bold: true}

// periodPieces are the dial's period's readout given: its range between -
// and +, a click on each pressing it, then two cells on, when it is between
// < and >, a click on each stepping back and on; each sign in bold
// accent.key, faint where its step does nothing. The range is in
// text.muted, and when too, but gone, in accent.attention, so it never
// reads as now.
func periodPieces(d dialed) []piece {
	when := mutedInk
	if d.gone {
		when = warningInk
	}
	return []piece{
		{line: line{{"-", signInk(d.shorterOff)}}, click: Target{On: OnPress, Key: "-"}},
		{line: line{{" " + d.ranged + " ", mutedInk}}},
		{line: line{{"+", signInk(d.longerOff)}}, click: Target{On: OnPress, Key: "+"}},
		{line: line{spaces(2)}},
		{line: line{{"<", signInk(d.backOff)}}, click: Target{On: OnStep, Step: -1}},
		{line: line{{" " + d.when + " ", when}}},
		{line: line{{">", signInk(d.onOff)}}, click: Target{On: OnStep, Step: 1}},
	}
}

// signInk is a sign of the readout's ink: faint where off says its step
// does nothing, else a key's.
func signInk(off bool) ink {
	if off {
		return stepOffInk
	}
	return keyInk
}

// ends is how many cells the pieces take, laid end to end.
func ends(pieces []piece) int {
	n := 0
	for _, p := range pieces {
		n += p.line.width()
	}
	return n
}
