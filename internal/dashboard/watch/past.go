package watch

import (
	"errors"
	"io/fs"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// historyTab is what the watch holds of History: the page its tab draws, the
// sub-tab shown built of the days held; the days of the last 12 months, read
// once a run, as History or Accounts is first shown, and of the earliest
// calendar year the Year steps back to that they don't reach, each brought
// up to today at each look since; the day the ledger began, where it tells
// it, and whether a failure to has been logged, the once; the day weeks
// start on; and the reads of the ledger asked for, one under way at a time,
// and whether a look since the last asks for the days since. held counts the
// changes to the last 12 months' days held, as each read and look brings
// them, for Accounts' page to be built afresh of them; and behind is set
// while the page lags the days held, as a look while History doesn't show
// leaves it, for historyBuilt to build it afresh once History shows. weeks
// is Weeks' page as built, where the page shows it, which the page holds
// stood at a moment, and what it was stood at, as weeksStood stands it.
type historyTab struct {
	page       dashboard.HistoryPage
	weeks      *views.WeeksPage
	at         *stoodAt
	last, back heldDays
	held       int
	behind     bool
	began      time.Time
	warned     bool
	starts     time.Weekday
	asked      int
	reading    bool
	due        bool
}

// heldDays are days of the ledger read for History, priced, read from the
// start of the day from, and to when to says: none while from is zero.
type heldDays struct {
	past     views.Past
	from, to time.Time
}

// held reports whether the days are read.
func (d heldDays) held() bool {
	return !d.from.IsZero()
}

// reach reports whether the days reach back to from: read from it or
// before, or from where the ledger began, where it's known, as began is, or
// before it, the ledger holding nothing earlier.
func (d heldDays) reach(from, began time.Time) bool {
	return d.held() && (!from.Before(d.from) || !began.IsZero() && !began.Before(d.from))
}

// yearReadMsg is a read of the ledger's days for History, the asked'th: the
// days of the last 12 months, or, back, of a calendar year stepped back to;
// and where the ledger began, as the read asked it.
type yearReadMsg struct {
	asked int
	days  heldDays
	back  bool
	beginning
}

// sinceMsg is a read of the ledger's days since those History holds were
// read to, the asked'th, read at at; and where the ledger began, where the
// read asked it.
type sinceMsg struct {
	asked int
	days  []ledger.Summary
	at    time.Time
	beginning
}

// beginning is where the ledger began, as a read asked it: the start of its
// first day, as LedgerStart tells it, zero where it doesn't, or wasn't
// asked; and why it failed to say, where it did.
type beginning struct {
	began time.Time
	err   error
}

// readHistory has the watch read the ledger's days for History and
// Accounts while either is shown, as they want them, one read at a time,
// what's wanted meanwhile waiting for it to land, the two sharing every read
// and the days it holds: the last 12 months, once a run, as either is first
// shown; while History shows, a calendar year the Year steps back to that
// the days held don't reach; and the days since those held were read to,
// once a look since the last asks.
func (m Model) readHistory() (Model, tea.Cmd) {
	history := m.view == dashboard.History
	shown := history || m.view == dashboard.Accounts
	if !shown || m.past.reading || m.cfg.Ledger == nil {
		return m, nil
	}
	now, year := m.now(), m.past.page.YearAsk.Year
	switch _, reached := m.past.reaching(year); {
	case !m.past.last.held():
		return m.readYear(0, now)
	case history && m.past.page.Shown == dashboard.HistoryYear && !reached:
		return m.readYear(year, now)
	case m.past.due:
		return m.readSince(now)
	}
	return m, nil
}

// asking notes a read of the ledger asked for, under way till it lands, and
// returns which it is: a look since the last is answered by it.
func (m Model) asking() (Model, int) {
	m.past.asked++
	m.past.reading, m.past.due = true, false
	return m, m.past.asked
}

// readYear reads the days the Year of the year given shows, 0 for the last
// 12 months, else a calendar year, from the first day it reads, as YearFrom
// has it, to today, at now, and prices them, as each is read once, knowing
// where the ledger began, as it was known: where the ledger holds nothing so
// far back, they begin where it does, which it asks alongside, as
// beganAsked has it, for yearRead to have them priced knowing it.
func (m Model) readYear(year int, now time.Time) (Model, tea.Cmd) {
	from := views.YearFrom(views.YearAsk{Year: year}, now, m.cfg.WeekStarts)
	in := views.HistoryInput{
		From: from, Now: now, Began: m.past.began, Prices: m.cfg.Prices, Accounts: m.cfg.Accounts,
		WeekStarts: m.cfg.WeekStarts, WeekWindow: m.cfg.WeekWindow, Family: m.cfg.Family,
	}
	l := m.cfg.Ledger
	start := m.past.beganAsked(l)
	m, asked := m.asking()
	return m, func() tea.Msg {
		msg := yearReadMsg{asked: asked, back: year != 0}
		if start != nil {
			msg.beginning = ledgerBegan(start, now)
		}
		msg.days = heldDays{past: views.NewPast(in, l.Days(from)), from: from, to: now}
		return msg
	}
}

// beganAsked is what a read of the ledger asks where it began, as
// ledgerBegan asks it: while that's unknown, as on a fresh install, or after
// a failure to say, the ledger, where it's a LedgerStart; else nil, not
// asking.
func (t historyTab) beganAsked(l views.Ledger) views.LedgerStart {
	start, ok := l.(views.LedgerStart)
	if !ok || !t.began.IsZero() {
		return nil
	}
	return start
}

// ledgerBegan is where the ledger began, at now, as it tells it: the start
// of its first day; zero where it holds nothing, or fails to say, and why.
func ledgerBegan(start views.LedgerStart, now time.Time) beginning {
	first, err := start.FirstDay(now)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return beginning{}
	case err != nil:
		return beginning{err: err}
	}
	return beginning{began: first}
}

