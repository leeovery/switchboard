package dashboard

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// WindowLanes are Runway's lanes over its live view of a window, as the watch
// builds them while Graphs shows, for its cards to say what Runway says of
// each account: the window, the timeline the lanes are drawn over, and the
// lanes, nil till they're built.
type WindowLanes struct {
	Window   views.RunwayWindow
	Timeline views.Timeline
	Lanes    *views.Lanes
}

// view is the view of their window the lanes are of.
func (w WindowLanes) view() laneView {
	return laneView{window: w.Window, timeline: w.Timeline}
}

// The turns a card's head gives way in, its words beside its label: first
// its words, their parts left off their end, whole, to the first; then its
// label, cut to its least; then its words go.
const (
	headWordsTurn = iota + 1
	headLabelTurn
	headWordsLeave
)

// cardHead is a burn-down card's head, as cardHeadOf lays it out: its label,
// on its first row; its words, placed from the card's first column on its
// row, the label's, 0, or the one under it, 1; and how far the cue on its
// words has faded, where they're cued.
type cardHead struct {
	label line
	words placed
	row   int
	cue   float64
	cued  bool
}

// cardHeadOf is the head of the card of the lane ln, in the view v of its
// window, width cells wide, at now: its label, the account's number, name and
// marks, as Runway's lanes label them; and its words, as cardWords has them.
// Beside, its words are on the label's row, ending at the card's last
// column, the label in a column of the account's name, as accountsColumn
// has it, its gap clear of the label at the least, the parts laid out by
// flex, giving way in the turns a head's do; where the words give way even
// so, the label takes its row alone, as within cuts it. Else the label
// takes the width, as within cuts it, and its words are on the row under
// it, from its first column, whole parts left off their end where they don't
// fit, as partsIn has them. They're cued as Runway cues the words under the
// lane, as wordsCue has it.
func (f Frame) cardHeadOf(v laneView, ln laidLane, width int, beside bool, now time.Time) cardHead {
	words := v.cardWords(ln, now)
	h := cardHead{label: ln.name.within(width), words: placed{said: partsIn(words, width)}, row: 1}
	if beside && len(words) > 0 {
		label := accountsColumn(nameGap, []accountName{ln.name})
		label.Narrows = headLabelTurn
		said := slices.Concat(words...)
		spans, _ := flex.Lay(width, label, flex.Part{Least: words[0].width(), Natural: said.width(), Narrows: headWordsTurn, Leaves: headWordsLeave})
		said = wholeParts(words, spans[1].Size)
		if len(said) > 0 {
			h.label = ln.name.line(min(spans[0].Size, width))
		}
		h.words, h.row = placed{x: width - said.width(), said: said}, 0
	}
	h.cue, h.cued = f.wordsCue(ln, v.timeline.NowColumn())
	return h
}

// within is the name in the cells given, its label cut short to keep its
// number and its marks, as labelIn cuts it, no cells kept beside it, as on a
// row of its own.
func (n accountName) within(cells int) line {
	return n.labelIn(cells - n.besideCells())
}

// partsIn are as many of the parts given as fit in cells cells, whole, from
// the first, as wholeParts has them; where even the first doesn't fit, that
// alone, cut short as fit cuts it.
func partsIn(parts []line, cells int) line {
	if said := wholeParts(parts, cells); said != nil || len(parts) == 0 {
		return said
	}
	return parts[0].fit(cells)
}

// draw draws the head from column x along row y: its label, then its words
// on their row, cued where they are, as cue picks them out.
func (h cardHead) draw(c *canvas, look Look, x, y int) {
	c.line(x, y, h.label)
	said := h.words.said
	c.line(x+h.words.x, y+h.row, said)
	if h.cued && said.width() > 0 {
		cue(c, look, x+h.words.x, y+h.row, said.width(), h.cue)
	}
}

// cardWords are the words of the card of the lane ln, in the view v of its
// window, at now, in parts, each whole, as a head leaves them off its end:
// the room its use leaves, as roomLeft has it, where it's read; then each
// phrase cardPhrases has, each phrase's parts as Runway's are, its first
// after prose.Separator, but where it's the first of all. Out of the pool,
// they're in text.faint, as the Overview words an account out of it, the
// room's figure bold.
func (v laneView) cardWords(ln laidLane, now time.Time) []line {
	out := ln.account.OutOfPool
	var phrases [][]line
	if ln.Use.Read {
		phrases = append(phrases, []line{ln.roomLeft()})
	}
	for _, p := range v.cardPhrases(ln, now) {
		if out {
			p = fainted(p)
		}
		phrases = append(phrases, p)
	}
	separator := span{prose.Separator, mutedInk}
	if out {
		separator.ink = faintInk
	}
	var words []line
	for i, p := range phrases {
		if i > 0 {
			p = append([]line{append(line{separator}, p[0]...)}, p[1:]...)
		}
		words = append(words, p...)
	}
	return words
}

// cardPhrases are the parts of each phrase a card of the lane ln, in the
// view v of its window, says at now after its room: what's said under the
// lane on Runway, as said has it, its phrases, in Runway's order, and those
// sharing a column in the order of when what each says of begins, as a
// reset there before a stretch starting after it, which Runway drops, as
// placeAlong does a phrase ending where the words before it run, though it
// places two stretches in one column both; or where it has none, what it
// says instead; and of a lane whose use isn't read, where that's nothing,
// its use's words, as Runway's label says them, as "not read".
func (v laneView) cardPhrases(ln laidLane, now time.Time) [][]line {
	says := v.said(ln, now)
	said := slices.SortedStableFunc(slices.Values(says.phrases), func(a, b phrase) int {
		return cmp.Or(cmp.Compare(a.col, b.col), a.at.Compare(b.at))
	})
	if len(said) == 0 {
		said = says.instead
	}
	var phrases [][]line
	for _, p := range said {
		phrases = append(phrases, p.parts)
	}
	if len(phrases) == 0 && !ln.Use.Read {
		phrases = append(phrases, []line{ln.useWords(runwayNames[v.window], now).short})
	}
	return phrases
}

// fainted are the parts given, each in text.faint, as inFaint has it.
func fainted(parts []line) []line {
	faint := make([]line, len(parts))
	for i, p := range parts {
		faint[i] = inFaint(p)
	}
	return faint
}

// roomLeft is the room the lane's use of its window leaves, as a card's
// words lead with it, as "66% left": its figure as roomFigure has it; then
// " left" in text.muted, or text.faint, where the pool leaves its account
// out.
func (ln laidLane) roomLeft() line {
	u, out := ln.Use, ln.account.OutOfPool
	left := span{" left", mutedInk}
	if out {
		left.ink = faintInk
	}
	return line{roomFigure(Ref{Account: ln.ID, Window: u.Key}, u.Used, out, u.Stale), left}
}

// roomFigure is the room the use given leaves of the window ref names, as a
// card's words and GLANCE say it, as "66%": in the ink figureInk gives the
// use, as usage colours a use, of an account out of the pool, or stale, as
// given, numbered as the room left in the window.
func roomFigure(ref Ref, used float64, out, stale bool) span {
	return span{prose.PercentLeft(used), figureInk(used, out, stale)}.numbered(Number{Kind: NumberRoom, Account: ref.Account, Of: ref.Window})
}
