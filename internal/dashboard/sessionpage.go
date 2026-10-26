package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// The blanks a session's page sets a row's parts apart by: wide, as its
// state and where it runs, its models, and its windows; and short, as the
// heading under a phone's chrome and the short now rows set them.
const (
	sessionApart      = 3
	sessionShortApart = 2
)

// turnsRuleAt is the column TURNS' rule starts at.
const turnsRuleAt = 9

// noTurns is what TURNS says of a session whose requests name no prompt,
// as one from before Claude Code named them.
const noTurns = "its requests name no prompt, so it has no turns"

var (
	// chosenTurn is the surface of the turn chosen.
	chosenTurn = hue{token: theme.BgSelection}
	// idleInk is a session idle, as row 6 says it.
	idleInk = ink{token: theme.TextFaint, bold: true}
)

// accountsOut is how a session's page says how many accounts it ran on
// have no row: "+2 more accounts".
var accountsOut = outForm{lead: "+", noun: "account", inks: subtleOut}

// turnsOut is how a session's page says how many of its turns are out of
// view, and how to scroll to them: "▼ 3 more turns   ·   j/k to scroll".
var turnsOut = outForm{
	noun: "turn", gap: "   ·   ", keys: "j/k", does: " to scroll",
	inks: outInks{mark: keyMarkInk, space: mutedInk, count: mutedInk, gap: mutedInk, keys: keyInk, does: mutedInk},
}

// sessionDrawn is a session's page as the frame draws it of a moment at
// now: the page open, laid out as sessionLayout lays it out; its accounts
// named as the Overview names them; whether its now rows take their wide
// form, as nowWide has it; its accounts' points' bounds, as pointsCells has
// them; and its accounts' rows' columns and its turns', as stayColumnsOf and
// turnColumnsOf lay them out.
type sessionDrawn struct {
	Frame
	shown        *SessionShown
	page         views.SessionPage
	m            status.Moment
	names        []accountName
	l            sessionLayout
	nowWide      bool
	windowsCells pointsBounds
	stayColumns  []stayColumn
	turnColumns  []turnColumn
	now          time.Time
}

// sessionDrawn is the session's page open, as the frame draws it of doc at
// now.
func (f Frame) sessionDrawn(doc status.Document, now time.Time) sessionDrawn {
	m := f.Moment(doc, now)
	names, page := namesOf(m), f.Session.Page.Live(f.Stream)
	points := pointsCells(measuredKeys(f.listedWindows(m), page.PointsKeys()))
	d := sessionDrawn{
		Frame: f, shown: f.Session, page: page, m: m, names: names,
		l:            f.pageLayout(),
		windowsCells: points,
		stayColumns:  stayColumnsOf(f.Width, stayNameCells(names), points),
		turnColumns:  turnColumnsOf(f.Width, turnFactsOf(page, m)),
		now:          now,
	}
	d.nowWide = d.nowFits()
	return d
}

// sessionPage draws the session's page open, in place of the view, of doc
// at now: its heading; what it's doing now and where it runs; its story; the
// accounts it ran on; and its turns, each as sessionLayout lays it out.
// While it loads, its heading alone.
func (f Frame) sessionPage(c *canvas, doc status.Document, now time.Time) {
	f.sessionDrawn(doc, now).draw(c)
}

// draw draws the page as d lays it out: its heading, then, once it's
// loaded, its now rows, wide or short as nowWide has them, where they
// haven't given way, its story, its accounts and its turns.
func (d sessionDrawn) draw(c *canvas) {
	d.heading(c)
	if d.shown.Loading {
		return
	}
	switch {
	case d.l.nowLeft:
	case d.nowWide:
		d.wideNow(c)
	default:
		d.shortNow(c)
	}
	d.sessionStory(c, d.m, d.l, d.shown.Story, d.chosen())
	d.accounts(c)
	d.turns(c)
}

// nowFits reports whether the session's now rows take their wide form:
// where every part of each fits whole at its least. Row 6: what it's doing
// and where it runs and why, as saidCells counts them, every time at its
// widest, as WhereSaidWidest says them, every mark's cells kept, as marksSpare
// counts them; then at its right when it started in its shortest form at
// its widest, as ranWidest counts it. Row 7: the account it runs on and its
// windows, the widest name and every window listed at 100%, as windows
// draws them. Else they take their short form.
func (d sessionDrawn) nowFits() bool {
	where := d.page.WhereSaidWidest(d.now)
	said := d.saidCells(where) + d.marksSpare(where)
	_, row6 := flex.Lay(d.Width, flex.Part{Least: said, Natural: said}, flex.Part{Least: d.ranWidest(), Gap: sessionApart})
	_, row7 := flex.Lay(d.Width, flex.Part{Least: d.row7Least(), Natural: d.row7Least()})
	return row6 && row7
}

