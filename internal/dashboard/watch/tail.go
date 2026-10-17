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
// gained since, as the Log's Requests fold it into their read, the traces
// counting what every read gives; the day the mark is of, which a read on
// another day turns from, none before its first read; whether a look since
// the last read asks what the ledger gained; whether the request stream
// opened since a read was last asked, the traces owed one asked after it,
// whether or not a reader follows the tail; whether a read of it is under
// way, one at a time; and how many reads of today's lines whole a reader
// has under way for itself alone, as a session's page reads its lines as it
// opens, which no read of the tail runs beside, so the mark never moves past
// where one ends without its reader taking in what lies between.
type ledgerTail struct {
	tailRead
	due     bool
	owed    bool
	reading bool
	whole   int
}

// tailRead is where a read of today's lines read them to: the mark, the
// day it's of, and when it read them.
type tailRead struct {
	mark ledger.Mark
	day  views.LedgerDay
	at   time.Time
}

// tailedMsg is what a read of the ledger's tail found: where it read
// today's lines to; the lines it read; and each reader as the read grew it,
// where it was read for one: the Log, as logGrown has it, a session's page,
// as pageTaken has it, and the store, as views.HeldSessions holds it, and
// whether its feed told otherwise of a request it holds, as Retell has it.
type tailedMsg struct {
	tailRead
	lines   tailLines
	log     *logGrown
	page    *pageTaken
	held    *views.HeldSessions
	retells bool
}

