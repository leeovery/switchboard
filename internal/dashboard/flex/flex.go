// Package flex lays parts out along a row's cells or down a column's rows:
// the arithmetic of every view's widths and heights, so no view works it out
// for itself. Each part has a least it's still readable at, a natural it
// takes whole and, where it grows, a most. Where the room runs short, parts
// narrow and give way in the turns they're given; where there's room to
// spare, the parts that grow take it.
package flex

import (
	"math"
	"slices"
)

// Part is a part of a row's cells or a column's rows. Narrows and Leaves
// count in one sequence of turns, the lowest taken first.
type Part struct {
	Least   int  // the fewest it's still readable at
	Natural int  // what it takes whole: a label's widest, a figure's width; held within Least and Most
	Most    int  // the most it takes; 0 for none
	Grows   bool // whether it takes room past its natural
	Narrows int  // the turn it narrows in, from its natural to its least: 0 freely, before any turn
	Leaves  int  // the turn it gives way in: 0 never
	Kept    int  // what it keeps where it gives way, no more than its least: 0 for a column, 1 for a section's heading
	Gap     int  // the cells or rows before it, where it isn't the first shown
	Odd     bool // whether it takes the odd cell where it shares evenly with others
}

// Span is where a part lies: from At, Size long; Left where it gave way. A
// part that gave way keeping nothing isn't shown: it lies where it would have,
// no cells long, with no gap before it.
type Span struct {
	At, Size int
	Left     bool
}

// Lay lays the parts out in room cells or rows, in their order, and reports
// whether they fit.
//
// Where the parts and their gaps pass room, the turns are taken, each only
// while they still pass it: the turn's parts that narrow in it narrow toward
// their leasts, the last first, a cell for each the room lacks; then, where it
// still lacks, the turn's parts that leave give way to their Kept, together. A
// part that narrows in a turn holds its natural until then, keeps what it
// narrowed to after, and takes none of what's spare.
//
// The free parts, those that narrow freely and haven't given way, stand at
// their leasts while the turns are taken, then take what's spare, so what a
// part frees at once goes to them, never back to a part narrowed before it:
//  1. each is raised toward its natural, in the parts' order, so names come
//     before bars;
//  2. those that grow with a most share what's left evenly, each up to its
//     most;
//  3. what's left after that, those that grow with no most share evenly.
//
// Where parts share an odd cell, one marked Odd takes it, else the first.
//
// false: even with every turn taken, the parts don't fit: the arrangement
// shifts. The spans then lie as the turns left them, past room.
func Lay(room int, parts ...Part) ([]Span, bool) {
	l := newLaying(parts)
	for _, turn := range l.turns() {
		lacking := l.taken() - room
		if lacking <= 0 {
			break
		}
		l.narrow(turn, lacking)
		if l.taken() > room {
			l.leave(turn)
		}
	}
	spare := room - l.taken()
	if spare < 0 {
		return l.spans(), false
	}
	spare = l.raise(spare)
	spare = l.share(spare, l.sharing(Part.growsToMost))
	l.share(spare, l.sharing(Part.growsOn))
	return l.spans(), true
}

// held is the part with its natural within its least and its most, and what
// it keeps where it gives way no more than its least.
func (p Part) held() Part {
	if p.Most > 0 {
		p.Most = max(p.Most, p.Least)
		p.Natural = min(p.Natural, p.Most)
	}
	p.Natural = max(p.Natural, p.Least)
	p.Kept = min(p.Kept, p.Least)
	return p
}

// growsToMost reports whether it grows up to a most.
func (p Part) growsToMost() bool {
	return p.Grows && p.Most > 0
}

// growsOn reports whether it grows with no most.
func (p Part) growsOn() bool {
	return p.Grows && p.Most == 0
}

// laying is the parts as Lay lays them, each held: its cells so far, and
// whether it's given way.
type laying struct {
	parts []Part
	sizes []int
	left  []bool
}

