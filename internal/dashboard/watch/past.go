package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// pastDays are the ledger's days the watch holds, which Accounts, Runway's
// and Trends' lanes and Trends' pages are built of: those of the last 12
// months, read once a run, as Accounts is first shown, Runway's lanes first
// want them or Trends first shows a section drawn of them, and of the
// earliest calendar year the lanes reach back to that they don't, each
// brought up to today at each look since; where the ledger began, as begun
// has it; the day weeks start on; and the reads of the ledger asked for,
// one under way at a time, and whether a look since the last asks for the
// days since. held counts the changes to the last 12 months' days held, as
// each read and look brings them, for the pages built of them to be built
// afresh.
type pastDays struct {
	begun
	last, back heldDays
	held       int
	starts     time.Weekday
	asked      int
	reading    bool
	due        bool
}

// heldDays are days of the ledger read, priced, read from the start of the
// day from, and to when to says: none while from is zero.
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

// yearReadMsg is a read of the ledger's days, the asked'th: the days of the
// last 12 months, or, back, of a calendar year the lanes reach back to; and
// where the ledger began, as the read asked it.
type yearReadMsg struct {
	asked int
	days  heldDays
	back  bool
	beginning
}

// sinceMsg is a read of the ledger's days since those held were read to,
// the asked'th, read at at; and where the ledger began, where the read asked
// it.
type sinceMsg struct {
	asked int
	days  []ledger.Summary
	at    time.Time
	beginning
}

// beginning is where the ledger began, as a read asked it: the start of its
// first day, as ledgerBegan tells it, zero where it doesn't, or wasn't
// asked; whether it holds no day yet, none, so begins with today; and why
// it failed to say, where it did.
type beginning struct {
	began time.Time
	none  bool
	err   error
}

// begun is where the pool shown's work of the ledger begins, as a reader of
// it learns it from the reads that ask, as beganAsked has them: the start of
// its first day, zero till one tells it, and whether that's today's as it
// holds no day yet, beganNone; and whether a failure to tell has been
// logged, the once.
type begun struct {
	began     time.Time
	beganNone bool
	warned    bool
}

