package dashboard

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// phraseGap is the fewest blank cells between two of the phrases under a
// lane.
const phraseGap = 2

// placed is a line placed from a column.
type placed struct {
	x    int
	said line
}

// phrase is what's said under a lane of a stretch, of a reset, or of the
// lane as a whole: its parts, whole ones dropped from the end where they
// don't fit; and the column it's said from, counted from the lanes' first,
// or, where ends is set, the column it ends at, said only where that clears
// the words before; and when what it says of begins, a stretch its start and
// a reset its time, zero where that's not known, as the lane as a whole.
type phrase struct {
	parts []line
	col   int
	ends  bool
	at    time.Time
}

// laneView is a view of Runway's lanes, as what's said under each is said
// of it: the window the view is named for, and the timeline its lanes are
// drawn over. Runway's is the view it shows; a Graphs card's, Runway's live
// view of the card's window.
type laneView struct {
	window   views.RunwayWindow
	timeline views.Timeline
}

// view is the view of its lanes Runway shown as r shows.
func (r RunwayShown) view() laneView {
	return laneView{window: r.Window, timeline: r.Timeline}
}

// runwayWords draws the words under each lane, as laid out, on the row under
// it, from the lanes' first column, as wordsUnder places them, none before
// wordsFrom: cued while its account newly stands so, as wordsCue has it.
func (f Frame) runwayWords(c *canvas, laid runwayLaid) {
	v := f.Runway.view()
	for _, ln := range laid.lanes {
		words := v.wordsUnder(ln, laid.m.Now, f.wordsFrom(ln))
		for _, w := range words {
			c.line(laid.x+w.x, ln.row+1, w.said)
		}
		if fade, ok := f.wordsCue(ln, v.timeline.NowColumn()); ok && len(words) > 0 {
			first, last := words[0], words[len(words)-1]
			cue(c, f.Look, laid.x+first.x, ln.row+1, last.x+last.said.width()-first.x, fade)
		}
	}
}

// wordsFrom is the first column, counted from the lanes' first, that the
// words under the lane ln may start from: with the account column collapsed,
// phraseGap after its use, which starts the row, as collapsedUse has it, as
// though the words before; else the first.
func (f Frame) wordsFrom(ln laidLane) int {
	if !f.Runway.Collapsed {
		return 0
	}
	return line{ln.collapsedUse()}.width() + phraseGap
}

// wordsUnder are the words under the lane ln at now, as said has them, each
// placed from a column counted from the lanes' first, none before from, as
// placeAlong places them: its phrases, or where none of them is placed,
// what's said instead.
func (v laneView) wordsUnder(ln laidLane, now time.Time, from int) []placed {
	cols := v.timeline.Columns
	says := v.said(ln, now)
	if words := placeAlong(cols, from, says.phrases...); words != nil {
		return words
	}
	return placeAlong(cols, from, says.instead...)
}

// laneSays is what's said under a lane: its phrases, in order; and what's
// said where none of them is, none where nothing is.
type laneSays struct {
	phrases, instead []phrase
}

// said is what's said under the lane ln at now: of an account whose first
// read failed, where the view holds now, that it can't be read, and why, in
// text.subtle; on a day or week gone that read nothing of it, that, in
// text.faint; else what phrasesOf says, and where none of that is said, of a
// lane with room throughout, that it has, in text.subtle, from the cell after
// now's column, or stepped back, the first.
func (v laneView) said(ln laidLane, now time.Time) laneSays {
	tl := v.timeline
	_, _, named := tl.Named()
	switch {
	case ln.account.Routing.Kind == status.RoutingUnreadable && tl.NowColumn() >= 0:
		return laneSays{phrases: []phrase{{parts: []line{{{unreadable(ln.account.Routing), dimInk}}}}}}
	case named && tl.Now.IsZero() && !slices.ContainsFunc(ln.Cells, func(c views.Cell) bool { return c != views.CellUnknown }):
		return laneSays{phrases: []phrase{{parts: []line{{{"nothing read that " + v.period(), faintInk}}}}}}
	}
	says := laneSays{phrases: v.phrasesOf(ln, v.datedAgainst(now))}
	if ln.Room {
		from := tl.NowColumn() + 1
		if named {
			from = 0
		}
		says.instead = []phrase{{parts: []line{{{"room all " + v.period(), dimInk}}}, col: from}}
	}
	return says
}

