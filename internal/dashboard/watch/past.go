package watch

import (
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
// once a run, as History is first shown, and of the earliest calendar year
// the Year steps back to that they don't reach, each brought up to today at
// each look since; the day the ledger began, where a read found it; the day
// weeks start on; and the reads of the ledger asked for, one under way at a
// time, and whether a look since the last asks for the days since.
type historyTab struct {
	page       dashboard.HistoryPage
	last, back heldDays
	began      time.Time
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
// before, or from where the ledger began, where that's known, as began is,
// or before it, the ledger holding nothing earlier.
func (d heldDays) reach(from, began time.Time) bool {
	return d.held() && (!from.Before(d.from) || !began.IsZero() && !began.Before(d.from))
}

// yearReadMsg is a read of the ledger's days for History, the asked'th: the
// days of the last 12 months, or, back, of a calendar year stepped back to;
// and where they begin after the day they were read from, as the ledger
// does, where it begins: zero where it isn't found.
type yearReadMsg struct {
	asked int
	days  heldDays
	back  bool
	began time.Time
}

// sinceMsg is a read of the ledger's days since those History holds were
// read to, the asked'th, read at at.
type sinceMsg struct {
	asked int
	days  []ledger.Summary
	at    time.Time
}

// readHistory has the watch read the ledger's days for History while it's
// shown, as it wants them, one read at a time, what's wanted meanwhile
// waiting for it to land: the last 12 months, once a run, as History is
// first shown; a calendar year the Year steps back to that the days held
// don't reach; and the days since those held were read to, once a look
// since the last asks.
func (m Model) readHistory() (Model, tea.Cmd) {
	if m.view != dashboard.History || m.past.reading || m.cfg.Ledger == nil {
		return m, nil
	}
	now, year := m.now(), m.past.page.YearAsk.Year
	switch _, reached := m.past.reaching(year); {
	case !m.past.last.held():
		return m.readYear(0, now)
	case m.past.page.Shown == dashboard.HistoryYear && !reached:
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
// 12 months, else a calendar year, from its grid's first day, as YearFrom
// has it, to today, at now, and prices them, as each is read once: where the
// ledger holds nothing so far back, they begin where it does.
func (m Model) readYear(year int, now time.Time) (Model, tea.Cmd) {
	from := views.YearFrom(views.YearAsk{Year: year}, now, m.cfg.WeekStarts)
	in := views.HistoryInput{
		From: from, Now: now, Prices: m.cfg.Prices, Accounts: m.cfg.Accounts,
		WeekStarts: m.cfg.WeekStarts, WeekWindow: m.cfg.WeekWindow, Family: m.cfg.Family,
	}
	m, asked := m.asking()
	l := m.cfg.Ledger
	return m, func() tea.Msg {
		days := l.Days(from)
		return yearReadMsg{asked: asked, days: heldDays{past: views.NewPast(in, days), from: from, to: now}, back: year != 0, began: beganOf(days, from)}
	}
}

// beganOf is the start of the first of the days read from from, where it's
// a day after from's, as the ledger's reads begin where it does: zero where
// they begin at from, the ledger holding it and maybe days before it too.
func beganOf(days []ledger.Summary, from time.Time) time.Time {
	if len(days) == 0 || days[0].Day <= dayfile.DateOf(from) {
		return time.Time{}
	}
	start, _, _ := dayfile.Day(days[0].Day)
	return start
}

// readSince reads the days since those held were read to, from the start of
// that day, at now: today's alone, but at the first look on a new date,
// which reads the days before it it hadn't finished, and today's.
func (m Model) readSince(now time.Time) (Model, tea.Cmd) {
	from := dayfile.DayStart(m.past.readTo(now).Local(), 0)
	m, asked := m.asking()
	l := m.cfg.Ledger
	return m, func() tea.Msg {
		return sinceMsg{asked: asked, days: l.Days(from), at: now}
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
// use; or a calendar year's, held for the Year to show. The page shown is
// built afresh.
func (m Model) yearRead(msg yearReadMsg) Model {
	if msg.asked != m.past.asked {
		return m
	}
	t := m.past
	t.reading = false
	if !msg.began.IsZero() {
		t.began = msg.began
	}
	if msg.back {
		t.back = msg.days
	} else {
		t.last = msg.days
		t.page = t.checked()
	}
	m.past = t.built()
	return m
}

// checked is the page, the Year showing all use where it shows a version the
// last 12 months didn't use, as one kept from a run since may be.
func (t historyTab) checked() dashboard.HistoryPage {
	p := t.page
	if v := p.YearAsk.Version; v != "" && !slices.Contains(t.last.past.Year(views.YearAsk{}).Versions, v) {
		p.YearAsk.Version = ""
	}
	return p
}

// sinceRead takes in the days read since those held were read to, but those
// of a read another overtook: the page drawn before them and after, at the
// one moment, now, each number on it they change brightens, as changed has
// it, as a look's changes do.
func (m Model) sinceRead(msg sinceMsg) Model {
	if msg.asked != m.past.asked {
		return m
	}
	m.past.reading = false
	now := m.now()
	was := m
	was.past = m.past.since(nil, now, now)
	m.past = m.past.since(msg.days, msg.at, now)
	m.numbers = m.numbers.changed(was.numbersDrawn(now), m.numbersDrawn(now), now)
	return m
}

// since is the tab at now, the days given, read at at, in place of those
// each year held held of them, as Past.Since has it, its page built afresh.
func (t historyTab) since(days []ledger.Summary, at, now time.Time) historyTab {
	for _, d := range []*heldDays{&t.last, &t.back} {
		if !d.held() {
			continue
		}
		d.past = d.past.Since(days, now)
		if len(days) > 0 {
			d.to = at
		}
	}
	return t.built()
}

// built is the tab, the page of the sub-tab shown built afresh of the days
// held, those of the rest let go: none before the last 12 months are read,
// and of the Year, none while the days of the year it shows are.
func (t historyTab) built() historyTab {
	p := t.page
	p.Year, p.Weeks, p.Days, p.Tokens = nil, nil, nil, nil
	p.Read = t.last.held()
	switch past := t.last.past; {
	case !p.Read:
	case p.Shown == dashboard.HistoryWeeks:
		p.Weeks = new(past.Weeks())
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

// earliest is the earliest calendar year the Year steps back to, at now: the
// ledger's first, where a read found where it began; else that of the
// earliest day read, whose year it holds, and maybe those before it, which
// stepping back to it reads; else, before any read, this one.
func (t historyTab) earliest(now time.Time) int {
	if !t.began.IsZero() {
		return t.began.Year()
	}
	first := now
	for _, d := range []heldDays{t.last, t.back} {
		if d.held() && d.from.Before(first) {
			first = d.from
		}
	}
	return first.Year()
}

// historyKey acts on a key of History's, reporting false for a key it
// doesn't take, or on another tab: [ and ] show the sub-tab before and
// after, round; < and > step the period shown, where there's one, back and
// on; v cycles what the sub-tab shown shows, where it cycles anything; and ←
// and →, on the Year shown for one account, step to the account before and
// after, round. A key that changes what's shown shows it as showHistory has
// it.
func (m Model) historyKey(key string) (Model, bool) {
	if m.view != dashboard.History {
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
	if next, ok := m.past.page.Showing(i); ok && m.view == dashboard.History {
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