// row7Least are the cells row 7 takes at its least, whole: while the session
// runs, its account's windows, the widest name and every window listed at
// 100%, as windows draws them, sessionApart beside its models; once it's over, its
// models and how long their lines are kept, at its widest, as
// views.KeptUntilSaidWidest says it.
func (d sessionDrawn) row7Least() int {
	if !d.page.Runs() {
		return d.endedModelsKept(views.KeptUntilSaidWidest(d.page.KeptUntil)).width()
	}
	full := func(string) line { return line{{prose.PercentWhole(), mutedInk}} }
	return sessionApart + d.windowsNamed(line{spaces(widestLabel(d.m))}, full).width()
}

// saidCells are the cells row 6's left takes, wide, saying where as given:
// what the session's doing at the widest its kind says, as doingWidest
// counts it, then where it runs and why, whole, as drawn.
func (d sessionDrawn) saidCells(where views.WhereSaid) int {
	return d.doingWidest() + sessionApart + slices.Concat(d.onSince(where.OnSince), d.words(where.Why, mutedInk)).width()
}

// marksSpare are the cells the marks the account where names doesn't carry
// now would take, ▲ and ◆, as a label's column keeps them.
func (d sessionDrawn) marksSpare(where views.WhereSaid) int {
	spare := 0
	for _, p := range where.OnSince {
		if p.Kind == status.PartAccount {
			spare += everyMark.width() - d.nameOf(p.Text).marks().width()
		}
	}
	return spare
}

// everyMark are the marks a name can carry, as a label's column keeps
// their cells: ◆ and ▲.
var everyMark = accountName{primary: true, best: true}.marks()

// doingWidest are the cells the widest of what a session of its kind says
// it's doing takes, as DoingSaidWidest says them.
func (d sessionDrawn) doingWidest() int {
	return ansi.StringWidth(widestOf(d.page.DoingSaidWidest(d.now)))
}

// ranWidest are the cells row 6's right takes at its least, its shortest
// form, how it has run, as RanSaidWidest says it.
func (d sessionDrawn) ranWidest() int {
	return ansi.StringWidth(d.page.RanSaidWidest(d.now))
}

// chosen is the turn chosen: the zero Turn where none is.
func (d sessionDrawn) chosen() views.Turn {
	if i := slices.IndexFunc(d.page.Turns, func(t views.Turn) bool { return t.Turn == d.shown.Turn }); i >= 0 {
		return d.page.Turns[i]
	}
	return views.Turn{}
}

// sessionTargets are the zones of the session's page of doc at now a click
// acts on: a row each of its turns shown, and of its turn open's requests
// shown, which chooses it.
func (f Frame) sessionTargets(doc status.Document, now time.Time) []Target {
	d := f.sessionDrawn(doc, now)
	rows, _, _ := d.turnsShown()
	targets := make([]Target, 0, len(rows))
	for i, r := range rows {
		if r.click.On == 0 {
			continue
		}
		t := r.click
		t.X, t.Y, t.Width = 0, d.l.turnsAt+i, d.Width
		targets = append(targets, t)
	}
	return targets
}

// heading draws the page's heading: SESSION, the session's id and its
// directory's name; wide, the directory, its home as ~, cut from its start
// to leave what's said of its loading, and the rule after it a cell at
// least; what's said of its loading, as loadingSaid has it; wide, the rule;
// and at the right, esc back.
func (d sessionDrawn) heading(c *canvas) {
	y, apart := d.l.heading, sessionShortApart
	if d.l.wide {
		apart = sessionApart
	}
	back := line{{"esc", keyInk}, {" back", mutedInk}}
	end := c.right(d.Width, y, back) - ruleGap
	loading := d.loadingSaid(apart)
	room := end - loading.width()
	x := c.text(0, y, "SESSION", dimInk) + apart
	x = c.text(x, y, status.ShownID(d.page.Session), titleInk)
	if name := status.DirName(d.page.Dir, room-x-apart); name != "" {
		x = c.text(x+apart, y, name, titleInk)
	}
	if d.l.wide {
		if path := status.DirPath(d.page.Dir, d.shown.Home, room-x-ruleGap-2); path != "" {
			x = c.text(x+ruleGap, y, path, mutedInk)
		}
	}
	x = c.line(x, y, loading)
	if d.l.wide {
		c.text(x+1, y, rule(end-x-1), borderInk)
	}
}