// unreadable is what's said of an account whose first read failed, as its
// routing r says it: that it can't be read, and why, as in
// "can't read it: HTTP 529 · Overloaded".
func unreadable(r status.Routing) string {
	if len(r.Clauses) == 0 {
		return r.Says
	}
	return r.Says + ": " + strings.Join(r.Clauses, " · ")
}

// phrasesOf are the phrases said under the lane ln, in the order of their
// columns, times told against the time given: each stretch without room not
// over, from the column its start falls in, as stretchParts words it; and in
// the week view, each reset of the week no stretch ends at, as in "resets Sun
// 21:00", muted, ending at its │.
func (v laneView) phrasesOf(ln laidLane, against time.Time) []phrase {
	var phrases []phrase
	for _, s := range ln.Stretches {
		if !s.Over {
			phrases = append(phrases, v.stretchPhrase(s, ln.account, against))
		}
	}
	if v.window == views.RunwayWeek {
		for _, r := range ln.Resets {
			if !r.Back {
				phrases = append(phrases, phrase{parts: []line{{{"resets " + prose.Dated(against, r.At), mutedInk}}}, col: r.Column, ends: true, at: r.At})
			}
		}
	}
	slices.SortStableFunc(phrases, func(a, b phrase) int { return cmp.Compare(a.col, b.col) })
	return phrases
}

// stretchPhrase is what's said of the stretch s without room of the account
// a, times told against the time given, as stretchParts words it, from the
// column its words start in.
func (v laneView) stretchPhrase(s views.LaneStretch, a status.AccountNow, against time.Time) phrase {
	return phrase{parts: v.stretchParts(s, a, against), col: s.Column, at: s.Start}
}

// stretchParts are what's said of the stretch s without room of the account
// a, times told against the time given: what holds it back, from when where
// that's known, as headOf words it, of the timeline shown; then, muted, when
// it ends, where that's known, as its Ends has it: as its own hold lifts, of
// one of a run of stretches but the last, as "  ·  lifts 19:00", even as the
// view's window resets, as a lift isn't room returning; else, of a stretch
// whose head says so, where room returns as the view's window resets, as it
// resets, as "  ·  resets Sun 01:00"; else as it's back, as "  ·  back
// 17:10".
func (v laneView) stretchParts(s views.LaneStretch, a status.AccountNow, against time.Time) []line {
	head, k, resets := headOf(s, a, v.timeline, against)
	parts := []line{{{head, k}}}
	ends, lifts := s.Ends()
	if ends.IsZero() {
		return parts
	}
	said := "back "
	switch {
	case lifts:
		said = "lifts "
	case resets && s.AsItResets:
		said = "resets "
	}
	return append(parts, line{{prose.Separator + said + prose.Dated(against, ends), mutedInk}})
}

// headOf is what's said first of the stretch s without room of the account
// a, on the timeline tl, times told against the time given, but a run-out's,
// told at tl's now as prose.RunOut tells it, as only a view holding now
// projects one, in its ink, and whether its end is said as the view's window
// resets, where it is: at its cap, as "capped 14:31", or a run-out reaching
// it, as "caps ~15:45"; a run-out reaching its limit, as "out ~16:05"; each
// bold in accent.attention, and each ending as the window resets, as each
// ends then. At its limit, as "out 13:58", the ~ and the ink telling it from
// a run-out; its token refused, as "token refused (401) 14:12", the status
// the refusal holding it gave, as refusedWith has it; or without a token of
// its own, as prose.Tokenless says it, "no token", or "wrong token" where
// the account holds another subscription's; each bold in state.destructive,
// and none ending as the window resets, as each lifts whenever it does. A
// start no one told leaves its time unsaid.
func headOf(s views.LaneStretch, a status.AccountNow, tl views.Timeline, against time.Time) (string, ink, bool) {
	when := ""
	if !s.Start.IsZero() {
		when = " " + prose.Dated(against, s.Start)
	}
	if s.Projected {
		when = " ~" + prose.RunOut(tl.Now, s.Start)
	}
	switch {
	case s.Cell == views.CellCapped && s.Projected:
		return "caps" + when, alertInk, true
	case s.Cell == views.CellCapped:
		return "capped" + when, alertInk, true
	case s.Cell == views.CellLimited && s.Projected:
		return "out" + when, alertInk, true
	case s.Cell == views.CellLimited:
		return "out" + when, exhaustedInk, false
	case s.Cell == views.CellRefused:
		return "token refused" + refusedWith(s, a, tl.NowColumn()) + when, exhaustedInk, false
	}
	return prose.Tokenless(a.TokenForeign), exhaustedInk, false
}