// readSince reads the days since those held were read to, from the start of
// that day, at now: today's alone, but at the first look on a new date,
// which reads the days before it it hadn't finished, and today's; and where
// the ledger began, as beganAsked has it.
func (m Model) readSince(now time.Time) (Model, tea.Cmd) {
	from := dayfile.DayStart(m.past.readTo(now).Local(), 0)
	l := m.cfg.Ledger
	start := m.past.beganAsked(l)
	m, asked := m.asking()
	return m, func() tea.Msg {
		msg := sinceMsg{asked: asked, at: now}
		if start != nil {
			msg.beginning = ledgerBegan(start, now)
		}
		msg.days = l.Days(from)
		return msg
	}
}

// readTo is when the days held were last read to, the earliest of them,
// and no later than now.
func (t historyTab) readTo(now time.Time) time.Time {
	to := now
	for _, d := range []heldDays{t.last, t.back} {
		if d.held() && d.to.Before(to) {
			to = d.to
		}
	}
	return to
}

// yearRead takes in the days of a year read for History, but those of a read
// another overtook: the last 12 months', which History draws from then on,
// the Year showing all use where it was kept showing a version they didn't
// use; or a calendar year's, held for the Year to show; and where the ledger
// began, where the read learned it. The page shown is built afresh.
func (m Model) yearRead(msg yearReadMsg) Model {
	if msg.asked != m.past.asked {
		return m
	}
	t := m.past
	t.reading = false
	if msg.back {
		t.back = msg.days
	} else {
		t.last, t.held = msg.days, t.held+1
		t.page = t.checked()
	}
	m.past = t.learned(msg.beginning).built()
	return m
}

// learned is the tab once a read has asked where the ledger began: where it
// told, held, and taken by every Past held, the 12 months' and a calendar
// year's alike; where it failed to say, logged, the first time alone, as
// each read after asks again.
func (t historyTab) learned(b beginning) historyTab {
	if b.err != nil && !t.warned {
		logger.Warn("couldn't read where the request ledger begins", "error", b.err)
		t.warned = true
	}
	if b.began.IsZero() {
		return t
	}
	t.began = b.began
	for _, d := range []*heldDays{&t.last, &t.back} {
		if d.held() {
			d.past = d.past.Begun(b.began)
		}
	}
	return t
}

// checked is the page, the Year showing all use where it shows a version the
// last 12 months didn't use, as one kept from a run since may be.
func (t historyTab) checked() dashboard.HistoryPage {
	p := t.page
	if v := p.YearAsk.Version; v != "" && !slices.Contains(t.last.past.Versions(), v) {
		p.YearAsk.Version = ""
	}
	return p
}

// sinceRead takes in the days read since those held were read to, and where
// the ledger began, where the read learned it, but those of a read another
// overtook. While History or Accounts shows, its page is drawn before the
// days and after, at the one moment, now, History's both knowing where the
// ledger began, Accounts' built as accountsBuilt builds it, each number on
// it the days change brightening, as changed has it, as a look's changes
// do; else History's page is left behind the days, as since leaves it, for
// historyBuilt to build once History shows.
func (m Model) sinceRead(msg sinceMsg) Model {
	if msg.asked != m.past.asked {
		return m
	}
	m.past = m.past.learned(msg.beginning)
	m.past.reading = false
	now := m.now()
	var was Model
	switch m.view {
	case dashboard.History:
		was = m
		was.past = m.past.since(nil, now, now).built()
		m.past = m.past.since(msg.days, msg.at, now).built()
	case dashboard.Accounts:
		was = m.accountsBuilt()
		m.past = m.past.since(msg.days, msg.at, now)
		m = m.accountsBuilt()
	default:
		m.past = m.past.since(msg.days, msg.at, now)
		return m
	}
	m.numbers = m.numbers.changed(was.numbersDrawn(now), m.numbersDrawn(now), now)
	return m
}

// since is the tab at now, the days given, read at at, in place of those
// each year held held of them, as Past.Since has it, its page behind them.
func (t historyTab) since(days []ledger.Summary, at, now time.Time) historyTab {
	t.held, t.behind = t.held+1, true
	for _, d := range []*heldDays{&t.last, &t.back} {
		if !d.held() {
			continue
		}
		d.past = d.past.Since(days, now)
		if len(days) > 0 {
			d.to = at
		}
	}
	return t
}

