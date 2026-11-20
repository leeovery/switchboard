package watch

import (
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// pastDay is what the watch holds of a day gone's hours, which Accounts'
// WORTH and Projects' WORTH BY chart a day the dial shows by: the fold of
// the day read, as views.ReadDayFold reads it, of the day with the date
// given, "" till one lands; how many reads have landed; whether it was read
// early, as the ledger's tail still owed the day's lines, as owed has them,
// to be read again once it has read them; and the date of the day a read
// under way reads, "" while none is, one at a time.
type pastDay struct {
	fold    views.DayFold
	date    string
	read    int
	early   bool
	reading string
}

// pastDayReadMsg is what a read of a day gone's fold found: the fold of the
// day with the given date, and whether it was read early, as pastDay's
// early has it.
type pastDayReadMsg struct {
	date  string
	early bool
	fold  views.DayFold
}

// pastDayWanted is the date of the day gone whose fold what shows charts by
// the hour at now: the dial's period's, as its Day has it, where it's a day
// gone, while Accounts shows it on a view that charts its worth by the hour,
// as accountsHourly has it, or Projects shows it; false where none does.
func (m Model) pastDayWanted(now time.Time) (string, bool) {
	date, gone, ok := m.period.Day(now, m.cfg.WeekStarts)
	if !ok || !gone {
		return "", false
	}
	return date, m.view == dashboard.Projects || m.view == dashboard.Accounts && m.accounts.shown.View() != dashboard.AccountsTokens
}

// readPastDay has the watch read the fold of the day gone what shows charts
// by the hour, as pastDayWanted has it, as a command, one read at a time,
// once for each day: where none is held of it, or where the one held was
// read early and the ledger's tail has since read the day's lines it owed,
// as owed has them, so a fold of yesterday read just after midnight takes
// in the lines that came in after it. Each is read at now, keeping the
// lines as long as the config keeps them, in the pool shown.
func (m Model) readPastDay() (Model, tea.Cmd) {
	now := m.now()
	date, wanted := m.pastDayWanted(now)
	d := m.pastDay
	if !wanted || d.reading != "" || m.cfg.Ledger == nil || d.date == date && !d.early {
		return m, nil
	}
	early := m.owes(date, now)
	if d.date == date && early {
		return m, nil
	}
	l, keep, pool := m.cfg.Ledger, m.cfg.LedgerKeep, m.cfg.PoolRead
	m.pastDay.reading = date
	return m, func() tea.Msg {
		return pastDayReadMsg{date: date, early: early, fold: views.ReadDayFold(l, date, keep, now, pool)}
	}
}

// pastDayRead takes in a read of a day gone's fold, the read no longer
// under way: held, counted, where the dial's period is still that day, else
// dropped, the dial having left it.
func (m Model) pastDayRead(msg pastDayReadMsg) Model {
	m.pastDay.reading = ""
	if date, gone, ok := m.period.Day(m.now(), m.cfg.WeekStarts); ok && gone && date == msg.date {
		m.pastDay = pastDay{fold: msg.fold, date: msg.date, read: m.pastDay.read + 1, early: msg.early}
	}
	return m
}

// pastDayHeld is the fold of the day gone the dial's period is at now, as
// the watch holds it, its hours for HoursFold to chart the day by; the zero
// DayFold where it holds none of it.
func (m Model) pastDayHeld(now time.Time) views.DayFold {
	if date, gone, ok := m.period.Day(now, m.cfg.WeekStarts); ok && gone && date == m.pastDay.date {
		return m.pastDay.fold
	}
	return views.DayFold{}
}

// hoursReady reports whether what shows can chart its day by the hour at
// now: but where it charts a day gone, as pastDayWanted has it, whose fold
// is yet to land, as before its first read, or as the dial steps to another
// day gone, its page waiting on it as on the days it's built of. Without a
// ledger there's nothing to wait on.
func (m Model) hoursReady(now time.Time) bool {
	date, wanted := m.pastDayWanted(now)
	return m.cfg.Ledger == nil || !wanted || m.pastDay.date == date
}

// pastDayEarly reports whether the fold of the day gone what shows charts by
// the hour was read early, as pastDay's early has it, so the ledger's tail
// is followed till it has read the day's lines it owes.
func (m Model) pastDayEarly(now time.Time) bool {
	date, wanted := m.pastDayWanted(now)
	return wanted && m.pastDay.date == date && m.pastDay.early
}

// owes reports whether the ledger's tail owes at now a read of the lines of
// the day with the given date, as owed has them.
func (m Model) owes(date string, now time.Time) bool {
	return slices.Contains(m.owed(now), date)
}

// owed are the dates of the days before now's whose lines the ledger's tail
// owes a read of, oldest first: those the sessions' days it fed hold owed,
// as Owed has them, and those it followed no further, as missed has them,
// as just past midnight, before its first read of the day.
func (m Model) owed(now time.Time) []string {
	owed := slices.Concat(m.sessionDays.tailed.Owed(now), m.tail.missed(views.Today(now)))
	return slices.Compact(slices.Sorted(slices.Values(owed)))
}

// hoursOf is what a page that charts the dial's day by the hour is built of
// at now, as HoursFold picks the fold it charts: of the ledger's tail's
// fold, the reads of it that told otherwise of the days it fed, while its
// fold is of that day, as it is of the day the tail was last read on, else
// -1, so a day gone never follows the tail; and of a day gone's fold read,
// how many have landed. The zero hoursOf of a period of more than a day.
type hoursOf struct {
	fed, read int
}

// hoursOf is what a page charting the dial's day by the hour is built of
// at now, as hoursOf has it.
func (m Model) hoursOf(now time.Time) hoursOf {
	date, _, ok := m.period.Day(now, m.cfg.WeekStarts)
	if !ok {
		return hoursOf{}
	}
	of := hoursOf{fed: -1, read: m.pastDay.read}
	if m.tail.day.Date == date {
		of.fed = m.sessionDays.fed
	}
	return of
}
