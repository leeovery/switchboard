package watch

import (
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// ledgerTail is today's lines of the request ledger as the watch follows
// them: one Mark for every reader of them, each folding in what the ledger
// gained since, as the Log's Requests fold it into their read; the day the
// mark is of, which a read on another day turns from, none before its first
// read; whether a look since the last read asks what the ledger gained;
// whether a read of it is under way, one at a time; and how many reads of
// today's lines whole a reader has under way for itself alone, as a
// session's page reads its lines as it opens, which no read of the tail
// runs beside, so the mark never moves past where one ends without its
// reader taking in what lies between.
type ledgerTail struct {
	tailRead
	due     bool
	reading bool
	whole   int
}

// tailRead is where a read of today's lines read them to: the mark, and the
// day it's of.
type tailRead struct {
	mark ledger.Mark
	day  views.LedgerDay
}

// tailedMsg is what a read of the ledger's tail found: where it read
// today's lines to; the lines it read; and each reader as the read grew it,
// where it was read for one: the Log, as logGrown has it, a session's page,
// as pageTaken has it, and Sessions' List, as listTaken has it.
type tailedMsg struct {
	tailRead
	lines tailLines
	log   *logGrown
	page  *pageTaken
	list  *listTaken
}

// tailLines are what a read of the ledger's tail read: the lines today's
// gained since the mark; and those of each day before today a reader reads
// again whole, by its date, each read once for every reader.
type tailLines struct {
	gained []ledger.Held
	days   map[string][]ledger.Held
}

// all are every line a read of the tail read, today's and the late days'.
func (r tailLines) all() [][]ledger.Held {
	return append([][]ledger.Held{r.gained}, slices.Collect(maps.Values(r.days))...)
}

// unread reports whether no reader has read the tail yet: the first to read
// today's lines whole reads them for it.
func (t ledgerTail) unread() bool {
	return t.day.Date == ""
}

// turned reports whether the day has turned since the tail was last read,
// today being today: never before its first read.
func (t ledgerTail) turned(today views.LedgerDay) bool {
	return !t.unread() && t.day.Date != today.Date
}

// missed are the dates of the days the tail followed no further, today
// being today, where the day turned since it was last read: the day it was
// read on, its lines after the mark unread, and each after it before today,
// none of whose lines it read. None before its first read.
func (t ledgerTail) missed(today views.LedgerDay) []string {
	if !t.turned(today) {
		return nil
	}
	return dayfile.Span(t.day.Start, dayfile.DayStart(today.Start, -1))
}

// busy reports whether a read of today's lines is under way: the tail's,
// or one a reader reads whole for itself alone.
func (t ledgerTail) busy() bool {
	return t.reading || t.whole > 0
}

// asking is the tail with a read of it under way, none due till a look
// since asks.
func (t ledgerTail) asking() ledgerTail {
	t.reading, t.due = true, false
	return t
}

// read is the tail once a read of it has landed, read to where r says.
func (t ledgerTail) read(r tailRead) ledgerTail {
	t.reading, t.tailRead = false, r
	return t
}

// wholeAsked is the tail with a reader's own read of today's lines whole
// under way, and wholeLanded the tail once it has landed.
func (t ledgerTail) wholeAsked() ledgerTail {
	t.whole++
	return t
}

func (t ledgerTail) wholeLanded() ledgerTail {
	t.whole--
	return t
}

// readTail reads, at now, what the ledger gained since the tail's mark, and
// the lines of the days before today its readers read again whole, each
// once, the late days each has; and grows each reader by them, each noting
// the days awaiting a line it reads, and each taking in every late day
// read, as lateDays has them, so a line a reader awaits reaches every
// reader in the read that finds it, as the stream lets its request go as
// lined has it: the Log, once opened, as logFold has it, shown or not, as
// the mark moves past what the read gives for every reader alike, its
// Requests built first of what it holds now, as requestsBuilt builds them,
// for what they await, the days before those its tables span left to it;
// the session's page open, as pageFold has it; and Sessions' List, once
// held, shown or not, as listFold has it.
func (m Model) readTail(now time.Time) (Model, tea.Cmd) {
	r, mark, today := m.logReaders(), m.tail.mark, views.Today(now)
	var log *logFold
	if m.log.opened {
		m = m.requestsBuilt(now)
		f := m.logFold(now)
		log, m.log.reading, m.log.awaited = &f, true, f.awaited
	}
	page := m.pageFold(now)
	list := m.listFold(now)
	m = m.pageAwaiting(page).listAwaiting(list)
	late := lateDays(log, page, list)
	if log != nil {
		log.late = slices.DeleteFunc(slices.Clone(late), func(date string) bool { return date < m.log.from.Date })
	}
	if page != nil {
		page.late = late
	}
	if list != nil {
		list.late = late
	}
	m.tail = m.tail.asking()
	return m, func() tea.Msg {
		read, next := readLines(r.ledger, mark, late)
		msg := tailedMsg{mark: next, day: today, lines: read}
		if log != nil {
			msg.log = log.grown(r, read, now)
		}
		if page != nil {
			msg.page = page.taken(read, today)
		}
		if list != nil {
			msg.list = list.taken(r.ledger, read, today)
		}
		return msg
	}
}

// followTail has the watch read the ledger's tail at each look whatever
// shows, as readTail reads it, where no other read of the ledger is under
// way: while a reader folds it, the Log once opened, Sessions' List once
// held or a session's page open, so each keeps up while another tab shows;
// or while the stream told of a request whose line it awaits, as the
// traffic lets one go only once a read of the tail has read its line.
func (m Model) followTail() (Model, tea.Cmd) {
	if !m.tail.due || m.tail.busy() || m.cfg.Ledger == nil || m.log.reading || m.sessionsTab.reading || m.pending != "" {
		return m, nil
	}
	if !m.log.opened && m.sessionsTab.held == nil && m.session == nil && !m.traffic.awaitsLines() {
		return m, nil
	}
	return m.readTail(m.now())
}

// lateDays are the dates of the days before today the readers that fold a
// read of the tail read again whole, each once, the oldest first: nil where
// a reader doesn't fold it.
func lateDays(log *logFold, page *pageFold, list *listFold) []string {
	var late []string
	if log != nil {
		late = append(late, log.late...)
	}
	if page != nil {
		late = append(late, page.late...)
	}
	if list != nil {
		late = append(late, list.late...)
	}
	return slices.Compact(slices.Sorted(slices.Values(late)))
}

// readLines reads, through l, today's lines since mark, and the late days'
// whole, returning them with the mark today's are read to.
func readLines(l views.Ledger, mark ledger.Mark, late []string) (tailLines, ledger.Mark) {
	gained, next, _ := l.Today(mark)
	read := tailLines{gained: gained, days: make(map[string][]ledger.Held, len(late))}
	for _, date := range late {
		read.days[date] = l.Day(date)
	}
	return read, next
}

// tailed takes in what a read of the ledger's tail found: the tail read to
// its mark; each reader grown by it, where it was read for one, as
// logGrown, pageTaken and listTaken have them; and then the requests the
// stream told of whose lines it read let go, as lined has it.
func (m Model) tailed(msg tailedMsg) Model {
	m.tail = m.tail.read(msg.tailRead)
	if msg.log != nil {
		m.log = m.log.grown(*msg.log)
	}
	if msg.page != nil {
		m = m.pageTook(*msg.page)
	}
	if msg.list != nil {
		m = m.listTook(*msg.list)
	}
	return m.lined(msg.lines.all()...)
}

// lined is m with the requests the stream told of whose lines are among
// those given let go, as the traffic's lined has it, once the Log, opened,
// has placed the lines it holds, as linesPlaced has it, while the stream
// still tells when each was sent: so a row stays where it sat in flight as
// its line takes over.
func (m Model) lined(lines ...[]ledger.Held) Model {
	if m.log.opened {
		m = m.linesPlaced(m.now())
	}
	m.traffic = m.traffic.lined(lines...)
	return m
}
