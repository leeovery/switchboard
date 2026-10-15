package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

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

// The columns an account's row starts its parts at: wide, its time there,
// its requests and its points; and on a phone, its points. Its worth ends
// at the last column, its points left out a window at a time where they'd
// come within worthApart cells of it.
const (
	stayAt        = 14
	requestsAt    = 40
	pointsAt      = 56
	phonePointsAt = 11
	worthApart    = 2
)

// turnsRuleAt is the column TURNS' rule starts at.
const turnsRuleAt = 9

// turnColumn is a column of the wide page's turns: its head, the column it
// starts at, the cells it takes, and whether what's in it is right-aligned.
type turnColumn struct {
	head      string
	at, cells int
	right     bool
}

// turnColumns are the wide page's turns' columns, in order: a turn's
// number, its start, how long it took, its requests, its tools, the tokens
// it read, wrote and put out, its worth, and the accounts it went to.
var turnColumns = []turnColumn{
	{head: "#", cells: 3, right: true},
	{head: "started", at: 5, cells: 9},
	{head: "took", at: 14, cells: 7},
	{head: "requests", at: 21, cells: 8, right: true},
	{head: "tools", at: 32, cells: 30},
	{head: "read", at: 64, cells: 7, right: true},
	{head: "written", at: 73, cells: 7, right: true},
	{head: "out", at: 82, cells: 6, right: true},
	{head: "worth", at: 90, cells: 7, right: true},
	{head: "on", at: 100, cells: 12},
}

// The wide page's turn chosen is lit across its columns, chosenCells, and
// the newest turn says ↑ asking from askingAt while a request of it waits.
const (
	chosenCells = 112
	askingAt    = 114
)

// The cells of a phone's turn row: its number right-aligned in
// phoneNumberCells at the least, then a blank; its start, in
// phoneStartCells; phoneTookApart blanks at the least, then its minutes
// right-aligned in phoneTookCells at the least, the mark of one going after
// them; and its requests right-aligned in phoneRequestsCells at the least, "
// req" after them; and at the right, the accounts it went to, in
// phoneOnCells, its worth sessionPhoneApart cells before them.
const (
	phoneNumberCells   = 2
	phoneStartCells    = 5
	phoneTookApart     = 2
	phoneTookCells     = 3
	phoneRequestsCells = 4
	phoneOnCells       = 9
)

// phoneTurnCells are where a phone's turn rows set their parts: each
// number ends before numberEnd, each start is from startAt, each turn's
// minutes end before tookEnd, the mark of one going after them, and its
// requests end before requestsEnd.
type phoneTurnCells struct {
	numberEnd, startAt, tookEnd, requestsEnd int
}

// phoneTurnCellsOf are where a phone's rows of the turns given set their
// parts at now: the numbers in phoneNumberCells, or past 99 as many as the
// widest takes, the minutes in phoneTookCells, or past 99 as many as the
// longest take, and the requests in phoneRequestsCells, or past 999 as many
// as the most take, each part after moving on as far on every row, so each
// figure is whole and the columns line up.
func phoneTurnCellsOf(turns []views.Turn, now time.Time) phoneTurnCells {
	number, took, requests := phoneNumberCells, phoneTookCells, phoneRequestsCells
	for _, t := range turns {
		number = max(number, len(strconv.Itoa(t.Turn)))
		minutes, _ := t.Took(now)
		took = max(took, len(minutes))
		requests = max(requests, len(prose.Grouped(t.Requests)))
	}
	c := phoneTurnCells{numberEnd: number, startAt: number + 1}
	c.tookEnd = c.startAt + phoneStartCells + phoneTookApart + took
	c.requestsEnd = c.tookEnd + ansi.StringWidth(prose.Ellipsis) + requests
	return c
}

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
// now: the page open, laid out as sessionLayout lays it out, its accounts
// named as the Overview names them, and on a phone, its turns' rows set out
// as phoneTurnCellsOf sets them.
type sessionDrawn struct {
	Frame
	shown      *SessionShown
	page       views.SessionPage
	m          status.Moment
	names      []accountName
	l          sessionLayout
	phoneCells phoneTurnCells
	now        time.Time
}

// sessionDrawn is the session's page open, as the frame draws it of doc at
// now.
func (f Frame) sessionDrawn(doc status.Document, now time.Time) sessionDrawn {
	m := f.Moment(doc, now)
	return sessionDrawn{
		Frame: f, shown: f.Session, page: f.Session.Page.Live(f.Stream), m: m, names: namesOf(m),
		l: f.sessionLayout(len(f.Session.Page.Accounts)), phoneCells: phoneTurnCellsOf(f.Session.Page.Turns, now), now: now,
	}
}