// historyBuilt is m with History's page built afresh of the days held, while
// History shows, where a look left it behind them: as History shows again
// after a look on Accounts; its Weeks stood as weeksStood stands it.
func (m Model) historyBuilt() Model {
	if m.view == dashboard.History && m.past.behind {
		m.past = m.past.built()
	}
	return m.weeksStood()
}

// weeksStood is m with History's Weeks, while it shows, stood at the moment
// of the message taken in, as views.WeeksPage.At stands it, where it's built
// afresh, the document or the policy is another, or the page stands
// otherwise since, as its Until has it: as Accounts' page is, so the two
// tell the accounts' weeks alike, never a frame at a time.
func (m Model) weeksStood() Model {
	if m.view != dashboard.History || m.past.weeks == nil {
		return m
	}
	now := m.now()
	if !m.past.at.holds(m.doc, now, m.cfg.Policy) {
		mo := m.moment(now)
		m.past.page.Weeks = new(m.past.weeks.At(mo))
		m.past.at = stoodTill(mo, m.past.weeks.Until(mo))
	}
	return m
}

// built is the tab, the page of the sub-tab shown built afresh of the days
// held, those of the rest let go: none before the last 12 months are read,
// and of the Year, none while the days of the year it shows are. Weeks'
// page is held as built, for weeksStood to stand.
func (t historyTab) built() historyTab {
	t.behind, t.weeks, t.at = false, nil, nil
	p := t.page
	p.Year, p.Weeks, p.Days, p.Tokens = nil, nil, nil, nil
	p.Read = t.last.held()
	switch past := t.last.past; {
	case !p.Read:
	case p.Shown == dashboard.HistoryWeeks:
		t.weeks = new(past.Weeks())
		p.Weeks = t.weeks
	case p.Shown == dashboard.HistoryDays:
		p.Days = new(past.Days())
	case p.Shown == dashboard.HistoryTokens:
		p.Tokens = new(past.Tokens(p.TokensAsk))
	default:
		if of, ok := t.reaching(p.YearAsk.Year); ok {
			p.Year = new(of.Year(p.YearAsk))
		}
	}
	t.page = p
	return t
}

// reaching is the days held the Year of the year given, 0 for the last 12
// months, is built of, reporting false where none reach back to its grid's
// first day: the last 12 months' where they do, else the calendar year's
// stepped back to.
func (t historyTab) reaching(year int) (views.Past, bool) {
	if year == 0 {
		return t.last.past, t.last.held()
	}
	from := views.YearFrom(views.YearAsk{Year: year}, t.last.to, t.starts)
	for _, d := range []heldDays{t.last, t.back} {
		if d.reach(from, t.began) {
			return d.past, true
		}
	}
	return views.Past{}, false
}

// earliest is the earliest calendar year the Year steps back to, at now:
// the ledger's first, where it tells where it began; else this one.
func (t historyTab) earliest(now time.Time) int {
	if t.began.IsZero() {
		return now.Year()
	}
	return t.began.Year()
}

// historyKey acts on a key of History's, reporting false for a key it
// doesn't take, or on another tab: [ and ] show the sub-tab before and
// after, round; < and > step the period shown, where there's one, back and
// on; v cycles what the sub-tab shown shows, where it cycles anything; and ←
// and →, on the Year shown for one account, step to the account before and
// after, round. A key that changes what's shown shows it as showHistory has
// it.
func (m Model) historyKey(key string) (Model, bool) {
	if m.shownView() != dashboard.History {
		return m, false
	}
	p, now := m.past.page, m.now()
	var next dashboard.HistoryPage
	var changed bool
	switch key {
	case "[", "]":
		next, changed = p.Turned(historySteps[key])
	case "<", ">":
		next, changed = p.Stepped(historySteps[key], m.past.earliest(now), now.Year())
	case "left", "right":
		next, changed = p.Moved(historySteps[key])
	case "v":
		next, changed = p.Cycled()
	default:
		return m, false
	}
	if changed {
		m = m.showHistory(next)
	}
	return m, true
}

// historySteps are the ways History's keys move: back, -1, or on, 1.
var historySteps = map[string]int{"[": -1, "]": 1, "<": -1, ">": 1, "left": -1, "right": 1}

// showSubTab shows History's sub-tab i, as a click on it does: on History,
// where it isn't the one shown already.
func (m Model) showSubTab(i int) Model {
	if next, ok := m.past.page.Showing(i); ok && m.shownView() == dashboard.History {
		return m.showHistory(next)
	}
	return m
}

// showHistory shows History as given, from its top, its page built afresh
// of the days held, and keeps what its views show, where that's changed, for
// the next watch to open with: never a calendar year stepped back to.
func (m Model) showHistory(p dashboard.HistoryPage) Model {
	was := m.past.page.Kept()
	m.past.page, m.scroll = p, 0
	m.past = m.past.built()
	if kept := p.Kept(); kept != was {
		m.keepPrefs("what History's views show", func(prefs *theme.Prefs) { prefs.History = kept }, "history", kept)
	}
	return m
}