// readPast has the watch read the ledger's days for Accounts while it's
// shown, for Runway or Trends while its lanes want them, as runwayYearWanted
// says, and for Trends while it shows a section drawn of them, as
// trendsDaysWanted says, as they want them, one read at a time, what's
// wanted meanwhile waiting for it to land, all sharing every read and the
// days it holds: the last 12 months, once a run, as any is first shown;
// while Runway or Trends does, the calendar year its lanes want; and the
// days since those held were read to, once a look since the last asks.
func (m Model) readPast() (Model, tea.Cmd) {
	runwayYear, runway := m.runwayYearWanted()
	shown := m.view == dashboard.Accounts || runway || m.trendsDaysWanted()
	if !shown || m.past.reading || m.cfg.Ledger == nil {
		return m, nil
	}
	now := m.now()
	switch _, reached := m.past.reaching(runwayYear); {
	case !m.past.last.held():
		return m.readYear(0, now)
	case runway && !reached:
		return m.readYear(runwayYear, now)
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

// readYear reads the days of the year given, 0 for the last 12 months, else
// a calendar year, from the first day its grid shows, as DaysFrom has it, to
// today, at now, in the pool shown, as ledgerDays reads them, and prices
// them, as each is read once, knowing where the ledger began, as it was
// known: where the ledger holds nothing so far back, they begin where it
// does, which it asks alongside, as beganAsked has it, for yearRead to have
// them priced knowing it.
func (m Model) readYear(year int, now time.Time) (Model, tea.Cmd) {
	from := views.DaysFrom(year, now, m.cfg.WeekStarts)
	in := views.HistoryInput{
		From: from, Now: now, Began: m.past.began, Prices: m.cfg.Prices, Accounts: m.cfg.Accounts, Removed: m.cfg.Removed, Moved: m.cfg.Moved,
		WeekStarts: m.cfg.WeekStarts, WeekWindow: m.cfg.WeekWindow, Family: m.cfg.Family,
	}
	l, start := m.ledgerDays(), m.past.beganAsked(m.ledgerStart())
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

// ledgerDays reads the request ledger's days for Accounts, Runway and
// Trends, each day the pool shown's alone, as its PoolRead's Days reads
// them: its work in the pool, and of its accounts, those that worked there
// or are the pool's.
func (m Model) ledgerDays() views.LedgerDays {
	return m.cfg.PoolRead.Days(m.cfg.Ledger)
}

// ledgerStart tells where the pool shown's work of the request ledger
// begins, as its PoolRead's Start tells it, for its days to begin there:
// nil where the ledger can't tell.
func (m Model) ledgerStart() views.LedgerStart {
	return m.cfg.PoolRead.Start(m.cfg.Ledger)
}

// beganAsked is what a read of the ledger asks where it began, as
// ledgerBegan asks it: while that's unknown, after a failure to say, or
// while the ledger holds no day, as on a fresh install, so begins with
// today, whichever that is, start, where it's given; else nil, not asking.
func (b begun) beganAsked(start views.LedgerStart) views.LedgerStart {
	if start == nil || !b.began.IsZero() && !b.beganNone {
		return nil
	}
	return start
}

// told is b once a read has asked where the ledger began: where it told,
// held; where it failed to say, logged, the first time alone, as each read
// after asks again.
func (b begun) told(n beginning) begun {
	if n.err != nil && !b.warned {
		logger.Warn("couldn't read where the request ledger begins", "error", n.err)
		b.warned = true
	}
	if !n.began.IsZero() {
		b.began, b.beganNone = n.began, n.none
	}
	return b
}

// ledgerBegan is where the ledger began, at now, as it tells it and
// ledger.Begins takes it, as a read of its days does: the start of its first
// day, or of now's where it holds none yet, saying so; zero where it fails
// to say, and why.
func ledgerBegan(start views.LedgerStart, now time.Time) beginning {
	first, err := start.FirstDay(now)
	began, none, err := ledger.Begins(first, err, now)
	return beginning{began: began, none: none, err: err}
}

// readSince reads the days since those held were read to, from the start of
// that day, at now, in the pool shown, as ledgerDays reads them: today's
// alone, but at the first look on a new date, which reads the days before it
// it hadn't finished, and today's; and where the ledger began, as beganAsked
// has it.
func (m Model) readSince(now time.Time) (Model, tea.Cmd) {
	from := dayfile.DayStart(m.past.readTo(now).Local(), 0)
	l, start := m.ledgerDays(), m.past.beganAsked(m.ledgerStart())
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
func (t pastDays) readTo(now time.Time) time.Time {
	to := now
	for _, d := range []heldDays{t.last, t.back} {
		if d.held() && d.to.Before(to) {
			to = d.to
		}
	}
	return to
}

// yearRead takes in the days of a year read, but those of a read another
// overtook: the last 12 months', which the pages are built of from then on;
// or a calendar year's, held for the lanes that reach back to it; and where
// the ledger began, where the read learned it.
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
	}
	m.past = t.learned(msg.beginning)
	return m
}

// learned is the days held once a read has asked where the ledger began, as
// begun's told has it, where it told taken by every Past held, the 12
// months' and a calendar year's alike.
func (t pastDays) learned(b beginning) pastDays {
	t.begun = t.told(b)
	if b.began.IsZero() {
		return t
	}
	for _, d := range []*heldDays{&t.last, &t.back} {
		if d.held() {
			d.past = d.past.Begun(b.began)
		}
	}
	return t
}

// usedAsk is what the Year's ask shows of the days given: all use where it
// asks for a version they didn't use; else what it asks.
func usedAsk(ask views.YearAsk, past views.Past) views.YearAsk {
	if v := ask.Version; v != "" && !slices.Contains(past.Versions(), v) {
		ask.Version = ""
	}
	return ask
}

// sinceRead takes in the days read since those held were read to, and where
// the ledger began, where the read learned it, but those of a read another
// overtook. While Accounts, Runway or Trends shows, its page is drawn before
// the days and after, at the one moment, now, Accounts' built as
// accountsBuilt builds it, Runway's and Trends' lanes as lanesBuilt builds
// them, and Trends' pages as trendsBuilt builds them, before the days of
// the held days where the ledger began, so learning that changes nothing
// drawn; each number on it the days change brightening, as changed has it,
// as a look's changes do.
func (m Model) sinceRead(msg sinceMsg) Model {
	if msg.asked != m.past.asked {
		return m
	}
	m.past = m.past.learned(msg.beginning)
	m.past.reading = false
	now := m.now()
	var was Model
	switch m.view {
	case dashboard.Accounts:
		was = m.accountsBuilt()
		m.past = m.past.since(msg.days, msg.at, now)
		m = m.accountsBuilt()
	case dashboard.Runway, dashboard.Graphs:
		was = m
		was.past = m.past.since(nil, now, now)
		was = was.trendsBuilt()
		m.past = m.past.since(msg.days, msg.at, now)
		m = m.lanesBuilt().trendsBuilt()
	default:
		m.past = m.past.since(msg.days, msg.at, now)
		return m
	}
	m.numbers = m.numbers.changed(was.numbersDrawn(now), m.numbersDrawn(now), now)
	return m
}

// since is the days held at now, the days given, read at at, as the read was
// asked, before the ledger summarised them, so they hold nothing they're
// taken to hold of what came after, in place of those each year held held
// of them, as Past.Since has it.
func (t pastDays) since(days []ledger.Summary, at, now time.Time) pastDays {
	t.held++
	for _, d := range []*heldDays{&t.last, &t.back} {
		if !d.held() {
			continue
		}
		d.past = d.past.Since(days, at).Since(nil, now)
		if len(days) > 0 {
			d.to = at
		}
	}
	return t
}

// reaching is the days held of the year given, 0 for the last 12 months,
// reporting false where none reach back to its grid's first day: the last
// 12 months' where they do, else the calendar year's read back to.
func (t pastDays) reaching(year int) (views.Past, bool) {
	if year == 0 {
		return t.last.past, t.last.held()
	}
	from := views.DaysFrom(year, t.last.to, t.starts)
	for _, d := range []heldDays{t.last, t.back} {
		if d.reach(from, t.began) {
			return d.past, true
		}
	}
	return views.Past{}, false
}
