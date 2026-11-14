package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/views"
)

// periodDays is what the watch holds of the sessions' days Projects and
// Sessions' List are built of, one read of the period serving both: the
// days read, as views.ReadSessionDays reads them, from the start of the day
// from, none till the first read lands, and how many reads have landed; the
// sessions' days the ledger's tail fed, today's at every read of it from
// the first, whether or not either has shown, and each day gone since one
// first asked for its own, as daysFeed has it, and how many reads told
// otherwise of them; and whether a read is under way, and how many were
// asked, a read another overtook passed over.
type periodDays struct {
	days    *views.SessionDays
	from    time.Time
	read    int
	tailed  views.SessionDays
	fed     int
	reading bool
	asked   int
}

// daysReadMsg is a read of the sessions' days, the asked'th: the days from
// the start of the day from to the day before now's, as readDays read them.
type daysReadMsg struct {
	asked int
	days  views.SessionDays
	from  time.Time
}

// readsDays reports whether what the watch shows is built of the sessions'
// days: Projects, and Sessions' List.
func (m Model) readsDays() bool {
	switch m.view {
	case dashboard.Projects:
		return true
	case dashboard.Sessions:
		return m.sessionsTab.shown.Shown == dashboard.SessionsList
	}
	return false
}

// readDays has the watch read the sessions' days while what it shows is
// built of them, as readsDays has it, one read at a time: once, as one first
// shows, from the start of the period's first day; and again where the dial
// steps to a period reaching further back than the days held. Each reads
// the days before today's alone, today's the tail's, as joinedDays joins
// them.
func (m Model) readDays() (Model, tea.Cmd) {
	d := m.sessionDays
	if !m.readsDays() || d.reading || m.cfg.Ledger == nil {
		return m, nil
	}
	now := m.now()
	from := m.period.From(now, m.cfg.WeekStarts)
	if d.days != nil && !from.Before(d.from) {
		return m, nil
	}
	l := m.cfg.Ledger
	m.sessionDays.asked++
	m.sessionDays.reading = true
	asked := m.sessionDays.asked
	return m, func() tea.Msg {
		return daysReadMsg{asked: asked, days: views.ReadSessionDays(l, from, now), from: from}
	}
}

// daysRead takes in a read of the sessions' days, but one another overtook:
// held in place of those held before, the days the tail fed letting go of
// those it stands for, as withoutRead has it, for what's built of them to
// be built afresh.
func (m Model) daysRead(msg daysReadMsg) Model {
	if msg.asked != m.sessionDays.asked {
		return m
	}
	m.sessionDays.reading = false
	m.sessionDays.days, m.sessionDays.from = &msg.days, msg.from
	m.sessionDays.tailed = m.sessionDays.withoutRead(m.sessionDays.tailed)
	m.sessionDays.read++
	return m
}

// withoutRead is days, as the ledger's tail fed them, without those the
// days read stand for, as views.SessionDays' Without has it: every one kept
// till a read has landed.
func (d periodDays) withoutRead(days views.SessionDays) views.SessionDays {
	if d.days == nil {
		return days
	}
	return days.Without(*d.days)
}

// daysFeed is what a read of the ledger's tail feeds the sessions' days:
// the days as the read was asked; whether a read of them had been asked by
// then, from which on the days keep each day gone, else today's alone, as
// the first read reads every day gone; and the dates of the days before
// today's the read reads again whole for them, each once, the oldest
// first, none till a read is asked.
type daysFeed struct {
	days  views.SessionDays
	keeps bool
	owed  []string
}

// daysFeed is what a read of the ledger's tail asked at now feeds the
// sessions' days, as daysFeed has it: the days read again for them those
// they owe, as Owed has them, and those the tail followed no further, as
// missed has them.
func (m Model) daysFeed(now time.Time) daysFeed {
	f := daysFeed{days: m.sessionDays.tailed, keeps: m.sessionDays.asked > 0}
	if f.keeps {
		owed := slices.Concat(f.days.Owed(now), m.tail.missed(views.Today(now)))
		f.owed = slices.Compact(slices.Sorted(slices.Values(owed)))
	}
	return f
}

// fed is the sessions' days fed, at now, what a read of the tail read: the
// days owed among those it read again whole, and every day it read whole
// taken in, so a line another reader awaited reaches them too; or, till a
// read of them is asked, today's alone, none read again for them.
func (f daysFeed) fed(read tailLines, now time.Time) views.SessionDays {
	if !f.keeps {
		return f.days.Tailed(read.gained, read.afresh, nil, nil, now).TodayAlone()
	}
	return f.days.Tailed(read.gained, read.afresh, f.owed, read.days, now)
}