// loadingSaid is what the heading says of the page while it loads, after
// what names the session: apart cells, then loading…, in text.subtle;
// nothing once it's loaded.
func (d sessionDrawn) loadingSaid(apart int) line {
	if !d.shown.Loading {
		return nil
	}
	return line{spaces(apart), {loadingWord, dimInk}}
}

// wideNow draws the wide page's two rows of what the session's doing now:
// what it's doing, then where it runs and why, cut to leave room for when
// it started and how it has run, at the right; and under them, where its
// models go and its account's windows, or once it's over, its models and
// how long its lines are kept.
func (d sessionDrawn) wideNow(c *canvas) {
	y := d.l.now
	where := d.page.Where(d.now)
	said := slices.Concat(d.doing(), line{spaces(sessionApart)}, d.onSince(where.OnSince), d.words(where.Why, mutedInk))
	c.line(0, y, said.fit(d.leftOf(c, y, d.startedFit())))
	if !d.page.Runs() {
		c.line(0, y+1, d.endedModels().fit(d.Width))
		return
	}
	c.line(0, y+1, d.models(d.leftOf(c, y+1, d.windows())))
}

// shortNow draws the short form's two rows of what the session's doing now: what
// it's doing and where it runs, since when; then why, short, and at the
// right how it has run.
func (d sessionDrawn) shortNow(c *canvas) {
	y := d.l.now
	where := d.page.Where(d.now)
	named := labelCutAt(d.shortNamesCells(where))
	said := []part{wordsPart(d.doing())}
	if len(where.OnSince) > 0 {
		said = slices.Concat(said, []part{wordsPart(line{spaces(sessionShortApart)})}, d.onSinceParts(where.OnSince, named))
	}
	c.line(0, y, fitParts(d.Width, said...))
	ran := line{{d.page.RanSaid(d.now), secondaryInk}}
	room := c.right(d.Width, y+1, ran) - sessionShortApart
	c.line(0, y+1, fitParts(room, d.wordsParts(where.Short, mutedInk, named)...))
}

// shortNamesCells are the cells the short now rows give each name they
// say, the same in both, as labelCut has them: a label's most where both
// rows fit whole, its names narrowing toward a label's least before
// anything else is cut. Each row is measured with the doing and how it has
// run at their widest, as startedFit measures, so the names hold still as
// the session asks, answers or idles and as its run goes on; and with where
// and why as said now.
func (d sessionDrawn) shortNamesCells(where views.WhereSaid) int {
	ran := ansi.StringWidth(d.page.RanSaidWidest(d.now))
	return labelCut(func(cut int) bool {
		named := labelCutAt(cut)
		row7 := d.doingWidest()
		if len(where.OnSince) > 0 {
			row7 += sessionShortApart + d.onSinceAt(where.OnSince, named).width()
		}
		row8 := d.wordsAt(where.Short, mutedInk, named).width() + sessionShortApart + ran
		return max(row7, row8) <= d.Width
	})
}

// labelCutAt names an account by its label cut to most cells, as
// labelledIn cuts it.
func labelCutAt(most int) func(string) string {
	return func(label string) string { return labelledIn(label, most) }
}

// leftOf draws right at the right of row y, and returns the cells left of
// it for what's drawn from its left, sessionApart cells short of it: the
// whole row where right is nothing.
func (d sessionDrawn) leftOf(c *canvas, y int, right line) int {
	if right.width() == 0 {
		return d.Width
	}
	return c.right(d.Width, y, right) - sessionApart
}

// doing is what the session's doing, as its page says it, bold: asking or
// answering in accent.key, refused in state.destructive, throttled in
// text.subtle, idle in text.faint, and ended in text.secondary; but what's
// in flight as last known as asKnown has it.
func (d sessionDrawn) doing() line {
	a := d.page.Activity
	k := idleInk
	switch a.Doing {
	case views.Asking, views.Answering:
		k = keyInk
	case views.Refused:
		k = exhaustedInk
	case views.Throttled:
		k = labelInk
	case views.Ended:
		k = strongInk
	}
	return line{{a.Said(d.now), asKnown(a, k)}}
}

// started says when the session started and how it has run since, as row
// 6's right says it: "started" and the day in text.subtle, the time in
// text.secondary, and how it has run in text.muted, "started yesterday
// 16:40 · resumed 09:12"; nothing where its start isn't known.
func (d sessionDrawn) started() line {
	said := d.page.StartedSaid(d.now)
	if said == "" {
		return nil
	}
	day, at := "", said
	if i := strings.LastIndex(said, " "); i >= 0 {
		day, at = said[:i+1], said[i+1:]
	}
	return line{{"started " + day, dimInk}, {at, secondaryInk}, {" · " + d.page.RanSaid(d.now), mutedInk}}
}