// refusedWith is the status the upstream refused the account a's token with,
// as " (401)", where the stretch s is the refusal holding it at the moment,
// as it reaches now's column, now, or past it: none of a refusal gone, whose
// status the moment no longer holds, nor of one given none.
func refusedWith(s views.LaneStretch, a status.AccountNow, now int) string {
	if !a.TokenRefused || a.Refusal.Status == 0 || now < 0 || s.Last < now {
		return ""
	}
	return " (" + strconv.Itoa(a.Refusal.Status) + ")"
}

// placeAlong places the phrases given, in order, along a row cols columns
// wide: each from its column, or, where all its parts don't fit from there,
// shifted left as far as they need, to end at the row's last column, but
// never sooner than from, nor phraseGap cells after the words before, as
// many of its parts, whole, as fit from where it starts; or, where it ends at
// its column, there, whole, where that clears both; and none from the first
// not ending at its column whose first part doesn't fit, wherever it would
// start.
func placeAlong(cols, from int, phrases ...phrase) []placed {
	var words []placed
	next := from
	for _, p := range phrases {
		said := slices.Concat(p.parts...)
		start := max(next, min(p.col, cols-said.width()))
		if p.ends {
			if start = p.col - said.width() + 1; start < next {
				continue
			}
		} else if said = wholeParts(p.parts, cols-start); said == nil {
			break
		}
		words = append(words, placed{x: start, said: said})
		next = start + said.width() + phraseGap
	}
	return words
}

// wholeParts are as many of the parts given as fit in cells cells, whole,
// from the first: none where the first doesn't.
func wholeParts(parts []line, cells int) line {
	var l line
	for _, p := range parts {
		if l.width()+p.width() > cells {
			break
		}
		l = append(l, p...)
	}
	return l
}

// wordsCue is how far the cue on the lane ln's words has faded, now's column
// now, reporting false where they're not cued: as its account newly stands
// at its cap or its limit, or with its token refused, as the frame's Changed
// has its state newly changed and its lane has it so at now; or as a window
// of it every model shares newly runs out, as the frame's TurnedOut has it,
// where its lane draws a run-out; the later cue, of both.
func (f Frame) wordsCue(ln laidLane, now int) (float64, bool) {
	fade, cued := 1.0, false
	if c, ok := f.Changed[ln.ID]; ok && ln.heldAt(now) {
		fade, cued = c, true
	}
	if !slices.ContainsFunc(ln.Stretches, func(s views.LaneStretch) bool { return s.Projected }) {
		return fade, cued
	}
	for _, w := range ln.account.Windows {
		if c, ok := f.TurnedOut[Ref{Account: ln.ID, Window: w.Key}]; ok && w.Shared {
			fade, cued = min(fade, c), true
		}
	}
	return fade, cued
}

// heldAt reports whether the lane has its account at its cap or its limit,
// or its token refused, in column col: not where col is none of its.
func (ln laidLane) heldAt(col int) bool {
	if col < 0 || col >= len(ln.Cells) {
		return false
	}
	return slices.Contains([]views.Cell{views.CellCapped, views.CellLimited, views.CellRefused}, ln.Cells[col])
}

// datedAgainst is the moment the times said under the lanes of the view v
// are dated against, as prose.Dated dates them: on a day stepped back to,
// today too, its start, so its own times read alone and the next day's with
// their weekday; else now, as in the live views and a week stepped back to.
func (v laneView) datedAgainst(now time.Time) time.Time {
	if first, last, ok := v.timeline.Named(); ok && first.Equal(last) {
		return v.timeline.Start
	}
	return now
}

// period is what the view v calls the span it looks across, as "room all
// day" and "the day's highest" say it: a day, of the 5-hour view, live or
// stepped back, or a week.
func (v laneView) period() string {
	if v.window == views.RunwayWeek {
		return "week"
	}
	return "day"
}
