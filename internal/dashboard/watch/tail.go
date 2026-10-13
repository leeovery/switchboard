package watch

import (
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// ledgerTail is today's lines of the request ledger as the watch follows
// them: one Mark for every reader of them, each folding in what the ledger
// gained since, as the Log's Requests fold it into their read; the day the
// mark is of, which a read on another day turns from, none before its first
// read; whether a look since the last read asks what the ledger gained; and
// whether a read of it is under way, one at a time.
type ledgerTail struct {
	tailRead
	due     bool
	reading bool
}

// tailRead is where a read of today's lines read them to: the mark, and the
// day it's of.
type tailRead struct {
	mark ledger.Mark
	day  views.LedgerDay
}

// tailedMsg is what a read of the ledger's tail found: where it read
// today's lines to; and each reader as the read grew it, where it was read
// for one: the Log, as logGrown has it.
type tailedMsg struct {
	tailRead
	log *logGrown
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