// startedForms are the forms row 6's right takes, fullest first: when the
// session started and how it has run, as started says it, then how it has
// run alone, as the short now rows say it, in text.secondary. None where its start
// isn't known.
func (d sessionDrawn) startedForms() []line {
	full := d.started()
	if full == nil {
		return nil
	}
	return []line{full, {{d.page.RanSaid(d.now), secondaryInk}}}
}

// startedFit is row 6's right in the fullest of its forms that fits whole
// beside its left as drawn, but for the doing at its widest, so it never
// changes as the session asks, answers or idles. It can change as a time's
// words or the marks do, unlike the switch nowFits makes.
func (d sessionDrawn) startedFit() line {
	forms := d.startedForms()
	if len(forms) == 0 {
		return nil
	}
	return firstFit(d.Width-d.saidCells(d.page.Where(d.now))-sessionApart, forms...)
}

// onSince is the account the session runs on and since when, as words say
// it: the account bold text.primary, with its marks, the rest in
// text.muted.
func (d sessionDrawn) onSince(w status.Words) line {
	return d.onSinceAt(w, asLabelled)
}

// onSinceAt is onSince with the account's name as named names it.
func (d sessionDrawn) onSinceAt(w status.Words, named func(string) string) line {
	return partsSaid(d.onSinceParts(w, named))
}

// onSinceParts are onSinceAt's words as the parts fitParts cuts, each as
// wordPart has it.
func (d sessionDrawn) onSinceParts(w status.Words, named func(string) string) []part {
	parts := make([]part, len(w))
	for i, p := range w {
		said := line{{p.Text, mutedInk}}
		if p.Kind == status.PartAccount {
			n := d.nameOf(p.Text)
			said = append(line{{named(n.label), titleInk}}, n.marks()...)
		}
		parts[i] = wordPart(p.Kind, said)
	}
	return parts
}

// words are words a session's page says, in k, every account they name by
// its name, and every span and figure, in text.secondary.
func (d sessionDrawn) words(w status.Words, k ink) line {
	return d.wordsAt(w, k, asLabelled)
}

// wordsAt are words as words says them, every account's name as name names
// it.
func (d sessionDrawn) wordsAt(w status.Words, k ink, name func(string) string) line {
	return partsSaid(d.wordsParts(w, k, name))
}

// wordsParts are wordsAt's words as the parts fitParts cuts, each as
// wordPart has it.
func (d sessionDrawn) wordsParts(w status.Words, k ink, name func(string) string) []part {
	parts := make([]part, len(w))
	for i, p := range w {
		said := span{p.Text, secondaryInk}
		switch p.Kind {
		case status.PartAccount:
			said.text = name(named(d.m, p.Text))
		case status.PartPlain, status.PartTime:
			said.ink = k
		}
		parts[i] = wordPart(p.Kind, line{said})
	}
	return parts
}

// wordPart is a part of words, of the kind given, said as given: an
// account's name or the words around it as words, cut where they run out of
// room; a time, a span or a figure as a figure, said whole or not at all.
func wordPart(kind status.PartKind, said line) part {
	if kind == status.PartPlain || kind == status.PartAccount {
		return wordsPart(said)
	}
	return figurePart(said, Number{})
}

// partsSaid are the parts given, one after another, whole.
func partsSaid(parts []part) line {
	var l line
	for _, p := range parts {
		l = append(l, p.said...)
	}
	return l
}

// windows are the account the session runs on and its windows now, as row
// 7's right has them: the account in text.secondary, "now" in
// text.subtle, then each window the Overview shows, its name in text.muted
// and its use as usage shows it; nothing where the moment lacks the
// account.
func (d sessionDrawn) windows() line {
	a, ok := d.m.Account(d.page.Account)
	if !ok {
		return nil
	}
	used := func(key string) line { return accountGauge(a, key, d.now).percentage() }
	return d.windowsNamed(line{{asLabelled(name(a)), secondaryInk}}, used)
}

// windowsNamed are row 7's windows as windows draws them, of the account
// named as given, each window listed with its percentage as given.
func (d sessionDrawn) windowsNamed(named line, percentage func(key string) line) line {
	l := append(slices.Clip(named), span{" now  ", dimInk})
	for i, w := range d.listedWindows(d.m) {
		if i > 0 {
			l = append(l, spaces(sessionApart))
		}
		l = append(l, span{strings.ToLower(w.name) + " ", mutedInk})
		l = append(l, percentage(w.key)...)
	}
	return l
}