// tailLines are what a read of the ledger's tail read: the lines today's
// gained since the mark, afresh where they're every one of today's, as
// Ledger's Today has it; and those of each day before today a reader reads
// again whole, by its date, each read once for every reader.
type tailLines struct {
	gained []ledger.Held
	afresh bool
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
// since asks, nor owed to the traces till the stream opens again.
func (t ledgerTail) asking() ledgerTail {
	t.reading, t.due, t.owed = true, false, false
	return t
}

// streamOpened is the tail once the request stream has opened: a read of it
// due, and owed to the traces, which only a read asked from then on pays, as
// one under way may have read the files before the stream opened, and a
// request that ended between told the stream nothing.
func (t ledgerTail) streamOpened() ledgerTail {
	t.due, t.owed = true, true
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
// once, the late days each has; and grows each reader by them, as
// tailReading folds them.
func (m Model) readTail(now time.Time) (Model, tea.Cmd) {
	m, t := m.askingTail(now, false)
	return m, func() tea.Msg {
		read, next := readLines(t.r.ledger, t.mark, t.late)
		msg, gains := t.took(read, next, t.log.wants(read))
		if msg.held != nil && t.log != nil {
			msg.log = t.log.grown(t.r, read, views.LogTaking{Held: *msg.held, Gains: gains, Stream: t.log.stream, Now: now})
		}
		return msg
	}
}

// tailReading is a read of the ledger's tail as it was asked: what it reads
// through, from the mark given, on the day it's of, at now; and the fold of
// each reader it grows, nil for one it doesn't, and the late days it reads
// again for them.
type tailReading struct {
	r     logReaders
	mark  ledger.Mark
	today views.LedgerDay
	now   time.Time
	log   *logFold
	page  *pageFold
	held  *heldFold
	late  []string
}

// askingTail is m with a read of the ledger's tail at now asked, and what it
// reads, as tailReading has it: from the tail's mark, or where the read
// opens the Log, afresh, every one of today's lines; each reader noting
// the days awaiting a line it reads, and each taking in every late day
// read, as lateDays has them, so a line a reader awaits reaches every
// reader in the read that finds it, as the stream lets its request go as
// lined has it: the Log, once opened, as logFold has it,
// shown or not, as the mark moves past what the read gives for every
// reader alike, its Requests built first of what it holds now, as
// requestsBuilt builds them, for what they await, the days before those its
// tables span left to it; the session's page open, as pageFold has it; and
// the store, once the List is held, the Log opened or a page open, or while
// it holds anything, as heldFold has it, or where the read opens the Log,
// as storeFold has it for the Log, whatever else it holds, shown or not,
// fed once for them all, the first.
func (m Model) askingTail(now time.Time, opens bool) (Model, tailReading) {
	t := tailReading{r: m.logReaders(), mark: m.tail.mark, today: views.Today(now), now: now}
	if opens {
		t.mark = ledger.Mark{}
	}
	if m.log.opened {
		m = m.requestsBuilt(now)
		f := m.logFold(now)
		t.log, m.log.reading, m.log.awaited = &f, true, f.awaited
	}
	t.page = m.pageFold(now)
	if opens {
		t.held = m.storeFold(now, true)
	} else {
		t.held = m.heldFold(now)
	}
	m = m.pageAwaiting(t.page).heldAwaiting(t.held)
	t.late = lateDays(t.log, t.page, t.held)
	if t.log != nil {
		t.log.late = m.log.spanned(t.late)
	}
	if t.held != nil {
		t.held.late = t.late
	}
	m.tail = m.tail.asking()
	return m, t
}

// took is what the read found of read, read to the mark next: the store fed
// it, wanting the sessions of the Log's lines, logged, too, and what the
// feed changed of each session; and the page open grown by it.
func (t tailReading) took(read tailLines, next ledger.Mark, logged []string) (tailedMsg, views.SessionGains) {
	msg := tailedMsg{mark: next, day: t.today, at: t.now, lines: read}
	var gains views.SessionGains
	if t.held != nil {
		fed, g := t.held.fed(t.r.ledger, read, logged, t.now)
		msg.held, msg.retells, gains = &fed, g.Retell(), g
	}
	if t.page != nil {
		msg.page = t.page.taken(t.today)
	}
	return msg, gains
}

// followTail has the watch read the ledger's tail at each look whatever
// shows, as readTail reads it, where no other read of the ledger is under
// way, while a reader follows it, as tailFollowed has it; and once after
// the request stream opens, whether or not one does, as the traces are owed
// a read, as streamOpened has it.
func (m Model) followTail() (Model, tea.Cmd) {
	if !m.tail.due || m.tail.busy() || m.cfg.Ledger == nil || m.log.reading || m.sessionsTab.reading || m.pending != "" {
		return m, nil
	}
	if !m.tail.owed && !m.tailFollowed() {
		return m, nil
	}
	return m.readTail(m.now())
}

// tailFollowed reports whether a reader follows the ledger's tail: one that
// folds it, the Log once opened, Sessions' List once held or a session's
// page open, so each keeps up while another tab shows; the stream, while it
// told of a request whose line it awaits, as the traffic lets one go only
// once a read of the tail has read its line; or the store, while it holds a
// session, for it to let go of those no reader wants.
func (m Model) tailFollowed() bool {
	return m.log.opened || m.sessionsTab.listed || m.session != nil || m.traffic.awaitsLines() || m.held.Len() > 0
}

// lateDays are the dates of the days before today the readers that fold a
// read of the tail read again whole, each once, the oldest first: nil where
// a reader doesn't fold it.
func lateDays(log *logFold, page *pageFold, held *heldFold) []string {
	var late []string
	if log != nil {
		late = append(late, log.late...)
	}
	if page != nil {
		late = append(late, page.late...)
	}
	if held != nil {
		late = append(late, held.late...)
	}
	return slices.Compact(slices.Sorted(slices.Values(late)))
}

// readLines reads, through l, today's lines since mark, and the late days'
// whole, returning them with the mark today's are read to.
func readLines(l views.Ledger, mark ledger.Mark, late []string) (tailLines, ledger.Mark) {
	gained, next, afresh := l.Today(mark)
	read := tailLines{gained: gained, afresh: afresh, days: make(map[string][]ledger.Held, len(late))}
	for _, date := range late {
		read.days[date] = l.Day(date)
	}
	return read, next
}

// tailed takes in what a read of the ledger's tail found: the tail read to
// its mark; the store as the read fed it, with the sessions taken into it
// as the read was under way, as heldWith keeps them, and none the read let
// go; each reader grown by it, where it was read for one, as logGrown and
// pageTaken have them, the page taking its session from the store; today's
// lines it read counted by the traces, whoever it was read for, as traced
// has it; and the lines it read of the requests the stream told of held, as
// lined has it, for letGo.
func (m Model) tailed(msg tailedMsg) Model {
	m.tail = m.tail.read(msg.tailRead)
	m = m.traced(msg.lines.gained)
	if msg.held != nil {
		m = m.holding(msg.held.With(m.merged), msg.retells || m.mergedAny)
	}
	m.merged, m.mergedAny = views.HeldSessions{}, false
	if msg.log != nil {
		m.log = m.log.grown(*msg.log)
	}
	if msg.page != nil {
		m = m.pageTook(*msg.page, msg.lines)
	}
	m.traffic = m.traffic.lined(msg.lines.all()...)
	return m
}

// letGo is m with the requests the stream told of let go whose lines a read
// of the ledger's tail has read, once no longer in flight, as the traffic's
// letGo has it, each reader having taken the line in as it folded the read,
// the Log placing it as the stream told it was sent: but for one whose row
// the Log, opened, shows without its line, as views.LogRead's ShowsTold has
// it, the line filed on a day before those it spans, kept till it reads back
// to that day, as awaitsBefore has it.
func (m Model) letGo() Model {
	if !m.traffic.holdsLines() {
		return m
	}
	log := m.log
	m.traffic = m.traffic.letGo(func(r views.Told) bool { return log.opened && log.read.ShowsTold(r, log.from) })
	return m
}