// tailFed is d with the sessions' days as a read of the ledger's tail fed
// them, without those the days read stand for, as withoutRead has it,
// counted as fed where they tell otherwise than those held, as Told has it,
// for what's built of them to be built afresh as it shows.
func (d periodDays) tailFed(days views.SessionDays) periodDays {
	days = d.withoutRead(days)
	if days.Told(d.tailed) {
		d.fed++
	}
	d.tailed = days
	return d
}

// daysReady reports whether what's built of the sessions' days can be built
// at now: of the sessions running alone, where there's no ledger; else
// where the days read reach back to the period's first day, and the tail
// was last read today, so its fold of a day gone never stands for that day.
// Not before the first read lands, nor before the tail's first read of the
// day, as just past midnight or after a night asleep, nor as the dial
// reaches further back, till the read of the days it reaches lands.
func (m Model) daysReady(now time.Time) bool {
	return m.cfg.Ledger == nil || m.daysReached(now) && m.tail.readOn(views.Today(now))
}

// daysReached reports whether the days read reach back to the first day of
// the period at now.
func (m Model) daysReached(now time.Time) bool {
	d := m.sessionDays
	return d.days != nil && !m.period.From(now, m.cfg.WeekStarts).Before(d.from)
}

// daysWaited is m waiting at now on the sessions' days, as daysReady has
// it, reporting whether what was built of them, built for the period given
// where built says, is kept: never where the days read don't reach back to
// the period's first day, for it to await their read; else where it was
// built for the period shown, never built again of the fold of a day gone.
// Where the tail alone is yet to be read on the day, as just past midnight
// or after a night asleep, a read of it is due at once, not at the next
// look, where none is under way.
func (m Model) daysWaited(now time.Time, built bool, period views.Period) (Model, bool) {
	if !m.daysReached(now) {
		return m, false
	}
	m.tail.due = m.tail.due || !m.tail.busy()
	return m, built && period == m.period
}

// joinedDays are the sessions' days, once they're ready, as daysReady has
// it: those read, joined to those the ledger's tail fed, as Joined joins
// them, so today's are the tail's, however late a read lands; none where
// there's no ledger.
func (m Model) joinedDays() views.SessionDays {
	if m.cfg.Ledger == nil {
		return views.SessionDays{}
	}
	return m.sessionDays.tailed.Joined(*m.sessionDays.days)
}

// periodBuiltOf is what a page of the sessions' days was built of: the
// reads of the days landed, and the reads of the ledger's tail that told
// otherwise of the days it fed; the period; the local day it was built on;
// and the sessions of the List it draws, by what's drawn of each, as
// runningKey keys them.
type periodBuiltOf struct {
	read, fed int
	period    views.Period
	day       string
	listed    []runningKey
}

// builtOf is what a page of the sessions' days built at now is built of, as
// periodBuiltOf has it, the List's sessions it draws as given.
func (m Model) builtOf(now time.Time, listed []runningKey) periodBuiltOf {
	return periodBuiltOf{read: m.sessionDays.read, fed: m.sessionDays.fed, period: m.period, day: dayfile.DateOf(now), listed: listed}
}

// same reports whether the page was built of what o says.
func (b periodBuiltOf) same(o periodBuiltOf) bool {
	return b.read == o.read && b.fed == o.fed && b.period == o.period && b.day == o.day && slices.Equal(b.listed, o.listed)
}

// runningKey is a session the List lists, by what of it a page of the
// sessions' days draws and is built of, but what changes as its answer
// streams: its id, where it runs, its account and model, its mark, and what
// it's doing.
type runningKey struct {
	id, dir, project, account, model string
	mark                             views.RowMark
	doing                            views.Doing
	since                            time.Time
	status                           int
	stale                            bool
}

// listingKeys are the sessions of the List given that of says, in its order, by
// what's drawn of each, as runningKey has it.
func listingKeys(list views.SessionList, of func(views.ListedSession) bool) []runningKey {
	var keys []runningKey
	for _, s := range list.Sessions {
		if of(s) {
			a := s.Activity
			keys = append(keys, runningKey{
				id: s.Session, dir: s.Dir, project: s.Project, account: s.Account, model: s.Model, mark: s.Mark,
				doing: a.Doing, since: a.Since, status: a.Status, stale: a.Stale,
			})
		}
	}
	return keys
}

// periodInput is what a page of the sessions' days is built of at now
// beside its days: the dial's period, the List given, the accounts' names,
// the prices, the day weeks start on and how a model's family is read.
func (m Model) periodInput(now time.Time, list views.SessionList) views.PeriodInput {
	return views.PeriodInput{
		Period: m.period, List: list, Names: m.moment(now), Prices: m.cfg.Prices, WeekStarts: m.cfg.WeekStarts, Family: m.cfg.Family, Now: now,
	}
}