// models are where the session's models go, as row 7 names them, each its
// id less claude- and its account in text.secondary, the arrow between in
// text.muted, sessionApart cells apart: as many as fit whole in room cells.
func (d sessionDrawn) models(room int) line {
	var l line
	for i, g := range d.page.Goes {
		next := slices.Clip(l)
		if i > 0 {
			next = append(next, spaces(sessionApart))
		}
		next = append(next, span{modelNamed(g.Model), secondaryInk}, span{moveArrow, mutedInk}, span{asLabelled(named(d.m, g.Account)), secondaryInk})
		if next.width() > room {
			break
		}
		l = next
	}
	return l
}

// endedModels are an ended session's models, going nowhere now, each in
// text.secondary and set apart by · in text.faint, then how long the ledger
// keeps its lines, in text.faint.
func (d sessionDrawn) endedModels() line {
	return d.endedModelsKept(views.KeptUntil(d.page.KeptUntil, d.now))
}

// endedModelsKept are an ended session's models as endedModels says them,
// how long their lines are kept said as kept.
func (d sessionDrawn) endedModelsKept(kept string) line {
	var l line
	for i, g := range d.page.Goes {
		if i > 0 {
			l = append(l, span{" · ", faintInk})
		}
		l = append(l, span{modelNamed(g.Model), secondaryInk})
	}
	return append(l, spaces(sessionApart), span{kept, faintInk})
}

// nameOf is the account with the given id's name as the Overview names it,
// numbered as status.Numbering numbers it, with its marks: its id,
// unnumbered, where the moment lacks it.
func (d sessionDrawn) nameOf(id string) accountName {
	if i := slices.IndexFunc(d.m.Accounts, func(a status.AccountNow) bool { return a.ID == id }); i >= 0 {
		return d.names[i]
	}
	return accountName{label: prose.Clean(id)}
}

// accounts draws the accounts the session ran on, a row each, as many as
// the layout has rows for, then a row saying how many more there are.
func (d sessionDrawn) accounts(c *canvas) {
	for i, s := range d.page.Stays(d.now)[:d.l.accounts] {
		d.account(c, d.l.accountsAt+i, s)
	}
	if d.l.more > 0 {
		c.line(0, d.l.accountsAt+d.l.accounts, outOfView(0, d.l.more, d.Width, accountsOut))
	}
}

// stayPart is one of the parts of the row of an account a session ran on.
type stayPart int

// The parts of an account's row, in its order: its name, its time there,
// its requests, the points of its windows the session took, as two parts,
// one column, their short words and what their wide words add, and its
// worth.
const (
	stayName stayPart = iota
	stayTime
	stayRequests
	stayPoints
	stayPointsWide
	stayWorth
)

// The turns an account's row gives way in as it narrows, in this order, so
// it keeps its windows before its other figures, its figures before its
// name, and its name, its points and its worth always: its points' words go
// short, losing no figure; its name narrows to its least; its requests
// leave, then its time there; then its points' windows leave from the end,
// down to one. Growing, they come back in the reverse order.
const (
	stayWordsNarrow = iota + 1
	stayNameNarrows
	stayRequestsLeave
	stayTimeLeaves
	stayWindowsNarrow
)

// The cells of an account's row's parts that hold theirs whole: its time
// there, as prose.StintWidest says it, "28 Sep 13:20 – yesterday 01:20"; its
// requests, as views.StayRequestsSaidWidest, "99,999+ requests"; and its
// worth, as views.StayWorthSaidWidest, "$9999.99+"; stayGap blanks between
// its parts.
var (
	stayTimeCells     = ansi.StringWidth(prose.StintWidest())
	stayRequestsCells = ansi.StringWidth(views.StayRequestsSaidWidest())
	stayWorthCells    = ansi.StringWidth(widestOf(views.StayWorthSaidWidest()))
)

const stayGap = 2

// stayColumn is a part of an account's row as it's laid out: which it is,
// from column x, width cells wide.
type stayColumn struct {
	part     stayPart
	x, width int
}

// pointsBounds are the cells an account's points take, as their windows
// listed bound them: one window alone, the widest, in short words; every
// window in short words; and every window in wide words.
type pointsBounds struct{ one, short, wide int }