// newLaying is the parts held, each at its natural, but a free part at its
// least.
func newLaying(parts []Part) *laying {
	l := &laying{parts: make([]Part, len(parts)), sizes: make([]int, len(parts)), left: make([]bool, len(parts))}
	for i, p := range parts {
		p = p.held()
		l.parts[i], l.sizes[i] = p, p.Natural
		if p.Narrows == 0 {
			l.sizes[i] = p.Least
		}
	}
	return l
}

// turns are the turns the parts narrow and give way in, the lowest first.
func (l *laying) turns() []int {
	var turns []int
	for _, p := range l.parts {
		for _, turn := range [...]int{p.Narrows, p.Leaves} {
			if turn > 0 {
				turns = append(turns, turn)
			}
		}
	}
	slices.Sort(turns)
	return slices.Compact(turns)
}

// narrow narrows the parts that narrow in the turn toward their leasts, the
// last first, by the cells lacking between them.
func (l *laying) narrow(turn, lacking int) {
	for i, p := range slices.Backward(l.parts) {
		if p.Narrows == turn && !l.left[i] {
			by := min(l.sizes[i]-p.Least, lacking)
			l.sizes[i] -= by
			lacking -= by
		}
	}
}

// leave has the parts that leave in the turn give way to what they keep.
func (l *laying) leave(turn int) {
	for i, p := range l.parts {
		if p.Leaves == turn {
			l.sizes[i], l.left[i] = p.Kept, true
		}
	}
}

// free reports whether the part narrows freely and hasn't given way.
func (l *laying) free(i int) bool {
	return l.parts[i].Narrows == 0 && !l.left[i]
}

// raise raises the free parts toward their naturals, in their order, with the
// cells spare, and returns those still spare.
func (l *laying) raise(spare int) int {
	for i, p := range l.parts {
		if l.free(i) {
			by := min(p.Natural-l.sizes[i], spare)
			l.sizes[i] += by
			spare -= by
		}
	}
	return spare
}

// sharing are the free parts that share what's spare, as such says, those
// marked Odd first, so the odd cell is theirs.
func (l *laying) sharing(such func(Part) bool) []int {
	var odd, rest []int
	for i, p := range l.parts {
		switch {
		case !l.free(i) || !such(p):
		case p.Odd:
			odd = append(odd, i)
		default:
			rest = append(rest, i)
		}
	}
	return append(odd, rest...)
}

// share shares the cells spare evenly among the parts given, each up to its
// most, the odd cells to the first, and returns those still spare.
func (l *laying) share(spare int, among []int) int {
	for spare > 0 {
		among = slices.DeleteFunc(among, func(i int) bool { return l.headroom(i) == 0 })
		if len(among) == 0 {
			break
		}
		each := max(spare/len(among), 1)
		for _, i := range among {
			by := min(each, l.headroom(i), spare)
			l.sizes[i] += by
			spare -= by
		}
	}
	return spare
}

// headroom is how many cells the part may still grow by: to its most, or
// without end where it has none.
func (l *laying) headroom(i int) int {
	if l.parts[i].Most == 0 {
		return math.MaxInt
	}
	return l.parts[i].Most - l.sizes[i]
}

// shown reports whether the part shows: all but one that gave way keeping
// nothing.
func (l *laying) shown(i int) bool {
	return !l.left[i] || l.sizes[i] > 0
}

// spans are where the parts lie, in turn from 0, each shown but the first
// after its gap.
func (l *laying) spans() []Span {
	spans := make([]Span, len(l.parts))
	at, first := 0, true
	for i, p := range l.parts {
		if l.shown(i) {
			if !first {
				at += p.Gap
			}
			first = false
		}
		spans[i] = Span{At: at, Size: l.sizes[i], Left: l.left[i]}
		at += l.sizes[i]
	}
	return spans
}

// taken is how many cells the parts take, their gaps among them.
func (l *laying) taken() int {
	spans := l.spans()
	if len(spans) == 0 {
		return 0
	}
	last := spans[len(spans)-1]
	return last.At + last.Size
}
