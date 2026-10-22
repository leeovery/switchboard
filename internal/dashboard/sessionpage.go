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
// state and where it runs, its models, and its windows; and on a phone.
const (
	sessionApart      = 3
	sessionPhoneApart = 2
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
// form, as nowWide has it; and its accounts' rows' columns and its turns',
// as stayColumnsOf and turnColumnsOf lay them out.
type sessionDrawn struct {
	Frame
	shown       *SessionShown
	page        views.SessionPage
	m           status.Moment
	names       []accountName
	l           sessionLayout
	nowWide     bool
	stayColumns []stayColumn
	turnColumns []turnColumn
	now         time.Time
}

// sessionDrawn is the session's page open, as the frame draws it of doc at
// now.
func (f Frame) sessionDrawn(doc status.Document, now time.Time) sessionDrawn {
	m := f.Moment(doc, now)
	names, page := namesOf(m), f.Session.Page.Live(f.Stream)
	d := sessionDrawn{
		Frame: f, shown: f.Session, page: page, m: m, names: names,
		l:           f.pageLayout(),
		stayColumns: stayColumnsOf(f.Width, stayNameCells(names), pointsCells(f.listedWindows(m))),
		turnColumns: turnColumnsOf(f.Width, turnFactsOf(page, m)),
		now:         now,
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
// loaded, its now rows, wide or short as nowWide has them, its story, its
// accounts and its turns.
func (d sessionDrawn) draw(c *canvas) {
	d.heading(c)
	if d.shown.Loading {
		return
	}
	if d.nowWide {
		d.wideNow(c)
	} else {
		d.phoneNow(c)
	}
	d.sessionStory(c, d.m, d.l, d.shown.Story, d.chosen())
	d.accounts(c)
	d.turns(c)
}

// nowFits reports whether the session's now rows take their wide form:
// where every part of each fits whole at its least. Row 6: what it's doing
// and where it runs and why, as saidCells counts them, every time at its
// widest, as WhereWidest says them, then at its right when it started in
// its shortest form at its widest, as ranWidest counts it. Row 7: the account it runs on and its windows, the widest name and
// every window listed at 100%, as windows draws them. Else they take their
// short form, a phone's, which says the why short and whole.
func (d sessionDrawn) nowFits() bool {
	said := d.saidCells(d.page.WhereWidest(d.now))
	_, row6 := flex.Lay(d.Width, flex.Part{Least: said, Natural: said}, flex.Part{Least: d.ranWidest(), Gap: sessionApart})
	right := widestLabel(d.m) + ansi.StringWidth(" now  ")
	for i, w := range d.listedWindows(d.m) {
		if i > 0 {
			right += sessionApart
		}
		right += ansi.StringWidth(strings.ToLower(w.name) + " 100%")
	}
	_, row7 := flex.Lay(d.Width, flex.Part{Grows: true}, flex.Part{Least: right, Gap: sessionApart})
	return row6 && row7
}

// saidCells are the cells row 6's left takes, wide, saying where as given:
// what the session's doing at the widest its kind says, as doingWidest
// counts it, then where it runs and why, whole, and the account it's on
// with every mark's cells, ▲ and ◆, as a label's column keeps them,
// whichever it carries now. Where and why aren't bounded, as an ended
// session's name every account it ran on, but change only as it moves, is
// resumed or ends, and as the clock moves a time's day on, where they're
// said as they are now: never as its turns come and go, or new sessions go
// elsewhere.
func (d sessionDrawn) saidCells(where views.WhereSaid) int {
	cells := slices.Concat(d.onSince(where.OnSince), d.words(where.Why, mutedInk)).width()
	for _, p := range where.OnSince {
		if p.Kind == status.PartAccount {
			cells += everyMark.width() - d.nameOf(p.Text).marks().width()
		}
	}
	return d.doingWidest() + sessionApart + cells
}

// everyMark are the marks a name can carry, as a label's column keeps
// their cells: ◆ and ▲.
var everyMark = accountName{primary: true, best: true}.marks()

// doingWidest are the cells the widest of what a session of its kind is
// doing says: once it's over, its end the day before, the widest
// prose.Past says a day, as "ended yesterday 18:46"; while it runs, the
// widest of asking, answering, throttled, refused, and idle to the
// IdleHour, after which it ends. A session's kind changes once, as it ends.
func (d sessionDrawn) doingWidest() int {
	if !d.page.Runs() {
		return ansi.StringWidth(views.Activity{Doing: views.Ended, Since: d.now.AddDate(0, 0, -1)}.Said(d.now))
	}
	widest := 0
	for _, a := range []views.Activity{
		{Doing: views.Asking}, {Doing: views.Answering}, {Doing: views.Throttled, Status: 429}, {Doing: views.Refused, Status: 403},
		{Doing: views.Idle, Since: d.now.Add(time.Second - views.IdleHour)},
	} {
		widest = max(widest, ansi.StringWidth(a.Said(d.now)))
	}
	return widest
}

// ranWidest are the cells row 6's right takes at its least, its shortest
// form, how it has run, at its widest for the session's own kind: once over,
// how long it ran, as "ran 23h 59m"; resumed, when, the day before, as
// "resumed yesterday 22:10"; else how long it has run, as "23h 59m". Nothing
// where its start isn't known.
func (d sessionDrawn) ranWidest() int {
	if d.page.Started.IsZero() {
		return 0
	}
	widest := views.SessionPage{Started: d.now.Add(time.Minute - 24*time.Hour), Activity: views.Activity{Doing: views.Idle}}
	switch {
	case !d.page.Runs():
		widest.Activity = views.Activity{Doing: views.Ended, Since: d.now}
	case !d.page.Resumed.IsZero():
		widest.Resumed = d.now.AddDate(0, 0, -1)
	}
	return ansi.StringWidth(widest.RanSaid(d.now))
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
	y, apart := d.l.heading, sessionPhoneApart
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

// phoneNow draws a phone's two rows of what the session's doing now: what
// it's doing and where it runs, since when; then why, short, and at the
// right how it has run.
func (d sessionDrawn) phoneNow(c *canvas) {
	y := d.l.now
	where := d.page.Where(d.now)
	said := d.doing()
	if len(where.OnSince) > 0 {
		said = slices.Concat(said, line{spaces(sessionPhoneApart)}, d.onSince(where.OnSince))
	}
	c.line(0, y, said.fit(d.Width))
	ran := line{{d.page.RanSaid(d.now), secondaryInk}}
	room := c.right(d.Width, y+1, ran) - sessionPhoneApart
	c.line(0, y+1, d.words(where.Short, mutedInk).fit(room))
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
// run alone, as a phone's says it, in text.secondary. None where its start
// isn't known.
func (d sessionDrawn) startedForms() []line {
	full := d.started()
	if full == nil {
		return nil
	}
	return []line{full, {{d.page.RanSaid(d.now), secondaryInk}}}
}

// startedFit is row 6's right in the fullest of its forms that fits whole
// beside what's said at its left, as saidCells counts it, where and why as
// they're said now, so the form holds still as the session's doing changes.
// Unlike nowFits, it doesn't measure their times at their widest: the right
// gives way within the wide form, so it can take its fuller form as soon as
// it fits, and changes only where a time's words do, at midnight or a week
// on, while the switch to the short form, which loses rows, holds still.
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
	var l line
	for _, p := range w {
		if p.Kind != status.PartAccount {
			l = append(l, span{p.Text, mutedInk})
			continue
		}
		n := d.nameOf(p.Text)
		l = append(append(l, span{nowLabel(n.label), titleInk}), n.marks()...)
	}
	return l
}

// words are words a session's page says, in k, every account they name by
// its name, and every span and figure, in text.secondary.
func (d sessionDrawn) words(w status.Words, k ink) line {
	l := make(line, len(w))
	for i, p := range w {
		switch p.Kind {
		case status.PartAccount:
			l[i] = span{nowLabel(named(d.m, p.Text)), secondaryInk}
		case status.PartPlain:
			l[i] = span{p.Text, k}
		default:
			l[i] = span{p.Text, secondaryInk}
		}
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
	l := line{{nowLabel(name(a)), secondaryInk}, {" now  ", dimInk}}
	for i, w := range d.listedWindows(d.m) {
		if i > 0 {
			l = append(l, spaces(sessionApart))
		}
		l = append(l, span{strings.ToLower(w.name) + " ", mutedInk})
		l = append(l, accountGauge(a, w.key, d.now).percentage()...)
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
		next = append(next, span{modelNamed(g.Model), secondaryInk}, span{moveArrow, mutedInk}, span{nowLabel(named(d.m, g.Account)), secondaryInk})
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
	var l line
	for i, g := range d.page.Goes {
		if i > 0 {
			l = append(l, span{" · ", faintInk})
		}
		l = append(l, span{modelNamed(g.Model), secondaryInk})
	}
	return append(l, spaces(sessionApart), span{views.KeptUntil(d.page.KeptUntil, d.now), faintInk})
}

// nowLabel is an account's label as the page's now rows draw it, as a
// label's column holds it at its most: cut with an ellipsis past
// labelColumnCells, so what's drawn is what nowFits measures.
func nowLabel(label string) string {
	return prose.Cut(label, labelColumnCells(label))
}

// nameOf is the account with the given id's name as the Overview names it,
// numbered in the config's order, with its marks: its id, unnumbered, where
// the moment lacks it.
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
// its requests, the points of its windows the session took, and its worth.
const (
	stayName stayPart = iota
	stayTime
	stayRequests
	stayPoints
	stayWorth
)

// The turns an account's row gives way in as it narrows, in this order, so
// it keeps its name, its points and its worth: its points narrow to their
// short words; its requests leave, then its time there; and its name
// narrows to its least.
const (
	stayPointsNarrow = iota + 1
	stayRequestsLeave
	stayTimeLeaves
	stayNameNarrows
)

// The cells of an account's row's parts that hold theirs whole: its time
// there, as "yesterday 13:20 – 16:01"; its requests, as "12,345 requests";
// and its worth, as "$114.11"; stayGap blanks between its parts.
const (
	stayTimeCells     = 24
	stayRequestsCells = 14
	stayWorthCells    = 7
	stayGap           = 2
)

// stayColumn is a part of an account's row as it's laid out: which it is,
// from column x, width cells wide.
type stayColumn struct {
	part     stayPart
	x, width int
}

// stayParts are the parts of an account's row as flex lays them, stayGap
// apart, each holding its natural until it narrows or leaves in its turn:
// its name from the least to the natural of names given; its points from
// the least to the natural of points given; and the rest at their cells.
// Nothing grows, so the spare lies between its points and its worth, which
// ends at the last column.
func stayParts(names, points partCells) []flex.Part {
	return []flex.Part{
		stayName:     {Least: names.least, Natural: names.natural, Narrows: stayNameNarrows},
		stayTime:     {Least: stayTimeCells, Natural: stayTimeCells, Leaves: stayTimeLeaves, Gap: stayGap},
		stayRequests: {Least: stayRequestsCells, Natural: stayRequestsCells, Leaves: stayRequestsLeave, Gap: stayGap},
		stayPoints:   {Least: points.least, Natural: points.natural, Narrows: stayPointsNarrow, Gap: stayGap},
		stayWorth:    {Least: stayWorthCells, Natural: stayWorthCells, Gap: stayGap},
	}
}

// partCells is a part's least and its natural.
type partCells struct{ least, natural int }

// stayColumnsOf are the parts of an account's row shown across a frame
// width cells wide, its names' and its points' cells those given, as
// stayParts lays them out.
func stayColumnsOf(width int, names, points partCells) []stayColumn {
	spans, _ := flex.Lay(width, stayParts(names, points)...)
	var columns []stayColumn
	for p, span := range spans {
		if !span.Left {
			columns = append(columns, stayColumn{part: stayPart(p), x: span.At, width: span.Size})
		}
	}
	return columns
}

// stayNameCells are the least and the natural of the names of accounts'
// rows, of the names given, as a label's column has them: each its number,
// its marks, ▲'s cells kept, as headCells counts them, and its label,
// labelLeast cells of it at the least and the widest, labelMost at the most,
// as labelColumnCells counts it, at the natural.
func stayNameCells(names []accountName) partCells {
	var cells partCells
	for _, n := range names {
		cells.least = max(cells.least, n.cells(labelLeast))
		cells.natural = max(cells.natural, n.cells(labelColumnCells(n.label)))
	}
	return cells
}

// pointsCells are the least and the natural of an account's points, of the
// windows given, each at its most, a hundred points: whole, in their short
// words and in their wide.
func pointsCells(windows []shownWindow) partCells {
	most := views.SessionAccount{Points: make(map[string]int, len(windows))}
	for _, w := range windows {
		most.Points[w.key] = 100
	}
	return partCells{least: pointsWhole(most, views.PointsShort), natural: pointsWhole(most, views.PointsWide)}
}

// pointsWhole are the cells the account's points take whole in form.
func pointsWhole(a views.SessionAccount, form views.PointsForm) int {
	return ansi.StringWidth(slices.Concat(a.PointsWords(form)...).String())
}

// account draws the row along row y of an account the session ran on, its
// parts as stayColumnsOf lays them out: its name; its time there and its
// requests, where they show; the points of its windows the session took, to
// stayGap cells short of its worth, wide where they fit whole, else short;
// and at the right, its worth.
func (d sessionDrawn) account(c *canvas, y int, s views.Stay) {
	worth := c.right(d.Width, y, line{{s.Cents(), secondaryInk}})
	for _, col := range d.stayColumns {
		switch col.part {
		case stayName:
			c.line(col.x, y, d.nameOf(s.Account).fitted(col.width+nameGap))
		case stayTime:
			c.text(col.x, y, prose.Cut(s.Said, col.width), secondaryInk)
		case stayRequests:
			c.text(col.x, y, prose.Cut(prose.Counted(s.Requests, "request"), col.width), mutedInk)
		case stayPoints:
			room, form := worth-stayGap-col.x, views.PointsWide
			if pointsWhole(s.SessionAccount, form) > room {
				form = views.PointsShort
			}
			c.line(col.x, y, d.points(s.SessionAccount, form, room))
		}
	}
}

// points are the points of the account's windows the session took, in
// form, the points bold text.primary, each window's numbered, and the rest
// text.muted: as many windows as fit whole in room cells.
func (d sessionDrawn) points(a views.SessionAccount, form views.PointsForm, room int) line {
	var l line
	keys := a.PointsKeys()
	for i, w := range a.PointsWords(form) {
		next := slices.Clip(l)
		for _, p := range w {
			k := mutedInk
			if p.Kind == status.PartFigure {
				k = titleInk.numbered(Number{Kind: NumberPoints, Account: a.Account, Of: keys[i]})
			}
			next = append(next, span{p.Text, k})
		}
		if next.width() > room {
			break
		}
		l = next
	}
	return l
}

// turns draws TURNS: its heading; its columns' heads; and its turns shown,
// as far as they're scrolled, the turn open's requests under its own, with a
// row saying how many turns are out of view; or where it has none, why.
func (d sessionDrawn) turns(c *canvas) {
	d.turnsHeading(c)
	if len(d.page.Turns) == 0 {
		if d.page.Totals.Requests > 0 {
			c.line(0, d.l.turnsHeading+2, line{{noTurns, faintInk}}.fit(d.Width))
		}
		return
	}
	for _, col := range d.turnColumns {
		col.draw(c, d.l.turnsHeading+2, col.head())
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
		took, going := t.Took(d.now)
		if going {
			took += prose.Ellipsis
		}
		return muted(took)
	case turnRequests:
		return muted(prose.Grouped(t.Requests))
	case turnTools:
		return muted(t.ToolsSaid())
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
		return muted(t.Cents())
	case turnOn:
		return d.turnOn(t.Accounts, col.width)
	case turnAsking:
		if said := d.page.Activity.WaitingSaid(); newest && said != "" {
			return line{{said, asKnown(d.page.Activity, keyInk)}}
		}
	}
	return nil
}

// turnOn names the accounts a turn went to, in text.secondary, in cells:
// the one, or each in turn, as "work → side". Where they don't fit, the
// arrow narrows, as "work→side", and each cut form keeps it narrow: a
// move's second name cut, its first whole, where that leaves moveToLeast
// cells of it, as "work→si…", else its two names cut as movedCells shares
// the room, as "w…→si…"; and two moves or more as the first and the last,
// an ellipsis between, each cut the same, as "w…→…→wo…" (C16).
func (d sessionDrawn) turnOn(accounts []string, cells int) line {
	names := make([]string, len(accounts))
	for i, id := range accounts {
		names[i] = named(d.m, id)
	}
	if said := strings.Join(names, moveArrow); len(names) < 2 || ansi.StringWidth(said) <= cells {
		return line{{said, secondaryInk}}.fit(cells)
	}
	first, last := names[0], names[len(names)-1]
	via := moveArrowTight
	if len(names) > 2 {
		via = moveArrowTight + prose.Ellipsis + moveArrowTight
	}
	room := cells - ansi.StringWidth(via)
	if from := ansi.StringWidth(first); len(names) == 2 && from+moveToLeast <= room {
		return line{{first + via + prose.Cut(last, room-from), secondaryInk}}
	}
	f, t := movedCells(ansi.StringWidth(first), ansi.StringWidth(last), room)
	return line{{prose.Cut(first, f) + via + prose.Cut(last, t), secondaryInk}}.fit(cells)
}