// stayParts are the parts of an account's row as flex lays them, stayGap
// apart, each holding its natural until it narrows or leaves in its turn:
// its name from the least to the natural of names given; its points from
// one window to every window in short words, and past it, what their wide
// words add; and the rest at their cells. Nothing grows, so the spare lies
// between its points and its worth, which ends at the last column.
func stayParts(names partCells, points pointsBounds) []flex.Part {
	return []flex.Part{
		stayName:       {Least: names.least, Natural: names.natural, Narrows: stayNameNarrows},
		stayTime:       {Least: stayTimeCells, Natural: stayTimeCells, Leaves: stayTimeLeaves, Gap: stayGap},
		stayRequests:   {Least: stayRequestsCells, Natural: stayRequestsCells, Leaves: stayRequestsLeave, Gap: stayGap},
		stayPoints:     {Least: points.one, Natural: points.short, Narrows: stayWindowsNarrow, Gap: stayGap},
		stayPointsWide: {Natural: points.wide - points.short, Narrows: stayWordsNarrow},
		stayWorth:      {Least: stayWorthCells, Natural: stayWorthCells, Gap: stayGap},
	}
}

// partCells is a part's least and its natural.
type partCells struct{ least, natural int }

// stayColumnsOf are the parts of an account's row shown across a frame
// width cells wide, its names' and its points' cells those given, as
// stayParts lays them out, its points' two parts one column. Where its
// requests or its time there leave, the rest are laid again without them,
// so what they free goes to the name and the points, in their turns,
// rather than lying blank beside a name cut short; where they come back,
// the two give those cells back.
func stayColumnsOf(width int, names partCells, points pointsBounds) []stayColumn {
	parts := stayParts(names, points)
	spans, _ := flex.Lay(width, parts...)
	if spans[stayRequests].Left || spans[stayTime].Left {
		for p, span := range spans {
			if span.Left {
				parts[p] = flex.Part{}
			}
		}
		spans, _ = flex.Lay(width, parts...)
	}
	var columns []stayColumn
	for p, span := range spans {
		last := len(columns) - 1
		switch {
		case stayPart(p) == stayPointsWide:
			if last >= 0 && columns[last].part == stayPoints {
				columns[last].width += span.Size
			}
		case !span.Left && span.Size > 0:
			columns = append(columns, stayColumn{part: stayPart(p), x: span.At, width: span.Size})
		}
	}
	return shortOfWorth(width, columns)
}

// shortOfWorth are the columns given across a frame width cells wide, as
// they're drawn below the row's leasts: the worth whole at the right, and
// every other column ending stayGap cells before it, so the rest give way
// there and never the worth.
func shortOfWorth(width int, columns []stayColumn) []stayColumn {
	worthAt := width - stayWorthCells
	for i, col := range columns {
		if col.part == stayWorth {
			columns[i].x = worthAt
			continue
		}
		columns[i].width = max(0, min(col.width, worthAt-stayGap-col.x))
	}
	return columns
}

// stayNameCells are the least and the natural of the names of accounts'
// rows, of the names given, as recordedAccounts has them, as a session may
// have run on an account no longer configured: each its number, its marks,
// ▲'s cells kept, as headCells counts them, and its label.
func stayNameCells(names []accountName) partCells {
	column := recordedAccounts(0, names)
	return partCells{least: column.Least, natural: column.Natural}
}

