package dashboard

import (
	"slices"
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayPool draws the pool's lane, as laid out: its rule across the row;
// its columns, with Σ pool beside them; its room, where it has one; and its
// words, from phraseGap cells after the room where they share its row, else
// from the lanes' column, as poolPhrases has them. With one account, there's
// none.
func (f Frame) runwayPool(c *canvas, laid runwayLaid) {
	fit := laid.fit
	if !fit.draws(fit.pool) {
		return
	}
	pool := f.Runway.Lanes.Pool
	c.text(0, fit.rule, rule(f.Width), borderInk)
	f.poolColumns(c, laid, pool)
	c.line(0, fit.pool+1, poolNamed(namesOf(laid.m)))
	from := laid.x
	if fit.draws(fit.room) {
		if end := f.poolRoom(c, laid, pool.Key); fit.room == fit.words {
			from = max(from, end+phraseGap)
		}
	}
	for _, p := range f.Runway.poolPhrases(pool, laid.m.Now, laid.x, from, f.Width) {
		c.line(p.x, fit.words, p.said)
	}
}

// poolColumns draws the pool's columns on its lane's rows, from the lanes'
// column, as fillUp fills a column: each as high as the share of the pool's
// accounts with room at the column's moment, of its rows, in eighths of a
// cell, in the colour usage gives the pool's use, the share without room,
// faded as the past is before now's column. A column where none has room is
// left blank, as is one where none of their states is known, as none of them
// counts with room.
func (f Frame) poolColumns(c *canvas, laid runwayLaid, pool views.Pool) {
	now, floor := f.Runway.Timeline.NowColumn(), laid.fit.pool+runwayPoolRows-1
	for i, col := range pool.Columns {
		share := float64(col.With) / float64(pool.Of)
		tone := usedInk(1-share, ink{token: theme.VizRamp1})
		if i < now {
			tone.fade = pastFade
		}
		fillUp(c, f.Look, laid.x+i, floor, halfEven(share*8*runwayPoolRows), func(int) ink { return tone })
	}
}

// poolRoom draws the pool's room in the window with the key given, the
// view's, along its row from runwayUseFrom, as usage words the pool's: the
// window's name, as in "5-hour ", in text.subtle, then as poolWords has it,
// as in "1.5 of 3 left · pinned", its figure brightening as it changes; and
// returns the column after it. Where none of the pool's accounts has the
// window, it draws nothing.
func (f Frame) poolRoom(c *canvas, laid runwayLaid, key string) int {
	pool := laid.m.Pool()
	i := slices.IndexFunc(pool.Windows, func(w status.PoolWindowNow) bool { return w.Key == key })
	if i < 0 {
		return runwayUseFrom
	}
	g := gauge{of: Ref{Window: key}, words: poolWords(pool.Windows[i].Room, len(pool.Accounts), pool.Pinned)}
	x := c.text(runwayUseFrom, laid.fit.room, runwayNames[f.Runway.Window]+" ", dimInk)
	return c.line(x, laid.fit.room, g.said(f.Width, f.Width-x))
}

// placed is a line placed from a column.
type placed struct {
	x    int
	said line
}

// poolPhrases are the pool's words, at now, a phrase for each of its dips,
// in time order: each from its dip's column, from the lanes' column x, or
// phraseGap cells after the phrase before, from column from at the least, as
// dipParts says it; or, where it would start nearer the phrase before than
// that, joined to it, as joining says. A dip already over isn't said in the
// live views. Words that don't fit before the column edge drop whole parts
// from the end, so a dip whose first part doesn't fit isn't said, nor any
// after it.
func (r RunwayShown) poolPhrases(pool views.Pool, now time.Time, x, from, edge int) []placed {
	against, nowAt := r.datedAgainst(now), r.Timeline.NowColumn()
	w := phrasing{after: from, endsWith: -1, edge: edge}
	for _, d := range pool.Dips {
		if d.Over && r.Back.IsZero() {
			continue
		}
		steps, underWay := saidSteps(d, nowAt)
		at := x + d.Column
		if and, ok := joining(steps, underWay, w.endsWith, against); ok && at < w.after {
			if !w.join(and) {
				break
			}
			continue
		}
		if !w.lay(max(at, w.after), dipParts(steps, pool.Of, underWay, against), steps) {
			break
		}
	}
	return w.phrases
}

// phrasing is the pool's words as they're laid along their row: the phrases
// laid; the first column the next may start at; the count the last ends on,
// which a dip joining it has, -1 before any; and the column they end before.
type phrasing struct {
	phrases               []placed
	after, endsWith, edge int
}

// lay lays a phrase of the parts given from column x, each a step of those
// given said, as many whole as fit, reporting false where any doesn't.
func (w *phrasing) lay(x int, parts []line, steps []views.Step) bool {
	p := placed{x: x}
	for i, part := range parts {
		if x+p.said.width()+part.width() > w.edge {
			if i > 0 {
				w.phrases = append(w.phrases, p)
			}
			return false
		}
		p.said = append(p.said, part...)
	}
	w.phrases = append(w.phrases, p)
	w.after, w.endsWith = x+p.said.width()+phraseGap, steps[len(parts)-1].With
	return true
}

// join joins the words given to the last phrase, reporting false where they
// don't fit.
func (w *phrasing) join(and line) bool {
	last := &w.phrases[len(w.phrases)-1]
	if last.x+last.said.width()+and.width() > w.edge {
		return false
	}
	last.said = append(last.said, and...)
	w.after += and.width()
	return true
}

// saidSteps are the steps of the dip d the pool's words say, now in the
// column given: under way, holding now, those from the one holding it;
// else all of them.
func saidSteps(d views.Dip, now int) (steps []views.Step, underWay bool) {
	if now < d.First || now > d.Last {
		return d.Steps, false
	}
	i := slices.IndexFunc(d.Steps, func(s views.Step) bool { return s.Last >= now })
	return d.Steps[i:], true
}

// dipParts are the parts of a dip's phrase, of the pool of the accounts
// given, a part a step said: the first its count of the pool, as in "8 of
// 10", in text.secondary, then in text.muted " with room", and when, as
// stepSpan has it, its start unsaid where it's under way; each after it
// chained on, ", " in text.muted, its count in text.secondary and " till" its
// end in text.muted, as in ", 9 till ~16:05", while the end of each, and of
// the one before it, is known.
func dipParts(steps []views.Step, of int, underWay bool, against time.Time) []line {
	lead := steps[0]
	if underWay {
		lead.From = views.Edge{}
	}
	parts := []line{{{strconv.Itoa(lead.With) + " of " + strconv.Itoa(of), secondaryInk}, {" with room" + stepSpan(lead, against), mutedInk}}}
	for i, s := range steps[1:] {
		if steps[i].Until.At.IsZero() || s.Until.At.IsZero() {
			break
		}
		parts = append(parts, line{{", ", mutedInk}, {strconv.Itoa(s.With), secondaryInk}, {" till " + edgeSaid(s.Until, against), mutedInk}})
	}
	return parts
}

// joining is a dip's phrase joined to the phrase before, as " and" and its
// span, in text.muted, as in "2 of 3 with room 15:02 – 17:10 and 19:12 –
// 23:58", reporting false where it doesn't join: a dip of one count, the
// same as the one the phrase before ends on, endsWith, and not under way.
func joining(steps []views.Step, underWay bool, endsWith int, against time.Time) (line, bool) {
	if underWay || len(steps) != 1 || steps[0].With != endsWith {
		return nil, false
	}
	return line{{" and" + stepSpan(steps[0], against), mutedInk}}, true
}

// stepSpan says when a step of a dip is, after a space, as known: from its
// start to its end, as in " 15:02 – 17:10"; till its end, where its start
// isn't known, as in " till 15:30"; from its start, where its end isn't, as
// in " from ~16:05"; and nothing where neither is. Each is said as edgeSaid
// says it.
func stepSpan(s views.Step, against time.Time) string {
	started, ends := !s.From.At.IsZero(), !s.Until.At.IsZero()
	switch {
	case started && ends:
		return " " + edgeSaid(s.From, against) + " – " + edgeSaid(s.Until, against)
	case ends:
		return " till " + edgeSaid(s.Until, against)
	case started:
		return " from " + edgeSaid(s.From, against)
	}
	return ""
}

// edgeSaid says when the pool's count changed, as prose.Dated dates it
// against the moment given: after ~ where a run-out projects it.
func edgeSaid(e views.Edge, against time.Time) string {
	if e.Projected {
		return "~" + prose.Dated(against, e.At)
	}
	return prose.Dated(against, e.At)
}

// datedAgainst is the moment the times Runway shown as r says are dated
// against, as prose.Dated dates them: on a day stepped back to, today too,
// its start, so its own times read alone and the next day's with their
// weekday; else now, as in the live views and a week stepped back to.
func (r RunwayShown) datedAgainst(now time.Time) time.Time {
	if first, last, ok := r.Timeline.Named(); ok && first.Equal(last) {
		return r.Timeline.Start
	}
	return now
}