// sessionPage draws the session's page open, in place of the view, of doc
// at now: its heading; what it's doing now and where it runs; its story; the
// accounts it ran on; and its turns, each as sessionLayout lays it out.
// While it loads, its heading alone.
func (f Frame) sessionPage(c *canvas, doc status.Document, now time.Time) {
	d := f.sessionDrawn(doc, now)
	d.heading(c)
	if d.shown.Loading {
		return
	}
	if d.l.wide {
		d.wideNow(c)
	} else {
		d.phoneNow(c)
	}
	f.sessionStory(c, d.m, d.l, d.shown.Story, d.chosen())
	d.accounts(c)
	d.turns(c)
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
			x = c.text(x+ruleGap, y, path, faintInk)
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
	c.line(0, y, said.fit(d.leftOf(c, y, d.started())))
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
// text.subtle, idle in text.faint, and ended in text.secondary.
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
	return line{{a.Said(d.now), k}}
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
		l = append(append(l, span{n.label, titleInk}), n.marks()...)
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
			l[i] = span{named(d.m, p.Text), secondaryInk}
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
	l := line{{name(a), secondaryInk}, {" now  ", dimInk}}
	for i, w := range usageWindows(d.m, nil, d.windowNamer(d.m)) {
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
		next = append(next, span{modelNamed(g.Model), secondaryInk}, span{moveArrow, mutedInk}, span{named(d.m, g.Account), secondaryInk})
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

// account draws the row along row y of an account the session ran on: its
// name; wide, its time there and its requests; the points of its windows
// the session took; and at the right, its worth.
func (d sessionDrawn) account(c *canvas, y int, s views.Stay) {
	worth := c.right(d.Width, y, line{{s.Cents(), secondaryInk}})
	if !d.l.wide {
		c.line(0, y, d.nameOf(s.Account).fitted(phonePointsAt))
		c.line(phonePointsAt, y, d.points(s.SessionAccount, views.PointsShort, worth-worthApart-phonePointsAt))
		return
	}
	c.line(0, y, d.nameOf(s.Account).fitted(stayAt))
	c.text(stayAt, y, prose.Cut(s.Said, requestsAt-stayAt-1), secondaryInk)
	c.text(requestsAt, y, prose.Cut(prose.Counted(s.Requests, "request"), pointsAt-requestsAt-1), mutedInk)
	c.line(pointsAt, y, d.points(s.SessionAccount, views.PointsWide, worth-worthApart-pointsAt))
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

// turns draws TURNS: its heading; wide, its columns' heads; and its turns
// shown, as far as they're scrolled, the turn open's requests under its own,
// with a row saying how many turns are out of view; or where it has none,
// why.
func (d sessionDrawn) turns(c *canvas) {
	d.turnsHeading(c)
	if len(d.page.Turns) == 0 {
		if d.page.Totals.Requests > 0 {
			c.line(0, d.l.turnsHeading+2, line{{noTurns, faintInk}}.fit(d.Width))
		}
		return
	}
	if d.l.wide {
		for _, col := range turnColumns {
			col.draw(c, d.l.turnsHeading+2, line{{col.head, dimInk}})
		}
	}
	rows, above, below := d.turnsShown()
	for i, r := range rows {
		r.draw(c, d.l.turnsAt+i)
	}
	c.line(0, d.l.moreTurns, outOfView(above, below, d.Width, turnsOut))
}

// turnsHeading draws TURNS' heading: TURNS, a rule, and at the right the
// session's totals: wide, its turns, requests, share from the cache and
// worth; on a phone, its worth alone.
func (d sessionDrawn) turnsHeading(c *canvas) {
	y, t := d.l.turnsHeading, d.page.Totals
	totals := line{{t.Cents(), strongInk}}
	if d.l.wide {
		totals = line{
			{prose.Counted(t.Turns, "turn"), secondaryInk},
			{" · " + prose.Counted(t.Requests, "request") + " · " + prose.Percent(t.FromCache) + " from cache · ", mutedInk},
			{t.Cents(), strongInk}, {" at API prices", mutedInk},
		}
	}
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

// turn draws the turn t's row along row y, the newest where newest says.
func (d sessionDrawn) turn(c *canvas, y int, t views.Turn, newest bool) {
	if !d.l.wide {
		d.phoneTurn(c, y, t)
		return
	}
	took, going := t.Took(d.now)
	if going {
		took += prose.Ellipsis
	}
	written := mutedInk
	if len(t.Accounts) > 1 {
		written = warningInk
	}
	cells := []line{
		{{strconv.Itoa(t.Turn), titleInk}}, {{prose.TimeOfDay(d.now, t.Started), mutedInk}}, {{took, mutedInk}},
		{{prose.Grouped(t.Requests), mutedInk}}, {{t.ToolsSaid(), mutedInk}}, {{prose.Tokens(t.Read), mutedInk}},
		{{prose.Tokens(t.Written), written}}, {{prose.Tokens(t.Out), mutedInk}}, {{t.Cents(), mutedInk}},
		d.turnOn(t.Accounts),
	}
	for i, col := range turnColumns {
		col.draw(c, y, cells[i])
	}
	if t.Turn == d.shown.Turn {
		c.surface(0, y, chosenCells, chosenTurn)
		c.emboldened(0, y, chosenCells)
	}
	if said := d.page.Activity.WaitingSaid(); newest && said != "" {
		c.text(askingAt, y, said, keyInk)
	}
}

// draw draws l in the column along row y, cut to its cells.
func (col turnColumn) draw(c *canvas, y int, l line) {
	l = l.fit(col.cells)
	if col.right {
		c.right(col.at+col.cells, y, l)
		return
	}
	c.line(col.at, y, l)
}

// turnOn names the accounts a turn went to, in text.secondary, in its
// column: the one, or each in turn, as "work → side"; where they don't fit,
// a move's two names cut as moveNames cuts them, and two moves or more as
// the first and the last, an ellipsis between, each cut as movedCells
// shares the room, as "w… → … → wo…" (C16).
func (d sessionDrawn) turnOn(accounts []string) line {
	names := make([]string, len(accounts))
	for i, id := range accounts {
		names[i] = named(d.m, id)
	}
	cells := turnColumns[len(turnColumns)-1].cells
	if said := strings.Join(names, moveArrow); len(names) < 2 || ansi.StringWidth(said) <= cells {
		return line{{said, secondaryInk}}.fit(cells)
	}
	first, last := names[0], names[len(names)-1]
	if len(names) == 2 {
		cut := moveNames(first, last, cells, secondaryInk)
		for i := range cut {
			cut[i].ink = secondaryInk
		}
		return cut
	}
	via := moveArrow + prose.Ellipsis + moveArrow
	f, t := movedCells(ansi.StringWidth(first), ansi.StringWidth(last), cells-ansi.StringWidth(via))
	return line{{prose.Cut(first, f) + via + prose.Cut(last, t), secondaryInk}}
}

// phoneTurn draws the turn t's row along row y as a phone has it: its
// number, its start, its minutes, its requests, and at the right its worth
// and the accounts it went to, lit across the row where it's chosen.
func (d sessionDrawn) phoneTurn(c *canvas, y int, t views.Turn) {
	took, going := t.Took(d.now)
	at := d.phoneCells
	c.right(at.numberEnd, y, line{{strconv.Itoa(t.Turn), titleInk}})
	c.text(at.startAt, y, prose.TimeOfDay(d.now, t.Started), secondaryInk)
	c.right(at.tookEnd, y, line{{took, mutedInk}})
	if going {
		c.text(at.tookEnd, y, prose.Ellipsis, mutedInk)
	}
	c.right(at.requestsEnd, y, line{{prose.Grouped(t.Requests), mutedInk}})
	c.text(at.requestsEnd, y, " req", mutedInk)
	on := d.Width - phoneOnCells
	c.right(on-sessionPhoneApart, y, line{{t.Cents(), mutedInk}})
	c.line(on, y, d.phoneOn(t.Accounts))
	if t.Turn == d.shown.Turn {
		c.surface(0, y, d.Width, chosenTurn)
	}
}

// phoneOn names the accounts a turn went to as a phone does, in
// text.secondary, in phoneOnCells: the one, or each in turn, a bold → in
// accent.attention between, as "work→side"; where they don't fit, → and the
// last.
func (d sessionDrawn) phoneOn(accounts []string) line {
	var l line
	for i, id := range accounts {
		if i > 0 {
			l = append(l, span{"→", alertInk})
		}
		l = append(l, span{named(d.m, id), secondaryInk})
	}
	if len(accounts) < 2 || l.width() <= phoneOnCells {
		return l.fit(phoneOnCells)
	}
	return line{{"→", alertInk}, {prose.Cut(named(d.m, accounts[len(accounts)-1]), phoneOnCells-1), secondaryInk}}
}