// measuredKeys are the keys of the windows an account's points are measured
// by: those listed, and those the session used, as views.SessionPage's
// PointsKeys has them, listed or not.
func measuredKeys(listed []shownWindow, used []string) []string {
	var keys []string
	for _, w := range listed {
		keys = append(keys, w.key)
	}
	for _, key := range used {
		if !slices.Contains(keys, key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// pointsCells are the bounds of an account's points, of the windows with the
// keys given, each at its most, as views.PointsWordsWidest says them, so the
// row's arrangement holds still as its figures change: one window alone,
// the widest, in short words; every window in short words; and every window
// in wide words.
func pointsCells(keys []string) pointsBounds {
	var bounds pointsBounds
	for _, key := range keys {
		bounds.one = max(bounds.one, wordsCells(views.PointsWordsWidest([]string{key}, views.PointsShort)))
	}
	bounds.short = wordsCells(views.PointsWordsWidest(keys, views.PointsShort))
	bounds.wide = wordsCells(views.PointsWordsWidest(keys, views.PointsWide))
	return bounds
}

// wordsCells are the cells the words given take, run together.
func wordsCells(words []status.Words) int {
	return ansi.StringWidth(slices.Concat(words...).String())
}

// account draws the row along row y of an account the session ran on, its
// parts as stayColumnsOf lays them out: its name; its time there and its
// requests, where they show; the points of its windows the session took,
// in their column, wide where it holds every window wide, else short; and
// at the right, its worth.
func (d sessionDrawn) account(c *canvas, y int, s views.Stay) {
	c.right(d.Width, y, line{{s.WorthSaid(), secondaryInk}})
	for _, col := range d.stayColumns {
		switch col.part {
		case stayName:
			n := d.nameOf(s.Account)
			n.label = asLabelled(n.label)
			c.line(col.x, y, n.fitted(col.width+nameGap))
		case stayTime:
			c.text(col.x, y, prose.Cut(s.Said, col.width), secondaryInk)
		case stayRequests:
			c.text(col.x, y, prose.Cut(s.RequestsSaid(), col.width), mutedInk)
		case stayPoints:
			form := views.PointsWide
			if col.width < d.windowsCells.wide {
				form = views.PointsShort
			}
			c.line(col.x, y, d.points(s.SessionAccount, form, col.width))
		}
	}
}

// points are the points of the account's windows the session took, in
// form, the points bold text.primary, each window's numbered, and the rest
// text.muted: as many windows as fit whole in room cells at their most, as
// windowsFit counts them, so what its figures say never moves how many.
func (d sessionDrawn) points(a views.SessionAccount, form views.PointsForm, room int) line {
	var l line
	keys := a.PointsKeys()
	for i, w := range a.PointsWords(form)[:windowsFit(keys, form, room)] {
		for _, p := range w {
			k := mutedInk
			if p.Kind == status.PartFigure {
				k = titleInk.numbered(Number{Kind: NumberPoints, Account: a.Account, Of: keys[i]})
			}
			l = append(l, span{p.Text, k})
		}
	}
	return l
}

// windowsFit are how many of the windows with the keys given fit whole in
// room cells, in form, from the first, each at its most, as
// views.PointsWordsWidest says them.
func windowsFit(keys []string, form views.PointsForm, room int) int {
	widest := views.PointsWordsWidest(keys, form)
	for n := len(widest); n > 0; n-- {
		if wordsCells(widest[:n]) <= room {
			return n
		}
	}
	return 0
}

// turns draws TURNS: its heading; its columns' heads; and its turns shown,
// as far as they're scrolled, the turn open's requests under its own, with a
// row saying how many turns are out of view; or where it has none, why.
func (d sessionDrawn) turns(c *canvas) {
	d.turnsHeading(c)
	if len(d.page.Turns) == 0 {
		if d.page.Totals.Requests > 0 {
			c.line(0, d.l.heads, line{{noTurns, faintInk}}.fit(d.Width))
		}
		return
	}
	for _, col := range d.turnColumns {
		col.draw(c, d.l.heads, col.head())
	}
	rows, above, below := d.turnsShown()
	for i, r := range rows {
		r.draw(c, d.l.turnsAt+i)
	}
	c.line(0, d.l.moreTurns, outOfView(above, below, d.Width, turnsOut))
}

// turnsHeading draws TURNS' heading: TURNS, a rule from turnsRuleAt, a cell
// of it at the least, and at the right the session's totals, its turns,
// requests, share from the cache and worth, or where they don't fit, its
// worth alone.
func (d sessionDrawn) turnsHeading(c *canvas) {
	y, t := d.l.turnsHeading, d.page.Totals
	all := line{
		{prose.Counted(t.Turns, "turn"), secondaryInk},
		{" · " + prose.Counted(t.Requests, "request") + " · " + prose.Percent(t.FromCache) + " from cache · ", mutedInk},
		{t.Cents(), strongInk}, {" at API prices", mutedInk},
	}
	totals := firstFit(d.Width-turnsRuleAt-1-ruleGap, all, line{{t.Cents(), strongInk}})
	end := c.right(d.Width, y, totals) - ruleGap
	c.text(0, y, "TURNS", dimInk)
	c.text(turnsRuleAt, y, rule(end-turnsRuleAt), borderInk)
}

// pageRow is a row of a session's turns: what it is, what a click on it
// does, none where nothing, and how it's drawn along a row.
type pageRow struct {
	turnsRow
	click Target
	draw  func(c *canvas, y int)
}

// turnsShown are the rows of the session's turns in view, newest first, the
// turn open's requests under its own, as turnsInView has them, and how many
// turns are out of view above them and below. Only those in view are made
// rows, as a session of weeks has thousands of turns, and its page is drawn
// at every tick.
func (d sessionDrawn) turnsShown() (rows []pageRow, above, below int) {
	laid, from, to := d.shown.turnsInView(d.l)
	rows = make([]pageRow, 0, to-from)
	for _, r := range laid[from:to] {
		rows = append(rows, d.pageRow(r))
	}
	return rows, turnsIn(laid[:from]), turnsIn(laid[to:])
}

// turn draws the turn t's row along row y, the newest where newest says, in
// the columns shown: lit across them, but ↑ asking's, its words bold, where
// it's chosen.
func (d sessionDrawn) turn(c *canvas, y int, t views.Turn, newest bool) {
	lit := 0
	for _, col := range d.turnColumns {
		col.draw(c, y, d.turnSaid(col, t, newest))
		if col.part != turnAsking {
			lit = col.end()
		}
	}
	if t.Turn == d.shown.Turn {
		c.surface(0, y, lit, chosenTurn)
		c.emboldened(0, y, lit)
	}
}

// turnSaid is what the turn t's row says in the column given, the newest's
// where newest says: its number, bold text.primary; its start; how long it
// took, … after it while it runs; its requests; its tools; the tokens it
// read, wrote, in accent.attention where it moved, and put out; its worth;
// the accounts it went to, as turnOn names them; and the newest's ↑ asking,
// bold accent.key, while a request of it waits; the rest in text.muted.
func (d sessionDrawn) turnSaid(col turnColumn, t views.Turn, newest bool) line {
	muted := func(s string) line { return line{{s, mutedInk}} }
	switch col.part {
	case turnNumber:
		return line{{strconv.Itoa(t.Turn), titleInk}}
	case turnStarted:
		return muted(prose.TimeOfDay(d.now, t.Started))
	case turnTook:
		return muted(t.TookSaid(d.now))
	case turnRequests:
		return muted(t.RequestsSaid())
	case turnTools:
		return muted(toolsWhole(t, col.width))
	case turnRead:
		return muted(prose.Tokens(t.Read))
	case turnWritten:
		if len(t.Accounts) > 1 {
			return line{{prose.Tokens(t.Written), warningInk}}
		}
		return muted(prose.Tokens(t.Written))
	case turnOut:
		return muted(prose.Tokens(t.Out))
	case turnWorth:
		return muted(t.WorthSaid())
	case turnOn:
		return d.turnOn(t.Accounts, col.width)
	case turnAsking:
		if said := d.page.Activity.WaitingSaid(); newest && said != "" {
			return line{{said, asKnown(d.page.Activity, keyInk)}}
		}
	}
	return nil
}

// toolsWhole are the tools the turn names, as many as fit whole in cells,
// as views.Turn's ToolsSaidFirst says them, those that don't leaving from
// the end, as an account's points leave whole windows; where even the first
// doesn't fit, it alone, cut.
func toolsWhole(t views.Turn, cells int) string {
	for n := views.ToolsNamed; n > 1; n-- {
		if said := t.ToolsSaidFirst(n); ansi.StringWidth(said) <= cells {
			return said
		}
	}
	return prose.Cut(t.ToolsSaidFirst(1), cells)
}

// turnOn names the accounts a turn went to, in text.secondary, in cells,
// each at a label's most, as asLabelled cuts it, in the first of these
// that fits whole: each in turn, "work → side"; the arrows narrowed,
// "work→side"; two moves or more as the first and the last, an ellipsis
// between, "work→…→side". Else cut: a move's second name, its first whole,
// down to moveToLeast cells, "work→si…", where that shows the shorter of
// the two at least as much as sharing; else the first and the last share
// the room, as movedCells shares it, the shorter kept whole where it can
// be, "perso…→side"; and where that leaves the first under two cells, the
// first alone, cut, "wo…→…".
func (d sessionDrawn) turnOn(accounts []string, cells int) line {
	names := make([]string, len(accounts))
	for i, id := range accounts {
		names[i] = asLabelled(named(d.m, id))
	}
	first, last := names[0], names[len(names)-1]
	via := moveArrowTight
	if len(names) > 2 {
		via = moveArrowTight + prose.Ellipsis + moveArrowTight
	}
	forms := []string{strings.Join(names, moveArrow), strings.Join(names, moveArrowTight), first + via + last}
	if i := slices.IndexFunc(forms, func(form string) bool { return ansi.StringWidth(form) <= cells }); len(names) < 2 || i >= 0 {
		return line{{forms[max(i, 0)], secondaryInk}}.fit(cells)
	}
	room := cells - ansi.StringWidth(via)
	from, to := ansi.StringWidth(first), ansi.StringWidth(last)
	f, t := movedCells(from, to, room)
	if len(names) == 2 && from+moveToLeast <= room && min(from, room-from) >= min(f, t) {
		return line{{first + via + prose.Cut(last, room-from), secondaryInk}}
	}
	if f < 2 {
		alone := moveArrowTight + prose.Ellipsis
		return line{{prose.Cut(first, cells-ansi.StringWidth(alone)) + alone, secondaryInk}}
	}
	return line{{prose.Cut(first, f) + via + prose.Cut(last, t), secondaryInk}}.fit(cells)
}
